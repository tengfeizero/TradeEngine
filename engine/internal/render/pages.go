package render

import (
	"fmt"
	"html/template"
	"path"

	"github.com/tengfeizero/TradeEngine/internal/icons"
	"github.com/tengfeizero/TradeEngine/internal/model"
)

// 页面级渲染：固定页、列表页、案例页、404。
//
// 所有页面共用同一个 base 模板与外层结构，差异只在 content 模板与 Data。
// 这是「结构不可被内容改动」的实现方式——内容再怎么写，也钻不出这层壳。

// CaseView 是案例详情。
type CaseView struct {
	Title     string
	Country   string
	Industry  string
	Customer  string
	Challenge []string
	Solution  []string
	Result    string
	Image     *ImageView
	Products  []Card
}

// pageData 是构造 Data 的公共部分，避免每个页面重复 15 行样板。
func (r *Renderer) pageData(lang string) *Data {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }
	d := &Data{
		Lang:        lang,
		DefaultLang: def,
		Brand:       r.site.Settings.Brand.Name,
		Logo:        r.image(r.site.Settings.Brand.Logo, "", "brand-logo", "140px", "eager"),
		Tagline:     tr(r.site.Settings.Brand.Tagline),
		Styles:      r.styles(),
		Skin:        r.skin.Name,
		SkinCSS:     template.CSS(r.skin.CSS), //nolint:gosec // 皮肤文件属于站点资产，与模板同级信任
		Nav:         r.nav(lang),
		UI:          r.ui(lang),
		FontPreload: r.fontPreloads(),
		GA4ID:       r.site.Settings.Analytics.GA4ID,
		GSCVerify:   r.site.Settings.Analytics.GSCVerify,
		// 默认分享图就取品牌卡：没有 cover 的页面（列表/法务/致谢页）
		// 分享出去也不该是一块空白。有 cover 的页面随后覆盖它。
		OGImage: r.ogFor(""),
	}
	// 联系浮层与页脚由 settings 驱动：没配联系方式就没有浮层，没配地址就没有地址行。
	if bar := r.contactBar(); len(bar) > 0 {
		d.ContactBar = bar
	}
	d.Footer = r.footerView(lang)
	return d
}

// finish 执行 content 模板并套上 base。
func (r *Renderer) finish(content string, data *Data) ([]byte, error) {
	body, err := r.execute(content, data)
	if err != nil {
		return nil, err
	}
	data.Body = template.HTML(body) //nolint:gosec // 已由子模板转义
	// 内容模板执行完毕，本页用到的图标已收集齐。sprite 必须在执行 base 前
	// 生成——它会被插在 <body> 开头，供正文的 <use> 引用。
	data.IconSprite = template.HTML(icons.Sprite(r.pageIcons)) //nolint:gosec // 注册表白名单，无用户输入
	r.pageIcons = nil
	r.pageFAQ = nil
	out, err := r.execute("base", data)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// Home 渲染首页。首页就是 slug=home 的固定页，不做特殊处理——
// 特殊处理会让「首页区块」与「内页区块」变成两套语义，迟早不一致。
func (r *Renderer) Home(lang string) ([]byte, string, error) {
	p := r.site.PageBySlug("home")
	if p == nil {
		return nil, "", fmt.Errorf("缺少 home 页面")
	}
	return r.Page(p, lang)
}

// Page 渲染一个固定页：/{lang}/{slug}/
func (r *Renderer) Page(p *model.Page, lang string) ([]byte, string, error) {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }

	blocks, err := r.blocks(lang, p.Blocks)
	if err != nil {
		return nil, "", fmt.Errorf("页面 %s: %w", p.Slug, err)
	}

	d := r.pageData(lang)
	rel := "/" + lang + "/"
	if p.Slug != "home" {
		rel = path.Join("/", lang, p.Slug) + "/"
	}
	title, desc := tr(p.Title), tr(p.Summary)
	if p.SEO != nil {
		if t, ok := p.SEO.Title.Resolve(lang, def); ok && t != "" {
			title = t
		}
		if s, ok := p.SEO.Description.Resolve(lang, def); ok && s != "" {
			desc = s
		}
	}
	d.Title = title + " | " + r.site.Settings.Brand.Name
	d.Description = desc
	d.Canonical = r.absURL(rel)
	d.Alternates = r.alternates(func(l string) string {
		if p.Slug == "home" {
			return homePath(l)
		}
		return path.Join("/", l, p.Slug) + "/"
	})
	d.Switcher = r.switcher(func(l string) string {
		if p.Slug == "home" {
			return homePath(l)
		}
		return path.Join("/", l, p.Slug) + "/"
	})
	d.PageTitle = tr(p.Title)
	d.ShowHead = len(p.Blocks) == 0 || p.Blocks[0].Type != "hero"
	// 页面里只要用了 inquiry 区块，就必须加载表单脚本，
	// 否则区块渲染出了表单却没有 JS 增强（蜜罐/速度检测仍生效，但体验退化）。
	for _, b := range p.Blocks {
		if b.Type == "inquiry" {
			d.Form = r.form(lang, "")
			break
		}
	}
	d.Cover = r.heroImage(p.Cover, tr(p.Title))
	d.OGImage = r.ogFor(p.Cover)
	d.Blocks = blocks
	d.NoIndex = p.SEO != nil && p.SEO.NoIndex
	// JSON-LD：FAQPage（本页 faq 区块，有才出）+ 首页 Organization。
	d.JSONLD = r.faqJSONLD()
	if p.Slug == "home" {
		d.JSONLD = appendLD(d.JSONLD, r.orgJSONLD(d.Tagline))
	}

	out, err := r.finish("page", d)
	if err != nil {
		return nil, "", err
	}
	relFile := path.Join(lang, "index.html")
	if p.Slug != "home" {
		relFile = path.Join(lang, p.Slug, "index.html")
	}
	return out, relFile, nil
}

