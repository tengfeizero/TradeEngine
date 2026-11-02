package inquiry

import (
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tengfeizero/TradeEngine/internal/store"
)

// ---------------------------------------------------------------- 测试替身

// fakeStore 是内存实现，避免测试依赖 SQLite 文件。
type fakeStore struct {
	mu        sync.Mutex
	rows      []store.Inquiry
	ipCount   int // 用于模拟已触发的限流次数
	insertErr error
}

func (f *fakeStore) Insert(inq *store.Inquiry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return f.insertErr
	}
	f.rows = append(f.rows, *inq)
	return nil
}

func (f *fakeStore) CountByIPSince(string, time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ipCount, nil
}

func (f *fakeStore) MarkNotified(string, time.Time) error { return nil }

func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows)
}

func (f *fakeStore) last() store.Inquiry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rows[len(f.rows)-1]
}

// fakeNotifier 记录通知次数。判为 spam 的询盘不该触发通知。
type fakeNotifier struct {
	mu   sync.Mutex
	sent []string
}

func (f *fakeNotifier) Notify(inq *store.Inquiry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, inq.ID)
}

func (f *fakeNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func testHandler(t *testing.T, st Store) (*Handler, *fakeNotifier) {
	t.Helper()
	n := &fakeNotifier{}
	h := NewHandler(
		Config{
			Site:             "test",
			DefaultLang:      "en",
			Langs:            []string{"en", "zh"},
			Require:          RequiredFrom(func(k string) bool { return k == FieldName || k == FieldEmail || k == FieldMessage }),
			RateLimitPerHour: 5,
			SuccessPath:      "/{lang}/thank-you/",
			SiteBaseURL:      "https://example.com",
		},
		st, n, nil, []byte("test-salt"),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(slug string) (string, bool) {
			if slug == "sencha" {
				return "Sencha Green Tea", true
			}
			return "", false
		},
	)
	return h, n
}

func postJSON(h *Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/inquiry", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "203.0.113.9:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func postForm(h *Handler, values url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/inquiry", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.9:5555"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---------------------------------------------------------------- 校验

func TestValidate(t *testing.T) {
	req := Required{Name: true, Email: true, Message: true}
	cases := []struct {
		name   string
		in     Input
		wantOK bool
		want   string
	}{
		{"合法", Input{Name: "Tom", Email: "tom@acme.com", Message: "Need 2 tons of sencha"}, true, ""},
		{"缺姓名", Input{Email: "tom@acme.com", Message: "Need 2 tons of sencha"}, false, "required"},
		{"邮箱格式错", Input{Name: "Tom", Email: "tom@acme", Message: "Need 2 tons of sencha"}, false, "invalid_email"},
		{"邮箱缺 @", Input{Name: "Tom", Email: "tom.acme.com", Message: "Need 2 tons of sencha"}, false, "invalid_email"},
		{"留言过短", Input{Name: "Tom", Email: "tom@acme.com", Message: "hi"}, false, "too_short"},
		{"留言空", Input{Name: "Tom", Email: "tom@acme.com"}, false, "required"},
		{"姓名过长", Input{Name: strings.Repeat("a", 81), Email: "tom@acme.com", Message: "Need 2 tons of sencha"}, false, "too_long"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.Validate(req)
			if c.wantOK {
				if err != nil {
					t.Fatalf("期望通过，实际报错：%v", err)
				}
				return
			}
			fe, ok := err.(*FieldError)
			if !ok {
				t.Fatalf("期望 FieldError，实际 %T (%v)", err, err)
			}
			if fe.Code != c.want {
				t.Fatalf("期望 %s，实际 %s", c.want, fe.Code)
			}
		})
	}
}

func TestValidateCountry(t *testing.T) {
	req := Required{Name: true, Email: true, Message: true}
	base := Input{Name: "Tom", Email: "tom@acme.com", Message: "Need 2 tons of sencha"}

	base.Country = "DE"
	if err := base.Validate(req); err != nil {
		t.Fatalf("合法国家码被拒：%v", err)
	}

	base.Country = "XX"
	err := base.Validate(req)
	fe, ok := err.(*FieldError)
	if !ok || fe.Code != "invalid_country" {
		t.Fatalf("期望 invalid_country，实际 %v", err)
	}
	// 国家码非法用 422：格式没错，是业务上不认识这个值
	if fe.Status != http.StatusUnprocessableEntity {
		t.Fatalf("期望 422，实际 %d", fe.Status)
	}
}

// ---------------------------------------------------------------- 垃圾打分

func TestSpamScore(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		lang string
		max  int // 上限（越小越严格）
		min  int
	}{
		{"正常英文询盘", Input{Name: "Thomas", Company: "Alpen Tee GmbH",
			Message: "We need 2 tons per year, organic certified. Please quote FOB Ningbo."}, "en", 0, 0},
		// 这是一个真实发生过的误判：中文留言里带 "CIF"/"FOB" 缩写，
		// 早期实现按「大写比例」判定，中文没有小写字母，比例算成 100% 直接加分。
		{"中文询盘带 FOB 缩写", Input{Name: "李经理", Message: "需要煎茶 800 公斤，请报 FOB 宁波价。"}, "zh", 0, 0},
		{"中文站里的中文询盘不该被扣分", Input{Name: "王采购", Message: "需要抹茶 500 公斤，请报 CIF 东京价。"}, "zh", 0, 0},
		{"英文站里的中文内容", Input{Name: "x", Message: "需要大量采购茶叶请联系我"}, "en", 25, 25},
		{"SEO 推销 + 多链接", Input{Name: "SEO Guru",
			Message: "We offer seo services and backlink packages http://a.cn http://b.cn http://c.cn http://d.cn"}, "en", 100, 80},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.in.SpamScore(c.lang)
			if got < c.min || got > c.max {
				t.Fatalf("期望分数在 [%d,%d]，实际 %d", c.min, c.max, got)
			}
		})
	}
}

