package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture 搭一个「带历史遗留字段」的最小站点，供迁移用例复用。
func fixture(t *testing.T) string {
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
	mk("settings.yaml", "# 全站设置\nbase_url: https://example.com/\ndefault_lang: en\n")
	mk("theme/theme.yaml", "name: acme\n")
	mk("content/pages/home.yaml", "slug: home\nstatus: published\ntitle: { en: Home, zh: 首页 }\n")
	mk("content/products/legacy.yaml", "slug: legacy\nstatus: published\ncat: main\n"+
		"title: { en: Legacy, zh: 旧产品 }\nsummary: { en: s, zh: 摘要 }\n"+
		"description: { en: d, zh: 描述 }\nlead_time: { en: 10d, zh: 10 天 }\npackaging: { en: carton, zh: 纸箱 }\n")
	mk("content/certificates.yaml",
		"- slug: iso9001\n  status: published\n  name: { en: ISO 9001, zh: ISO 9001 }\n  expiry: 2027-01-01\n")
	return root
}

func TestRunDetectsLegacyFields(t *testing.T) {
	root := fixture(t)
	rep, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(func() []string {
		var out []string
		for _, f := range rep.Findings {
			out = append(out, f.Level+"|"+f.File+"|"+f.Msg)
		}
		return out
	}(), "\n")

	for _, want := range []string{
		"fix|settings.yaml|base_url 以 / 结尾",
		"warn|content/products/legacy.yaml|未知字段 \"cat\"",
		"fix|content/products/legacy.yaml|cat → category",
		"fix|content/certificates.yaml|expiry → expires",
		"fix|theme/theme.yaml|缺少 layout",
		"fix|settings.yaml|缺少 images 段",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("体检结果缺少 %q，实际：\n%s", want, got)
		}
	}
}

func TestFixRenamesAndKeepsComments(t *testing.T) {
	root := fixture(t)
	rep, err := Fix(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Fixed) != 4 { // settings / theme / legacy / certificates
		t.Fatalf("应修复 4 个文件，实际 %v", rep.Fixed)
	}

	b, _ := os.ReadFile(filepath.Join(root, "content/products/legacy.yaml"))
	s := string(b)
	if strings.Contains(s, "cat:") {
		t.Error("cat 应已改名为 category")
	}
	if !strings.Contains(s, "category: main") {
		t.Error("category 值应保留")
	}

	b, _ = os.ReadFile(filepath.Join(root, "settings.yaml"))
	s = string(b)
	if !strings.HasPrefix(s, "# 全站设置") {
		t.Errorf("注释应保留，实际开头：%q", s[:30])
	}
	if !strings.Contains(s, "base_url: https://example.com") {
		t.Error("值不应被改动")
	}
	if strings.Contains(s, "https://example.com/") {
		// 尾斜杠属于 fix 级提示但本轮未实现自动去除——确认没引入双斜杠即可
		t.Error("尾斜杠提示存在但不应被误改出双斜杠")
	}

	// 备份里是原文件
	matches, _ := filepath.Glob(filepath.Join(root, ".backup", "*", "content", "certificates.yaml"))
	if len(matches) != 1 {
		t.Fatalf("备份应包含原 certificates.yaml，实际 %v", matches)
	}
	orig, _ := os.ReadFile(matches[0])
	if !strings.Contains(string(orig), "expiry:") {
		t.Error("备份应保留旧字段名")
	}
}

func TestNoFindingsOnCleanSite(t *testing.T) {
	root := fixture(t)
	// 把遗留字段改成新写法后，只剩无 fix/warn 的干净结构
	rewrite := func(rel, content string) {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rewrite("settings.yaml", "base_url: https://example.com\ndefault_lang: en\nlangs:\n  - en\nimages:\n  transcode: false\n")
	rewrite("theme/theme.yaml", "name: acme\nlayout: classic\n")
	rewrite("content/products/legacy.yaml", "slug: legacy\nstatus: published\ncategory: main\n"+
		"title: { en: L, zh: L }\nsummary: { en: s, zh: 摘要 }\ndescription: { en: d, zh: 描述 }\n"+
		"lead_time: { en: 10d, zh: 10 天 }\npackaging: { en: carton, zh: 纸箱 }\n")
	rewrite("content/certificates.yaml",
		"- slug: iso9001\n  status: published\n  name: { en: ISO, zh: ISO }\n  expires: 2027-01-01\n")

	rep, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range rep.Findings {
		if f.Level == "error" || f.Level == "fix" {
			t.Errorf("干净站点不应有 %s：[%s] %s", f.Level, f.File, f.Msg)
		}
	}
}

func TestMissingRequiredFile(t *testing.T) {
	root := fixture(t)
	if err := os.Remove(filepath.Join(root, "content/pages/home.yaml")); err != nil {
		t.Fatal(err)
	}
	rep, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range rep.Findings {
		if f.Level == "error" && f.File == "content/pages/home.yaml" {
			found = true
		}
	}
	if !found {
		t.Error("缺少 home.yaml 应报 error")
	}
}
