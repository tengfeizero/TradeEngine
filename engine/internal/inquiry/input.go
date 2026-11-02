package inquiry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/tengfeizero/TradeEngine/internal/geo"
)

// 字段启用键名。与 settings.inquiry.fields 中的取值一致。
const (
	FieldName    = "name"
	FieldCompany = "company"
	FieldEmail   = "email"
	FieldPhone   = "phone"
	FieldCountry = "country"
	FieldProduct = "product"
	FieldVolume  = "volume"
	FieldMessage = "message"
)

// 长度上限。上限不是为了限制客户，而是挡住「往表单里灌 1MB 文本」的攻击。
const (
	maxName    = 80
	maxCompany = 120
	maxEmail   = 254
	maxPhone   = 40
	minMessage = 10
	maxMessage = 4000
	// maxVolume 是采购量估算字段的长度上限——值来自下拉框，正常永远短于这个。
	maxVolume = 80
)

// Input 是归一化后的提交内容。JSON 与表单两种编码都落到这个结构。
type Input struct {
	Lang       string `json:"lang"`
	Name       string `json:"name"`
	Company    string `json:"company"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Country    string `json:"country"`
	Product    string `json:"product"`
	Volume     string `json:"volume"`
	Message    string `json:"message"`
	SourcePage string `json:"source_page"`
	Referrer   string `json:"referrer"`

	UTMSource   string
	UTMMedium   string
	UTMCampaign string

	TurnstileToken string
	// Honeypot 是蜜罐字段：对真人不可见，被填即判机器人。
	Honeypot string
	// RenderTS 是页面渲染时刻的毫秒时间戳，用来判断「提交过快」。
	RenderTS string
}

// FieldError 是可返回给前端的字段级错误。
// 只暴露「哪个字段、什么规则」，不回显原始内容（避免被打点探测）。
type FieldError struct {
	Status int    `json:"-"`
	Code   string `json:"error"`
	Field  string `json:"field,omitempty"`
}

func (e *FieldError) Error() string {
	if e.Field != "" {
		return e.Code + " (" + e.Field + ")"
	}
	return e.Code
}

func newErr(status int, code, field string) *FieldError {
	return &FieldError{Status: status, Code: code, Field: field}
}

var errTooLarge = newErr(http.StatusBadRequest, "payload_too_large", "")

// 邮箱校验不追求 RFC 5322 全覆盖（那需要几百字符的正则且仍会误判），
// 只判断「格式上能发得出去」，最终有效性由退信决定。
var emailRe = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)

// maxBody 是请求体上限。一个 6 字段的表单不可能超过 32KB。
const maxBody = 32 << 10

// Parse 从请求中解析提交内容。支持 JSON 与 urlencoded 表单两种编码——
// 表单是给禁用 JS 的企业浏览器留的后路（B2B 采购环境常见）。
// 第二个返回值表示是否为表单提交（决定响应是 303 跳转还是 JSON）。
func Parse(r *http.Request) (*Input, bool, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)

	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}

	var in *Input
	var isForm bool
	var err error

	switch ct {
	case "application/x-www-form-urlencoded", "multipart/form-data":
		isForm = true
		in, err = parseForm(r)
	case "application/json", "":
		in, err = parseJSON(r)
	default:
		// 有些客户端不带 Content-Type 直接发 JSON，有些不带就发表单；
		// 这里按「先按 JSON 试，失败再按表单试」处理，避免真实客户提交失败。
		isForm = false
		in, err = parseJSON(r)
		if err != nil {
			isForm = true
			in, err = parseForm(r)
		}
	}
	if err != nil {
		return nil, isForm, err
	}
	normalize(in)
	return in, isForm, nil
}

// parseJSON 把 JSON 解到 map 再取值。
//
// 不直接解到 struct 的原因：前端容易发来未知字段或把数字发成字符串，
// 直接 struct 解析会因为一个多余字段让整次提交失败——这是真实客户，不该被技术细节挡住。
func parseJSON(r *http.Request) (*Input, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if isTooLarge(err) {
			return nil, errTooLarge
		}
		return nil, newErr(http.StatusBadRequest, "invalid_body", "")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, newErr(http.StatusBadRequest, "empty_body", "")
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, newErr(http.StatusBadRequest, "invalid_json", "")
	}
	str := func(k string) string {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	in := &Input{
		Lang:           str("lang"),
		Name:           str("name"),
		Company:        str("company"),
		Email:          str("email"),
		Phone:          str("phone"),
		Country:        str("country"),
		Product:        str("product"),
		Volume:         str("volume"),
		Message:        str("message"),
		SourcePage:     str("source_page"),
		Referrer:       str("referrer"),
		TurnstileToken: str("turnstile_token"),
		Honeypot:       str("_hp"),
		RenderTS:       str("render_ts"),
	}
	if utm, ok := m["utm"].(map[string]any); ok {
		g := func(k string) string {
			if s, ok := utm[k].(string); ok {
				return strings.TrimSpace(s)
			}
			return ""
		}
		in.UTMSource, in.UTMMedium, in.UTMCampaign = g("source"), g("medium"), g("campaign")
	}
	return in, nil
}

func parseForm(r *http.Request) (*Input, error) {
	if err := r.ParseForm(); err != nil {
		if isTooLarge(err) {
			return nil, errTooLarge
		}
		return nil, newErr(http.StatusBadRequest, "invalid_form", "")
	}
	v := r.PostForm
	get := func(k string) string { return strings.TrimSpace(v.Get(k)) }
	return &Input{
		Lang:           get("lang"),
		Name:           get("name"),
		Company:        get("company"),
		Email:          get("email"),
		Phone:          get("phone"),
		Country:        get("country"),
		Product:        get("product"),
		Volume:         get("volume"),
		Message:        get("message"),
		SourcePage:     get("source_page"),
		Referrer:       get("referrer"),
		UTMSource:      get("utm_source"),
		UTMMedium:      get("utm_medium"),
		UTMCampaign:    get("utm_campaign"),
		TurnstileToken: get("turnstile_token"),
		Honeypot:       v.Get("_hp"),
		RenderTS:       get("render_ts"),
	}, nil
}

func isTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return true
	}
	return strings.Contains(err.Error(), "request body too large")
}

// normalize 做与编码无关的清洗。
func normalize(in *Input) {
	trim := func(s string) string { return strings.TrimSpace(s) }
	in.Name = trim(in.Name)
	in.Company = trim(in.Company)
	in.Email = strings.ToLower(trim(in.Email))
	in.Phone = trim(in.Phone)
	in.Country = strings.ToUpper(trim(in.Country))
	in.Product = trim(in.Product)
	in.Message = trim(in.Message)
	if len([]rune(in.Volume)) > maxVolume {
		in.Volume = string([]rune(in.Volume)[:maxVolume])
	}
	in.SourcePage = trim(in.SourcePage)
	in.Lang = trim(in.Lang)
	// 统一换行，避免邮件正文出现奇怪空行
	in.Message = strings.ReplaceAll(in.Message, "\r\n", "\n")
}

// Required 是必填项集合。
type Required struct {
	Name, Email, Message bool
}

// RequiredFrom 由字段配置推导必填项。
func RequiredFrom(isRequired func(string) bool) Required {
	return Required{
		Name:    isRequired(FieldName),
		Email:   isRequired(FieldEmail),
		Message: isRequired(FieldMessage),
	}
}

// Validate 做字段级校验。
func (in *Input) Validate(req Required) error {
	if req.Name && in.Name == "" {
		return newErr(http.StatusBadRequest, "required", FieldName)
	}
	if in.Name != "" && len([]rune(in.Name)) > maxName {
		return newErr(http.StatusBadRequest, "too_long", FieldName)
	}
	if len([]rune(in.Company)) > maxCompany {
		return newErr(http.StatusBadRequest, "too_long", FieldCompany)
	}

	if req.Email && in.Email == "" {
		return newErr(http.StatusBadRequest, "required", FieldEmail)
	}
	if in.Email != "" {
		if len(in.Email) > maxEmail || !emailRe.MatchString(in.Email) {
			return newErr(http.StatusBadRequest, "invalid_email", FieldEmail)
		}
	}

	if len([]rune(in.Phone)) > maxPhone {
		return newErr(http.StatusBadRequest, "too_long", FieldPhone)
	}

	if req.Message && in.Message == "" {
		return newErr(http.StatusBadRequest, "required", FieldMessage)
	}
	if in.Message != "" {
		if len([]rune(in.Message)) < minMessage {
			return newErr(http.StatusBadRequest, "too_short", FieldMessage)
		}
		if len([]rune(in.Message)) > maxMessage {
			return newErr(http.StatusBadRequest, "too_long", FieldMessage)
		}
	}

	// 国家码用 422：格式没错，是业务上不认识这个值
	if in.Country != "" && !geo.Valid(in.Country) {
		return newErr(http.StatusUnprocessableEntity, "invalid_country", FieldCountry)
	}
	return nil
}

// TooFast 判断是否为「页面渲染后 3 秒内提交」。
// 真人不可能在 3 秒内读完表单并填完，这类请求一律视为机器人。
// 返回 false 表示时间戳不可解析或已过期——此时不做判断，宁可放过。
func (in *Input) TooFast(now time.Time) bool {
	ms, err := parseMillis(in.RenderTS)
	if err != nil {
		return false
	}
	delta := now.UnixMilli() - ms
	// 时间戳在未来（客户端时钟不准）或超过 24 小时，都不作为判断依据
	if delta < 0 || delta > 24*3600*1000 {
		return false
	}
	return delta < 3000
}

func parseMillis(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("空时间戳")
	}
	var ms int64
	if _, err := fmt.Sscanf(s, "%d", &ms); err != nil {
		return 0, err
	}
	return ms, nil
}

// 疑似垃圾内容的高频词。命中一个 +40，够触发 spam 标记。
var spamWords = []string{
	"seo services", "backlink", "guest post", "crypto", "casino", "viagra",
	"loan offer", "bitcoin investment", "click here to win",
	"免费推广", "代开发票", "网站排名", "加微信",
}

// SpamScore 返回 0–100 的垃圾可疑度。
//
// 关键取舍：**只打分，不删单**。判为 spam 的询盘照样入库、只是被标记，
// 因为「漏掉一条真询盘」的代价远大于「多存一条垃圾」。
func (in *Input) SpamScore(lang string) int {
	score := 0
	body := strings.ToLower(in.Name + " " + in.Company + " " + in.Message)

	// 链接数量：正常询盘极少贴链接
	links := strings.Count(body, "http://") + strings.Count(body, "https://") +
		strings.Count(body, "www.") + strings.Count(body, "[url=")
	switch {
	case links > 3:
		score += 40
	default:
		score += 15 * links
	}

	for _, w := range spamWords {
		if strings.Contains(body, w) {
			score += 40
			break
		}
	}

	// 英文站里出现中文：可能是同行或爬虫批量提交，但也可能是华人采购，所以只加 25
	if lang != "zh" && containsCJK(in.Message) {
		score += 25
	}

	// 全大写刷屏。注意不能只看比例：中文留言里出现「CIF」「FOB」这种缩写时，
	// ASCII 字母全是大写，比例是 100%，会把正常询盘误判。所以要求至少 20 个字母。
	if len([]rune(in.Message)) > 20 && shouty(in.Message) {
		score += 10
	}

	// 长串且完全没有空格（典型的机器拼接内容）
	if len([]rune(in.Message)) > 40 && !strings.Contains(in.Message, " ") {
		score += 20
	}

	if score > 100 {
		score = 100
	}
	return score
}

func containsCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// shouty 判断是否「整段全大写」。字母太少时不判定，避免缩写造成误报。
func shouty(s string) bool {
	var letters, upper int
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			letters++
		case r >= 'A' && r <= 'Z':
			letters++
			upper++
		}
	}
	if letters < 20 {
		return false
	}
	return float64(upper)/float64(letters) > 0.6
}
