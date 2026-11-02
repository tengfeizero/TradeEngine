// Package notify 负责「询盘落库之后」的通知：邮件 + webhook，带重试。
//
// 三条设计约束（来自《11》10.3/10.4）：
//  1. 通知必须是**尽力而为**：它失败只影响提醒速度，绝不能让客户提交失败；
//  2. 通知在**落库之后**触发：反过来做，邮件服务一挂就丢单；
//  3. 通知要带**来源页 + UTM + 语言**：没有这些信息，后续无法做渠道归因。
package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tradeengine/tradeengine/internal/store"
)

// Config 是通知配置。密钥类信息走环境变量，不写进 settings.yaml（会被提交到 Git）。
type Config struct {
	Recipients []string
	SiteName   string
	SiteURL    string

	// SMTP
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	From     string

	// Webhook 支持企业微信机器人 / Slack / 通用 JSON
	WebhookURL string

	// RetryDelays 是每次重试前的等待时间，长度为重试次数。
	RetryDelays []time.Duration
}

// Notifier 是异步通知器。
type Notifier struct {
	cfg    Config
	log    *slog.Logger
	queue  chan *store.Inquiry
	onSent func(id string, at time.Time)
	wg     sync.WaitGroup
	once   sync.Once
	http   *http.Client
}

const queueSize = 256

// New 构造通知器并启动 2 个投递协程。
//
// 为什么是固定 2 个而不是「每条询盘一个 goroutine」：客户在广告投放期可能瞬间来十几条，
// 无上限 goroutine 会同时打爆 SMTP 连接数并触发退信；固定 2 个既能并行又不会失控。
func New(cfg Config, log *slog.Logger, onSent func(id string, at time.Time)) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	if len(cfg.RetryDelays) == 0 {
		cfg.RetryDelays = []time.Duration{2 * time.Second, 6 * time.Second, 18 * time.Second}
	}
	n := &Notifier{
		cfg:    cfg,
		log:    log,
		queue:  make(chan *store.Inquiry, queueSize),
		onSent: onSent,
		http:   &http.Client{Timeout: 8 * time.Second},
	}
	for i := 0; i < 2; i++ {
		n.wg.Add(1)
		go n.worker()
	}
	return n
}

// Notify 把询盘放入投递队列。这是非阻塞调用：
// 队列满时直接丢弃并记错误——询盘已经在库里，通知丢了还能靠 CLI 补看。
func (n *Notifier) Notify(inq *store.Inquiry) {
	select {
	case n.queue <- inq:
	default:
		n.log.Error("通知队列已满，跳过本次通知（询盘已入库）", "id", inq.ID)
	}
}

// Close 关闭队列并等待在途投递完成。
func (n *Notifier) Close() {
	n.once.Do(func() { close(n.queue) })
	n.wg.Wait()
}

func (n *Notifier) worker() {
	defer n.wg.Done()
	for inq := range n.queue {
		n.deliver(inq)
	}
}

// deliver 依次尝试邮件与 webhook，任一成功即视为已通知。
func (n *Notifier) deliver(inq *store.Inquiry) {
	hasMail := len(n.cfg.Recipients) > 0 && n.cfg.SMTPHost != ""
	hasHook := n.cfg.WebhookURL != ""

	if !hasMail && !hasHook {
		// 没配通知渠道不是错误，但必须让人知道「询盘只躺在库里」
		n.log.Warn("未配置邮件或 webhook，询盘仅入库", "id", inq.ID)
		return
	}

	var mailErr, hookErr error
	mailOK, hookOK := false, false
	if hasMail {
		mailErr = n.withRetry("email", inq.ID, func() error { return n.sendMail(inq) })
		mailOK = mailErr == nil
	}
	if hasHook {
		hookErr = n.withRetry("webhook", inq.ID, func() error { return n.postWebhook(inq) })
		hookOK = hookErr == nil
	}

	// 只要有一个「配置了的」渠道成功，就算已通知。
	// 注意不能写成 `mailErr == nil || hookErr == nil`：没配置邮件时 mailErr 恒为 nil，
	// 那样 webhook 全部失败也会被当成已通知，日报兜底就会漏掉这条询盘。
	if mailOK || hookOK {
		if n.onSent != nil {
			n.onSent(inq.ID, time.Now())
		}
		return
	}
	// 两条链路都失败：只记日志。询盘仍在库里，可用 engine inquiries 补看
	n.log.Error("通知全部失败，请手动查看", "id", inq.ID,
		"mail_err", mailErr, "webhook_err", hookErr)
}

