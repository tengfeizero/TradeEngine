// Package render 把内存模型渲染成静态 HTML。
//
// 主题只允许改 token 与布局变体，不允许改结构——见《11》第七节。
// 所有页面共用 base 外壳，内容只能通过区块组合进来。
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tradeengine/tradeengine/internal/icons"
	"github.com/tradeengine/tradeengine/internal/media"
	"github.com/tradeengine/tradeengine/internal/model"
	"github.com/tradeengine/tradeengine/internal/skins"
)

//go:embed templates/*.tmpl
var tmplFS embed.FS

//go:embed assets/inquiry.js
var inquiryJS []byte

// InquiryJS 返回表单增强脚本，由构建流程写入 /assets/inquiry.js。
func InquiryJS() []byte { return inquiryJS }

// Alt 是一条 hreflang 备用链接。
type Alt struct {
	Hreflang string
	Href     string
}

// SpecRow 是参数表的一行。
type SpecRow struct {
	Label string
	Value string
}

// SpecGroupView 是参数表的一组。
type SpecGroupView struct {
	Name string
	Rows []SpecRow
}

// ProductView 是产品页的展示模型（已按语言解析完成）。
type ProductView struct {
	Title        string
	Subtitle     string
	Summary      string
	Paragraphs   []string
	Highlights   []string
	Applications []string
	SpecGroups   []SpecGroupView
	ModelNo      string
	Moq          string
	LeadTime     string
	Packaging    string
	Sample       string
	HSCode       string
	Cover        *ImageView
	Gallery      []*ImageView
	CategoryName string
	CategoryHref string
	Carton       string
	Price        string
	CertCards    []Card
	Related      []Card
	Cases        []Card
	// Datasheet 是产品规格书下载（M5-9），内容里 datasheet 指向站内 PDF 才有。
	Datasheet *NavLink
	// SpecsCSV 是参数导出（B1 C10）：构建期为每个产品生成一份 CSV，
	// 买家可以直接把参数表丢进自己的比价表——比截图专业，也比 PDF 好处理。
	SpecsCSV *NavLink
	// Facts 是 GPSR 要求的制造商与欧盟责任方（B1 R2）。未配置时为空。
	Facts []Fact
}

// Data 是模板的上下文。
type Data struct {
	Lang        string
	DefaultLang string
	Brand       string
	Logo        *ImageView
	Tagline     string
	Title       string
	Description string
	Canonical   string
	OGImage     string
	Alternates  []Alt
	Switcher    []Alt
	JSONLD      template.HTML
	Styles      template.CSS
	// Skin 是皮肤名，写进 <html data-skin>；SkinCSS 是这套皮肤的整段样式，
	// 挂在基础层之后，靠「后写覆盖」拿到优先级，皮肤里不需要堆 !important。
	Skin        string
	SkinCSS     template.CSS
	FontPreload []string
	// IconSprite 是本页用到的图标的 <symbol> 定义块，由 finish 在内容渲染完成后
	// 收集生成，插在 <body> 开头。正文里的 <use href="#i-..."> 都指向它。
	IconSprite template.HTML
	Body       template.HTML
	Nav        []NavLink

	// 页面壳
	PageTitle string
	ShowHead  bool
	Intro     string
	Crumb     []NavLink
	Cover     *ImageView
	NoIndex   bool

	// 内容
	Blocks  template.HTML
	Cards   []Card
	Groups  []NavLink
	Case    *CaseView
	Form    *InquiryForm
	Product *ProductView

	BackHref string
	UI       map[string]string

	// ContactBar 是全站常驻联系浮层（WhatsApp/邮箱/电话），无配置则不输出。
	ContactBar []ContactLink
	// Footer 是升级后的页脚：地址 + 直达联系 + 法律页出口。
	Footer *FooterView
	// GA4ID 非空时输出 Cookie 同意条，用户同意后才注入 gtag（D2/E7）。
	GA4ID string
	// GSCVerify 非空时输出 google-site-verification meta（M5-4 收尾）。
	GSCVerify string
	// Catalog 非空时页脚出现目录下载入口（M5-9）。
	Catalog *NavLink
}

// NavLink 是导航项。
type NavLink struct {
	Label string
	Href  string
}