func TestTooFast(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		ts   string
		want bool
	}{
		{"1 秒前提交", msString(now.Add(-1 * time.Second)), true},
		{"5 秒前提交", msString(now.Add(-5 * time.Second)), false},
		{"空时间戳", "", false},
		{"未来时间戳（时钟不准）", msString(now.Add(10 * time.Second)), false},
		{"超过 24 小时", msString(now.Add(-48 * time.Hour)), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := Input{RenderTS: c.ts}
			if got := in.TooFast(now); got != c.want {
				t.Fatalf("期望 %v，实际 %v", c.want, got)
			}
		})
	}
}

func msString(t time.Time) string {
	return strconvItoa(t.UnixMilli())
}

func strconvItoa(v int64) string {
	return big.NewInt(v).String()
}

// ---------------------------------------------------------------- 解析

func TestParseJSON(t *testing.T) {
	body := `{"lang":"en","name":" Thomas ","email":"TOM@ACME.COM","country":"de",
		"product":"sencha","message":"  hello there friend  ",
		"utm":{"source":"google","medium":"cpc","campaign":"eu"},"_hp":"","render_ts":"1700000000000",
		"some_future_field":"ignored"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	in, isForm, err := Parse(req)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if isForm {
		t.Fatal("JSON 请求被识别成表单")
	}
	if in.Name != "Thomas" {
		t.Errorf("姓名未 trim：%q", in.Name)
	}
	if in.Email != "tom@acme.com" {
		t.Errorf("邮箱未小写：%q", in.Email)
	}
	if in.Country != "DE" {
		t.Errorf("国家码未大写：%q", in.Country)
	}
	if in.Message != "hello there friend" {
		t.Errorf("留言未 trim：%q", in.Message)
	}
	if in.UTMSource != "google" || in.UTMCampaign != "eu" {
		t.Errorf("UTM 解析错误：%+v", in)
	}
	// 未知字段必须被忽略而不是报错：前端加字段不该让真实客户提交失败
	if err != nil {
		t.Errorf("未知字段导致失败：%v", err)
	}
}

func TestParseForm(t *testing.T) {
	form := url.Values{
		"name":       {"王采购"},
		"email":      {"wang@example.cn"},
		"country":    {"jp"},
		"message":    {"需要抹茶 500 公斤"},
		"utm_source": {"linkedin"},
		"_hp":        {""},
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	in, isForm, err := Parse(req)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if !isForm {
		t.Fatal("表单请求未被识别")
	}
	if in.Country != "JP" {
		t.Errorf("国家码未大写：%q", in.Country)
	}
	if in.UTMSource != "linkedin" {
		t.Errorf("UTM 解析错误：%q", in.UTMSource)
	}
}

func TestParseEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	_, _, err := Parse(req)
	fe, ok := err.(*FieldError)
	if !ok || fe.Code != "empty_body" {
		t.Fatalf("期望 empty_body，实际 %v", err)
	}
}

// ---------------------------------------------------------------- Handler

func TestHandlerCreatesInquiry(t *testing.T) {
	st := &fakeStore{}
	h, n := testHandler(t, st)
	rec := postJSON(h, `{"lang":"en","name":"Thomas","company":"Alpen Tee","email":"t@alpen.de",
		"country":"DE","product":"sencha","message":"We need 2 tons of sencha, FOB Ningbo.",
		"source_page":"/en/products/green-tea/sencha/","utm":{"source":"google"}}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("期望 201，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if st.count() != 1 {
		t.Fatalf("期望落库 1 条，实际 %d", st.count())
	}
	got := st.last()
	if got.ProductTitle != "Sencha Green Tea" {
		t.Errorf("产品快照未写入：%q", got.ProductTitle)
	}
	if got.UTMSource != "google" {
		t.Errorf("UTM 未入库：%q", got.UTMSource)
	}
	if got.IPHash == "" {
		t.Error("IP 哈希为空")
	}
	if got.Status != "new" {
		t.Errorf("期望状态 new，实际 %s", got.Status)
	}
	if n.count() != 1 {
		t.Errorf("期望触发 1 次通知，实际 %d", n.count())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是 JSON：%v", err)
	}
	if resp["ok"] != true || resp["id"] == "" {
		t.Errorf("响应体异常：%v", resp)
	}
}

