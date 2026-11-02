package inquiry

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tradeengine/tradeengine/internal/store"
)

// Store 是 Handler 需要的持久化能力（接口化便于测试时替换成内存实现）。
type Store interface {
	Insert(*store.Inquiry) error
	CountByIPSince(ipHash string, since time.Time) (int, error)
	MarkNotified(id string, at time.Time) error
}

// Notifier 负责「落库之后」的通知。实现必须是非阻塞的：
// 通知慢不能拖慢客户看到结果的速度，更不能让提交失败。
type Notifier interface {
	Notify(inq *store.Inquiry)
}

// Config 是 Handler 的运行参数。
type Config struct {
	// Site 写入 site 列，多站点共用一个实例时用于区分来源。
	Site string
	// DefaultLang 用于判断「中文内容出现在英文站」这类启发式规则。
	DefaultLang string
	// Langs 是站点实际启用的语言。提交上来的 lang 必须在这个清单内——
	// 它会拼进 303 跳转地址，不做白名单就是一处路径注入。
	Langs []string
	// Require 决定哪些字段必填。
	Require Required
	// RateLimitPerHour 单 IP 每小时上限。
	RateLimitPerHour int
	// RateLimitPerDay 单 IP 每 24 小时上限。小时限流挡不住「每小时十个、
	// 连发一整天」的慢速灌库，因此还需要一道日闸。
	RateLimitPerDay int
	// SuccessPath 是无 JS 提交后的跳转目标，支持 {lang} 占位符。
	SuccessPath string
	// SiteBaseURL 用于把相对 SuccessPath 拼成绝对地址。
	// 静态站部署在 Cloudflare Pages、API 在 VPS 时，303 必须跳回静态站域名，
	// 否则客户会被带到 api.example.com/thank-you/ 这个不存在的页面。
	SiteBaseURL string
	// TrustProxy 为 true 时从 X-Forwarded-For / CF-Connecting-IP 取客户端 IP。
	TrustProxy bool
}

// Handler 实现 POST /api/inquiry。
type Handler struct {
	cfg       Config
	store     Store
	notifier  Notifier
	turnstile *Turnstile
	salt      []byte
	log       *slog.Logger
	now       func() time.Time
	// ipLock 让「复查限流 + 落库」对同一 IP 原子化，堵住并发绕过（见 ratelimit.go）
	ipLock *ipLocker
	// productTitle 根据 slug 返回产品标题快照；返回 ok=false 表示产品不存在。
	productTitle func(slug string) (string, bool)
}

// NewHandler 构造 Handler。
func NewHandler(cfg Config, st Store, n Notifier, ts *Turnstile, salt []byte, log *slog.Logger,
	productTitle func(string) (string, bool)) *Handler {
	if cfg.RateLimitPerHour <= 0 {
		cfg.RateLimitPerHour = 10
	}
	if cfg.RateLimitPerDay <= 0 {
		cfg.RateLimitPerDay = 30
	}
	if cfg.SuccessPath == "" {
		cfg.SuccessPath = "/thank-you/"
	}
	if cfg.DefaultLang == "" {
		cfg.DefaultLang = "en"
	}
	if log == nil {
		log = slog.Default()
	}
	return &Handler{
		cfg: cfg, store: st, notifier: n, turnstile: ts,
		salt: salt, log: log, now: time.Now, productTitle: productTitle,
		ipLock: &ipLocker{},
	}
}

