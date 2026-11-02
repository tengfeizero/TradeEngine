package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer 造一个带最小产物的服务：首页 + 一个资产 + 一个 404 页。
func newTestServer(t *testing.T, allowOrigin string) http.Handler {
	t.Helper()
	dir := t.TempDir()

	write := func(rel, body string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("en/index.html", "<html>home</html>")
	write("en/products/sencha/index.html", "<html>sencha</html>")
	write("assets/inquiry.js", "console.log(1)")
	write("404.html", "<html>not found</html>")
	// 故意放一个「不该被读到」的文件，用于验证路径穿越防护
	write("secret.txt", "TOP-SECRET")

	var origins []string
	if allowOrigin != "" {
		origins = []string{allowOrigin}
	}

	s := New(Options{
		PublicDir:      dir,
		InquiryHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) }),
		AllowedOrigins: origins,
		Version:        "test",
	})
	return s.Handler()
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthz(t *testing.T) {
	rec := get(newTestServer(t, ""), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v", err)
	}
	if body["ok"] != true || body["version"] != "test" {
		t.Errorf("响应体异常：%v", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("健康检查不该被缓存")
	}
}

// 目录式 URL 必须补 index.html，否则所有页面内链都会 404。
func TestStaticServesDirectoryIndex(t *testing.T) {
	h := newTestServer(t, "")
	for _, p := range []string{"/en/", "/en/products/sencha/"} {
		rec := get(h, p)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s 期望 200，实际 %d", p, rec.Code)
		}
	}
	rec := get(h, "/en/products/sencha/")
	if !strings.Contains(rec.Body.String(), "sencha") {
		t.Errorf("返回了错误的文件：%s", rec.Body.String())
	}
}

// HTML 不缓存（内容更新要立刻生效），其他资源短缓存。
func TestStaticCacheHeaders(t *testing.T) {
	h := newTestServer(t, "")
	if got := get(h, "/en/").Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("HTML 期望 no-cache，实际 %q", got)
	}
	if got := get(h, "/assets/inquiry.js").Header().Get("Cache-Control"); !strings.Contains(got, "max-age") {
		t.Errorf("静态资源期望带 max-age，实际 %q", got)
	}
}

func TestStaticNotFoundUsesCustomPage(t *testing.T) {
	rec := get(newTestServer(t, ""), "/nope/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("期望 404，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not found") {
		t.Errorf("未使用自定义 404 页：%s", rec.Body.String())
	}
}

// 路径穿越必须被挡住：静态站目录里可能和配置、备份文件同级。
func TestStaticRejectsTraversal(t *testing.T) {
	h := newTestServer(t, "")
	paths := []string{
		"/../secret.txt",
		"/%2e%2e/secret.txt",
		"/en/../../secret.txt",
		"/..%2fsecret.txt",
	}
	for _, p := range paths {
		rec := get(h, p)
		if rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "TOP-SECRET") {
			t.Fatalf("路径穿越成功读到敏感文件：%s", p)
		}
	}
}

func TestCORSAllowedOrigin(t *testing.T) {
	h := newTestServer(t, "https://mistypeak.example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/inquiry", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://mistypeak.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://mistypeak.example.com" {
		t.Fatalf("白名单来源应被放行，实际 %q", got)
	}
	// Vary 必须带 Origin，否则 CDN 会把 A 站的响应缓存给 B 站
	if !strings.Contains(rec.Header().Get("Vary"), "Origin") {
		t.Error("缺少 Vary: Origin")
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	h := newTestServer(t, "https://mistypeak.example.com")

	req := httptest.NewRequest(http.MethodPost, "/api/inquiry", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// 不返回 ACAO，浏览器就会拦下这次跨源请求
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("未授权来源不该拿到 ACAO，实际 %q", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := get(newTestServer(t, ""), "/en/")
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("缺少 X-Content-Type-Options: nosniff")
	}
	if rec.Header().Get("Referrer-Policy") == "" {
		t.Error("缺少 Referrer-Policy")
	}
}

// 一个处理器的 panic 不该让整个服务挂掉。
func TestRecovererReturns500(t *testing.T) {
	s := New(Options{
		PublicDir: t.TempDir(),
		InquiryHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		}),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/inquiry", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("期望 500，实际 %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "boom") {
		t.Error("响应泄漏了 panic 内容")
	}
}
