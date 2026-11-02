package studio

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/contentdb"
)

// newEnv 造一个「文件站点 + 内容库 + 两个用户（admin/editor）」的测试环境。
// 产品带 specs：表单保存必须原样保留 schema 之外的字段。
func newEnv(t *testing.T) (handler http.Handler, dbPath string) {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("engine.yaml", "engine_version: 1\noutput: .build/public\n")
	write("settings.yaml", `base_url: https://example.test
default_lang: en
langs: [en, zh]
brand:
  name: Test Brand
  tagline: { en: Tagline, zh: 标语 }
contact:
  email: sales@example.test
`)
	write("content/products/sencha.yaml", `slug: sencha
status: draft
title: { en: Sencha, zh: 煎茶 }
summary: { en: Tasty, zh: 好喝 }
category: green-tea
featured: false
moq: 500
moq_unit: kg
order: 1
specs:
  - group: { en: Basic, zh: 基础 }
    items:
      - key: altitude
        label: { en: Altitude, zh: 海拔 }
        value_num: 1200
        filterable: true
`)
	write("content/pages/home.yaml", `slug: home
status: published
title: { en: Home, zh: 首页 }
blocks:
  - type: richtext
    body: { en: "Hello", zh: "你好" }
`)

	dbPath = filepath.Join(root, ".data", "content.db")
	if _, err := contentdb.Import(root, dbPath, false); err != nil {
		t.Fatalf("Import: %v", err)
	}

	env, err := contentdb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	if err := env.CreateUser("boss", "admin-pass-123", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := env.CreateUser("clerk", "editor-pass-123", "editor"); err != nil {
		t.Fatal(err)
	}

	h, err := Handler(Options{
		Root: root, DBPath: dbPath, Version: "test",
		Log:     discardLogger(),
		Rebuild: func() (BuildResult, error) { return BuildResult{OK: true, Pages: 1, Rebuilt: "now"}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := h.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	})
	return h, dbPath
}

// login 走真实登录流程拿会话 Cookie。
func login(t *testing.T, h http.Handler, username, password string) *http.Cookie {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	req := httptest.NewRequest("POST", "/studio/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("登录 %s：期望 302，得到 %d", username, rec.Code)
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookie {
			return ck
		}
	}
	t.Fatal("登录成功但没有会话 Cookie")
	return nil
}

// csrfOf 从编辑页 HTML 里抓 CSRF 令牌。
func csrfOf(t *testing.T, h http.Handler, ck *http.Cookie, path string) string {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET %s：期望 200，得到 %d", path, rec.Code)
	}
	re := regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`)
	m := re.FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatalf("页面 %s 没有 CSRF 令牌", path)
	}
	return m[1]
}

func post(t *testing.T, h http.Handler, ck *http.Cookie, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if ck != nil {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestEditPageRendersToEnd 守住「模板错误静默截断页面」这一类回归：
// 编辑页必须渲染出最后一个字段（排序值）与提交按钮。
func TestEditPageRendersToEnd(t *testing.T) {
	h, _ := newEnv(t)
	ck := login(t, h, "boss", "admin-pass-123")
	req := httptest.NewRequest("GET", "/studio/entities/products/sencha", nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("编辑页: %d", rec.Code)
	}
	for _, want := range []string{`name="order"`, `type="submit"`, `name="title.en"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("编辑页缺少 %q——模板可能渲染中途出错", want)
		}
	}
	if strings.Contains(body, "渲染出错") {
		t.Fatal("编辑页渲染报错")
	}
}

func TestUnauthenticatedRedirectsToLogin(t *testing.T) {	h, _ := newEnv(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/studio/", nil))
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "/studio/login") {
		t.Fatalf("未登录访问总览应跳登录页，得到 %d", rec.Code)
	}
	// 登录页本身可达
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/studio/login", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "登录") {
		t.Fatalf("登录页不可达: %d", rec.Code)
	}
}

func TestLoginRejectsBadPassword(t *testing.T) {
	h, dbPath := newEnv(t)
	rec := post(t, h, nil, "/studio/login", url.Values{
		"username": {"boss"}, "password": {"wrong"},
	})
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("错误口令应跳回登录页并带错误，得到 %d", rec.Code)
	}
	// 失败也进审计
	env, _ := contentdb.Open(dbPath)
	defer env.Close()
	entries, _ := env.ListAudit(10)
	if len(entries) == 0 || entries[0].Action != "login_fail" {
		t.Fatal("登录失败未记入审计")
	}
}