// ServeHTTP 严格按《11》10.2 的处理顺序执行：先便宜后贵。
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := h.now()

	// ① 方法与编码
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, newErr(http.StatusMethodNotAllowed, "method_not_allowed", ""))
		return
	}

	// ② 体积限制 + 解析（Parse 内部已限 32KB）
	in, isForm, err := Parse(r)
	if err != nil {
		h.fail(w, r, err, isForm, "")
		return
	}

	// ③ 蜜罐：填了就是机器人。返回成功、不落库、不告诉对方被识别
	if strings.TrimSpace(in.Honeypot) != "" {
		h.log.Warn("蜜罐命中，静默丢弃", "ip", HashIP(ClientIP(r, h.cfg.TrustProxy), h.salt)[:12])
		h.respondCreated(w, r, NewID(h.now()), isForm, h.cfg.DefaultLang)
		return
	}

	// ④ 提交速度：3 秒内填完表单不可能是真人
	if in.TooFast(h.now()) {
		h.log.Warn("提交过快，静默丢弃", "render_ts", in.RenderTS)
		h.respondCreated(w, r, NewID(h.now()), isForm, h.cfg.DefaultLang)
		return
	}

	// ⑤ 限流：库里的计数是持久的，重启不清零
	ipHash := HashIP(ClientIP(r, h.cfg.TrustProxy), h.salt)
	if over, window := h.overLimit(ipHash); over {
		h.rejectRateLimited(w, window, ipHash)
		return
	}

	// ⑥ 人机校验
	if err := h.turnstile.Verify(r.Context(), in.TurnstileToken, ClientIP(r, h.cfg.TrustProxy)); err != nil {
		h.fail(w, r, err, isForm, "")
		return
	}

	// ⑦ 字段校验
	if err := in.Validate(h.cfg.Require); err != nil {
		h.fail(w, r, err, isForm, "")
		return
	}

	// 语言要在打分之前定下来：启发式规则依赖「客户从哪个语言的页面提交」
	lang := h.resolveLang(in.Lang)

	// ⑧ 内容启发式打分（只标记，不拦截）
	score := in.SpamScore(lang)

	// 产品快照：客户看到的产品名和他提交时一致，产品改名也不影响历史记录
	productTitle := ""
	if in.Product != "" && h.productTitle != nil {
		title, ok := h.productTitle(in.Product)
		if !ok {
			// 产品下架了但客户页面还开着，这时不该拒绝——问的是这个产品，记下来就行
			h.log.Warn("询盘指向的产品不存在", "product", in.Product)
		}
		productTitle = title
	}

	inq := &store.Inquiry{
		ID:           NewID(h.now()),
		CreatedAt:    h.now(),
		Site:         h.cfg.Site,
		Lang:         lang,
		Country:      in.Country,
		Name:         in.Name,
		Company:      in.Company,
		Email:        in.Email,
		Phone:        in.Phone,
		ProductSlug:  in.Product,
		Volume:       in.Volume,
		ProductTitle: productTitle,
		Message:      in.Message,
		SourcePage:   truncate(in.SourcePage, 512),
		Referrer:     truncate(in.Referrer, 512),
		UTMSource:    truncate(in.UTMSource, 120),
		UTMMedium:    truncate(in.UTMMedium, 120),
		UTMCampaign:  truncate(in.UTMCampaign, 120),
		IPHash:       ipHash,
		UserAgent:    truncate(r.UserAgent(), 400),
		SpamScore:    score,
		Status:       "new",
	}
	if score >= spamThreshold {
		// 「误判不删，只标记」：宁可多一条垃圾，不可丢一条真询盘
		inq.Status = "spam"
	}

	// ⑨ 先落库。这一步成功，询盘就永不丢单。
	//
	// 落库与限流复检必须在同一把 IP 锁内完成：⑤ 的检查到这里之间隔着
	// 人机校验和字段校验，同一 IP 的并发请求会一起读到「尚未到上限」而
	// 同时放行。实测 100 并发下阈值 10 被放过 18 条，锁内复查后精确为 10。
	unlock := h.ipLock.lock(ipHash)
	if over, window := h.overLimit(ipHash); over {
		unlock()
		h.rejectRateLimited(w, window, ipHash)
		return
	}
	err = h.store.Insert(inq)
	unlock()
	if err != nil {
		h.log.Error("落库失败", "err", err, "name", in.Name, "email", in.Email)
		writeError(w, newErr(http.StatusInternalServerError, "internal", ""))
		return
	}

	// ⑩ 通知（异步，失败不影响响应）
	if inq.Status != "spam" && h.notifier != nil {
		h.notifier.Notify(inq)
	}

	// ⑪ 记录转化事件
	h.log.Info("收到询盘",
		"id", inq.ID,
		"country", inq.Country,
		"product", inq.ProductSlug,
		"spam_score", inq.SpamScore,
		"status", inq.Status,
		"source", inq.SourcePage,
		"utm_source", inq.UTMSource,
		"ms", h.now().Sub(start).Milliseconds(),
	)

	h.respondCreated(w, r, inq.ID, isForm, lang)
}

// overLimit 检查该 IP 是否已触达小时或日上限，返回命中的窗口。
//
// 两道闸的分工：小时闸挡突发（脚本一次灌几百条），日闸挡慢速渗透
// （每小时九个、连发一整天）。只有小时闸时，单 IP 一天最多能灌 240 条。
//
// 计数查询失败不能挡住真实客户——记日志后放行，与「宁可多一条垃圾，
// 不可丢一条真询盘」是同一条原则。
func (h *Handler) overLimit(ipHash string) (bool, string) {
	hour, err := h.store.CountByIPSince(ipHash, h.now().Add(-time.Hour))
	if err != nil {
		h.log.Error("限流计数失败", "err", err)
	} else if hour >= h.cfg.RateLimitPerHour {
		return true, "hour"
	}
	day, err := h.store.CountByIPSince(ipHash, h.now().Add(-24*time.Hour))
	if err != nil {
		h.log.Error("限流计数失败", "err", err)
	} else if day >= h.cfg.RateLimitPerDay {
		return true, "day"
	}
	return false, ""
}

// rejectRateLimited 返回 429，并用 Retry-After 告诉对方多久后可以再试。
func (h *Handler) rejectRateLimited(w http.ResponseWriter, window, ipHash string) {
	retry := "3600"
	if window == "day" {
		retry = "86400"
	}
	h.log.Warn("限流拦截", "ip", ipHash[:12], "window", window)
	w.Header().Set("Retry-After", retry)
	writeError(w, newErr(http.StatusTooManyRequests, "rate_limited", ""))
}

