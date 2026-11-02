package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	// 每个测试用独立文件，避免相互污染
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("打开数据库失败：%v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func sample(id string) *Inquiry {
	return &Inquiry{
		ID:           id,
		CreatedAt:    time.Now(),
		Site:         "zentea",
		Lang:         "en",
		Country:      "DE",
		Name:         "Thomas Müller",
		Company:      "Alpen Tee GmbH",
		Email:        "t.mueller@alpentee.de",
		ProductSlug:  "sencha-green-tea",
		ProductTitle: "Wuyi High Mountain Sencha",
		Message:      "We need 2 tons per year.",
		SourcePage:   "/en/products/green-tea/sencha/",
		UTMSource:    "google",
		IPHash:       "abc123",
		Status:       "new",
	}
}

func TestInsertAndList(t *testing.T) {
	st := newTestStore(t)
	if err := st.Insert(sample("01M5ET04V58R8Q1MESVR8V62NH")); err != nil {
		t.Fatalf("写入失败：%v", err)
	}

	list, err := st.List(ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("查询失败：%v", err)
	}
	if len(list) != 1 {
		t.Fatalf("期望 1 条，实际 %d", len(list))
	}
	got := list[0]
	if got.ProductTitle != "Wuyi High Mountain Sencha" {
		t.Errorf("产品快照丢失：%q", got.ProductTitle)
	}
	if got.Email != "t.mueller@alpentee.de" {
		t.Errorf("邮箱错误：%q", got.Email)
	}
	if got.CreatedAt.IsZero() {
		t.Error("created_at 未解析")
	}
	// 中文内容必须能原样往返，不能被编码破坏
	if got.Name != "Thomas Müller" {
		t.Errorf("非 ASCII 字符往返失败：%q", got.Name)
	}
}

func TestInsertChineseRoundTrip(t *testing.T) {
	st := newTestStore(t)
	in := sample("01M5ET04V58R8Q1MESVR8V62NJ")
	in.Name = "李经理"
	in.Company = "上海茗香贸易"
	in.Message = "需要煎茶 800 公斤，请报 FOB 宁波价。"
	if err := st.Insert(in); err != nil {
		t.Fatalf("写入失败：%v", err)
	}

	// 用原始 SQL 读回，确认存的是 UTF-8 而不是被转义过的形式
	var name, msg string
	if err := st.db.QueryRow(`SELECT name, message FROM inquiry WHERE id=?`, in.ID).Scan(&name, &msg); err != nil {
		t.Fatalf("读取失败：%v", err)
	}
	if name != "李经理" {
		t.Errorf("姓名往返失败：%q", name)
	}
	if msg != in.Message {
		t.Errorf("留言往返失败：%q", msg)
	}
}

func TestListFilterByStatus(t *testing.T) {
	st := newTestStore(t)
	a := sample("01M5ET04V58R8Q1MESVR8V62NK")
	b := sample("01M5ET04V58R8Q1MESVR8V62NM")
	b.Status = "spam"
	if err := st.Insert(a); err != nil {
		t.Fatal(err)
	}
	if err := st.Insert(b); err != nil {
		t.Fatal(err)
	}

	spam, err := st.List(ListOptions{Status: "spam"})
	if err != nil {
		t.Fatal(err)
	}
	if len(spam) != 1 || spam[0].Status != "spam" {
		t.Fatalf("状态过滤失效：%+v", spam)
	}
}

// 限流计数必须持久化：内存计数在进程重启后会清零，攻击者只要等你重启就能绕过。
func TestCountByIPSince(t *testing.T) {
	st := newTestStore(t)
	stale := sample("01M5ET04V58R8Q1MESVR8V62NP")
	stale.CreatedAt = time.Now().Add(-2 * time.Hour)
	if err := st.Insert(stale); err != nil {
		t.Fatal(err)
	}
	fresh := sample("01M5ET04V58R8Q1MESVR8V62NQ")
	if err := st.Insert(fresh); err != nil {
		t.Fatal(err)
	}

	n, err := st.CountByIPSince("abc123", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("1 小时窗口内应只有 1 条，实际 %d", n)
	}

	all, err := st.CountByIPSince("abc123", time.Now().Add(-3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if all != 2 {
		t.Fatalf("3 小时窗口内应有 2 条，实际 %d", all)
	}
}

func TestMarkNotified(t *testing.T) {
	st := newTestStore(t)
	in := sample("01M5ET04V58R8Q1MESVR8V62NR")
	if err := st.Insert(in); err != nil {
		t.Fatal(err)
	}

	n, err := st.CountUnnotified()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("期望 1 条未通知，实际 %d", n)
	}

	if err := st.MarkNotified(in.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	n, err = st.CountUnnotified()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("标记后应为 0 条未通知，实际 %d", n)
	}
}

// 同一数据库重复打开必须幂等：服务重启时不该因为「表已存在」而崩。
func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "twice.db")

	first, err := Open(dsn)
	if err != nil {
		t.Fatalf("首次打开失败：%v", err)
	}
	if err := first.Insert(sample("01M5ET04V58R8Q1MESVR8V62NS")); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(dsn)
	if err != nil {
		t.Fatalf("二次打开失败：%v", err)
	}
	defer second.Close()

	list, err := second.List(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("重开后数据丢失，期望 1 条，实际 %d", len(list))
	}
}
