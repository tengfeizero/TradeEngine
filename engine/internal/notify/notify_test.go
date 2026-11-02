package notify

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tengfeizero/TradeEngine/internal/store"
)

func sample() *store.Inquiry {
	return &store.Inquiry{
		ID:           "01M5ET04V58R8Q1MESVR8V62NH",
		CreatedAt:    time.Now(),
		Site:         "zentea",
		Lang:         "en",
		Country:      "DE",
		Name:         "Thomas Müller",
		Company:      "Alpen Tee GmbH",
		Email:        "t.mueller@alpentee.de",
		ProductSlug:  "sencha-green-tea",
		ProductTitle: "Wuyi High Mountain Sencha",
		Message:      "We need 2 tons per year, organic certified.",
		SourcePage:   "/en/products/green-tea/sencha-green-tea/",
		UTMSource:    "google",
		UTMMedium:    "cpc",
		UTMCampaign:  "green-tea-eu",
		SpamScore:    0,
		Status:       "new",
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// 企业微信机器人要 msgtype/text 结构，其它结构它不认。
func TestWebhookWeComFormat(t *testing.T) {
	var got map[string]any
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		raw = string(b)
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(Config{
		SiteName:    "Misty Peak",
		WebhookURL:  srv.URL,
		RetryDelays: []time.Duration{10 * time.Millisecond},
	}, quietLogger(), nil)
	defer n.Close()

	n.deliver(sample())

	if got == nil {
		t.Fatal("webhook 未被调用")
	}
	// URL 里没有 qyapi，应走通用 JSON 分支；这里验证通用结构
	if got["event"] != "inquiry.created" {
		t.Fatalf("通用格式缺少 event 字段：%v", got)
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("通用格式缺少 data：%v", got)
	}
	if data["name"] != "Thomas Müller" {
		t.Errorf("payload 中客户姓名错误：%v", data["name"])
	}
	// ip_hash 是可反查的准标识符，发给第三方 webhook 没有任何必要
	if strings.Contains(raw, "ip_hash") {
		t.Errorf("webhook payload 泄漏了 ip_hash：%s", raw)
	}
	// 字段名要是 snake_case，对接方才能直接消费
	if _, ok := data["product_slug"]; !ok {
		t.Errorf("字段未使用 snake_case 命名：%v", data)
	}
}

func TestWebhookDetectsWeCom(t *testing.T) {
	// 只验证格式判定，不真的发请求
	n := &Notifier{cfg: Config{WebhookURL: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=x"}}
	p := n.webhookPayload(sample())
	if p["msgtype"] != "text" {
		t.Fatalf("企业微信应使用 msgtype/text，实际：%v", p)
	}
	text, _ := p["text"].(map[string]string)
	if !strings.Contains(text["content"], "Thomas Müller") {
		t.Errorf("webhook 正文缺少客户信息：%q", text["content"])
	}
}

func TestWebhookDetectsSlack(t *testing.T) {
	n := &Notifier{cfg: Config{WebhookURL: "https://hooks.slack.com/services/x"}}
	p := n.webhookPayload(sample())
	if _, ok := p["text"]; !ok {
		t.Fatalf("Slack 应使用 text 字段，实际：%v", p)
	}
}

// 通知失败要重试：SMTP/webhook 偶发 5xx 很常见，第一次失败就放弃等于丢提醒。
func TestWebhookRetriesThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if atomic.LoadInt32(&calls) < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var notified int32
	n := New(Config{
		SiteName:    "Misty Peak",
		WebhookURL:  srv.URL,
		RetryDelays: []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond},
	}, quietLogger(), func(string, time.Time) { atomic.AddInt32(&notified, 1) })
	defer n.Close()

	n.deliver(sample())

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("期望重试到第 3 次成功，实际调用 %d 次", got)
	}
	if atomic.LoadInt32(&notified) != 1 {
		t.Error("成功通知后未回调 MarkNotified")
	}
}

// 全部重试失败时不能 panic，也不能阻塞队列——询盘已在库里，通知失败只影响速度。
func TestWebhookAllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var notified int32
	n := New(Config{
		SiteName:    "Misty Peak",
		WebhookURL:  srv.URL,
		RetryDelays: []time.Duration{time.Millisecond},
	}, quietLogger(), func(string, time.Time) { atomic.AddInt32(&notified, 1) })
	defer n.Close()

	n.deliver(sample())

	if atomic.LoadInt32(&notified) != 0 {
		t.Error("全部失败时不该标记为已通知，否则日报兜底会漏掉它")
	}
}

func TestNotifyWithoutChannelsIsSilent(t *testing.T) {
	n := New(Config{SiteName: "x"}, quietLogger(), nil)
	defer n.Close()
	// 没配邮件也没配 webhook：什么都不做，但不能 panic
	n.deliver(sample())
}

func TestComposeSetsReplyToAndSource(t *testing.T) {
	n := &Notifier{cfg: Config{
		Recipients: []string{"sales@mistypeak.example.com"},
		SiteName:   "Misty Peak",
	}}
	msg, err := n.compose(sample(), "no-reply@mistypeak.example.com")
	if err != nil {
		t.Fatalf("生成邮件失败：%v", err)
	}
	s := string(msg)

	// Reply-To 指向客户：销售点「回复」就能直接对话，不用复制粘贴地址
	if !strings.Contains(s, "Reply-To: ") || !strings.Contains(s, "t.mueller@alpentee.de") {
		t.Errorf("缺少 Reply-To 客户地址：\n%s", s)
	}
	// 询盘编号进标题，便于邮件里检索
	if !strings.Contains(s, "01M5ET04V58R8Q1MESVR8V62NH") {
		t.Error("标题缺少询盘编号")
	}
	// 来源页与 UTM 是渠道归因的依据，必须在正文里
	for _, want := range []string{"/en/products/green-tea/sencha-green-tea/", "google / cpc / green-tea-eu"} {
		if !strings.Contains(s, want) {
			t.Errorf("正文缺少来源信息 %q：\n%s", want, s)
		}
	}
	// 主题含非 ASCII 时必须 RFC 2047 编码，否则邮件客户端会显示乱码
	if !strings.Contains(s, "Subject: =?UTF-8?") {
		t.Errorf("主题未做编码：\n%s", s)
	}
}

// 通知队列满时不能阻塞请求处理：询盘已经落库，通知丢了可以靠 CLI 补看。
func TestQueueFullDoesNotBlock(t *testing.T) {
	// 阻塞式 webhook，模拟投递卡住（真实场景是 SMTP 无响应）
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := New(Config{WebhookURL: srv.URL, RetryDelays: []time.Duration{time.Millisecond}}, quietLogger(), nil)

	done := make(chan struct{})
	go func() {
		for i := 0; i < queueSize+50; i++ {
			n.Notify(sample())
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		close(block)
		n.Close()
		t.Fatal("队列满时 Notify 阻塞了")
	}

	// 必须放行在途请求，否则 n.Close() 会一直等 worker 返回
	close(block)
	n.Close()
}