// ProductIndex 渲染产品总览：/{lang}/products/
func (r *Renderer) ProductIndex(lang string) ([]byte, string, error) {
	ui := r.ui(lang)
	d := r.pageData(lang)
	rel := path.Join("/", lang, "products") + "/"

	var cards []Card
	for _, p := range r.site.PublishedProducts() {
		cards = append(cards, r.productCard(p, lang))
	}
	if len(cards) == 0 {
		return nil, "", fmt.Errorf("没有可发布的产品，产品总览页将为空")
	}
	d.PageTitle = ui["all_products"]
	d.Description = ui["all_products_intro"]
	d.Title = ui["all_products"] + " | " + r.site.Settings.Brand.Name
	d.Canonical = r.absURL(rel)
	d.Alternates = r.alternates(func(l string) string { return path.Join("/", l, "products") + "/" })
	d.Switcher = r.switcher(func(l string) string { return path.Join("/", l, "products") + "/" })
	d.Cards = cards
	d.Groups = r.categoryGroups(lang, nil)
	d.ShowHead = true
	d.Form = r.form(lang, "")

	out, err := r.finish("list", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "products", "index.html"), nil
}

// Category 渲染分类页：/{lang}/products/{slug}/
func (r *Renderer) Category(c *model.Category, lang string) ([]byte, string, error) {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }
	ui := r.ui(lang)

	products := r.site.ProductsInCategory(c.Slug)
	if len(products) == 0 {
		return nil, "", fmt.Errorf("品类 %s 下没有已发布产品", c.Slug)
	}
	d := r.pageData(lang)
	rel := path.Join("/", lang, "products", c.Slug) + "/"
	d.PageTitle = tr(c.Name)
	d.Intro = tr(c.Desc)
	d.Description = tr(c.Desc)
	if d.Description == "" {
		d.Description = tr(c.Name)
	}
	d.Title = tr(c.Name) + " | " + r.site.Settings.Brand.Name
	d.Canonical = r.absURL(rel)
	d.Alternates = r.alternates(func(l string) string { return path.Join("/", l, "products", c.Slug) + "/" })
	d.Switcher = r.switcher(func(l string) string { return path.Join("/", l, "products", c.Slug) + "/" })
	d.Cover = r.heroImage(c.Cover, tr(c.Name))
	d.OGImage = r.ogFor(c.Cover)
	d.ShowHead = true
	d.Form = r.form(lang, "")
	d.Crumb = []NavLink{{Label: ui["home"], Href: "/" + lang + "/"}, {Label: ui["all_products"], Href: "/" + lang + "/products/"}}
	d.JSONLD = r.breadcrumbJSONLD(d.Crumb)
	for _, p := range products {
		d.Cards = append(d.Cards, r.productCard(p, lang))
	}

	out, err := r.finish("list", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "products", c.Slug, "index.html"), nil
}