// withRetry 按配置的间隔重试。失败次数用尽后返回最后一次错误。
func (n *Notifier) withRetry(channel, id string, fn func() error) error {
	var err error
	for attempt := 0; attempt <= len(n.cfg.RetryDelays); attempt++ {
		if attempt > 0 {
			time.Sleep(n.cfg.RetryDelays[attempt-1])
		}
		if err = fn(); err == nil {
			return nil
		}
		n.log.Warn("通知投递失败", "channel", channel, "id", id, "attempt", attempt+1, "err", err)
	}
	return err
}

// ---------------------------------------------------------------- 邮件

func (n *Notifier) sendMail(inq *store.Inquiry) error {
	from := n.cfg.From
	if from == "" {
		from = n.cfg.SMTPUser
	}
	if from == "" {
		return fmt.Errorf("未配置发件地址")
	}
	msg, err := n.compose(inq, from)
	if err != nil {
		return err
	}
	return n.sendMsg(msg)
}

// SendReport 发送一封人工构造的邮件（未读询盘日报）。
//
// 与 Notify 的区别：它是**同步**的，因为 cron 场景需要拿到真实成败来决定
// 是否标记「已通知」；异步投递在进程退出时会被丢掉。
func (n *Notifier) SendReport(subject, body string) error {
	if n.cfg.SMTPHost == "" {
		return fmt.Errorf("未配置 SMTP（ENGINE_SMTP_HOST）")
	}
	if len(n.cfg.Recipients) == 0 {
		return fmt.Errorf("未配置收件人（settings.inquiry.recipients）")
	}
	from := n.cfg.From
	if from == "" {
		from = n.cfg.SMTPUser
	}
	if from == "" {
		return fmt.Errorf("未配置发件地址（ENGINE_MAIL_FROM）")
	}

	var b bytes.Buffer
	b.WriteString("From: " + formatAddr(n.cfg.SiteName, from) + "\r\n")
	b.WriteString("To: " + strings.Join(n.cfg.Recipients, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("Message-ID: <digest-" + strconv.FormatInt(time.Now().Unix(), 10) + "@" + messageIDDomain(from) + ">\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return n.sendMsg(b.Bytes())
}

// sendMsg 按端口选择 TLS 方式投递已拼好的邮件。
func (n *Notifier) sendMsg(msg []byte) error {
	port := n.cfg.SMTPPort
	if port == 0 {
		port = 587
	}
	addr := net.JoinHostPort(n.cfg.SMTPHost, strconv.Itoa(port))

	// 465 是隐式 TLS（连上就加密），587 是显式 STARTTLS。两者握手方式不同，必须分开处理。
	if port == 465 {
		return n.sendImplicitTLS(addr, msg)
	}
	return n.sendSTARTTLS(addr, msg)
}

func (n *Notifier) sendSTARTTLS(addr string, msg []byte) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("连接 SMTP 失败: %w", err)
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: n.cfg.SMTPHost}); err != nil {
			return fmt.Errorf("STARTTLS 失败: %w", err)
		}
	}
	return n.authAndSend(c, msg)
}

func (n *Notifier) sendImplicitTLS(addr string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: n.cfg.SMTPHost})
	if err != nil {
		return fmt.Errorf("TLS 连接失败: %w", err)
	}
	c, err := smtp.NewClient(conn, n.cfg.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("SMTP 握手失败: %w", err)
	}
	defer c.Close()
	return n.authAndSend(c, msg)
}