func TestHoneypotSilentlyAccepted(t *testing.T) {
	st := &fakeStore{}
	h, _ := testHandler(t, st)
	rec := postJSON(h, `{"name":"Bot","email":"bot@spam.com",
		"message":"buy cheap backlinks here now","_hp":"filled"}`)

	// 对机器人要给成功响应，否则它就知道自己被识别了，会换策略
	if rec.Code != http.StatusCreated {
		t.Fatalf("期望 201，实际 %d", rec.Code)
	}
	if st.count() != 0 {
		t.Fatalf("蜜罐命中的请求不应落库，实际落库 %d 条", st.count())
	}
}

func TestTooFastSilentlyAccepted(t *testing.T) {
	st := &fakeStore{}
	h, _ := testHandler(t, st)
	rec := postJSON(h, `{"name":"Flash","email":"f@x.com",
		"message":"submitted instantly by a bot","render_ts":"`+strconvItoa(time.Now().UnixMilli())+`"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("期望 201，实际 %d", rec.Code)
	}
	if st.count() != 0 {
		t.Fatalf("提交过快不应落库，实际 %d", st.count())
	}
}

func TestRateLimited(t *testing.T) {
	st := &fakeStore{ipCount: 5} // 已达上限（测试配置为 5/h）
	h, _ := testHandler(t, st)
	rec := postJSON(h, `{"name":"Tom","email":"t@x.com","message":"Need 2 tons of sencha"}`)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("期望 429，实际 %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 响应缺少 Retry-After")
	}
	if st.count() != 0 {
		t.Error("限流请求不应落库")
	}
}

// dailyStore 让小时计数与 24 小时计数取不同值：只有日闸才能真正拦住它。
type dailyStore struct {
	hour, day int
}

func (d *dailyStore) Insert(*store.Inquiry) error                       { return nil }
func (d *dailyStore) CountByIPSince(_ string, since time.Time) (int, error) {
	if time.Since(since) > 2*time.Hour { // 24h 窗口
		return d.day, nil
	}
	return d.hour, nil
}
func (d *dailyStore) MarkNotified(string, time.Time) error { return nil }

// 小时闸挡不住「每小时九个、连发一整天」：8 小时就能灌七十条。
// 这里锁住的是第二道闸——24 小时 30 条上限，且 Retry-After 要提示一天。
func TestDailyRateLimit(t *testing.T) {
	h, _ := testHandler(t, &dailyStore{hour: 1, day: 30})
	rec := postJSON(h, `{"name":"Tom","email":"t@x.com","message":"Need 2 tons of sencha"}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("触达日上限时期望 429，实际 %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "86400" {
		t.Errorf("日闸的 Retry-After 应为 86400，实际 %q", got)
	}

	// 未到日上限时必须照常放行——日闸不能误杀正常客户
	h2, _ := testHandler(t, &dailyStore{hour: 1, day: 29})
	if rec := postJSON(h2, `{"name":"Tom","email":"t@x.com","message":"Need 2 tons of sencha"}`); rec.Code != http.StatusCreated {
		t.Fatalf("未触达上限期望 201，实际 %d", rec.Code)
	}
}

func TestFormSubmitRedirectsWithLang(t *testing.T) {
	st := &fakeStore{}
	h, _ := testHandler(t, st)
	rec := postForm(h, url.Values{
		"lang":    {"zh"},
		"name":    {"王采购"},
		"email":   {"wang@example.cn"},
		"message": {"需要抹茶 500 公斤，请报 CIF 东京价。"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("期望 303，实际 %d：%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	want := "https://example.com/zh/thank-you/"
	if loc != want {
		t.Fatalf("期望跳转 %s，实际 %s", want, loc)
	}
}

// lang 会拼进跳转地址，必须收敛到站点启用的语言白名单，
// 否则就是一处开放跳转 / 路径注入。
func TestFormSubmitSanitizesLang(t *testing.T) {
	st := &fakeStore{}
	h, _ := testHandler(t, st)
	rec := postForm(h, url.Values{
		"lang":    {"../../evil"},
		"name":    {"Tom"},
		"email":   {"t@x.com"},
		"message": {"Need 2 tons of sencha please"},
	})

	loc := rec.Header().Get("Location")
	if strings.Contains(loc, "evil") {
		t.Fatalf("lang 未被白名单收敛，跳转地址含注入内容：%s", loc)
	}
	if loc != "https://example.com/en/thank-you/" {
		t.Fatalf("期望回落到默认语言，实际 %s", loc)
	}
}

func TestFormSubmitValidationError(t *testing.T) {
	st := &fakeStore{}
	h, _ := testHandler(t, st)
	rec := postForm(h, url.Values{
		"name":    {"Tom"},
		"email":   {"not-an-email"},
		"message": {"Need 2 tons of sencha please"},
	})

	// 无 JS 提交拿到的是错误页，而不是跳到「提交成功」页骗人
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，实际 %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("期望 HTML 错误页，实际 %s", ct)
	}
	if !strings.Contains(rec.Body.String(), "email") {
		t.Errorf("错误页未说明原因：%s", rec.Body.String())
	}
	if st.count() != 0 {
		t.Error("校验失败的请求不应落库")
	}
}

func TestSpamMarkedNotDeleted(t *testing.T) {
	st := &fakeStore{}
	h, n := testHandler(t, st)
	rec := postJSON(h, `{"lang":"en","name":"SEO Guru","email":"seo@spam.com",
		"message":"We offer seo services and backlink packages http://a.cn http://b.cn http://c.cn http://d.cn"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("期望 201，实际 %d", rec.Code)
	}
	// 关键设计：判为 spam 仍要入库，只是标记——宁可多一条垃圾，不可丢一条真询盘
	if st.count() != 1 {
		t.Fatalf("垃圾询盘也应入库，实际 %d 条", st.count())
	}
	if st.last().Status != "spam" {
		t.Errorf("期望状态 spam，实际 %s", st.last().Status)
	}
	if n.count() != 0 {
		t.Errorf("垃圾询盘不应触发通知，实际 %d 次", n.count())
	}
}

func TestStoreFailureNotifiesNothing(t *testing.T) {
	st := &fakeStore{insertErr: io.ErrUnexpectedEOF}
	h, n := testHandler(t, st)
	rec := postJSON(h, `{"name":"Tom","email":"t@x.com","message":"Need 2 tons of sencha"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("期望 500，实际 %d", rec.Code)
	}
	if n.count() != 0 {
		t.Error("落库失败时不应发通知（否则会通知一条查不到的询盘）")
	}
	// 内部错误不能把细节暴露给调用方
	if strings.Contains(rec.Body.String(), "EOF") {
		t.Errorf("响应泄漏了内部错误：%s", rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	st := &fakeStore{}
	h, _ := testHandler(t, st)
	req := httptest.NewRequest(http.MethodGet, "/api/inquiry", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("期望 405，实际 %d", rec.Code)
	}
}

// ---------------------------------------------------------------- 并发限流

// windowStore 按 ip_hash 实时统计，和真实 SQLite 实现同语义；
// delay 用来放大「读计数」与「写入」之间的竞态窗口，让缺失互斥的实现在测试里必然翻车。
type windowStore struct {
	mu    sync.Mutex
	rows  []store.Inquiry
	delay time.Duration
}

func (s *windowStore) Insert(inq *store.Inquiry) error {
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, *inq)
	return nil
}

func (s *windowStore) CountByIPSince(ipHash string, since time.Time) (int, error) {
	time.Sleep(s.delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.rows {
		if r.IPHash == ipHash && r.CreatedAt.After(since) {
			n++
		}
	}
	return n, nil
}

func (s *windowStore) MarkNotified(string, time.Time) error { return nil }

func (s *windowStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.rows)
}

// 同一 IP 的并发提交必须严格受限于阈值。
// 这里锁住的是一处真实缺陷：限流检查（步骤⑤）与落库（步骤⑨）隔着人机校验
// 和字段校验，中间没有互斥，100 并发会全部读到 count=0 而一起放行。
// 压测中阈值 10 曾放过 18 条；加上 IP 锁后必须精确等于阈值。
func TestRateLimitNotBypassedByConcurrency(t *testing.T) {
	const limit = 5
	st := &windowStore{delay: time.Millisecond}
	h := NewHandler(
		Config{
			Site:             "test",
			DefaultLang:      "zh",
			Langs:            []string{"en", "zh"},
			Require:          RequiredFrom(func(k string) bool { return k == FieldName || k == FieldEmail || k == FieldMessage }),
			RateLimitPerHour: limit,
			SuccessPath:      "/{lang}/thank-you/",
		},
		st, &fakeNotifier{}, nil, []byte("test-salt"),
		slog.New(slog.NewTextHandler(io.Discard, nil)), nil,
	)

	const n = 40
	var wg sync.WaitGroup
	var created, tooMany, other int32

	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			// 同一 IP：RemoteAddr 固定
			body := `{"name":"并发客户` + string(rune('A'+i%26)) + `","email":"c` +
				string(rune('a'+i%26)) + `@example.com","message":"需要煎茶 800 公斤，请报 FOB 宁波价。"}`
			rec := postJSON(h, body)
			switch rec.Code {
			case http.StatusCreated:
				atomic.AddInt32(&created, 1)
			case http.StatusTooManyRequests:
				atomic.AddInt32(&tooMany, 1)
			default:
				atomic.AddInt32(&other, 1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if other != 0 {
		t.Fatalf("出现非预期状态码 %d 个", other)
	}
	if got := st.count(); got != limit {
		t.Errorf("落库 %d 条，应为 %d 条——限流被并发绕过了", got, limit)
	}
	if created != limit {
		t.Errorf("返回 201 的次数 %d，应为 %d", created, limit)
	}
	if tooMany != n-limit {
		t.Errorf("返回 429 的次数 %d，应为 %d", tooMany, n-limit)
	}
}