// CaseIndex 渲染案例总览：/{lang}/cases/
func (r *Renderer) CaseIndex(lang string) ([]byte, string, error) {
	ui := r.ui(lang)
	d := r.pageData(lang)
	def := r.site.DefaultLang()
	rel := path.Join("/", lang, "cases") + "/"

	for _, c := range r.site.PublishedCases() {
		title, _ := c.Title.Resolve(lang, def)
		industry, _ := c.Industry.Resolve(lang, def)
		d.Cards = append(d.Cards, Card{
			Href:  path.Join("/", lang, "cases", c.Slug) + "/",
			Title: title, Sub: industry, Meta: c.Country,
			Image: r.cardImage(c.Cover, title),
		})
	}
	if len(d.Cards) == 0 {
		return nil, "", fmt.Errorf("没有已发布案例，案例总览页将为空")
	}
	d.PageTitle = ui["all_cases"]
	d.Description = ui["all_cases_intro"]
	d.Title = ui["all_cases"] + " | " + r.site.Settings.Brand.Name
	d.Canonical = r.absURL(rel)
	d.ShowHead = true
	d.Form = r.form(lang, "")
	d.Alternates = r.alternates(func(l string) string { return path.Join("/", l, "cases") + "/" })
	d.Switcher = r.switcher(func(l string) string { return path.Join("/", l, "cases") + "/" })

	out, err := r.finish("list", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "cases", "index.html"), nil
}

// CaseDetail 渲染案例详情：/{lang}/cases/{slug}/
func (r *Renderer) CaseDetail(c *model.Case, lang string) ([]byte, string, error) {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }
	ui := r.ui(lang)

	cv := &CaseView{
		Title:     tr(c.Title),
		Country:   c.Country,
		Industry:  tr(c.Industry),
		Customer:  c.Customer,
		Challenge: splitParagraphs(tr(c.Challenge)),
		Solution:  splitParagraphs(tr(c.Solution)),
		Result:    tr(c.Result),
		Image:     r.heroImage(c.Cover, tr(c.Title)),
	}
	for _, slug := range c.Products {
		if p := r.site.ProductBySlug(slug); p != nil && p.Status == "published" {
			cv.Products = append(cv.Products, r.productCard(p, lang))
		}
	}

	rel := path.Join("/", lang, "cases", c.Slug) + "/"
	d := r.pageData(lang)
	d.PageTitle = cv.Title
	d.Description = cv.Result
	if d.Description == "" {
		d.Description = cv.Industry
	}
	d.Title = cv.Title + " | " + r.site.Settings.Brand.Name
	d.Canonical = r.absURL(rel)
	d.Alternates = r.alternates(func(l string) string { return path.Join("/", l, "cases", c.Slug) + "/" })
	d.Switcher = r.switcher(func(l string) string { return path.Join("/", l, "cases", c.Slug) + "/" })
	d.OGImage = r.ogFor(c.Cover)
	d.Crumb = []NavLink{{Label: ui["home"], Href: "/" + lang + "/"}, {Label: ui["all_cases"], Href: "/" + lang + "/cases/"}}
	d.JSONLD = r.breadcrumbJSONLD(d.Crumb)
	d.Case = cv
	d.Form = r.form(lang, "")

	out, err := r.finish("case", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "cases", c.Slug, "index.html"), nil
}

// ScenarioIndex 渲染场景总览：/{lang}/scenarios/
//
// 场景页的价值在 SEO：买家搜「hotel coffee machine supplier」这类场景词时，
// 一个聚合了场景文案 + 相关产品的独立页比产品标签过滤的动态页更容易命中。
func (r *Renderer) ScenarioIndex(lang string) ([]byte, string, error) {
	ui := r.ui(lang)
	def := r.site.DefaultLang()
	d := r.pageData(lang)
	rel := path.Join("/", lang, "scenarios") + "/"

	for i := range r.site.Scenarios {
		sc := &r.site.Scenarios[i]
		if sc.Status != "published" || len(r.site.ProductsInScenario(sc.Slug)) == 0 {
			continue
		}
		name, _ := sc.Name.Resolve(lang, def)
		sub, _ := sc.Summary.Resolve(lang, def)
		card := Card{
			Href:  path.Join("/", lang, "scenarios", sc.Slug) + "/",
			Title: name,
			Sub:   sub,
			Count: len(r.site.ProductsInScenario(sc.Slug)),
		}
		if sc.Icon != "" && icons.Has(sc.Icon) {
			card.Icon = template.HTML(icons.Use(sc.Icon))
			r.useIcon(sc.Icon)
		} else {
			card.Image = r.cardImage(sc.Cover, name)
		}
		d.Cards = append(d.Cards, card)
	}
	if len(d.Cards) == 0 {
		return nil, "", fmt.Errorf("没有可发布的场景（需已发布且关联至少一个产品）")
	}
	d.PageTitle = ui["all_scenarios"]
	d.Description = ui["all_scenarios_intro"]
	d.Title = ui["all_scenarios"] + " | " + r.site.Settings.Brand.Name
	d.Canonical = r.absURL(rel)
	d.ShowHead = true
	d.Form = r.form(lang, "")
	d.Alternates = r.alternates(func(l string) string { return path.Join("/", l, "scenarios") + "/" })
	d.Switcher = r.switcher(func(l string) string { return path.Join("/", l, "scenarios") + "/" })

	out, err := r.finish("list", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "scenarios", "index.html"), nil
}

