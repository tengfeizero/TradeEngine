package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tengfeizero/TradeEngine/internal/loader"
	"github.com/tengfeizero/TradeEngine/internal/model"
)

const exampleSite = "../../examples/zentea"

// loadExample 加载真实示例站（仅基线测试用）。
func loadExample(t *testing.T) *Validator {
	t.Helper()
	site, err := loader.NewFileLoader(filepath.FromSlash(exampleSite)).Load()
	if err != nil {
		t.Fatalf("加载示例站失败: %v", err)
	}
	return New(site)
}

// fixtureFiles 是自包含的最小站点夹具（与 engine new 骨架同构）。
// 结构性测试必须用它，不许碰真实示例站——示例内容会随项目演进，
// 耦合上去的测试迟早像今天这样集体误报。
var fixtureFiles = map[string]string{
	"engine.yaml": "engine_version: 1\noutput: .build/public\n",
	"settings.yaml": `base_url: https://fixture.example.com
schema_version: 1
default_lang: en
langs: [en, zh]
theme: default

brand:
  name: Fixture Co
  tagline:
    en: Test tagline
    zh: 测试标语

company:
  legal_name: Fixture Co., Ltd.
  founded_year: 2020
  address:
    country: CN
    region: Zhejiang
    city: Hangzhou
    zip: "310000"

contact:
  email: sales@fixture.example.com
  phone: "+86 000 0000 0000"

markets:
  - code: EU
    name: { en: Europe, zh: 欧洲 }
    langs: [en]

nav:
  - label: { en: Home, zh: 首页 }
    href: "/"
  - label: { en: Contact, zh: 联系 }
    href: "/contact/"

images:
  transcode: false
  quality: 82
  widths: [640]
  max_kb: 400

inquiry:
  recipients: [sales@fixture.example.com]
  fields: [name, email, message]
  required: [name, email, message]
  success_url: /{lang}/thank-you/
  api_base: ""
  allowed_origins: []
  rate_limit_per_hour: 10

store:
  driver: sqlite
  dsn: .data/inquiries.db
`,
	"theme/theme.yaml": `name: fixture
layout: classic
colors:
  primary: "#2F6B3F"
  ink: "#1A1A1A"
  paper: "#F7F4EC"
  accent: "#B08A3E"
fonts:
  heading: Georgia, serif
  body: system-ui, sans-serif
radius: 2px
`,
	"content/categories.yaml": `- slug: main
  status: published
  order: 1
  name: { en: Main range, zh: 主营产品 }
  desc:
    en: Fixture category.
    zh: 夹具品类。
`,
	"content/pages/home.yaml": `slug: home
status: published
title:
  en: Home
  zh: 首页
summary:
  en: Fixture home.
  zh: 夹具首页。
blocks:
  - type: hero
    headline:
      en: Fixture headline
      zh: 夹具标题
    sub:
      en: Fixture sub.
      zh: 夹具副标题。
    cta_text:
      en: Request a quote
      zh: 获取报价
    cta_href: "#inquiry"
  - type: product_grid
    title:
      en: Products
      zh: 产品
    limit: 6
  - type: inquiry
`,
	"content/pages/contact.yaml": `slug: contact
status: published
title:
  en: Contact
  zh: 联系我们
summary:
  en: Fixture contact.
  zh: 夹具联系。
blocks:
  - type: richtext
    body:
      en: Fixture contact body.
      zh: 夹具联系内容。
  - type: inquiry
`,
	"content/products/sample-product.yaml": `slug: sample-product
status: published
order: 1
featured: true
category: main
model_no: FIX-01
moq: 100
moq_unit: pcs
hs_code: "0000000000"

title:
  en: Sample product
  zh: 示例产品
subtitle:
  en: Fixture subtitle
  zh: 夹具副标题
summary:
  en: Fixture summary.
  zh: 夹具简介。
description:
  en: |
    Fixture description.
  zh: |
    夹具描述。
lead_time:
  en: 15-20 days after deposit
  zh: 定金后 15-20 天
packaging:
  en: 1 pc / carton
  zh: 1 台 / 纸箱

specs:
  - group:
      en: Product data
      zh: 产品参数
    items:
      - key: material
        label:
          en: Material
          zh: 材质
        value:
          en: Steel
          zh: 钢
      - key: size
        label:
          en: Size
          zh: 尺寸
        value_num: 100
        unit: pcs
        filterable: true
`,
}