func (n *Notifier) authAndSend(c *smtp.Client, msg []byte) error {
	if n.cfg.SMTPUser != "" {
		if ok, _ := c.Extension("AUTH"); ok {
			auth := smtp.PlainAuth("", n.cfg.SMTPUser, n.cfg.SMTPPass, n.cfg.SMTPHost)
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("SMTP 认证失败: %w", err)
			}
		}
	}
	from := n.cfg.From
	if from == "" {
		from = n.cfg.SMTPUser
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("MAIL FROM 失败: %w", err)
	}
	for _, to := range n.cfg.Recipients {
		if err := c.Rcpt(to); err != nil {
			return fmt.Errorf("RCPT TO 失败 (%s): %w", to, err)
		}
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("DATA 失败: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("写入正文失败: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("结束 DATA 失败: %w", err)
	}
	return c.Quit()
}

// compose 生成 RFC 5322 邮件。
//
// Reply-To 设为客户邮箱是最实用的一个细节：销售只需要点「回复」就能直接跟客户对话，
// 不用复制粘贴地址——响应速度是 B 端成交第一杠杆。
func (n *Notifier) compose(inq *store.Inquiry, from string) ([]byte, error) {
	var b bytes.Buffer

	subject := n.subject(inq)
	b.WriteString("From: " + formatAddr(n.cfg.SiteName, from) + "\r\n")
	b.WriteString("To: " + strings.Join(n.cfg.Recipients, ", ") + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n")
	if inq.Email != "" {
		b.WriteString("Reply-To: " + formatAddr(inq.Name, inq.Email) + "\r\n")
	}
	b.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	// 同一询盘的重复通知（重试）可被邮件客户端折叠
	b.WriteString("Message-ID: <" + inq.ID + "@" + messageIDDomain(from) + ">\r\n")
	b.WriteString("\r\n")
	b.WriteString(n.body(inq))
	return b.Bytes(), nil
}

func (n *Notifier) subject(inq *store.Inquiry) string {
	parts := []string{"[询盘 " + inq.ID + "]"}
	switch {
	case inq.Company != "":
		parts = append(parts, inq.Company)
	case inq.Name != "":
		parts = append(parts, inq.Name)
	}
	if inq.ProductTitle != "" {
		parts = append(parts, inq.ProductTitle)
	} else if inq.Country != "" {
		parts = append(parts, inq.Country)
	}
	return strings.Join(parts, " · ")
}

func (n *Notifier) body(inq *store.Inquiry) string {
	var b strings.Builder
	line := func(k, v string) {
		if v != "" {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	line("客户", inq.Name)
	line("公司", inq.Company)
	line("邮箱", inq.Email)
	line("电话", inq.Phone)
	line("国家", inq.Country)
	line("年采购量", inq.Volume)
	line("感兴趣产品", inq.ProductTitle)
	line("语言", inq.Lang)
	b.WriteString("\n--- 留言 ---\n")
	b.WriteString(inq.Message + "\n")
	b.WriteString("\n--- 来源 ---\n")
	line("来源页", inq.SourcePage)
	line("Referrer", inq.Referrer)
	line("UTM", strings.Join(nonEmpty(inq.UTMSource, inq.UTMMedium, inq.UTMCampaign), " / "))
	line("提交时间", inq.CreatedAt.Format("2006-01-02 15:04:05 MST"))
	line("询盘编号", inq.ID)
	if inq.SpamScore > 0 {
		line("垃圾可疑度", strconv.Itoa(inq.SpamScore))
	}
	return b.String()
}

func formatAddr(name, addr string) string {
	if name == "" {
		return "<" + addr + ">"
	}
	if strings.ContainsAny(name, "()<>@,;:\\\".[]") {
		return mime.QEncoding.Encode("UTF-8", name) + " <" + addr + ">"
	}
	return name + " <" + addr + ">"
}

func messageIDDomain(from string) string {
	if i := strings.LastIndex(from, "@"); i >= 0 && i+1 < len(from) {
		return from[i+1:]
	}
	return "localhost"
}

// ---------------------------------------------------------------- Webhook

func (n *Notifier) postWebhook(inq *store.Inquiry) error {
	if n.cfg.WebhookURL == "" {
		return fmt.Errorf("未配置 webhook")
	}
	payload := n.webhookPayload(inq)
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, n.cfg.WebhookURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook 返回 %d", resp.StatusCode)
	}
	return nil
}

// webhookPayload 按目标平台生成不同格式。
// 企业微信机器人要 msgtype，Slack 要 text，其它一律送结构化 JSON（方便自建服务消费）。
func (n *Notifier) webhookPayload(inq *store.Inquiry) map[string]any {
	text := n.body(inq)
	switch detectWebhookKind(n.cfg.WebhookURL) {
	case "wecom":
		return map[string]any{"msgtype": "text", "text": map[string]string{"content": text}}
	case "slack":
		return map[string]any{"text": "```" + text + "```"}
	case "lark":
		return map[string]any{"msg_type": "text", "content": map[string]string{"text": text}}
	default:
		return map[string]any{
			"event": "inquiry.created",
			"site":  n.cfg.SiteName,
			"data":  toPublic(inq),
		}
	}
}

// publicInquiry 是发给第三方 webhook 的询盘表示。
//
// 为什么不直接序列化 store.Inquiry：一是字段名会变成 Go 命名（ID/ProductSlug），
// 对接方难以使用；二是会把 ip_hash 一起发出去——那是可反查的准标识符，
// 发给 Slack 或企业微信没有任何必要，属于纯粹的数据外泄。
type publicInquiry struct {
	ID           string `json:"id"`
	CreatedAt    string `json:"created_at"`
	Lang         string `json:"lang"`
	Country      string `json:"country,omitempty"`
	Name         string `json:"name"`
	Company      string `json:"company,omitempty"`
	Email        string `json:"email"`
	Phone        string `json:"phone,omitempty"`
	ProductSlug  string `json:"product_slug,omitempty"`
	ProductTitle string `json:"product_title,omitempty"`
	Volume       string `json:"volume,omitempty"`
	Message      string `json:"message"`
	SourcePage   string `json:"source_page,omitempty"`
	Referrer     string `json:"referrer,omitempty"`
	UTMSource    string `json:"utm_source,omitempty"`
	UTMMedium    string `json:"utm_medium,omitempty"`
	UTMCampaign  string `json:"utm_campaign,omitempty"`
	SpamScore    int    `json:"spam_score"`
}

func toPublic(inq *store.Inquiry) publicInquiry {
	return publicInquiry{
		ID:           inq.ID,
		CreatedAt:    inq.CreatedAt.Format(time.RFC3339),
		Lang:         inq.Lang,
		Country:      inq.Country,
		Name:         inq.Name,
		Company:      inq.Company,
		Email:        inq.Email,
		Phone:        inq.Phone,
		ProductSlug:  inq.ProductSlug,
		ProductTitle: inq.ProductTitle,
		Volume:       inq.Volume,
		Message:      inq.Message,
		SourcePage:   inq.SourcePage,
		Referrer:     inq.Referrer,
		UTMSource:    inq.UTMSource,
		UTMMedium:    inq.UTMMedium,
		UTMCampaign:  inq.UTMCampaign,
		SpamScore:    inq.SpamScore,
	}
}

func detectWebhookKind(u string) string {
	switch {
	case strings.Contains(u, "qyapi.weixin.qq.com"):
		return "wecom"
	case strings.Contains(u, "hooks.slack.com"):
		return "slack"
	case strings.Contains(u, "open.feishu.cn"):
		return "lark"
	}
	return ""
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
