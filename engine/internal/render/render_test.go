package render

import (
	"strings"
	"testing"

	"github.com/tradeengine/tradeengine/internal/icons"
	"github.com/tradeengine/tradeengine/internal/model"
	"github.com/tradeengine/tradeengine/internal/skins"
)

// styles 的 :root 是一个 12 参的 fmt.Sprintf——参数与占位符错一位，
// 页面不会报错，只是「主色跑到 ink、muted 变成字体栈」。
// 这个测试给每个槽一个可识别的值，断言它们各自落在正确的变量名后面。
func TestStylesVariablesAligned(t *testing.T) {
	r, err := New(&model.Site{Theme: model.Theme{
		Skin: "tech",
		Colors: map[string]string{
			"primary":      "#111111",
			"primary-fill": "#222222",
			"secondary":    "#333333",
			"ink":          "#444444",
			"paper":        "#555555",
			"accent":       "#666666",
			"surface":      "#777777",
			"muted":        "#888888",
		},
		Fonts: map[string]string{
			"heading": "FontA",
			"body":    "FontB",
			"mono":    "FontC",
		},
		Radius: "3px",
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"--primary:#111111":      "",
		"--primary-fill:#222222": "",
		"--secondary:#333333":    "",
		"--ink:#444444":          "",
		"--paper:#555555":        "",
		"--accent:#666666":       "",
		"--surface:#777777":      "",
		"--muted:#888888":        "",
		"--font-heading:FontA":   "",
		"--font-body:FontB":      "",
		"--font-mono:FontC":      "",
		"--radius:3px":           "",
	}
	for want := range cases {
		if !strings.Contains(string(r.styles()), want) {
			t.Errorf(":root 缺少 %q——变量与参数可能错位", want)
		}
	}
}

// tech 布局的深色基线只允许兜底：主题显式给出的色必须原样输出。
// 曾有版本把基线写死在 CSS 里盖掉主题，「按设计稿改配色毫无反应」就是它。
func TestThemeColorsOverrideTechBaseline(t *testing.T) {
	r, err := New(&model.Site{Theme: model.Theme{
		Skin:   "tech",
		Colors: map[string]string{"paper": "#0A0E1A", "ink": "#E2E8F0"},
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	out := string(r.styles())
	if !strings.Contains(out, "--paper:#0A0E1A") || !strings.Contains(out, "--ink:#E2E8F0") {
		t.Fatalf("主题色被 tech 基线覆盖: %s", out)
	}
}

// font_faces 生成 @font-face 且 preload 选标题字体最粗一档。
func TestFontFacesAndPreload(t *testing.T) {
	r, err := New(&model.Site{Theme: model.Theme{
		Fonts: map[string]string{"heading": "'Space Grotesk', sans-serif"},
		FontFaces: []model.FontFace{
			{Family: "Space Grotesk", File: "media/fonts/sg-500.woff2", Weight: "500"},
			{Family: "Space Grotesk", File: "media/fonts/sg-700.woff2", Weight: "700"},
		},
	}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.styles()), "@font-face{font-family:'Space Grotesk';src:url('/media/fonts/sg-500.woff2')") {
		t.Fatal("缺少 sg-500 的 @font-face")
	}
	pre := r.fontPreloads()
	if len(pre) != 1 || pre[0] != "/media/fonts/sg-700.woff2" {
		t.Fatalf("preload 应选中 700 一档，实际 %v", pre)
	}
}

// headline 的 {高亮词} 转成 <span class="hl">，HTML 特殊字符先转义。
func TestHighlightEscapes(t *testing.T) {
	if got := string(highlight("Fly Beyond {Li<mi&ts}.")); got != `Fly Beyond <span class="hl">Li&lt;mi&amp;ts</span>.` {
		t.Fatalf("高亮转义错误: %s", got)
	}
	if got := string(highlight("no braces")); strings.Contains(got, "span") {
		t.Fatalf("无花括号不应产生标签: %s", got)
	}
}

// features 区块与场景图标卡：契约是「图标只定义一次、按用到的收集」。
// 若 sprite 没生成，<use href="#i-cpu"> 会指向不存在的符号——页面上是
// 一片空白，控制台一声不响。这条测试就是钉住 sprite 与 use 的配对。
func TestFeaturesIconsAndSprite(t *testing.T) {
	site := &model.Site{
		Settings: model.Settings{
			Brand:       model.Brand{Name: "Fixture"},
			BaseURL:     "https://fixture.example.com",
			DefaultLang: "en",
			Langs:       []string{"en"},
		},
		Pages: []model.Page{{
			Base:  model.Base{Slug: "home", Status: "published"},
			Title: model.I18n{"en": "Home"},
			Blocks: []model.Block{
				{Type: "features", Extra: map[string]any{
					"style": "numbered",
					"items": []any{
						map[string]any{"icon": "cpu", "title": model.I18n{"en": "Flight control"}, "checks": []any{model.I18n{"en": "Dual IMU"}}},
						map[string]any{"icon": "camera", "title": model.I18n{"en": "Vision"}},
					},
				}},
				{Type: "scenario_grid", Extra: map[string]any{"title": model.I18n{"en": "Use cases"}}},
			},
		}},
		Scenarios: []model.Scenario{{
			Base: model.Base{Slug: "film", Status: "published"},
			Name: model.I18n{"en": "Aerial film"},
			Icon: "shield",
		}},
	}
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := r.Page(&site.Pages[0], "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, want := range []string{
		`id="i-cpu"`,                  // features 的图标符号
		`id="i-camera"`,               // 第二个图标
		`id="i-check"`,                // checks 列表用到的勾选图标
		`id="i-shield"`,               // 场景卡的图标
		`<use href="#i-cpu">`,         // 正文引用
		`<div class="no">01</div>`,    // numbered 风格自动编号
		`<div class="card icon-card"`, // 场景卡走图标卡而不是图片卡
	} {
		if !strings.Contains(html, want) {
			t.Errorf("页面缺少 %q", want)
		}
	}
	if strings.Contains(html, `id="i-cpu"></symbol>`) {
		t.Error("symbol 定义不应为空")
	}
}

// 未知图标在渲染层被丢弃：宁可没有图标，也不产出指向不存在符号的 <use>。
func TestUnknownIconDropped(t *testing.T) {
	if icons.Use("nope") != "" {
		t.Fatal("未知图标应返回空串")
	}
	sprite := icons.Sprite([]string{"cpu", "cpu", "nope", ""})
	if strings.Count(sprite, `id="i-cpu"`) != 1 {
		t.Fatalf("sprite 应去重且忽略未知名: %s", sprite)
	}
}

// 能力网格是二维网格：皮肤与断点都写死列数，最容易在移动端挤成两栏。
// 基础层保底，皮肤层再压一次——两段都要在，视觉回归由截图验证。
func TestFeaturesMobileCollapse(t *testing.T) {
	css := baseCSS(t)
	for _, want := range []string{
		".features{display:grid",
		".features.numbered{grid-template-columns:repeat(2,1fr)",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("基础层缺少 %q", want)
		}
	}
	// 900px 断点内必须重压一次列数，否则皮肤选择器会赢
	idx := strings.Index(css, "@media (max-width:900px)")
	if idx < 0 || !strings.Contains(css[idx:], "grid-template-columns:1fr") {
		t.Error("900px 断点内缺少能力网格的单列覆盖")
	}
	// 皮肤层：每套皮肤都要自带同一断点，否则三列/两列的皮肤在手机上不塌
	for _, sk := range skins.Presets() {
		if !strings.Contains(sk.CSS, "html[data-skin="+sk.Name+"]") {
			t.Errorf("皮肤 %s 的规则没有作用域前缀，会污染其他皮肤", sk.Name)
		}
		if !strings.Contains(sk.CSS, "@media (max-width:900px)") {
			t.Errorf("皮肤 %s 缺少 900px 断点，移动端列数不会塌", sk.Name)
		}
	}
}

// baseCSS 取出 base 模板里的 <style> 片段（模板是嵌入式资源，直接读文件）。
func baseCSS(t *testing.T) string {
	t.Helper()
	b, err := tmplFS.ReadFile("templates/base.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