// loadFixture 在临时目录落一个最小站点并加载。基线必须零错误——
// 任何规则改动若让夹具不再干净，说明规则或夹具有一方错了，就地解决。
func loadFixture(t *testing.T) *Validator {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range fixtureFiles {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	site, err := loader.NewFileLoader(dir).Load()
	if err != nil {
		t.Fatalf("加载夹具失败: %v", err)
	}
	return New(site)
}

func hasCode(rep *Report, code string) bool {
	for _, e := range rep.Errors {
		if e.Code == code {
			return true
		}
	}
	return false
}

// 基线：示例站必须零错误，否则后续用例失去意义。
func TestExampleSiteIsClean(t *testing.T) {
	rep := loadFixture(t).Run()
	if !rep.OK() {
		for _, e := range rep.Errors {
			t.Errorf("意外错误: [%s] %s %s — %s", e.Code, e.Entity, e.Field, e.Message)
		}
	}
}

// 规则 1：slug 唯一性
func TestSlugDuplicate(t *testing.T) {
	v := loadFixture(t)
	dup := v.site.Products[0] // 复制产品，仅改标题与型号，slug 保持相同
	dup.ModelNo = "FIX-02"
	dup.Title = map[string]string{"en": "Another product", "zh": "另一个产品"}
	v.site.Products = append(v.site.Products, dup)
	if rep := v.Run(); !hasCode(rep, "SLUG_DUPLICATE") {
		t.Fatal("重复 slug 应被拦截")
	}
}

// 规则 1：slug 格式
func TestSlugFormat(t *testing.T) {
	v := loadFixture(t)
	v.site.Products[0].Slug = "Sencha_Tea"
	if rep := v.Run(); !hasCode(rep, "SLUG_INVALID") {
		t.Fatal("非 kebab-case 的 slug 应被拦截")
	}
}

// 规则 2：引用完整性
func TestRefNotFound(t *testing.T) {
	v := loadFixture(t)
	v.site.Products[0].Category = "no-such-category"
	if rep := v.Run(); !hasCode(rep, "REF_NOT_FOUND") {
		t.Fatal("指向不存在品类的引用应被拦截")
	}
}

// 规则 3：必译字段缺失必须导致构建失败（不是警告）
func TestMissingRequiredTranslation(t *testing.T) {
	v := loadFixture(t)
	delete(v.site.Products[0].Title, "zh")
	rep := v.Run()
	if !hasCode(rep, "I18N_MISSING") {
		t.Fatal("缺失必译字段应导致错误")
	}
	if rep.OK() {
		t.Fatal("有 I18N_MISSING 时报告不应为 OK")
	}
}

// 规则 4：可选翻译缺失只应产生警告
func TestMissingOptionalTranslationIsWarning(t *testing.T) {
	v := loadFixture(t)
	delete(v.site.Products[0].Subtitle, "zh")
	rep := v.Run()
	if !rep.OK() {
		t.Fatalf("可选翻译缺失不应导致失败，实际错误: %v", rep.Errors)
	}
	found := false
	for _, w := range rep.Warnings {
		if w.Code == "I18N_FALLBACK" {
			found = true
		}
	}
	if !found {
		t.Fatal("可选翻译缺失应产生 I18N_FALLBACK 警告")
	}
}

// 规则 5 / 6：参数结构化红线
func TestFilterableSpecNeedsNumber(t *testing.T) {
	v := loadFixture(t)
	v.site.Products[0].Specs[0].Items[0].Filterable = true // origin 只有 value，没有 value_num
	if rep := v.Run(); !hasCode(rep, "SPEC_NOT_FILTERABLE") {
		t.Fatal("filterable 参数缺 value_num 应被拦截")
	}
}

func TestNumericSpecNeedsUnit(t *testing.T) {
	v := loadFixture(t)
	v.site.Products[0].Specs[0].Items[1].Unit = ""
	if rep := v.Run(); !hasCode(rep, "SPEC_UNIT_MISSING") {
		t.Fatal("数值型参数缺 unit 应被拦截")
	}
}

// 规则 9：HS Code 格式
func TestHSCodeInvalid(t *testing.T) {
	v := loadFixture(t)
	v.site.Products[0].HSCode = "ABC123"
	if rep := v.Run(); !hasCode(rep, "HS_CODE_INVALID") {
		t.Fatal("非法 HS Code 应被拦截")
	}
}

// 规则 14：媒体引用必须存在
func TestMediaMissing(t *testing.T) {
	v := loadFixture(t)
	v.site.Products[0].Cover = "media/products/does-not-exist.png"
	if rep := v.Run(); !hasCode(rep, "MEDIA_NOT_FOUND") {
		t.Fatal("不存在的媒体文件应被拦截")
	}
}

// 规则 22：公司信息（Schema.org 必需）
func TestCompanyMissing(t *testing.T) {
	v := loadFixture(t)
	v.site.Settings.Company.LegalName = ""
	if rep := v.Run(); !hasCode(rep, "COMPANY_INVALID") {
		t.Fatal("缺失工商注册名应被拦截")
	}
}

// 规则 23：base_url 必须 https
func TestBaseURLNotHTTPS(t *testing.T) {
	v := loadFixture(t)
	v.site.Settings.BaseURL = "http://mistypeak.example.com"
	if rep := v.Run(); !hasCode(rep, "BASE_URL_INVALID") {
		t.Fatal("非 https 的 base_url 应被拦截")
	}
}

// 规则 19：首页必须有 hero
func TestHomeWithoutHero(t *testing.T) {
	v := loadFixture(t)
	for i := range v.site.Pages {
		if v.site.Pages[i].Slug == "home" {
			v.site.Pages[i].Blocks = nil
		}
	}
	if rep := v.Run(); !hasCode(rep, "HOME_NO_HERO") {
		t.Fatal("首页缺 hero 应被拦截")
	}
}

// 语言代码必须符合 BCP 47
func TestInvalidLangCode(t *testing.T) {
	v := loadFixture(t)
	v.site.Settings.Langs = []string{"en", "zh_CN"}
	v.site.Settings.DefaultLang = "en"
	if rep := v.Run(); !hasCode(rep, "LANG_INVALID") {
		t.Fatal("zh_CN 这类写法应被拦截")
	}
}

// 主题 token 白名单：拼错的色板键不会进 CSS，改了也没效果。
// 文档里写成 primary_fill（下划线）而引擎只认 primary-fill，就是这类静默失效的现场。
func TestThemeUnknownColorToken(t *testing.T) {
	v := loadFixture(t)
	v.site.Theme.Colors["primary_fill"] = "#8C2F1B" // 下划线：错
	if rep := v.Run(); !hasCode(rep, "THEME_UNKNOWN_TOKEN") {
		t.Fatal("拼错的色板键应被拦截")
	}

	ok := loadFixture(t)
	ok.site.Theme.Colors["primary-fill"] = "#8C2F1B" // 连字符：对
	if rep := ok.Run(); !rep.OK() {
		t.Fatalf("合法色板键不应报错: %+v", rep.Errors)
	}
}

func TestThemeUnknownFontToken(t *testing.T) {
	v := loadFixture(t)
	v.site.Theme.Fonts["font-body"] = "Arial"
	if rep := v.Run(); !hasCode(rep, "THEME_UNKNOWN_TOKEN") {
		t.Fatal("拼错的字体槽应被拦截")
	}
}

func TestSkinUnknown(t *testing.T) {
	v := loadFixture(t)
	v.site.Theme.Skin = "magazine" // 不存在的皮肤会让整站观感静默退回基线
	rep := v.Run()
	if !hasCode(rep, "SKIN_UNKNOWN") {
		t.Fatal("未知皮肤应被拦截")
	}
	// 报错要给出可用清单与拼写建议，否则用户只能靠猜
	msg := ""
	for _, e := range rep.Errors {
		if e.Code == "SKIN_UNKNOWN" {
			msg = e.Message
		}
	}
	if !strings.Contains(msg, "classic") || !strings.Contains(msg, "editorial") {
		t.Errorf("错误信息应列出可用皮肤: %s", msg)
	}
}

// 旧键 layout 仍要能用（存量站点不改配置），但报错时要点名 layout 而不是 skin。
func TestLegacyLayoutKeyStillWorks(t *testing.T) {
	v := loadFixture(t)
	v.site.Theme.Skin = ""
	v.site.Theme.Layout = "editorial"
	if rep := v.Run(); hasCode(rep, "SKIN_UNKNOWN") {
		t.Fatal("旧键 layout 的合法值不应报错")
	}
	v.site.Theme.Layout = "magazin"
	found := false
	for _, e := range v.Run().Errors {
		if e.Code == "SKIN_UNKNOWN" && e.Field == "layout" {
			found = true
		}
	}
	if !found {
		t.Fatal("旧键写错时应报 SKIN_UNKNOWN 且 field=layout")
	}
}

// 图标名白名单：拼错一个字母不会报错，卡片只会静默丢掉图标
// （或退回封面图），肉眼很难发现。与拼错区块 type 同类，构建期必须拦住。
func TestIconUnknownInFeatureBlock(t *testing.T) {
	v := loadFixture(t)
	for i := range v.site.Pages {
		if v.site.Pages[i].Slug != "home" {
			continue
		}
		v.site.Pages[i].Blocks = append(v.site.Pages[i].Blocks, model.Block{
			Type: "features",
			Extra: map[string]any{
				"items": []any{
					map[string]any{"icon": "cpu", "title": "ok"},
					map[string]any{"icon": "microship", "title": "拼错"},
				},
			},
		})
	}
	if rep := v.Run(); !hasCode(rep, "ICON_UNKNOWN") {
		t.Fatal("features 里拼错的图标名应被拦截")
	}

	ok := loadFixture(t)
	for i := range ok.site.Pages {
		if ok.site.Pages[i].Slug != "home" {
			continue
		}
		ok.site.Pages[i].Blocks = append(ok.site.Pages[i].Blocks, model.Block{
			Type:  "features",
			Extra: map[string]any{"items": []any{map[string]any{"icon": "cpu", "title": "ok"}}},
		})
	}
	if rep := ok.Run(); !rep.OK() {
		t.Fatalf("合法图标不应报错: %+v", rep.Errors)
	}
}

// 场景图标：配了图标但名字写错，场景卡会静默退回封面图（没有封面就是空壳）。
func TestIconUnknownInScenario(t *testing.T) {
	v := loadFixture(t)
	if len(v.site.Scenarios) == 0 {
		v.site.Scenarios = append(v.site.Scenarios, model.Scenario{})
	}
	v.site.Scenarios[0].Icon = "cameraa"
	if rep := v.Run(); !hasCode(rep, "ICON_UNKNOWN") {
		t.Fatal("场景里拼错的图标名应被拦截")
	}
}

// M5-7：richtext 正文里落单的 ** 会以字面星号上页面，必须给警告。
// 成对的 ** 是合法加粗语法，不应触发。
func TestBoldPairsWarning(t *testing.T) {
	v := loadFixture(t)
	bold := "body:/n  en: OK **paired** stays silent.\n  zh: 中文 **成对** 不报警。\n"
	odd := "body:/n  en: Broken ** literal.\n  zh: 落单 ** 字面。\n"
	v.site.Pages[0].Blocks = append(v.site.Pages[0].Blocks,
		model.Block{Type: "richtext", Extra: map[string]any{
			"title": map[string]any{"en": "T"},
			"body":  map[string]any{"en": "OK **paired** stays silent.", "zh": "中文 **成对** 不报警。"},
		}},
		model.Block{Type: "richtext", Extra: map[string]any{
			"title": map[string]any{"en": "T2"},
			"body":  map[string]any{"en": "Broken ** literal.", "zh": "落单 ** 字面。"},
		}},
	)
	_ = bold
	_ = odd
	rep := v.Run()
	hasWarn := false
	for _, w := range rep.Warnings {
		if w.Code == "MARKDOWN_BOLD_LITERAL" {
			hasWarn = true
		}
		if w.Code == "MARKDOWN_BOLD_LITERAL" && w.Field == "blocks[0].body" {
			t.Fatalf("成对的 ** 不应触发: %+v", w)
		}
	}
	if !hasWarn {
		t.Fatal("落单的 ** 应触发 MARKDOWN_BOLD_LITERAL 警告")
	}
}

// ---------------------------------------------------------------- R2 / C9 / D6 / D7

// hasWarnCode 与 hasCode 对应，用于警告级规则。
func hasWarnCode(rep *Report, code string) bool {
	for _, w := range rep.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

// R2：markets 含 EU 却没填欧盟责任方 → 警告；补上后消失。
func TestGPSRResponsibleMissing(t *testing.T) {
	v := loadFixture(t)
	if rep := v.Run(); !hasWarnCode(rep, "GPSR_RESPONSIBLE_MISSING") {
		t.Fatal("EU 市场缺欧盟责任方应触发 GPSR_RESPONSIBLE_MISSING")
	}
	v.site.Settings.Compliance.EUResponsible = model.Party{Name: "Fixture EU B.V.", Address: "Rotterdam, NL"}
	if rep := v.Run(); hasWarnCode(rep, "GPSR_RESPONSIBLE_MISSING") {
		t.Fatal("补上责任方后警告应消失")
	}
}

// R2：德国市场缺 LUCID 注册号 → 警告。
func TestEPRMissing(t *testing.T) {
	v := loadFixture(t)
	v.site.Settings.Compliance.EUResponsible = model.Party{Name: "Fixture EU B.V."}
	v.site.Settings.Markets = append(v.site.Settings.Markets, model.Market{Code: "DE"})
	if rep := v.Run(); !hasWarnCode(rep, "EPR_MISSING") {
		t.Fatal("DE 市场缺包装法注册号应触发 EPR_MISSING")
	}
}

// C9：有产品但没有商业模式政策页 → 警告；补页后消失。
func TestBusinessTermsMissing(t *testing.T) {
	v := loadFixture(t)
	if rep := v.Run(); !hasWarnCode(rep, "BUSINESS_TERMS_MISSING") {
		t.Fatal("有产品但无 business-terms 页应触发 BUSINESS_TERMS_MISSING")
	}
	v.site.Pages = append(v.site.Pages, model.Page{
		Base: model.Base{Slug: "business-terms", Status: "published"},
	})
	if rep := v.Run(); hasWarnCode(rep, "BUSINESS_TERMS_MISSING") {
		t.Fatal("补上商业模式政策页后警告应消失")
	}
}

// D6：第三方审核缺编号或日期 → 错误（这类「证据」没有锚点就不是证据）。
func TestFactoryAuditIncomplete(t *testing.T) {
	v := loadFixture(t)
	home := &v.site.Pages[0]
	home.Blocks = append(home.Blocks, model.Block{Type: "factory", Extra: map[string]any{
		"audits": []any{
			map[string]any{"name": model.I18n{"en": "BSCI audit"}, "number": "", "date": ""},
		},
	}})
	if rep := v.Run(); !hasCode(rep, "FACTORY_AUDIT_INCOMPLETE") {
		t.Fatal("审核缺编号/日期应触发 FACTORY_AUDIT_INCOMPLETE")
	}
	home.Blocks[len(home.Blocks)-1] = model.Block{Type: "factory", Extra: map[string]any{
		"audits": []any{
			map[string]any{"name": model.I18n{"en": "BSCI audit"}, "number": "BSCI-26-1", "date": "2026-06-04"},
		},
	}}
	if rep := v.Run(); hasCode(rep, "FACTORY_AUDIT_INCOMPLETE") {
		t.Fatal("补齐编号与日期后不应再报")
	}
}

// D7：证言缺署名或公司 → 错误。
func TestTestimonialIncomplete(t *testing.T) {
	v := loadFixture(t)
	home := &v.site.Pages[0]
	home.Blocks = append(home.Blocks, model.Block{Type: "testimonials", Extra: map[string]any{
		"items": []any{
			map[string]any{"quote": model.I18n{"en": "Great partner."}, "author": "", "company": ""},
		},
	}})
	if rep := v.Run(); !hasCode(rep, "TESTIMONIAL_INCOMPLETE") {
		t.Fatal("证言缺署名/公司应触发 TESTIMONIAL_INCOMPLETE")
	}
}