// uiText 是引擎内置的界面文案与单位词典。
// 这类字符串不该由客户填，也不该硬编码在模板里——属于「引擎给死」的部分。
var uiText = map[string]map[string]string{
	"en": {
		"home": "Home", "trade_terms": "Trade terms", "specifications": "Specifications",
		"description": "Description", "products": "Products",
		"all_products": "All products", "all_products_intro": "Browse our full range by category.",
		"all_cases": "Case studies", "all_cases_intro": "How buyers in different markets work with us.", "all_scenarios": "Application scenarios", "all_scenarios_intro": "Where our products perform in real production.",
		"not_found": "Page not found", "not_found_body": "The page you are looking for has moved or never existed. Start from the product list.",
		"read_more": "Read more", "download": "Download", "valid_until": "Valid until",
		"applications": "Applications", "related": "Related products", "industry": "Industry", "country": "Market", "account": "Account", "quote": "Request a quote", "more_in_category": "More in this category",
		"challenge": "Challenge", "solution": "Solution", "result": "Result", "customer": "Customer",
		"unit_m": "m", "unit_days": "days", "unit_months": "months",
		"unit_kg": "kg", "unit_pcs": "pcs", "unit_set": "set", "unit_ton": "t",
		"moq": "MOQ", "lead_time": "Lead time", "packaging": "Packaging",
		"hs_code": "HS Code", "model_no": "Model no.", "sample": "Sample",
		"carton": "Carton", "price": "Price range", "certificates": "Certificates",
		"thanks_title": "Inquiry received", "back_home": "Back to home",
		"thanks_body":   "Thank you. Our sales team has received your inquiry and will reply within 12 hours on business days.",
		"required_hint": "Required", "field_error": "Please check this field.", "skip": "Skip to content",
		"contact": "Contact", "legal": "Legal", "company_reg": "Company", "catalog": "Download catalogue (PDF)",
		"datasheet": "Download datasheet",
		"catalog_intent": "Please send me the full product catalogue and price list.",
		"cookie_text":   "We use cookies for traffic analytics only. No ad tracking, no third-party sharing.",
		"cookie_accept": "Accept", "cookie_deny": "Decline",
		"manufacturer":  "Manufacturer", "eu_responsible": "EU responsible person",
		"commercial_register": "Commercial register", "vat_id": "VAT ID", "managing_director": "Managing director",
		"epr_de": "Packaging EPR (DE)", "epr_fr": "Packaging EPR (FR)", "epr_weee": "WEEE registration",
		"compliance_title": "Manufacturer and market compliance",
		"specs_csv":        "Specifications (CSV)", "sku_list": "SKU list (CSV)", "media_pack": "Product images (ZIP)",
		"factory_capacity": "Capacity", "factory_equipment": "Equipment", "factory_qc": "Quality control",
		"factory_audits": "Third-party audits", "factory_retention": "Sample retention", "factory_photos": "On the floor",
		"audit_number": "Report no.", "audit_valid": "Valid until",
	},
	"zh": {
		"home": "首页", "trade_terms": "贸易条款", "specifications": "规格参数",
		"description": "产品说明", "products": "产品",
		"all_products": "全部产品", "all_products_intro": "按品类浏览我们的产品线。",
		"all_cases": "客户案例", "all_cases_intro": "不同市场的买家如何与我们合作。", "all_scenarios": "应用场景", "all_scenarios_intro": "产品在真实生产环境中的表现。",
		"not_found": "页面不存在", "not_found_body": "您访问的页面已移动或不存在，可以从产品列表重新开始。",
		"read_more": "查看详情", "download": "下载", "valid_until": "有效期至",
		"applications": "应用场景", "related": "相关产品", "industry": "行业", "country": "市场", "account": "客户", "quote": "获取报价", "more_in_category": "同类产品",
		"challenge": "客户问题", "solution": "解决方案", "result": "合作结果", "customer": "客户",
		"unit_m": "米", "unit_days": "天", "unit_months": "个月",
		"unit_kg": "公斤", "unit_pcs": "件", "unit_set": "套", "unit_ton": "吨",
		"moq": "起订量", "lead_time": "交期", "packaging": "包装",
		"hs_code": "HS 编码", "model_no": "型号", "sample": "样品",
		"carton": "外箱", "price": "价格区间", "certificates": "资质认证",
		"thanks_title": "询盘已提交", "back_home": "返回首页",
		"thanks_body":   "已收到您的询盘，销售团队会在工作日 12 小时内回复。",
		"required_hint": "必填", "field_error": "请检查此项。", "skip": "跳到正文",
		"contact": "联系我们", "legal": "法律信息", "company_reg": "公司", "catalog": "下载产品目录（PDF）",
		"datasheet": "下载规格书",
		"catalog_intent": "请把完整产品目录与报价单发给我。",
		"cookie_text":   "本站仅在获得同意后使用 Cookie 做流量统计，无广告追踪、不与第三方共享。",
		"cookie_accept": "同意", "cookie_deny": "拒绝",
		"manufacturer":  "制造商", "eu_responsible": "欧盟责任方",
		"commercial_register": "工商登记", "vat_id": "增值税号", "managing_director": "法定代表人",
		"epr_de": "德国包装法注册号", "epr_fr": "法国包装法注册号", "epr_weee": "电子废弃物注册号",
		"compliance_title": "制造商与市场合规",
		"specs_csv":        "规格参数（CSV）", "sku_list": "SKU 清单（CSV）", "media_pack": "产品图包（ZIP）",
		"factory_capacity": "产能", "factory_equipment": "设备", "factory_qc": "质量控制",
		"factory_audits": "第三方审核", "factory_retention": "留样制度", "factory_photos": "车间实拍",
		"audit_number": "报告编号", "audit_valid": "有效期至",
	},
}