// ScenarioDetail 渲染场景详情：/{lang}/scenarios/{slug}/
func (r *Renderer) ScenarioDetail(sc *model.Scenario, lang string) ([]byte, string, error) {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }
	ui := r.ui(lang)

	products := r.site.ProductsInScenario(sc.Slug)
	if len(products) == 0 {
		return nil, "", fmt.Errorf("场景 %s 下没有已发布产品", sc.Slug)
	}
	d := r.pageData(lang)
	rel := path.Join("/", lang, "scenarios", sc.Slug) + "/"
	d.PageTitle = tr(sc.Name)
	d.Intro = tr(sc.Summary)
	d.Description = tr(sc.Summary)
	if d.Description == "" {
		d.Description = tr(sc.Name)
	}
	d.Title = tr(sc.Name) + " | " + r.site.Settings.Brand.Name
	d.Canonical = r.absURL(rel)
	d.Alternates = r.alternates(func(l string) string {
		return path.Join("/", l, "scenarios", sc.Slug) + "/"
	})
	d.Switcher = r.switcher(func(l string) string {
		return path.Join("/", l, "scenarios", sc.Slug) + "/"
	})
	d.Cover = r.heroImage(sc.Cover, tr(sc.Name))
	d.OGImage = r.ogFor(sc.Cover)
	d.ShowHead = true
	d.Form = r.form(lang, "")
	d.Crumb = []NavLink{{Label: ui["home"], Href: "/" + lang + "/"}, {Label: ui["all_scenarios"], Href: "/" + lang + "/scenarios/"}}
	d.JSONLD = r.breadcrumbJSONLD(d.Crumb)
	for _, p := range products {
		d.Cards = append(d.Cards, r.productCard(p, lang))
	}

	out, err := r.finish("list", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "scenarios", sc.Slug, "index.html"), nil
}

// NotFound 渲染 404 页。
func (r *Renderer) NotFound(lang string) ([]byte, string, error) {
	ui := r.ui(lang)
	d := r.pageData(lang)
	d.PageTitle = ui["not_found"]
	d.Title = ui["not_found"] + " | " + r.site.Settings.Brand.Name
	d.NoIndex = true
	d.BackHref = homePath(lang)
	d.Cards = nil
	d.Crumb = []NavLink{{Label: ui["home"], Href: "/" + lang + "/"}, {Label: ui["all_products"], Href: "/" + lang + "/products/"}}
	d.JSONLD = r.breadcrumbJSONLD(d.Crumb)

	out, err := r.finish("notfound", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "404.html"), nil
}

// categoryGroups 把产品按品类分组，供总览页做「品类跳转」。
// only 非空时只保留该品类。
func (r *Renderer) categoryGroups(lang string, only []string) []NavLink {
	def := r.site.DefaultLang()
	allow := map[string]bool{}
	for _, s := range only {
		allow[s] = true
	}
	var out []NavLink
	for i := range r.site.Categories {
		c := &r.site.Categories[i]
		if c.Status != "published" || len(r.site.ProductsInCategory(c.Slug)) == 0 {
			continue
		}
		if len(allow) > 0 && !allow[c.Slug] {
			continue
		}
		name, _ := c.Name.Resolve(lang, def)
		out = append(out, NavLink{Label: name, Href: path.Join("/", lang, "products", c.Slug) + "/"})
	}
	return out
}
