package skins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 头部注释是皮肤的自描述：解析错了，engine skins 的分组与深浅色兜底就全错。
func TestMetaParsing(t *testing.T) {
	css := `/*!
 * @skin demo
 * @label 演示皮肤
 * @category 测试类
 * @tone dark
 */
html{color:red}`
	name, label, category, tone := meta(css)
	if name != "demo" || label != "演示皮肤" || category != "测试类" || tone != "dark" {
		t.Fatalf("头部解析错误: %q %q %q %q", name, label, category, tone)
	}
	// 缺省 tone 必须是 light，不能是空串——Dark() 靠它判断
	if _, _, _, tone := meta("/* no header */ body{}"); tone != "light" {
		t.Fatalf("缺省 tone 应为 light，得到 %q", tone)
	}
}

// 预置皮肤的基本盘：每套都带着作用域前缀与移动端断点，缺一个就污染或破版。
func TestPresetsWellFormed(t *testing.T) {
	ps := Presets()
	if len(ps) < 6 {
		t.Fatalf("内置皮肤应有 6 套，得到 %d", len(ps))
	}
	seen := map[string]bool{}
	for _, s := range ps {
		if seen[s.Name] {
			t.Errorf("皮肤名重复: %s", s.Name)
		}
		seen[s.Name] = true
		if !strings.Contains(s.CSS, "html[data-skin="+s.Name+"]") {
			t.Errorf("皮肤 %s 规则缺少作用域前缀", s.Name)
		}
		if !strings.Contains(s.CSS, "@media (max-width:900px)") {
			t.Errorf("皮肤 %s 缺少 900px 断点", s.Name)
		}
		if s.Label == "" || s.Category == "" {
			t.Errorf("皮肤 %s 头部信息不全: label=%q category=%q", s.Name, s.Label, s.Category)
		}
	}
	if !seen[Default] {
		t.Fatalf("缺省皮肤 %q 不在内置清单里", Default)
	}
}

// 未知名必须报错，且报错要给出接近的候选——「写错一个字母静默换皮」就是这类坑。
func TestResolveUnknownSuggests(t *testing.T) {
	if _, err := Resolve("", "organicc"); err == nil {
		t.Fatal("未知皮肤应报错")
	} else if !strings.Contains(err.Error(), "organic") {
		t.Errorf("报错应提示最接近的名字 organic: %v", err)
	}
}

// ValidName 挡路径穿越：站点皮肤目录是客户可写的。
func TestValidNameRejectsTraversal(t *testing.T) {
	for _, bad := range []string{"", "..", "../evil", "a/b", "A", "skin name", strings.Repeat("x", 49)} {
		if ValidName(bad) {
			t.Errorf("%q 应被拒绝", bad)
		}
	}
	for _, good := range []string{"classic", "my-skin", "client_2026"} {
		if !ValidName(good) {
			t.Errorf("%q 应被接受", good)
		}
	}
}

// 站点 skins/ 目录优先于内置预置——客户改内置皮肤不需要等发版。
func TestSiteOverrideWins(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "skins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	css := "/*!\n * @skin organic\n * @label 客户改过的有机\n * @category 站点自定义\n * @tone light\n */\nhtml[data-skin=organic] body{background:#000}"
	if err := os.WriteFile(filepath.Join(dir, "organic.css"), []byte(css), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Resolve(root, "organic")
	if err != nil {
		t.Fatal(err)
	}
	if s.Preset {
		t.Fatal("站点覆盖版不应被标记为内置")
	}
	if !strings.Contains(s.CSS, "#000") {
		t.Error("应读到站点版本的 CSS")
	}
	// 全量清单里也要出现覆盖版
	for _, sk := range All(root) {
		if sk.Name == "organic" && sk.Preset {
			t.Fatal("All() 应返回站点覆盖版")
		}
	}
}
