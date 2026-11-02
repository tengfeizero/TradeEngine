package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSite(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mk := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("engine.yaml", "engine_version: 1\noutput: .build/public\n")
	mk("settings.yaml", "base_url: https://example.com\ndefault_lang: en\n")
	mk("theme/theme.yaml", "name: acme\nlayout: classic\n")
	mk("content/pages/home.yaml", "slug: home\nstatus: published\ntitle: { en: Home, zh: 首页 }\n")
	mk("content/products/tea.yaml", "slug: tea\nstatus: published\ntitle: { en: Tea, zh: 茶 }\n")
	return root
}

func harness(t *testing.T, root string) http.Handler {
	t.Helper()
	return Handler(Options{
		Root:      root,
		PublicDir: filepath.Join(root, ".build", "public"),
		Rebuild: func() (BuildStatus, error) {
			return BuildStatus{OK: true, Pages: 2, Rebuilt: "now"}, nil
		},
	})
}

func TestListFiles(t *testing.T) {
	h := harness(t, testSite(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/files", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var out struct {
		Files       []fileEntry `json:"files"`
		Root        string      `json:"root"`
		DefaultLang string      `json:"default_lang"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.DefaultLang != "en" {
		t.Errorf("default_lang = %q", out.DefaultLang)
	}
	paths := map[string]bool{}
	for _, f := range out.Files {
		paths[f.Path] = true
	}
	for _, want := range []string{"settings.yaml", "theme/theme.yaml", "content/pages/home.yaml", "content/products/tea.yaml"} {
		if !paths[want] {
			t.Errorf("文件清单缺少 %s", want)
		}
	}
}

func TestSaveRoundTripWithBackup(t *testing.T) {
	root := testSite(t)
	h := harness(t, root)
	body := `{"path":"content/products/tea.yaml","content":"slug: tea\nstatus: draft\ntitle: { en: Tea, zh: 茶 }\n"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/admin/api/save", strings.NewReader(body)))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["ok"] != true {
		t.Fatalf("保存失败：%s", rec.Body.String())
	}
	b, _ := os.ReadFile(filepath.Join(root, "content/products/tea.yaml"))
	if !strings.Contains(string(b), "status: draft") {
		t.Error("新内容应已写盘")
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".backup", "admin", "*", "content", "products", "tea.yaml"))
	if len(matches) != 1 {
		t.Fatalf("应生成 1 份备份，实际 %v", matches)
	}
}

func TestSaveRejectsBrokenYAML(t *testing.T) {
	root := testSite(t)
	h := harness(t, root)
	orig, _ := os.ReadFile(filepath.Join(root, "settings.yaml"))
	body := `{"path":"settings.yaml","content":"base_url: [unclosed\n  bad"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/admin/api/save", strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["syntax"] == nil {
		t.Error("语法错误应返回 syntax 字段")
	}
	after, _ := os.ReadFile(filepath.Join(root, "settings.yaml"))
	if string(after) != string(orig) {
		t.Error("语法错误的 YAML 不应落盘")
	}
}

func TestSanitizePath(t *testing.T) {
	opts := Options{Root: "/tmp/site"}
	bad := []string{
		"", "../settings.yaml", "..\\settings.yaml", "main.go",
		"content/pages/../../settings.yaml", "content/notes.txt",
		"theme/../engine.yaml",
	}
	for _, p := range bad {
		if _, ok := sanitizePath(opts, p); ok {
			t.Errorf("%q 应被拒绝", p)
		}
	}
	good := []string{
		"settings.yaml", "theme/theme.yaml", "content/pages/home.yaml",
		"content/products/tea.yaml", "content/cases/eu-2025.yaml",
		"content/certificates.yaml",
	}
	for _, p := range good {
		if _, ok := sanitizePath(opts, p); !ok {
			t.Errorf("%q 应被放行", p)
		}
	}
}

func TestCreateProduct(t *testing.T) {
	root := testSite(t)
	h := harness(t, root)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/admin/api/new",
		strings.NewReader(`{"kind":"product","slug":"oolong"}`)))
	if rec.Code != 200 {
		t.Fatalf("创建失败：%s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "content/products/oolong.yaml")); err != nil {
		t.Error("新文件应已创建")
	}
	// 重复创建 → 409
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest("POST", "/admin/api/new",
		strings.NewReader(`{"kind":"product","slug":"oolong"}`)))
	if rec2.Code != http.StatusConflict {
		t.Errorf("重复创建应 409，实际 %d", rec2.Code)
	}
	// 非法 slug → 400
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, httptest.NewRequest("POST", "/admin/api/new",
		strings.NewReader(`{"kind":"product","slug":"../evil"}`)))
	if rec3.Code != 400 {
		t.Errorf("非法 slug 应 400，实际 %d", rec3.Code)
	}
}

func TestInquiriesEmptyWhenNoDB(t *testing.T) {
	h := Handler(Options{Root: testSite(t), PublicDir: "unused"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/api/inquiries", nil))
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Errorf("无库时应返回空列表，实际 %s", rec.Body.String())
	}
}