// spamThreshold 是「标记为 spam」的分值线。60 分需要至少两条强信号叠加，
// 单个可疑特征（比如贴了一个链接）不会把正常询盘打成垃圾。
const spamThreshold = 60

// respondCreated 返回成功：表单提交走 303 跳转（保证禁用 JS 也能用），JSON 提交返回 201。
func (h *Handler) respondCreated(w http.ResponseWriter, r *http.Request, id string, isForm bool, lang string) {
	if isForm {
		// 303 而不是 302：强制用 GET 重新请求，避免刷新时重复提交
		http.Redirect(w, r, h.successURL(lang), http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id})
}

// resolveLang 把提交上来的 lang 收敛到站点实际启用的语言。
// 不在白名单内一律落回默认语言——lang 会参与拼接跳转地址，不能直接信任。
func (h *Handler) resolveLang(in string) string {
	lang := strings.ToLower(strings.TrimSpace(in))
	for _, l := range h.cfg.Langs {
		if lang == strings.ToLower(l) {
			return l
		}
	}
	if h.cfg.DefaultLang != "" {
		return h.cfg.DefaultLang
	}
	return "en"
}

// successURL 把配置里的 success_url 解析成可跳转的绝对地址。
func (h *Handler) successURL(lang string) string {
	if lang == "" {
		lang = h.cfg.DefaultLang
	}
	p := strings.ReplaceAll(h.cfg.SuccessPath, "{lang}", lang)
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	base := strings.TrimRight(h.cfg.SiteBaseURL, "/")
	if base == "" {
		return p
	}
	return base + p
}

// fail 统一处理错误：区分字段级错误与内部错误。
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error, isForm bool, lang string) {
	var fe *FieldError
	if !asFieldError(err, &fe) {
		h.log.Error("询盘处理失败", "err", err)
		writeError(w, newErr(http.StatusInternalServerError, "internal", ""))
		return
	}
	if fe.Status >= 500 {
		h.log.Error("询盘处理失败", "code", fe.Code)
	}
	if isForm {
		// 无 JS 提交没有前端接错误：直接给一个带说明的原生错误页，
		// 而不是跳到「提交成功」页——那会骗客户以为发出去了。
		h.writeFormError(w, r, fe)
		return
	}
	writeError(w, fe)
}

// writeFormError 输出一个极简错误页（无 JS 场景）。
// 内容只用固定文案 + 错误码，不回显任何用户输入，避免反射型注入。
func (h *Handler) writeFormError(w http.ResponseWriter, r *http.Request, fe *FieldError) {
	back := r.Referer()
	if back == "" || strings.HasPrefix(back, "//") {
		back = "/"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(fe.Status)
	hint := formErrorHint(fe)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<meta name="robots" content="noindex"><title>Inquiry not sent</title>` +
		`<style>body{font:16px/1.7 system-ui,sans-serif;max-width:520px;margin:80px auto;padding:0 24px;color:#1A1A1A}` +
		`a{color:#2F6B3F}</style></head><body>` +
		`<h1>Inquiry not sent</h1>` +
		`<p>` + html.EscapeString(hint) + `</p>` +
		`<p><a href="` + html.EscapeString(back) + `">Back to the form</a></p>` +
		`<p style="color:#888;font-size:13px">Error code: ` + html.EscapeString(fe.Code) + `</p>` +
		`</body></html>`))
}

// formErrorHint 把错误码翻译成客户能看懂的一句话。
// 只覆盖客户能自己修正的情况，其余一律给「请重试或直接发邮件」。
func formErrorHint(fe *FieldError) string {
	switch fe.Code {
	case "required":
		return "Please fill in all required fields and submit again."
	case "invalid_email":
		return "That email address does not look valid. Please check it and submit again."
	case "too_short":
		return "Your message is too short. Please describe your requirement in a little more detail."
	case "too_long":
		return "Your message is too long. Please shorten it and submit again."
	case "invalid_country":
		return "Please choose your country from the list."
	case "turnstile_expired":
		return "The verification expired because the form was open for a long time. Please submit again."
	case "turnstile_failed", "turnstile_missing":
		return "Human verification failed. Please reload the page and submit again."
	case "rate_limited":
		return "Too many submissions from your network. Please try again later, or email us directly."
	case "payload_too_large", "invalid_json", "invalid_form", "empty_body":
		return "The form data could not be read. Please reload the page and submit again."
	}
	return "Something went wrong on our side. Please try again, or email us directly."
}

func asFieldError(err error, target **FieldError) bool {
	return errors.As(err, target)
}

func writeError(w http.ResponseWriter, fe *FieldError) {
	status := fe.Status
	if status == 0 {
		status = http.StatusBadRequest
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(fe)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// 按字节截断可能切坏 UTF-8，回退到合法边界
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}
