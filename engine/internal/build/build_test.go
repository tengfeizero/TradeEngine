package build

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newSite 在临时目录里造一个「刚好通过校验」的最小站点。
// 集成测试比单元测试更值钱的地方：它证明 24 条规则、渲染、产物路径、
// sitemap 与链接校验能一起跑通——这些环节单独测都过，串起来照样会断。
func newSite(t *testing.T) string {
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

	write("engine.yaml", "engine_version: 1\noutput: .build/public\n")
	write("settings.yaml", `base_url: https://example.test
schema_version: 1
default_lang: en
langs: [en, zh]
theme: t

brand:
  name: Test Brand
  tagline: { en: Test tagline, zh: 测试标语 }

company:
  legal_name: Test Co., Ltd.
  address: { country: CN, region: Zhejiang, city: Yiwu }

contact:
  email: sales@example.test

markets:
  - code: EU
    name: { en: Europe, zh: 欧洲 }

nav:
  - label: { en: Home, zh: 首页 }
    href: "/"
  - label: { en: Products, zh: 产品 }
    href: "/products/"
  - label: { en: Contact, zh: 联系 }
    href: "/contact/"

inquiry:
  recipients: [sales@example.test]

store:
  driver: sqlite
  dsn: .data/inquiries.db
`)
	write("theme/theme.yaml", "name: t\nlayout: editorial\n")
	write("content/pages/home.yaml", `slug: home
status: published
title: { en: Home, zh: 首页 }
blocks:
  - type: hero
    headline: { en: Welcome, zh: 欢迎 }
    sub: { en: B2B supplier, zh: B2B 供应商 }
    cta_text: { en: Quote, zh: 报价 }
    cta_href: "/contact/"
  - type: product_grid
    limit: 4
  - type: inquiry
`)
	write("content/pages/contact.yaml", `slug: contact
status: published
title: { en: Contact, zh: 联系 }
blocks:
  - type: inquiry
`)
	write("content/categories.yaml", `- slug: green
  status: published
  name: { en: Green, zh: 绿茶 }
`)
	write("content/products/tea.yaml", `slug: tea
status: published
category: green
title: { en: Test Tea, zh: 测试茶 }
summary: { en: A tea, zh: 一款茶 }
description: { en: Long description, zh: 详细描述 }
lead_time: { en: 15 days, zh: 15 天 }
packaging: { en: 25kg bag, zh: 25 公斤袋 }
moq: 100
moq_unit: kg
hs_code: "0902100000"
cover: media/tea.png
specs:
  - group: { en: Data, zh: 参数 }
    items:
      - key: origin
        label: { en: Origin, zh: 产地 }
        value: { en: Wuyi, zh: 武义 }
`)
	// 一张 4×3 的纯色 PNG：验证图片管线能读到真实尺寸
	write("media/tea.png", "")
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 40, B: 40, A: 255})
		}
	}
	f, err := os.Create(filepath.Join(dir, "media", "tea.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBuildEmitsM1PagesAndSitemap(t *testing.T) {
	dir := newSite(t)
	res, err := Run(dir)
	if err != nil {
		t.Fatalf("构建失败: %v", err)
	}
	for _, e := range res.Report.Errors {
		t.Errorf("不应有错误: [%s] %s", e.Code, e.Message)
	}

	want := []string{
		"en/index.html",
		"en/products/index.html",
		"en/products/green/index.html",
		"en/products/green/tea/index.html",
		"en/contact/index.html",
		"en/thank-you/index.html",
		"en/404.html",
		"404.html",
		"sitemap.xml",
		"robots.txt",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(res.OutputDir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("缺少产物 %s", rel)
		}
	}
	// 媒体文件名必须带内容哈希（immutable 缓存的前提），
	// 且 HTML 里的引用同步改写为哈希文件名。
	matches, _ := filepath.Glob(filepath.Join(res.OutputDir, "media", "tea.*.png"))
	if len(matches) != 1 {
		t.Fatalf("media/tea.png 应以内容哈希文件名产出，实际 %v", matches)
	}
	hashed := "media/" + filepath.Base(matches[0])
	if hashed == "media/tea.png" {
		t.Fatal("哈希文件名不应与原文件名相同")
	}
	b, _ := os.ReadFile(filepath.Join(res.OutputDir, "en", "products", "green", "tea", "index.html"))
	if !strings.Contains(string(b), `"/`+hashed+`"`) {
		t.Errorf("HTML 引用应改写为 %s", hashed)
	}

	sm, err := os.ReadFile(filepath.Join(res.OutputDir, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(sm)
	// 5 类页面（首页 / 联系 / 产品总览 / 分类 / 产品）× 2 语言
	if n := strings.Count(s, "<url>"); n != 10 {
		t.Errorf("sitemap 应有 10 条，实际 %d", n)
	}
	// 每个 URL 的 hreflang 组必须包含自身，否则整组无效
	if !strings.Contains(s, `hreflang="zh" href="https://example.test/zh/products/"`) {
		t.Error("sitemap 缺少 zh 产品总览的 hreflang 互链")
	}
	if strings.Contains(s, "thank-you") {
		t.Error("thank-you 页不应进入 sitemap")
	}

	html, err := os.ReadFile(filepath.Join(res.OutputDir, "en", "products", "green", "tea", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	// 防 CLS：图片必须带真实宽高
	if !strings.Contains(page, `width="4" height="3"`) {
		t.Error("产品封面图缺少宽高属性")
	}
	// 结构化数据不应写假价格（D9：全部询价）
	if strings.Contains(page, `"offers"`) {
		t.Error("不应输出 offers 价格结构化数据")
	}
	if !strings.Contains(page, `data-skin="editorial"`) {
		t.Error("主题皮肤没有生效")
	}
	// 皮肤 CSS 必须真的被内联进来，否则 data-skin 只是个标记
	if !strings.Contains(page, "html[data-skin=editorial]") {
		t.Error("皮肤样式没有注入")
	}
	if !strings.Contains(page, `hreflang="x-default"`) {
		t.Error("缺少 x-default")
	}
}

func TestBuildFailsOnUnknownBlock(t *testing.T) {
	dir := newSite(t)
	// 把 home 的 hero 改成拼错的类型：这类错误在页面上完全看不出来
	home := filepath.Join(dir, "content", "pages", "home.yaml")
	b, err := os.ReadFile(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(home, []byte(strings.Replace(string(b), "type: hero", "type: Hero", 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Report.Errors) == 0 {
		t.Fatal("未知区块类型必须构建失败")
	}
	code := res.Report.Errors[0].Code
	if code != "BLOCK_UNKNOWN" && code != "HOME_NO_HERO" {
		t.Errorf("期望 BLOCK_UNKNOWN/HOME_NO_HERO，实际 %s", code)
	}
	// 有错误时不能产出半成品
	if _, err := os.Stat(filepath.Join(res.OutputDir, "en", "index.html")); err == nil {
		t.Error("有错误时不应产出页面")
	}
}

func TestBuildRejectsBrokenNav(t *testing.T) {
	dir := newSite(t)
	path := filepath.Join(dir, "settings.yaml")
	b, _ := os.ReadFile(path)
	out := strings.Replace(string(b), `href: "/contact/"`, `href: "/nowhere/"`, 1)
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range res.Report.Errors {
		if e.Code == "NAV_BROKEN" {
			found = true
		}
	}
	if !found {
		t.Error("导航指向不存在的页面必须报 NAV_BROKEN")
	}
}

// 骨架必须自带能构建的主题。
//
// 这里锁住一个真实缺陷：`heading: "Inter", system-ui, sans-serif` 是非法 YAML
// ——引号只包住了第一个字体名，逗号后面的裸值让解析器在第 8 行报错。
// classic 的 `Georgia, serif` 没有引号所以一路正常，于是三种布局里只有
// tech / editorial 的新站「建完就构建失败」，而 CI 的 new→build 冒烟用的是默认 classic。
func TestScaffoldBuildsForEveryLayout(t *testing.T) {
	for _, layout := range []string{"classic", "editorial", "tech"} {
		t.Run(layout, func(t *testing.T) {
			dir := t.TempDir()
			if err := Scaffold(dir, "Smoke "+layout, layout); err != nil {
				t.Fatalf("生成骨架失败: %v", err)
			}
			res, err := Run(dir)
			if err != nil {
				t.Fatalf("%s 骨架构建失败: %v", layout, err)
			}
			if len(res.Report.Errors) != 0 {
				t.Fatalf("%s 骨架应零错误，实际: %+v", layout, res.Report.Errors)
			}
			if res.Report.Pages == 0 {
				t.Fatalf("%s 骨架未产出页面", layout)
			}
		})
	}
}