// ui 返回指定语言的界面文案；缺失键回退到英语，再回退到键名本身。
func (r *Renderer) ui(lang string) map[string]string {
	out := map[string]string{}
	for k, v := range uiText["en"] {
		out[k] = v
	}
	if m, ok := uiText[lang]; ok {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func (r *Renderer) unitLabel(ui map[string]string, unit string) string {
	if unit == "" {
		return ""
	}
	if v, ok := ui["unit_"+unit]; ok {
		return v
	}
	return unit
}

// Renderer 持有站点、模板与图片库。
type Renderer struct {
	site  *model.Site
	tmpl  *template.Template
	media *media.Library
	// skin 是本站在用的审美皮肤。它决定 data-skin 与整段皮肤 CSS，
	// 是「同一套 HTML 换一张脸」的唯一开关（见 internal/skins）。
	skin skins.Skin
	// pageIcons 收集「当前正在渲染的这一页」用到的图标名。
	// 构建流程是单线程的（见 build.go），按页清空即可，不需要锁。
	pageIcons []string
	// pageFAQ 收集「当前正在渲染的这一页」faq 区块里的问答，
	// 渲染完成后拼成 FAQPage 结构化数据（M5-5）。与 pageIcons 同生命周期。
	pageFAQ []QA
}

// useIcon 登记本页用到的一个图标；重复登记无副作用。
// 未知名字在这里就丢弃（校验层会报 ICON_UNKNOWN），不产出悬空的 <use>。
func (r *Renderer) useIcon(name string) {
	if name == "" || !icons.Has(name) {
		return
	}
	for _, n := range r.pageIcons {
		if n == name {
			return
		}
	}
	r.pageIcons = append(r.pageIcons, name)
}

// New 构造渲染器。skinOverride 非空时临时换皮（engine build --skin 预览用），
// 不写回 theme.yaml——这样设计可以在同一个站上连续比几套皮而不改配置。
func New(site *model.Site, skinOverride string) (*Renderer, error) {
	name := skinOverride
	if name == "" {
		name = site.Theme.SkinName()
	}
	skin, err := skins.Resolve(site.Root, name)
	if err != nil {
		return nil, err
	}
	t, err := template.ParseFS(tmplFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("解析模板失败: %w", err)
	}
	return &Renderer{site: site, tmpl: t, media: media.NewLibrary(site.Root), skin: skin}, nil
}

// MediaWarnings 返回图片管线发现的问题，由构建流程合并进报告。
func (r *Renderer) MediaWarnings() []string { return r.media.Warnings() }

// absURL 把站内路径转成绝对地址，用于 canonical / hreflang / JSON-LD。
func (r *Renderer) absURL(p string) string {
	base := strings.TrimRight(r.site.Settings.BaseURL, "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return base + p
}

// ogFor 把内容图转成 og:image 需要的绝对地址。
// og:image 必须是绝对地址：Facebook / LinkedIn 不会解析相对路径。
//
// 页面没有 cover 时不留空：回退到品牌分享卡（M5-3）——
// 分享出去至少是一张带品牌色的门面图，而不是空白。
func (r *Renderer) ogFor(rel string) string {
	if rel == "" {
		rel = r.brandCard()
		if rel == "" {
			return ""
		}
	}
	return r.absURL("/" + strings.TrimPrefix(rel, "/"))
}

// brandCard 返回品牌分享卡的站内路径；站点没生成该文件时返回空。
func (r *Renderer) brandCard() string {
	if r.media.Get("media/brand-card.jpg") != nil {
		return "media/brand-card.jpg"
	}
	return ""
}

// homePath 返回首页路径：/{lang}/
func homePath(lang string) string { return "/" + lang + "/" }

// thanksPath 返回「询盘已提交」页路径：/{lang}/thank-you/
// 这一页由引擎自动生成——settings 默认 success_url 指向它，
// 如果它不存在，客户提交完看到 404，是最伤信任的一类 bug。
func thanksPath(lang string) string { return "/" + lang + "/thank-you/" }

// productPath 返回产品路径：/{lang}/products/{category}/{slug}/
func productPath(lang, category, slug string) string {
	return path.Join("/", lang, "products", category, slug) + "/"
}

// alternates 生成 hreflang 互链：其它语言 + x-default + 当前语言。
//
// 当前语言也要输出：Google 明确要求 hreflang 组必须包含自身，
// 少这一条整组都无效——这是最常见的 hreflang 实现错误。
func (r *Renderer) alternates(rel func(lang string) string) []Alt {
	def := r.site.DefaultLang()
	var out []Alt
	for _, l := range r.site.Langs() {
		if l == def {
			continue
		}
		out = append(out, Alt{Hreflang: l, Href: r.absURL(rel(l))})
	}
	out = append(out, Alt{Hreflang: "x-default", Href: r.absURL(rel(def))})
	out = append([]Alt{{Hreflang: def, Href: r.absURL(rel(def))}}, out...)
	return out
}

// switcher 是页头的语言切换器：列出全部语言的同一页面。
func (r *Renderer) switcher(rel func(lang string) string) []Alt {
	var out []Alt
	for _, l := range r.site.Langs() {
		out = append(out, Alt{Hreflang: l, Href: rel(l)})
	}
	return out
}

// Product 渲染产品详情页，返回 HTML 与相对输出路径。
func (r *Renderer) Product(p *model.Product, lang string) ([]byte, string, error) {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }
	ui := r.ui(lang)

	categorySlug := p.Category
	catName, catHref := "", ""
	if c := r.site.CategoryBySlug(p.Category); c != nil {
		catName, _ = c.Name.Resolve(lang, def)
		catHref = path.Join("/", lang, "products", c.Slug) + "/"
	}

	pv := &ProductView{
		Title:        tr(p.Title),
		Subtitle:     tr(p.Subtitle),
		Summary:      tr(p.Summary),
		Paragraphs:   splitParagraphs(tr(p.Description)),
		ModelNo:      p.ModelNo,
		Moq:          formatMoq(p.Moq, r.unitLabel(ui, p.MoqUnit)),
		LeadTime:     tr(p.LeadTime),
		Packaging:    tr(p.Packaging),
		Sample:       tr(p.SamplePolicy),
		HSCode:       p.HSCode,
		Cover:        r.heroImage(p.Cover, tr(p.Title)),
		Gallery:      r.gallery(p.Gallery, tr(p.Title)),
		CategoryName: catName,
		CategoryHref: catHref,
	}
	for _, h := range p.Highlights {
		if s := tr(h); s != "" {
			pv.Highlights = append(pv.Highlights, s)
		}
	}
	for _, a := range p.Applications {
		if s := tr(a); s != "" {
			pv.Applications = append(pv.Applications, s)
		}
	}
	for _, g := range p.Specs {
		gv := SpecGroupView{Name: tr(g.Group)}
		for _, it := range g.Items {
			val := tr(it.Value)
			if it.ValueNum != nil {
				val = formatNumber(*it.ValueNum)
				if u := r.unitLabel(ui, it.Unit); u != "" {
					val += " " + u
				}
			}
			gv.Rows = append(gv.Rows, SpecRow{Label: tr(it.Label), Value: val})
		}
		pv.SpecGroups = append(pv.SpecGroups, gv)
	}
	if p.Carton != nil {
		pv.Carton = fmt.Sprintf("%d pcs · %.3f CBM · %.1f kg",
			p.Carton.QtyPerCarton, p.Carton.CBM, p.Carton.GrossWeight)
	}
	if p.Price != nil && p.Price.Min > 0 {
		if p.Price.Max > p.Price.Min {
			pv.Price = fmt.Sprintf("%s %.2f – %.2f %s", p.Price.Currency, p.Price.Min, p.Price.Max, p.Price.Incoterm)
		} else {
			pv.Price = fmt.Sprintf("%s %.2f %s", p.Price.Currency, p.Price.Min, p.Price.Incoterm)
		}
	}
	// 规格书下载（M5-9）：文件确实存在才出链接，不渲染死链。
	if p.Datasheet != "" && r.media.Exists(p.Datasheet) {
		pv.Datasheet = &NavLink{Label: ui["datasheet"], Href: "/" + strings.TrimPrefix(p.Datasheet, "/")}
	}
	// 参数导出 CSV（C10）：构建期与页面同目录生成，链接永远有效。
	pv.SpecsCSV = &NavLink{
		Label: ui["specs_csv"],
		Href:  productPath(lang, categorySlug, p.Slug) + "specs.csv",
	}
	// 法定披露（R2）：只有配置了合规信息才出现，没有就整块不渲染。
	pv.Facts = r.gpsrFacts(lang)
	// 证书与关联内容：产品页是转化的最后一屏，把「凭什么信你」放在这里最有效
	today := nowDate()
	for _, slug := range p.Certificates {
		c := r.site.CertificateBySlug(slug)
		if c == nil || c.Status != "published" || (c.Expires != "" && c.Expires < today) {
			continue
		}
		pv.CertCards = append(pv.CertCards, Card{
			Title: tr(c.Name), Sub: c.Issuer, Meta: c.Expires, Download: c.File,
			Image: r.cardImage(c.Logo, tr(c.Name)),
		})
	}
	for _, slug := range p.Related {
		if rp := r.site.ProductBySlug(slug); rp != nil && rp.Status == "published" {
			pv.Related = append(pv.Related, r.productCard(rp, lang))
		}
	}
	if len(pv.Related) == 0 && categorySlug != "" {
		for _, rp := range r.site.ProductsInCategory(categorySlug) {
			if rp.Slug == p.Slug {
				continue
			}
			pv.Related = append(pv.Related, r.productCard(rp, lang))
			if len(pv.Related) >= 3 {
				break
			}
		}
	}
	for _, slug := range p.Cases {
		c := r.site.CaseBySlug(slug)
		if c == nil || c.Status != "published" {
			continue
		}
		title, _ := c.Title.Resolve(lang, def)
		industry, _ := c.Industry.Resolve(lang, def)
		pv.Cases = append(pv.Cases, Card{
			Href:  path.Join("/", lang, "cases", c.Slug) + "/",
			Title: title, Sub: industry, Meta: c.Country,
			Image: r.cardImage(c.Cover, title),
		})
	}

	rel := productPath(lang, categorySlug, p.Slug)
	title := pv.Title
	if p.SEO != nil {
		if t, ok := p.SEO.Title.Resolve(lang, def); ok && t != "" {
			title = t
		}
	}
	desc := pv.Summary
	if p.SEO != nil {
		if d, ok := p.SEO.Description.Resolve(lang, def); ok && d != "" {
			desc = d
		}
	}

	d := r.pageData(lang)
	d.Title = title + " | " + r.site.Settings.Brand.Name
	d.Description = desc
	d.Canonical = r.absURL(rel)
	d.Alternates = r.alternates(func(l string) string { return productPath(l, categorySlug, p.Slug) })
	d.Switcher = r.switcher(func(l string) string { return productPath(l, categorySlug, p.Slug) })
	d.PageTitle = pv.Title
	d.OGImage = r.ogFor(p.Cover)
	d.Crumb = []NavLink{
		{Label: ui["home"], Href: "/" + lang + "/"},
		{Label: ui["all_products"], Href: "/" + lang + "/products/"},
	}
	if catName != "" {
		d.Crumb = append(d.Crumb, NavLink{Label: catName, Href: catHref})
	}
	d.Product = pv
	d.Form = r.form(lang, p.Slug)
	d.NoIndex = p.SEO != nil && p.SEO.NoIndex
	d.JSONLD = appendLD(r.productJSONLD(p, pv, rel), r.breadcrumbJSONLD(d.Crumb))

	out, err := r.finish("product", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "products", categorySlug, p.Slug, "index.html"), nil
}

// Thanks 渲染「询盘已提交」页。它必须存在，否则无 JS 提交后的 303 会落到 404。
func (r *Renderer) Thanks(lang string) ([]byte, string, error) {
	ui := r.ui(lang)
	d := r.pageData(lang)
	d.Title = ui["thanks_title"] + " | " + r.site.Settings.Brand.Name
	d.Canonical = r.absURL(thanksPath(lang))
	d.Alternates = r.alternates(thanksPath)
	d.Switcher = r.switcher(thanksPath)
	d.BackHref = homePath(lang)
	// 转化完成页没有搜索价值，加 noindex 避免它和产品页抢排名
	d.NoIndex = true

	out, err := r.finish("thanks", d)
	if err != nil {
		return nil, "", err
	}
	return out, path.Join(lang, "thank-you", "index.html"), nil
}

func (r *Renderer) execute(name string, data *Data) (string, error) {
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("渲染模板 %s 失败: %w", name, err)
	}
	return buf.String(), nil
}

func (r *Renderer) nav(lang string) []NavLink {
	def := r.site.DefaultLang()
	var out []NavLink
	for _, n := range r.site.Settings.Nav {
		label, _ := n.Label.Resolve(lang, def)
		out = append(out, NavLink{Label: label, Href: r.localHref(lang, n.Href)})
	}
	return out
}

// styles 由 theme token 生成 CSS 变量。结构由模板固定，主题改不了结构。
func (r *Renderer) styles() template.CSS {
	c := map[string]string{
		"primary": "#2F6B3F", "ink": "#1A1A1A", "paper": "#F7F4EC",
		"accent": "#B08A3E", "surface": "#FFFFFF", "muted": "rgba(0,0,0,.68)",
	}
	// 暗色皮肤的基线色板。以前它写在 base.tmpl 的
	// html[data-layout=tech]{--paper:#0B0F14;...} 里，位置在 :root 之后，
	// 于是主题里写的 paper / ink / surface / muted 全部被静默覆盖——
	// 客户按设计稿改配色，改完毫无反应。现在默认值只做「主题未提供」时的兜底，
	// 判据是皮肤自己声明的 @tone（见 internal/skins）。
	if r.skin.Dark() {
		for k, v := range map[string]string{
			"paper": "#0B0F14", "surface": "#121820",
			"ink": "#E8EDF2", "muted": "rgba(232,237,242,.6)",
		} {
			c[k] = v
		}
	}
	for k, v := range r.site.Theme.Colors {
		c[k] = v
	}
	fonts := map[string]string{
		"heading": "Georgia, serif",
		"body":    "system-ui, sans-serif",
		"mono":    "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
	}
	for k, v := range r.site.Theme.Fonts {
		fonts[k] = v
	}
	radius := r.site.Theme.Radius
	if radius == "" {
		radius = "2px"
	}
	// --primary 同时承担两种角色：深色背景上的文字色，和按钮的填充色。
	// 暗色主题里这两者常常不能取同一个值——文字要够亮才看得清，填充要够深白字才够对比。
	// 所以拆出 --primary-fill（只用于填充），未设置时回落到 --primary，浅色主题行为不变。
	fill := c["primary-fill"]
	if fill == "" {
		fill = c["primary"]
	}
	// --secondary 只给渐变、次级强调用（按钮渐变、标题渐变词）。
	// 不写就跟随 primary——单色主题不该被迫多配一个色。
	secondary := c["secondary"]
	if secondary == "" {
		secondary = c["primary"]
	}
	// 文本安全派生色：token 直接当正文用对比度常不达标（muted 3.2、primary 3.6~4.0）。
	// 朝 ink 混 45% 后，实测三套色板最差对 4.7:1 ≥ 4.5。深色主题 ink 是亮色，混合同样朝
	// 「对比方向」走，公式对明暗两种主题都成立。文字规则一律用 *-text，填充/边框仍用原 token。
	return template.CSS(fmt.Sprintf(`%s:root{--primary:%s;--primary-fill:%s;--secondary:%s;--ink:%s;--paper:%s;--accent:%s;--surface:%s;--muted:%s;--muted-text:color-mix(in srgb,var(--muted) 55%%,var(--ink));--primary-text:color-mix(in srgb,var(--primary) 55%%,var(--ink));--accent-text:color-mix(in srgb,var(--accent) 55%%,var(--ink));--font-heading:%s;--font-body:%s;--font-mono:%s;--radius:%s}`,
		r.fontFaceCSS(),
		c["primary"], fill, secondary, c["ink"], c["paper"], c["accent"], c["surface"], c["muted"],
		fonts["heading"], fonts["body"], fonts["mono"], radius))
}

// fontFaceCSS 把 theme.font_faces 展开成 @font-face。
//
// 必须是 <style> 里最靠前的一段：字体声明先于使用它的规则，改起来也只需动 YAML。
func (r *Renderer) fontFaceCSS() string {
	if len(r.site.Theme.FontFaces) == 0 {
		return ""
	}
	var b strings.Builder
	for _, f := range r.site.Theme.FontFaces {
		if f.Family == "" || f.File == "" {
			continue
		}
		weight := f.Weight
		if weight == "" {
			weight = "400"
		}
		style := f.Style
		if style == "" {
			style = "normal"
		}
		fmt.Fprintf(&b, "@font-face{font-family:'%s';src:url('/%s') format('woff2');font-weight:%s;font-style:%s;font-display:swap}",
			f.Family, strings.TrimPrefix(f.File, "/"), weight, style)
	}
	return b.String()
}

// fontPreloads 挑出标题字体里最粗的一档做 preload。
//
// 只 preload 一条：首屏 <h1> 用的就是它，晚到会让标题在 swap 时跳一下（CLS）。
// 正文字体不 preload——它体积大、且首屏已有标题占位，交给 font-display:swap 更划算。
func (r *Renderer) fontPreloads() []string {
	heading := r.site.Theme.Fonts["heading"]
	var best *model.FontFace
	bestWeight := -1
	for i, f := range r.site.Theme.FontFaces {
		if f.File == "" || !strings.Contains(heading, f.Family) {
			continue
		}
		w := 400
		if n, err := strconv.Atoi(strings.Fields(f.Weight + " ")[0]); err == nil {
			w = n
		}
		if w > bestWeight {
			bestWeight, best = w, &r.site.Theme.FontFaces[i]
		}
	}
	if best == nil {
		return nil
	}
	return []string{"/" + strings.TrimPrefix(best.File, "/")}
}

func (r *Renderer) orgJSONLD(tagline string) template.HTML {
	s := r.site.Settings
	obj := map[string]any{
		"@context":  "https://schema.org",
		"@type":     "Organization",
		"name":      s.Brand.Name,
		"legalName": s.Company.LegalName,
		"url":       s.BaseURL,
		"address": map[string]any{
			"@type":           "PostalAddress",
			"addressCountry":  s.Company.Address.Country,
			"addressRegion":   s.Company.Address.Region,
			"addressLocality": s.Company.Address.City,
			"streetAddress":   s.Company.Address.Street,
			"postalCode":      s.Company.Address.Zip,
		},
	}
	if tagline != "" {
		obj["description"] = tagline
	}
	if s.Contact.Email != "" {
		obj["email"] = s.Contact.Email
	}
	if s.Contact.Phone != "" {
		obj["telephone"] = s.Contact.Phone
	}
	// M5-5：logo 与 sameAs（社媒主页）——Rich Results 里 Organization 的两项基础字段。
	if lv := r.image(s.Brand.Logo, "", "", "", ""); lv != nil {
		obj["logo"] = r.absURL(lv.Src)
	}
	var sameAs []string
	for _, href := range s.Social {
		if href != "" {
			sameAs = append(sameAs, href)
		}
	}
	if len(sameAs) > 0 {
		obj["sameAs"] = sameAs
	}
	return marshalLD(obj)
}

// breadcrumbJSONLD 由视觉面包屑生成 BreadcrumbList（M5-5）。
// 视觉与结构化必须同源：一个手工维护、一个自动生成迟早分叉。
func (r *Renderer) breadcrumbJSONLD(crumb []NavLink) template.HTML {
	if len(crumb) == 0 {
		return ""
	}
	items := make([]map[string]any, 0, len(crumb))
	for i, n := range crumb {
		items = append(items, map[string]any{
			"@type":    "ListItem",
			"position": i + 1,
			"name":     n.Label,
			"item":     r.absURL(n.Href),
		})
	}
	return marshalLD(map[string]any{
		"@context":        "https://schema.org",
		"@type":           "BreadcrumbList",
		"itemListElement": items,
	})
}

// faqJSONLD 把本页 faq 区块的问答拼成 FAQPage（M5-5）。
func (r *Renderer) faqJSONLD() template.HTML {
	if len(r.pageFAQ) == 0 {
		return ""
	}
	entities := make([]map[string]any, 0, len(r.pageFAQ))
	for _, qa := range r.pageFAQ {
		entities = append(entities, map[string]any{
			"@type":          "Question",
			"name":           qa.Q,
			"acceptedAnswer": map[string]any{"@type": "Answer", "text": strings.Join(qa.A, "\n\n")},
		})
	}
	return marshalLD(map[string]any{
		"@context":   "https://schema.org",
		"@type":      "FAQPage",
		"mainEntity": entities,
	})
}

// appendLD 把多段 JSON-LD 拼成一段安全 HTML。
func appendLD(parts ...template.HTML) template.HTML {
	var b strings.Builder
	for _, p := range parts {
		if p != "" {
			b.WriteString(string(p))
			b.WriteString("\n")
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return template.HTML(b.String()) //nolint:gosec // 各段均出自 marshalLD 的 json.Marshal
}

// productJSONLD 注意：D9 决定全部「询价」，因此刻意省略 offers，不写假价格。
func (r *Renderer) productJSONLD(p *model.Product, pv *ProductView, rel string) template.HTML {
	obj := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Product",
		"name":        pv.Title,
		"description": pv.Summary,
		"sku":         p.ModelNo,
		"url":         r.absURL(rel),
		"brand":       map[string]any{"@type": "Brand", "name": r.site.Settings.Brand.Name},
	}
	if p.Cover != "" {
		obj["image"] = r.absURL("/" + p.Cover)
	}
	if pv.CategoryName != "" {
		obj["category"] = pv.CategoryName
	}
	return marshalLD(obj)
}

func marshalLD(obj map[string]any) template.HTML {
	b, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return template.HTML(`<script type="application/ld+json">` + string(b) + `</script>`) //nolint:gosec
}

func splitParagraphs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n\n")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

var boldRe = regexp.MustCompile(`\*\*(.+?)\*\*`)

// boldHTML 先整体 HTML 转义，再把 **x** 换成 <strong>（M5-7）。
// 转义在前、替换在后：尖括号与引号永远进不了标签，`**` 是唯一的标记语法。
func boldHTML(s string) template.HTML {
	e := html.EscapeString(s)
	e = boldRe.ReplaceAllString(e, "<strong>$1</strong>")
	return template.HTML(e) //nolint:gosec // 已整体转义，仅回插白名单标签
}

// boldParagraphs 按空行分段并逐段做加粗转换。
func boldParagraphs(s string) []template.HTML {
	ps := splitParagraphs(s)
	if len(ps) == 0 {
		return nil
	}
	out := make([]template.HTML, 0, len(ps))
	for _, p := range ps {
		out = append(out, boldHTML(p))
	}
	return out
}

// nowDate 返回今天（YYYY-MM-DD），用于证书有效期判断。
func nowDate() string { return time.Now().Format("2006-01-02") }

func formatMoq(v float64, unit string) string {
	return strings.TrimSpace(formatNumber(v) + " " + unit)
}

func formatNumber(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%g", v)
}
