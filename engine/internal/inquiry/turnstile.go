package inquiry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// turnstileVerifyURL 是 Cloudflare Turnstile 的服务端校验地址。
const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// Turnstile 校验人机令牌。
//
// 为什么选 Turnstile 而不是 reCAPTCHA（D8）：不依赖 Google、不用 Cookie，
// 欧盟客户的合规解释成本低得多，且免费。
type Turnstile struct {
	Secret   string
	Endpoint string // 可在测试中替换
	client   *http.Client
}

// NewTurnstile 构造校验器。secret 为空表示不启用。
func NewTurnstile(secret string) *Turnstile {
	return &Turnstile{
		Secret:   secret,
		Endpoint: turnstileVerifyURL,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Enabled 表示是否配置了密钥。
func (t *Turnstile) Enabled() bool { return t != nil && t.Secret != "" }

type turnstileResp struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

// Verify 校验令牌。
//
// 网络失败时的策略：**放行**（返回 nil），只记日志。
// 理由与「误判不删单」一致——Turnstile 挂了不该让客户无法询盘，
// 后面还有限流和人工复核兜底。反过来（网络失败就拒绝）会直接损失订单。
func (t *Turnstile) Verify(ctx context.Context, token, remoteIP string) error {
	if !t.Enabled() {
		return nil
	}
	if token == "" {
		return newErr(http.StatusBadRequest, "turnstile_missing", "")
	}

	form := url.Values{}
	form.Set("secret", t.Secret)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}

	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = turnstileVerifyURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil // 构造失败属于代码问题，放行以保证不丢单
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil // 见上方注释：网络异常放行
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return nil
	}
	var tr turnstileResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil
	}
	if tr.Success {
		return nil
	}
	// 令牌过期/重复使用对真实用户是「填太慢了」，提示他重试比直接拒绝更友好
	for _, c := range tr.ErrorCodes {
		if c == "timeout-or-duplicate" {
			return newErr(http.StatusBadRequest, "turnstile_expired", "")
		}
	}
	return newErr(http.StatusBadRequest, "turnstile_failed", "")
}