func TestEditorSavesDraftAndCannotPublish(t *testing.T) {
	h, dbPath := newEnv(t)
	ck := login(t, h, "clerk", "editor-pass-123")
	csrf := csrfOf(t, h, ck, "/studio/entities/products/sencha")

	// editor 保存草稿：改标题 + 起订量
	rec := post(t, h, ck, "/studio/entities/products/sencha/save", url.Values{
		"csrf":        {csrf},
		"title.en":    {"Sencha Prime"},
		"title.zh":    {"高级煎茶"},
		"summary.en":  {"Tasty"},
		"summary.zh":  {"好喝"},
		"moq":         {"300"},
		"moq_unit":    {"kg"},
		"featured":    {"on"},
		"description": {},
		"order":       {"5"},
	})
	if rec.Code != http.StatusFound {
		t.Fatalf("editor 保存草稿：期望 302，得到 %d：%s", rec.Code, rec.Body.String())
	}

	env, err := contentdb.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	row, err := env.GetEntity("products", "sencha")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(row.Data, "Sencha Prime") || !strings.Contains(row.Data, "高级煎茶") {
		t.Fatalf("标题未写入 payload:\n%s", row.Data)
	}
	// 提升列随 payload 同步
	if !strings.Contains(row.Data, "featured: true") {
		t.Fatal("featured 未写入")
	}
	var ord int
	_ = env.QueryRowInt("SELECT ord FROM products WHERE slug = 'sencha'", &ord)
	if ord != 5 {
		t.Fatalf("order 列未同步，得到 %d", ord)
	}

	// specs（schema 之外）必须原样保留
	if !strings.Contains(row.Data, "altitude") || !strings.Contains(row.Data, "filterable: true") {
		t.Fatal("schema 之外的 specs 字段丢失")
	}

	// editor 提交审核：允许
	rec = post(t, h, ck, "/studio/entities/products/sencha/action", url.Values{"csrf": {csrf}, "do": {"submit"}})
	if rec.Code != http.StatusFound {
		t.Fatalf("editor 提交审核被拒: %d", rec.Code)
	}
	// editor 发布：必须被拒
	rec = post(t, h, ck, "/studio/entities/products/sencha/action", url.Values{"csrf": {csrf}, "do": {"publish"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 发布应 403，得到 %d", rec.Code)
	}
	// 状态仍是 in_review
	row, _ = env.GetEntity("products", "sencha")
	if row.Status != StatusInReview {
		t.Fatalf("被拒绝的发布不应改状态，得到 %s", row.Status)
	}

	// admin 发布：允许
	ckA := login(t, h, "boss", "admin-pass-123")
	csrfA := csrfOf(t, h, ckA, "/studio/entities/products/sencha")
	rec = post(t, h, ckA, "/studio/entities/products/sencha/action", url.Values{"csrf": {csrfA}, "do": {"publish"}})
	if rec.Code != http.StatusFound {
		t.Fatalf("admin 发布被拒: %d", rec.Code)
	}
	row, _ = env.GetEntity("products", "sencha")
	if row.Status != StatusPublished {
		t.Fatalf("发布后状态应为 published，得到 %s", row.Status)
	}
	// 关键：payload 里的 status 也变了（build --db 解析 payload）
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(row.Data), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["status"] != StatusPublished {
		t.Fatalf("payload status 未同步：%v", doc["status"])
	}
}

func TestEditorCannotEditPublished(t *testing.T) {
	h, _ := newEnv(t)
	ckA := login(t, h, "boss", "admin-pass-123")
	csrfA := csrfOf(t, h, ckA, "/studio/entities/pages/home")
	// home 已是 published；editor 保存必须 403
	ck := login(t, h, "clerk", "editor-pass-123")
	rec := post(t, h, ck, "/studio/entities/pages/home/save", url.Values{
		"csrf": {"whatever"}, "title.en": {"Hacked"},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 改已发布内容应 403，得到 %d", rec.Code)
	}
	// admin 可以改
	rec = post(t, h, ckA, "/studio/entities/pages/home/save", url.Values{
		"csrf": {csrfA}, "title.en": {"Welcome"}, "title.zh": {"首页"},
	})
	if rec.Code != http.StatusFound {
		t.Fatalf("admin 改已发布内容被拒: %d", rec.Code)
	}
}

func TestMissingCSRFRejected(t *testing.T) {
	h, _ := newEnv(t)
	ck := login(t, h, "boss", "admin-pass-123")
	rec := post(t, h, ck, "/studio/entities/products/sencha/save", url.Values{
		"title.en": {"X"},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺 CSRF 应 403，得到 %d", rec.Code)
	}
}

func TestStatusTransitionGuards(t *testing.T) {
	h, dbPath := newEnv(t)
	ckA := login(t, h, "boss", "admin-pass-123")
	csrfA := csrfOf(t, h, ckA, "/studio/entities/products/sencha")
	// draft 不能直接 publish（必须先 submit）
	rec := post(t, h, ckA, "/studio/entities/products/sencha/action", url.Values{"csrf": {csrfA}, "do": {"publish"}})
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatalf("draft 直接 publish 应被状态机拒绝，得到 %d", rec.Code)
	}
	// 非法动作名
	rec = post(t, h, ckA, "/studio/entities/products/sencha/action", url.Values{"csrf": {csrfA}, "do": {"nuke"}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("未知动作应 400，得到 %d", rec.Code)
	}
	env, _ := contentdb.Open(dbPath)
	defer env.Close()
	row, _ := env.GetEntity("products", "sencha")
	if row.Status != StatusDraft {
		t.Fatal("失败的动作不应改变状态")
	}
}

func TestUsersManagement(t *testing.T) {
	h, dbPath := newEnv(t)
	// editor 访问用户管理：403
	ck := login(t, h, "clerk", "editor-pass-123")
	rec := post(t, h, ck, "/studio/users/add", url.Values{"username": {"x"}, "password": {"12345678"}, "role": {"editor"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("editor 加用户应 403，得到 %d", rec.Code)
	}
	// admin 创建 + 删除
	ckA := login(t, h, "boss", "admin-pass-123")
	csrf := csrfOf(t, h, ckA, "/studio/users")
	rec = post(t, h, ckA, "/studio/users/add", url.Values{
		"csrf": {csrf}, "username": {"alice"}, "password": {"alice-pass-123"}, "role": {"editor"},
	})
	if rec.Code != http.StatusFound {
		t.Fatalf("创建用户被拒: %d", rec.Code)
	}
	env, _ := contentdb.Open(dbPath)
	defer env.Close()
	users, _ := env.ListUsers()
	var aliceID int64
	for _, u := range users {
		if u.Username == "alice" {
			aliceID = u.ID
		}
	}
	if aliceID == 0 {
		t.Fatal("用户 alice 未创建")
	}
	// 最后一个 admin 不能删
	var bossID int64
	for _, u := range users {
		if u.Username == "boss" {
			bossID = u.ID
		}
	}
	rec = post(t, h, ckA, "/studio/users/delete", url.Values{"csrf": {csrf}, "id": {itoa64(bossID)}})
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "err=") {
		t.Fatal("删除最后一个管理员应被拒绝")
	}
	// 删除普通用户 OK，且其会话随之失效
	ckAlice := login(t, h, "alice", "alice-pass-123")
	rec = post(t, h, ckA, "/studio/users/delete", url.Values{"csrf": {csrf}, "id": {itoa64(aliceID)}})
	if rec.Code != http.StatusFound {
		t.Fatalf("删除普通用户被拒: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/studio/", nil)
	req.AddCookie(ckAlice)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatal("被删用户的会话应立即失效")
	}
}

func TestBuildPreviewAndAuditTrail(t *testing.T) {
	h, dbPath := newEnv(t)
	ck := login(t, h, "boss", "admin-pass-123")
	csrf := csrfOf(t, h, ck, "/studio/")
	rec := post(t, h, ck, "/studio/build", url.Values{"csrf": {csrf}})
	if rec.Code != http.StatusFound || !strings.Contains(rec.Header().Get("Location"), "msg=") {
		t.Fatalf("构建预览失败: %d", rec.Code)
	}
	env, _ := contentdb.Open(dbPath)
	defer env.Close()
	entries, _ := env.ListAudit(10)
	found := false
	for _, e := range entries {
		if e.Action == "build_ok" {
			found = true
		}
	}
	if !found {
		t.Fatal("构建未记入审计")
	}
}
