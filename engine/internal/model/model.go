// Package model 定义外贸交付引擎的内容模型。
//
// 设计原则（见《11 数据模型与 API 设计》）：
//   - 字段级翻译：所有需翻译的字段用 I18n（map[lang]string）承载
//   - 参数结构化：规格参数必须是键值对，禁止塞进正文
//   - 模型即契约：struct tag 声明必填 / 翻译要求 / 校验约束
package model

// I18n 是字段级翻译的载体，key 为 BCP 47 语言标签（en / zh / pt-BR）。
type I18n map[string]string

// Resolve 返回指定语言的值；缺失时回退到 defaultLang。
// 第二个返回值表示是否为「精确命中」（false 表示发生了 fallback）。
func (m I18n) Resolve(lang, defaultLang string) (string, bool) {
	if v, ok := m[lang]; ok && v != "" {
		return v, true
	}
	if v, ok := m[defaultLang]; ok && v != "" {
		return v, false
	}
	// 最后兜底：任意非空值，保证页面不出现空白
	for _, v := range m {
		if v != "" {
			return v, false
		}
	}
	return "", false
}

// Empty 判断该字段是否完全没有内容。
func (m I18n) Empty() bool {
	for _, v := range m {
		if v != "" {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- 站点配置

// EngineConfig 对应 engine.yaml，声明引擎版本与输出约定。
type EngineConfig struct {
	EngineVersion int    `yaml:"engine_version"`
	Output        string `yaml:"output"`
}

// Settings 对应 settings.yaml，全站唯一。
type Settings struct {
	BaseURL       string            `yaml:"base_url"`
	SchemaVersion int               `yaml:"schema_version"`
	DefaultLang   string            `yaml:"default_lang"`
	Langs         []string          `yaml:"langs"`
	Theme         string            `yaml:"theme"`
	Brand         Brand             `yaml:"brand"`
	Company       Company           `yaml:"company"`
	Contact       Contact           `yaml:"contact"`
	Markets       []Market          `yaml:"markets"`
	Social        map[string]string `yaml:"social"`
	Analytics     Analytics         `yaml:"analytics"`
	Catalog       Catalog           `yaml:"catalog"`
	Compliance    Compliance        `yaml:"compliance"`
	Inquiry       Inquiry           `yaml:"inquiry"`
	Store         StoreConfig       `yaml:"store"`
	Nav           []NavItem         `yaml:"nav"`
	Images        Images            `yaml:"images"`
}

// Images 是图片管线的站点级配置。
//
// 引擎默认**不转码**：转码依赖 cwebp / avifenc，把构建成败绑在一个外部二进制上
// 是交付事故的常见来源。开启后若工具缺失，会退化为「只用原图」并给出警告，
// 而不是让构建失败。
type Images struct {
	Transcode bool  `yaml:"transcode"`
	Quality   int   `yaml:"quality"`
	Widths    []int `yaml:"widths"`
	MaxKB     int   `yaml:"max_kb"`
}

// Quality 返回 WebP 质量，默认 82（视觉无损与体积的平衡点）。
func (i Images) QualityValue() int {
	if i.Quality <= 0 || i.Quality > 100 {
		return 82
	}
	return i.Quality
}

// WidthSet 返回要生成的派生宽度，默认 640 / 960 / 1280。
func (i Images) WidthSet() []int {
	if len(i.Widths) == 0 {
		return []int{640, 960, 1280}
	}
	return i.Widths
}

// MaxBytes 返回单图体积上限，默认 400KB。
func (i Images) MaxBytes() int64 {
	if i.MaxKB <= 0 {
		return 400 * 1024
	}
	return int64(i.MaxKB) * 1024
}

// StoreConfig 声明运行时唯一写实体（询盘）的持久化位置。
// D7 决定用 SQLite：单文件、零运维、可直接备份到对象存储。
type StoreConfig struct {
	Driver string `yaml:"driver"` // 目前只支持 sqlite
	DSN    string `yaml:"dsn"`    // 默认 .data/inquiries.db
}

type Brand struct {
	Name    string `yaml:"name" i18n:"-"`
	Logo    string `yaml:"logo"`
	Tagline I18n   `yaml:"tagline" i18n:"req"`
}

type Company struct {
	LegalName   string  `yaml:"legal_name"`
	FoundedYear int     `yaml:"founded_year"`
	Address     Address `yaml:"address"`
}

type Address struct {
	Country string `yaml:"country"`
	Region  string `yaml:"region"`
	City    string `yaml:"city"`
	Street  string `yaml:"street"`
	Zip     string `yaml:"zip"`
}

type Contact struct {
	Email    string `yaml:"email"`
	Phone    string `yaml:"phone"`
	WhatsApp string `yaml:"whatsapp"`
	WeChat   string `yaml:"wechat"`
}

type Market struct {
	Code  string   `yaml:"code"`
	Name  I18n     `yaml:"name" i18n:"opt"`
	Langs []string `yaml:"langs"`
}

type Analytics struct {
	GA4ID     string `yaml:"ga4_id"`
	GSCVerify string `yaml:"gsc_verify"`
}

// Catalog 是可下载目录（M5-9）：配置后页脚出现下载入口，
// 链接可带 ?intent=catalog 让询盘表单预填一条留言（留资钩子）。
type Catalog struct {
	File  string `yaml:"file"`  // 站内相对路径，如 media/catalog.pdf
	Label I18n   `yaml:"label" i18n:"opt"` // 不填用 UI 文案的默认标签
}

// ---------------------------------------------------------------- 市场准入

// Compliance 承载目标市场准入的法律信息（B1 v2.0 R2）。
//
// 这些不是品牌文案，是法律实体信息：
//   - EU GPSR（2024/12 起强制）要求在线商品页明示**制造商**与**欧盟责任方**；
//   - 德国 Impressum 要求注册号、注册法院、法人、VAT、可送达联系方式；
//   - 包装法 EPR（德国 LUCID、法国 Triman）要求展示注册号。
//
// 引擎不替客户编造这些值，只提供字段位 + 渲染位 + 缺失告警。
// 字段为空时对应区块整体不输出——宁可没有，不可印一个假的欧盟责任方。
type Compliance struct {
	EUResponsible Party       `yaml:"eu_responsible"`
	LegalNotice   LegalNotice `yaml:"legal_notice"`
	EPR           EPR         `yaml:"epr"`
}

// Party 是一个法定主体（欧盟责任方、进口商等）。
type Party struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
	Email   string `yaml:"email"`
	Phone   string `yaml:"phone"`
}

// LegalNotice 是 Impressum / 法律声明要素。
type LegalNotice struct {
	RegisterNo       string `yaml:"register_no"`
	RegisterCourt    string `yaml:"register_court"`
	VATID            string `yaml:"vat_id"`
	ManagingDirector string `yaml:"managing_director"`
	Email            string `yaml:"email"`
	Phone            string `yaml:"phone"`
}

// EPR 是生产者延伸责任注册号（包装法/电子废弃物）。
type EPR struct {
	DEPackaging string `yaml:"de_packaging"` // 德国 LUCID 包装注册号
	FRPackaging string `yaml:"fr_packaging"` // 法国 Triman/包装
	WEEE        string `yaml:"weee"`         // 电子废弃物
}

// EUCountries 是欧盟成员国 ISO-3166-1 alpha-2 代码。
// settings.markets 既可能写 "EU"（区域），也可能写 "DE"（国家），两种都算命中。
var EUCountries = []string{
	"AT", "BE", "BG", "HR", "CY", "CZ", "DK", "EE", "FI", "FR", "DE", "GR",
	"HU", "IE", "IT", "LV", "LT", "LU", "MT", "NL", "PL", "PT", "RO", "SK",
	"SI", "ES", "SE",
}

// Inquiry 是询盘配置（见《11》5.9）。
// 字段数量刻意保持少：每多一个字段转化率就掉一点。
type Inquiry struct {
	Recipients       []string `yaml:"recipients"`
	Fields           []string `yaml:"fields"`
	Required         []string `yaml:"required"`
	ProductField     bool     `yaml:"product_field"`
	SuccessURL       string   `yaml:"success_url"`
	Webhook          string   `yaml:"webhook"`
	TurnstileSiteKey string   `yaml:"turnstile_sitekey"`

	// APIBase 为空表示与静态站同源（引擎同时托管静态页与 API）。
	// 静态站部署在 Cloudflare Pages 时，这里填 API 子域，例如 https://api.example.com。
	APIBase string `yaml:"api_base"`
	// AllowedOrigins 是跨源提交的白名单。APIBase 与静态站不同源时必须配置。
	AllowedOrigins []string `yaml:"allowed_origins"`
	// RateLimitPerHour 单 IP 每小时上限，默认 10。
	RateLimitPerHour int `yaml:"rate_limit_per_hour"`
	// RateLimitPerDay 单 IP 每 24 小时上限，默认 30。
	// 只有小时闸时，单 IP 一天最多能灌 240 条——日闸才是「硬封」。
	RateLimitPerDay int `yaml:"rate_limit_per_day"`
}

// DefaultInquiryFields 是默认启用的表单字段。
// 「年采购量」按 B1 A4 的要求默认在列（国家/年采购量/目标市场/用途四项里，
// 国家与年采购量由字段承担，目标市场与用途留给留言框）——它让销售第一封
// 回信就能报价，而不是先往复两轮问量。各站可在 settings.yaml 里自行裁剪。
var DefaultInquiryFields = []string{"name", "company", "email", "country", "product", "volume", "message"}

// EffectiveFields 返回实际启用的字段（未配置时用默认值）。
func (i Inquiry) EffectiveFields() []string {
	if len(i.Fields) == 0 {
		return DefaultInquiryFields
	}
	return i.Fields
}

// EffectiveRequired 返回实际必填项。
func (i Inquiry) EffectiveRequired() []string {
	if len(i.Required) == 0 {
		return []string{"name", "email", "message"}
	}
	return i.Required
}

// IsRequired 判断某字段是否必填。
func (i Inquiry) IsRequired(key string) bool {
	for _, k := range i.EffectiveRequired() {
		if k == key {
			return true
		}
	}
	return false
}

// HasField 判断某字段是否启用。
func (i Inquiry) HasField(key string) bool {
	for _, k := range i.EffectiveFields() {
		if k == key {
			return true
		}
	}
	return false
}

// SuccessPath 返回无 JS 提交后的跳转路径。
// 默认带 {lang} 占位符，由服务端按客户提交的语言填入——中文客户不该被
// 送到英文「Thank you」页。该页面由构建流程自动生成。
func (i Inquiry) SuccessPath() string {
	if i.SuccessURL == "" {
		return "/{lang}/thank-you/"
	}
	return i.SuccessURL
}

// RateLimit 返回每小时限流阈值。
func (i Inquiry) RateLimit() int {
	if i.RateLimitPerHour <= 0 {
		return 10
	}
	return i.RateLimitPerHour
}

// RateLimitDay 返回每 24 小时限流阈值。
func (i Inquiry) RateLimitDay() int {
	if i.RateLimitPerDay <= 0 {
		return 30
	}
	return i.RateLimitPerDay
}

type NavItem struct {
	Label I18n   `yaml:"label" i18n:"req"`
	Href  string `yaml:"href"`
}

// ---------------------------------------------------------------- 内容实体

// Base 是所有内容实体的公共字段。
type Base struct {
	Slug      string `yaml:"slug"`
	Status    string `yaml:"status"`
	Order     int    `yaml:"order"`
	UpdatedAt string `yaml:"updated_at"`
	SEO       *SEO   `yaml:"seo"`
}

type SEO struct {
	Title       I18n   `yaml:"title" i18n:"opt"`
	Description I18n   `yaml:"description" i18n:"opt"`
	OGImage     string `yaml:"og_image"`
	NoIndex     bool   `yaml:"noindex"`
}

// Page 是固定区块页面（home / about / factory / certifications / cases / contact / service）。
type Page struct {
	Base    `yaml:",inline"`
	Title   I18n    `yaml:"title" i18n:"req"`
	Summary I18n    `yaml:"summary" i18n:"opt"`
	Cover   string  `yaml:"cover"`
	Blocks  []Block `yaml:"blocks"`
}

// Block 是页面区块。Type 决定用哪个渲染器，其余字段由渲染层二次解码。
//
// 为什么用 map 而不是巨型 struct：区块还在演进，每加一个区块就往 model 里塞
// 十几个只有某类站才用的字段，模型很快就会变成垃圾场。区块的契约在渲染层。
type Block struct {
	Type  string         `yaml:"type"`
	Extra map[string]any `yaml:",inline"`
}

// BlockTypes 是引擎支持的区块白名单。
// 不在表里的 type 会让内容静默消失——这是最难排查的一类 bug，因此宁可直接失败。
var BlockTypes = []string{
	"hero", "richtext", "image_text", "stats", "features",
	"product_grid", "category_grid", "scenario_grid",
	"certificate_grid", "case_grid",
	"process", "team", "faq", "cta", "logos", "gallery", "inquiry",
	"factory", "testimonials", "legal_facts",
}

// IsBlockType 判断区块类型是否受支持。
func IsBlockType(t string) bool {
	for _, b := range BlockTypes {
		if b == t {
			return true
		}
	}
	return false
}

// Product 是核心实体，约 42 个字段，与《11》5.3 节一一对应。
type Product struct {
	Base      `yaml:",inline"`
	Title     I18n     `yaml:"title" i18n:"req"`
	Subtitle  I18n     `yaml:"subtitle" i18n:"opt"`
	Category  string   `yaml:"category"`
	Scenarios []string `yaml:"scenarios"`
	ModelNo   string   `yaml:"model_no"`
	Featured  bool     `yaml:"featured"`

	Summary      I18n   `yaml:"summary" i18n:"req"`
	Description  I18n   `yaml:"description" i18n:"req"`
	Highlights   []I18n `yaml:"highlights" i18n:"opt"`
	Applications []I18n `yaml:"applications" i18n:"opt"`

	Specs []SpecGroup `yaml:"specs"`

	Moq          float64 `yaml:"moq"`
	MoqUnit      string  `yaml:"moq_unit"`
	Price        *Price  `yaml:"price"`
	LeadTime     I18n    `yaml:"lead_time" i18n:"req"`
	Packaging    I18n    `yaml:"packaging" i18n:"req"`
	Carton       *Carton `yaml:"carton"`
	SamplePolicy I18n    `yaml:"sample_policy" i18n:"opt"`
	HSCode       string  `yaml:"hs_code"`

	Certificates []string `yaml:"certificates"`
	TestReports  []string `yaml:"test_reports"`
	Markets      []string `yaml:"markets"`

	Cover     string   `yaml:"cover"`
	Gallery   []string `yaml:"gallery"`
	VideoURL  string   `yaml:"video_url"`
	Datasheet string   `yaml:"datasheet"`

	Related []string `yaml:"related"`
	Cases   []string `yaml:"cases"`
}

type SpecGroup struct {
	Group I18n       `yaml:"group" i18n:"req"`
	Items []SpecItem `yaml:"items"`
}

type SpecItem struct {
	Key        string   `yaml:"key"`
	Label      I18n     `yaml:"label" i18n:"req"`
	Value      I18n     `yaml:"value" i18n:"opt"`
	ValueNum   *float64 `yaml:"value_num"`
	Unit       string   `yaml:"unit"`
	Filterable bool     `yaml:"filterable"`
}

type Price struct {
	Min      float64 `yaml:"min"`
	Max      float64 `yaml:"max"`
	Currency string  `yaml:"currency"`
	Incoterm string  `yaml:"incoterm"`
}

type Carton struct {
	QtyPerCarton int     `yaml:"qty_per_carton"`
	CBM          float64 `yaml:"cbm"`
	GrossWeight  float64 `yaml:"gross_weight_kg"`
}

type Category struct {
	Base   `yaml:",inline"`
	Name   I18n   `yaml:"name" i18n:"req"`
	Desc   I18n   `yaml:"desc" i18n:"opt"`
	Parent string `yaml:"parent"`
	Cover  string `yaml:"cover"`
}

type Scenario struct {
	Base    `yaml:",inline"`
	Name    I18n   `yaml:"name" i18n:"req"`
	Summary I18n   `yaml:"summary" i18n:"opt"`
	Icon    string `yaml:"icon"`
	Cover   string `yaml:"cover"`
}

type Certificate struct {
	Base    `yaml:",inline"`
	Name    I18n     `yaml:"name" i18n:"req"`
	Issuer  string   `yaml:"issuer"`
	Number  string   `yaml:"number"`
	Issued  string   `yaml:"issued"`
	Expires string   `yaml:"expires"`
	Markets []string `yaml:"markets"`
	File    string   `yaml:"file"`
	Logo    string   `yaml:"logo"`
}

type Case struct {
	Base      `yaml:",inline"`
	Title     I18n     `yaml:"title" i18n:"req"`
	Country   string   `yaml:"country"`
	Industry  I18n     `yaml:"industry" i18n:"req"`
	Customer  string   `yaml:"customer"`
	Challenge I18n     `yaml:"challenge" i18n:"req"`
	Solution  I18n     `yaml:"solution" i18n:"req"`
	Result    I18n     `yaml:"result" i18n:"req"`
	Products  []string `yaml:"products"`
	Cover     string   `yaml:"cover"`
}

// Theme 只承载设计 token 与皮肤名，不允许改结构。
//
// Skin 是审美皮肤名（classic / editorial / tech / industrial / organic / minimal
// 或站点 skins/ 目录里的自定义皮肤）：同一套 HTML，换一张 CSS 就换一种设计。
// 它只改观感，不增删组件——所以换肤永远不会把询盘表单换没。
//
// Layout 是 0.10.0 之前的旧键，值与皮肤名同义；保留是为了存量站点不用改配置。
type Theme struct {
	Name   string            `yaml:"name"`
	Skin   string            `yaml:"skin"`
	Layout string            `yaml:"layout"`
	Colors map[string]string `yaml:"colors"`
	Fonts  map[string]string `yaml:"fonts"`
	Radius string            `yaml:"radius"`
	// FontFaces 是自托管字体。设计稿的观感有一半来自字形——Space Grotesk 的
	// 几何窄体、Cormorant 的高对比衬线，用系统字体栈是模拟不出来的。
	// 但直接 <link> Google Fonts 在买家所在地（中东/南美/东南亚）常年不稳，
	// 且渲染阻塞。所以字体文件进站、由构建期输出 @font-face。
	FontFaces []FontFace `yaml:"font_faces"`
}

// FontFace 对应一条 @font-face 声明。
type FontFace struct {
	Family string `yaml:"family"`
	File   string `yaml:"file"`   // 相对站点根，如 media/fonts/inter-400.woff2
	Weight string `yaml:"weight"` // "400" / "500" / "300 700" 可变字重
	Style  string `yaml:"style"`  // 默认 normal
}

// SkinName 返回该站要穿的皮肤名。
//
// 优先级：skin 键 > 旧 layout 键 > classic。
// 这里不做白名单判断——皮肤是否存在由 skins 包解析（站点目录里可能有自定义皮肤），
// 校验层负责把「不存在的皮肤」变成构建错误。
func (t Theme) SkinName() string {
	if t.Skin != "" {
		return t.Skin
	}
	if t.Layout != "" {
		return t.Layout
	}
	return "classic"
}

// LayoutName 是 SkinName 的旧名，保留给存量调用点。
func (t Theme) LayoutName() string { return t.SkinName() }

// Site 是 Loader 的产出，也是 Validator 与 Renderer 的唯一输入。
type Site struct {
	Root         string
	Config       EngineConfig
	Settings     Settings
	Pages        []Page
	Products     []Product
	Categories   []Category
	Scenarios    []Scenario
	Certificates []Certificate
	Cases        []Case
	Theme        Theme
}

// Langs 返回配置的语言列表；未配置时回退到 en。
func (s *Site) Langs() []string {
	if len(s.Settings.Langs) == 0 {
		return []string{"en"}
	}
	return s.Settings.Langs
}

// DefaultLang 返回默认语言。
func (s *Site) DefaultLang() string {
	if s.Settings.DefaultLang == "" {
		return "en"
	}
	return s.Settings.DefaultLang
}

// CategoryBySlug 查找品类。
func (s *Site) CategoryBySlug(slug string) *Category {
	for i := range s.Categories {
		if s.Categories[i].Slug == slug {
			return &s.Categories[i]
		}
	}
	return nil
}

// ProductBySlug 查找产品。
func (s *Site) ProductBySlug(slug string) *Product {
	for i := range s.Products {
		if s.Products[i].Slug == slug {
			return &s.Products[i]
		}
	}
	return nil
}

// PageBySlug 查找页面。
func (s *Site) PageBySlug(slug string) *Page {
	for i := range s.Pages {
		if s.Pages[i].Slug == slug {
			return &s.Pages[i]
		}
	}
	return nil
}

// CaseBySlug 查找案例。
func (s *Site) CaseBySlug(slug string) *Case {
	for i := range s.Cases {
		if s.Cases[i].Slug == slug {
			return &s.Cases[i]
		}
	}
	return nil
}

// CertificateBySlug 查找证书。
func (s *Site) CertificateBySlug(slug string) *Certificate {
	for i := range s.Certificates {
		if s.Certificates[i].Slug == slug {
			return &s.Certificates[i]
		}
	}
	return nil
}

// ScenarioBySlug 查找场景。
func (s *Site) ScenarioBySlug(slug string) *Scenario {
	for i := range s.Scenarios {
		if s.Scenarios[i].Slug == slug {
			return &s.Scenarios[i]
		}
	}
	return nil
}

// PublishedProducts 返回已发布产品（保持加载顺序）。
func (s *Site) PublishedProducts() []*Product {
	out := make([]*Product, 0, len(s.Products))
	for i := range s.Products {
		if s.Products[i].Status == "published" {
			out = append(out, &s.Products[i])
		}
	}
	return out
}

// ProductsInScenario 返回关联了某场景的已发布产品。
func (s *Site) ProductsInScenario(slug string) []*Product {
	out := make([]*Product, 0)
	for i := range s.Products {
		p := &s.Products[i]
		if p.Status != "published" {
			continue
		}
		for _, sc := range p.Scenarios {
			if sc == slug {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// ProductsInCategory 返回某品类下的已发布产品。
func (s *Site) ProductsInCategory(slug string) []*Product {
	out := make([]*Product, 0)
	for i := range s.Products {
		p := &s.Products[i]
		if p.Status == "published" && p.Category == slug {
			out = append(out, p)
		}
	}
	return out
}

// PublishedCases 返回已发布案例。
func (s *Site) PublishedCases() []*Case {
	out := make([]*Case, 0, len(s.Cases))
	for i := range s.Cases {
		if s.Cases[i].Status == "published" {
			out = append(out, &s.Cases[i])
		}
	}
	return out
}

// PublishedCertificates 返回已发布且未过期的证书。
//
// 过期证书照样留在内容里（历史凭证），但前台不展示：给买家看到一张过期证书，
// 比不放证书更伤信任。
func (s *Site) PublishedCertificates(now string) []*Certificate {
	out := make([]*Certificate, 0, len(s.Certificates))
	for i := range s.Certificates {
		c := &s.Certificates[i]
		if c.Status != "published" {
			continue
		}
		if c.Expires != "" && now != "" && c.Expires < now {
			continue
		}
		out = append(out, c)
	}
	return out
}
