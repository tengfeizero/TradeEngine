// Package validate 实现《11 数据模型与 API 设计》第六节的构建期校验规则。
//
// 规则分两级，分界原则：
//   - Error：会导致「上线后客户看到错误信息」的问题 → 构建失败
//   - Warn ：只影响质量评分的问题 → 记入报告但继续构建
//
// 这条分界很重要：规则太严，人就会加 --skip-check，整套校验就废了。
package validate

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/icons"
	"github.com/tengfeizero/TradeEngine/internal/model"
	"github.com/tengfeizero/TradeEngine/internal/skins"
)

// Issue 是一条校验结果。
type Issue struct {
	Code    string `json:"code"`
	Entity  string `json:"entity,omitempty"`
	Field   string `json:"field,omitempty"`
	Lang    string `json:"lang,omitempty"`
	Message string `json:"message"`
}

// Report 是校验与构建的产出，写入 .build/report.json，也是交付凭证。
type Report struct {
	Errors   []Issue `json:"errors"`
	Warnings []Issue `json:"warnings"`
}

func (r *Report) addError(code, entity, field, msg string) {
	r.Errors = append(r.Errors, Issue{Code: code, Entity: entity, Field: field, Message: msg})
}

func (r *Report) addWarn(code, entity, field, msg string) {
	r.Warnings = append(r.Warnings, Issue{Code: code, Entity: entity, Field: field, Message: msg})
}

func (r *Report) addI18n(level, entity, field, lang, msg string) {
	is := Issue{Code: "I18N_MISSING", Entity: entity, Field: field, Lang: lang, Message: msg}
	if level == "req" {
		r.Errors = append(r.Errors, is)
		return
	}
	is.Code = "I18N_FALLBACK"
	r.Warnings = append(r.Warnings, is)
}

// OK 表示没有错误。
func (r *Report) OK() bool { return len(r.Errors) == 0 }

var (
	slugRe     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	hsCodeRe   = regexp.MustCompile(`^\d{6,10}$`)
	currencyRe = regexp.MustCompile(`^[A-Z]{3}$`)
	langRe     = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
	digitRe    = regexp.MustCompile(`\d`)
)

var moqUnits = map[string]bool{"kg": true, "pcs": true, "set": true, "ton": true}

var videoHosts = map[string]bool{
	"youtube.com": true, "www.youtube.com": true, "youtu.be": true,
	"vimeo.com": true, "www.vimeo.com": true, "player.vimeo.com": true,
}

// Validator 校验一个已加载的站点。
type Validator struct {
	site *model.Site
	rep  *Report
}

// New 创建校验器。
func New(site *model.Site) *Validator {
	return &Validator{site: site, rep: &Report{}}
}

// Run 执行全部规则。
// 每次运行重置报告：同一校验器跑两遍，第二遍不该带着第一遍的警告
// （否则「修完再验」的流程会永远显示旧的警告）。
func (v *Validator) Run() *Report {
	v.rep = &Report{}
	v.checkSettings()
	v.checkSlugs()
	v.checkRefs()
	v.checkI18n()
	v.checkProducts()
	v.checkCertificates()
	v.checkPages()
	v.checkCases()
	v.checkBlocks()
	v.checkTheme()
	v.checkNav()
	v.checkDuplicateTitles()

	if len(v.rep.Errors) > 0 {
		return v.rep
	}
	// 规则 20（hreflang 完整性）属于渲染/构建层的结构性校验，
	// 由 build 阶段在写出文件后执行，见 build.verifyHreflang。
	return v.rep
}

// 规则 22 / 23 / 24：站点级必填项
func (v *Validator) checkSettings() {
	s := &v.site.Settings

	if strings.TrimSpace(s.Company.LegalName) == "" {
		v.rep.addError("COMPANY_INVALID", "settings", "company.legal_name", "工商注册名不能为空（Schema.org Organization 必需）")
	}
	if strings.TrimSpace(s.Company.Address.Country) == "" || strings.TrimSpace(s.Company.Address.City) == "" {
		v.rep.addError("COMPANY_INVALID", "settings", "company.address", "公司地址至少需要 country + city（Schema.org Organization 必需）")
	}
	if !strings.HasPrefix(s.BaseURL, "https://") {
		v.rep.addError("BASE_URL_INVALID", "settings", "base_url", "base_url 必须是 https 绝对地址，当前为 "+s.BaseURL)
	}
	if s.SchemaVersion != 1 {
		v.rep.addError("SCHEMA_VERSION", "settings", "schema_version",
			fmt.Sprintf("内容模型版本 %d 与引擎支持的版本 1 不匹配，请运行 engine doctor", s.SchemaVersion))
	}
	if s.DefaultLang == "" {
		v.rep.addError("LANG_INVALID", "settings", "default_lang", "default_lang 不能为空")
	}
	if len(s.Langs) == 0 {
		v.rep.addError("LANG_INVALID", "settings", "langs", "langs 至少需要一个语言")
	}
	for _, l := range s.Langs {
		if !langRe.MatchString(l) {
			v.rep.addError("LANG_INVALID", "settings", "langs", "语言代码必须符合 BCP 47（如 en / zh / pt-BR），不支持 zh_CN 这类写法："+l)
		}
	}
	if s.DefaultLang != "" && len(s.Langs) > 0 && !contains(s.Langs, s.DefaultLang) {
		v.rep.addError("LANG_INVALID", "settings", "default_lang", "default_lang 必须包含在 langs 中")
	}
	if len(s.Inquiry.Recipients) == 0 {
		v.rep.addWarn("INQUIRY_NO_RECIPIENT", "settings", "inquiry.recipients", "未配置询盘收件邮箱，上线后将收不到询盘")
	}
	v.checkCompliance()
}

// checkCompliance 检查目标市场准入信息（B1 v2.0 R2）。
//
// 只在**明显命中**市场时提醒：market 写 EU / UK / DE 或任一欧盟国家代码。
// 判断依据是客户自己填的 markets，引擎不猜他在哪些国家卖。
// 全部是警告不是错误：引擎无法确认法律事实，只负责把缺口顶到报告第一屏。
func (v *Validator) checkCompliance() {
	c := v.site.Settings.Compliance
	eu, de := false, false
	for _, m := range v.site.Settings.Markets {
		code := strings.ToUpper(strings.TrimSpace(m.Code))
		if code == "EU" || code == "EEA" || code == "UK" || code == "GB" {
			eu = true
		}
		if code == "DE" {
			de = true
		}
		for _, c := range model.EUCountries {
			if code == c {
				eu = true
			}
		}
	}
	if !eu {
		return
	}
	if strings.TrimSpace(c.EUResponsible.Name) == "" {
		v.rep.addWarn("GPSR_RESPONSIBLE_MISSING", "settings", "compliance.eu_responsible",
			"目标市场含欧盟/英国，但未配置欧盟责任方：EU GPSR（2024/12 起强制）要求在线商品页明示名称与地址")
	}
	if de && strings.TrimSpace(c.EPR.DEPackaging) == "" {
		v.rep.addWarn("EPR_MISSING", "settings", "compliance.epr.de_packaging",
			"目标市场含德国，但未配置包装法注册号（LUCID）：德国 VerpackG 要求展示")
	}
}

// 规则 1：slug 格式与唯一性
func (v *Validator) checkSlugs() {
	seen := map[string]map[string]bool{}
	add := func(kind, slug, entity string) {
		if seen[kind] == nil {
			seen[kind] = map[string]bool{}
		}
		if slug != "" && seen[kind][slug] {
			v.rep.addError("SLUG_DUPLICATE", entity, "slug", fmt.Sprintf("%s 的 slug 重复：%s", kind, slug))
		}
		seen[kind][slug] = true
		if !slugRe.MatchString(slug) {
			v.rep.addError("SLUG_INVALID", entity, "slug", "slug 必须为 kebab-case（小写字母/数字/连字符）："+slug)
		}
	}
	checkStatus := func(entity, status string) {
		if status != "draft" && status != "published" {
			v.rep.addError("STATUS_INVALID", entity, "status", "status 只能是 draft 或 published，当前为 "+status)
		}
	}

	for i := range v.site.Products {
		p := &v.site.Products[i]
		if p.Slug == "" {
			v.rep.addError("SLUG_INVALID", "product", "slug", "产品缺少 slug（或文件名非法）")
			continue
		}
		add("product", p.Slug, "product/"+p.Slug)
		checkStatus("product/"+p.Slug, p.Status)
	}
	for i := range v.site.Pages {
		p := &v.site.Pages[i]
		add("page", p.Slug, "page/"+p.Slug)
		checkStatus("page/"+p.Slug, p.Status)
	}
	for i := range v.site.Categories {
		c := &v.site.Categories[i]
		add("category", c.Slug, "category/"+c.Slug)
	}
	for i := range v.site.Scenarios {
		s := &v.site.Scenarios[i]
		add("scenario", s.Slug, "scenario/"+s.Slug)
	}
	for i := range v.site.Certificates {
		c := &v.site.Certificates[i]
		add("certificate", c.Slug, "certificate/"+c.Slug)
	}
	for i := range v.site.Cases {
		c := &v.site.Cases[i]
		add("case", c.Slug, "case/"+c.Slug)
	}
}

// 规则 2：引用完整性
func (v *Validator) checkRefs() {
	for i := range v.site.Products {
		p := &v.site.Products[i]
		ent := "product/" + p.Slug
		if p.Category == "" {
			v.rep.addError("REF_MISSING", ent, "category", "产品必须归属一个品类")
		} else if v.site.CategoryBySlug(p.Category) == nil {
			v.rep.addError("REF_NOT_FOUND", ent, "category", "category 指向不存在的品类："+p.Category)
		}
		for _, s := range p.Scenarios {
			if !v.hasScenario(s) {
				v.rep.addError("REF_NOT_FOUND", ent, "scenarios", "scenario 指向不存在的场景："+s)
			}
		}
		for _, c := range p.Certificates {
			if !v.hasCertificate(c) {
				v.rep.addError("REF_NOT_FOUND", ent, "certificates", "certificate 指向不存在的证书："+c)
			}
		}
		for _, c := range p.Cases {
			if !v.hasCase(c) {
				v.rep.addError("REF_NOT_FOUND", ent, "cases", "cases 指向不存在的案例："+c)
			}
		}
		for _, r := range p.Related {
			if r == p.Slug {
				v.rep.addError("REF_SELF", ent, "related", "related 不能引用自身")
			} else if v.site.ProductBySlug(r) == nil {
				v.rep.addError("REF_NOT_FOUND", ent, "related", "related 指向不存在的产品："+r)
			}
		}
	}
	for i := range v.site.Cases {
		c := &v.site.Cases[i]
		for _, ps := range c.Products {
			if v.site.ProductBySlug(ps) == nil {
				v.rep.addError("REF_NOT_FOUND", "case/"+c.Slug, "products", "products 指向不存在的产品："+ps)
			}
		}
	}
}

// 规则 3 / 4：基于 struct tag 的翻译完整性检查
func (v *Validator) checkI18n() {
	langs := v.site.Langs()

	var walkValue func(entity, path string, rv reflect.Value)
	walkValue = func(entity, path string, rv reflect.Value) {
		switch rv.Kind() {
		case reflect.Ptr:
			if rv.IsNil() {
				return
			}
			walkValue(entity, path, rv.Elem())
		case reflect.Slice:
			for i := 0; i < rv.Len(); i++ {
				walkValue(entity, fmt.Sprintf("%s[%d]", path, i), rv.Index(i))
			}
		case reflect.Struct:
			rt := rv.Type()
			if rt.PkgPath() != modelPkgPath {
				return
			}
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				if f.PkgPath != "" {
					continue
				}
				fv := rv.Field(i)
				p := f.Name
				if path != "" {
					p = path + "." + f.Name
				}
				tag := f.Tag.Get("i18n")
				if fv.Type() == i18nType {
					v.checkI18nMap(entity, p, tag, fv, langs)
					continue
				}
				if fv.Kind() == reflect.Slice && fv.Type().Elem() == i18nType {
					for j := 0; j < fv.Len(); j++ {
						v.checkI18nMap(entity, fmt.Sprintf("%s[%d]", p, j), tag, fv.Index(j), langs)
					}
					continue
				}
				walkValue(entity, p, fv)
			}
		}
	}

	walkValue("settings", "", reflect.ValueOf(v.site.Settings))
	for i := range v.site.Products {
		walkValue("product/"+v.site.Products[i].Slug, "", reflect.ValueOf(v.site.Products[i]))
	}
	for i := range v.site.Pages {
		walkValue("page/"+v.site.Pages[i].Slug, "", reflect.ValueOf(v.site.Pages[i]))
	}
	for i := range v.site.Categories {
		walkValue("category/"+v.site.Categories[i].Slug, "", reflect.ValueOf(v.site.Categories[i]))
	}
	for i := range v.site.Scenarios {
		walkValue("scenario/"+v.site.Scenarios[i].Slug, "", reflect.ValueOf(v.site.Scenarios[i]))
	}
	for i := range v.site.Certificates {
		walkValue("certificate/"+v.site.Certificates[i].Slug, "", reflect.ValueOf(v.site.Certificates[i]))
	}
	for i := range v.site.Cases {
		walkValue("case/"+v.site.Cases[i].Slug, "", reflect.ValueOf(v.site.Cases[i]))
	}
}

func (v *Validator) checkI18nMap(entity, field, tag string, fv reflect.Value, langs []string) {
	if tag != "req" && tag != "opt" {
		return
	}
	m, ok := fv.Interface().(model.I18n)
	if !ok {
		return
	}
	// 语言键白名单。
	//
	// 这条规则拦的是一类无声的内容截断：YAML 流式映射 { en: A, 20 MP, zh: B }
	// 在规范里是合法的——逗号后面的裸值被当成「没有值的键」。于是 en 被截到第一个
	// 逗号为止，剩下的句子变成一堆莫名其妙的语言代码，而解析器一句错都不报。
	// 站内看起来只是「文案短了一截」，很难联想到 YAML。
	for k := range m {
		if !contains(langs, k) {
			v.rep.addError("I18N_UNKNOWN_KEY", entity, field,
				fmt.Sprintf("i18n 字段出现非语言代码的键 %q（多因把含逗号的值写成 YAML 流式映射 {...}，逗号把句子切断了）", k))
		}
	}
	// 空 map 是合法状态：数值型参数只有 value_num + unit，本来就没有文案。
	// 只有必译字段为空才是错误，可选字段为空直接跳过（不再产生误导性的 fallback 警告）。
	if m.Empty() {
		if tag == "req" {
			v.rep.addError("I18N_EMPTY", entity, field, "必译字段所有语言均为空")
		}
		return
	}
	for _, l := range langs {
		if strings.TrimSpace(m[l]) == "" {
			if tag == "req" {
				v.rep.addI18n("req", entity, field, l, "必译字段缺失 "+l+" 文案")
			} else {
				v.rep.addI18n("opt", entity, field, l, "可选翻译缺失，将回退到默认语言")
			}
		}
	}
}

// 规则 5 / 6 / 7 / 8 / 9 / 10 / 14 / 16 / 17 / 18
func (v *Validator) checkProducts() {
	marketCodes := map[string]bool{}
	for _, m := range v.site.Settings.Markets {
		marketCodes[m.Code] = true
	}

	for i := range v.site.Products {
		p := &v.site.Products[i]
		ent := "product/" + p.Slug

		// 规则 5 / 6：参数结构化
		if len(p.Specs) == 0 {
			v.rep.addError("SPECS_MISSING", ent, "specs", "产品必须提供结构化参数表（B 端买家第一关注项）")
		}
		for gi, g := range p.Specs {
			for ii, it := range g.Items {
				f := fmt.Sprintf("specs[%d].items[%d]", gi, ii)
				if it.Key == "" {
					v.rep.addError("SPEC_KEY_MISSING", ent, f, "参数项缺少 key")
				}
				if it.Filterable && it.ValueNum == nil {
					v.rep.addError("SPEC_NOT_FILTERABLE", ent, f,
						"filterable: true 的参数字段必须提供 value_num（否则无法筛选与比价）")
				}
				if it.ValueNum != nil && it.Unit == "" {
					v.rep.addError("SPEC_UNIT_MISSING", ent, f, "数值型参数必须提供 unit")
				}
				if it.ValueNum == nil && it.Value.Empty() {
					v.rep.addError("SPEC_VALUE_MISSING", ent, f, "参数项既无 value_num 也无 value")
				}
			}
		}

		// 规则 7：价格
		if p.Price != nil {
			if p.Price.Min > p.Price.Max {
				v.rep.addError("PRICE_INVALID", ent, "price", "price.min 不能大于 price.max")
			}
			if !currencyRe.MatchString(p.Price.Currency) {
				v.rep.addError("PRICE_INVALID", ent, "price.currency", "币种必须是 ISO 4217 三位大写代码，如 USD")
			}
		}

		// 规则 8：MOQ
		if p.Moq <= 0 {
			v.rep.addError("MOQ_INVALID", ent, "moq", "moq 必须大于 0")
		}
		if !moqUnits[p.MoqUnit] {
			v.rep.addError("MOQ_UNIT_INVALID", ent, "moq_unit", "moq_unit 只能是 kg / pcs / set / ton，当前为 "+p.MoqUnit)
		}

		// 规则 9：HS Code（可选字段——未填不校验，填了必须是 6–10 位数字）
		if p.HSCode != "" && !hsCodeRe.MatchString(p.HSCode) {
			v.rep.addError("HS_CODE_INVALID", ent, "hs_code", "hs_code 必须是 6–10 位数字，当前为 "+p.HSCode)
		}

		// 规则 10：市场
		for _, m := range p.Markets {
			if !marketCodes[m] {
				v.rep.addError("MARKET_UNKNOWN", ent, "markets", "markets 中的 "+m+" 未在 settings.markets 声明")
			}
		}

		// 规则 14：媒体文件存在
		media := []string{p.Cover, p.Datasheet}
		media = append(media, p.Gallery...)
		media = append(media, p.TestReports...)
		for _, m := range media {
			if m == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(v.site.Root, filepath.FromSlash(m))); err != nil {
				v.rep.addError("MEDIA_NOT_FOUND", ent, "media", "引用的媒体文件不存在："+m)
			}
		}

		// 规则 16：视频域名
		if p.VideoURL != "" && !validVideoURL(p.VideoURL) {
			v.rep.addError("VIDEO_HOST_INVALID", ent, "video_url", "video_url 只允许 YouTube / Vimeo")
		}

		// 规则：产品必须有封面图。没有封面的产品卡片是一个空白方块，
		// 在列表页看起来像「还没做完的站」。
		if p.Status == "published" && p.Cover == "" {
			v.rep.addWarn("PRODUCT_NO_COVER", ent, "cover", "产品缺少封面图，列表卡片将没有视觉锚点")
		}

		// 规则：单图体积。外贸站最常见的性能问题就是一整页 5MB 的原图直出。
		if p.Cover != "" {
			if st, err := os.Stat(filepath.Join(v.site.Root, filepath.FromSlash(p.Cover))); err == nil {
				limit := v.site.Settings.Images.MaxBytes()
				if st.Size() > limit {
					v.rep.addWarn("MEDIA_TOO_LARGE", ent, "cover",
						fmt.Sprintf("封面图 %.1fMB 超过建议上限 %.0fKB，请压缩或生成派生尺寸",
							float64(st.Size())/1024/1024, float64(limit)/1024))
				}
			}
		}

		// 规则 17：产品应关联场景
		if len(p.Scenarios) == 0 {
			v.rep.addWarn("SCENARIO_EMPTY", ent, "scenarios", "产品未关联任何应用场景，会降低站内交叉导流")
		}

		// 规则 18：SEO 长度
		v.checkSEOLength(ent, p.SEO, p.Title, p.Summary)
	}
}

// 规则 11 / 12 / 13 / 14
func (v *Validator) checkCertificates() {
	now := time.Now()
	for i := range v.site.Certificates {
		c := &v.site.Certificates[i]
		ent := "certificate/" + c.Slug
		if strings.TrimSpace(c.Number) == "" {
			v.rep.addError("CERT_NUMBER_MISSING", ent, "number", "证书编号不能为空（B 端买家必看）")
		}
		if c.File == "" {
			v.rep.addError("MEDIA_NOT_FOUND", ent, "file", "证书必须提供可下载文件")
		} else if _, err := os.Stat(filepath.Join(v.site.Root, filepath.FromSlash(c.File))); err != nil {
			v.rep.addError("MEDIA_NOT_FOUND", ent, "file", "证书文件不存在："+c.File)
		}
		if c.Expires != "" {
			d, err := time.Parse("2006-01-02", c.Expires)
			if err != nil {
				v.rep.addError("CERT_DATE_INVALID", ent, "expires", "日期格式必须为 YYYY-MM-DD")
			} else if d.Before(now) {
				v.rep.addWarn("CERT_EXPIRED", ent, "expires", "证书已过期，前台将不展示（有效期 "+c.Expires+"）")
			} else if d.Sub(now) < 90*24*time.Hour {
				v.rep.addWarn("CERT_EXPIRING", ent, "expires", "证书将在 90 天内到期，请提前续证")
			}
		}
	}
}

// 规则 19：首页必须有 hero 区块
func (v *Validator) checkPages() {
	var home *model.Page
	for i := range v.site.Pages {
		if v.site.Pages[i].Slug == "home" {
			home = &v.site.Pages[i]
			break
		}
	}
	if home == nil {
		v.rep.addError("HOME_MISSING", "page/home", "slug", "缺少 home 页面")
		return
	}
	hasHero := false
	for _, b := range home.Blocks {
		if b.Type == "hero" {
			hasHero = true
			break
		}
	}
	if !hasHero {
		v.rep.addError("HOME_NO_HERO", "page/home", "blocks", "首页必须包含 hero 区块")
	}
	// 隐私政策（B1 G5/D2）：面向 EU/UK 的站点没有独立隐私页是门禁级缺陷，
	// 但引擎无法确认目标市场是否含 EU——所以做成警告而非错误，交付门禁（B6）里拦截。
	hasPrivacy := false
	for i := range v.site.Pages {
		p := &v.site.Pages[i]
		if p.Slug == "privacy-policy" && p.Status == "published" {
			hasPrivacy = true
			break
		}
	}
	if !hasPrivacy {
		v.rep.addWarn("PRIVACY_MISSING", "site", "pages",
			"没有已发布的 privacy-policy 页面（B1 G5/D2：面向 EU/UK 必须有独立隐私政策；engine new 脚手架自带，可参照 B4 修改）")
	}
	// 商业模式政策（B1 v2.0 C9）：OEM/白标/独家/IP/报价有效期是 B 端第一轮谈判
	// 必问的四件事。站上没有一页写清，买家只能发邮件问——这一问就可能问丢。
	if len(v.site.PublishedProducts()) > 0 {
		found := false
		for _, slug := range []string{"business-terms", "oem-policy", "private-label", "oem-odm"} {
			if v.hasPage(slug) {
				found = true
				break
			}
		}
		if !found {
			v.rep.addWarn("BUSINESS_TERMS_MISSING", "site", "pages",
				"没有商业模式政策页（B1 C9：OEM/白标最小量、独家代理、IP 归属、报价有效期；engine new 自带 business-terms 起草版）")
		}
	}
	// 德国 Impressum（R2）：目标市场含 DE 却没有法律声明页，等于把合规风险
	// 留给客户自己。同样是警告——市场判断来自客户填的 markets。
	for _, m := range v.site.Settings.Markets {
		if strings.EqualFold(strings.TrimSpace(m.Code), "DE") && !v.hasPage("legal-notice") {
			v.rep.addWarn("LEGAL_NOTICE_MISSING", "site", "pages",
				"目标市场含德国，但没有 legal-notice 页（Impressum 要求注册号、注册法院、法人、VAT 与可送达联系方式）")
			break
		}
	}
	for i := range v.site.Pages {
		p := &v.site.Pages[i]
		if p.Cover != "" {
			if _, err := os.Stat(filepath.Join(v.site.Root, filepath.FromSlash(p.Cover))); err != nil {
				v.rep.addError("MEDIA_NOT_FOUND", "page/"+p.Slug, "cover", "引用的媒体文件不存在："+p.Cover)
			}
		}
	}
}

// hasPage 判断某个 slug 是否有已发布页面。
func (v *Validator) hasPage(slug string) bool {
	for i := range v.site.Pages {
		if v.site.Pages[i].Slug == slug && v.site.Pages[i].Status == "published" {
			return true
		}
	}
	return false
}

// 规则：区块类型白名单。
//
// 一个拼错的 type（hero → Hero）不会报错，只会让整块内容消失，
// 而且页面看起来「正常」——这是最难排查的一类问题，必须构建期拦住。
func (v *Validator) checkBlocks() {
	for i := range v.site.Pages {
		p := &v.site.Pages[i]
		for j, b := range p.Blocks {
			if b.Type == "" {
				v.rep.addError("BLOCK_TYPE_MISSING", "page/"+p.Slug, fmt.Sprintf("blocks[%d]", j),
					"区块缺少 type")
				continue
			}
			if !model.IsBlockType(b.Type) {
				v.rep.addError("BLOCK_UNKNOWN", "page/"+p.Slug, fmt.Sprintf("blocks[%d]", j),
					"未知区块类型 "+b.Type+"，支持："+strings.Join(model.BlockTypes, " / "))
				continue
			}
			// 图标名同样是白名单：拼错一个字母，卡片不会报错，只是静默变成
			// 无图无图标的空壳（或退回封面图）——和拼错区块 type 同类，必须拦住。
			v.checkIconFields("page/"+p.Slug, fmt.Sprintf("blocks[%d]", j), b.Extra)
			v.checkEvidenceBlock(p, j)
			// 富文本加粗（M5-7）：**x** 会在渲染层转成 <strong>。落单的 **
			// 会以字面星号出现在页面上——作者本意是加粗，读者看到的是排版错误。
			if b.Type == "richtext" {
				v.checkBoldPairs("page/"+p.Slug, fmt.Sprintf("blocks[%d]", j), b.Extra)
			}
		}
	}
	for i := range v.site.Scenarios {
		sc := &v.site.Scenarios[i]
		if sc.Icon == "" {
			continue
		}
		if !icons.Has(sc.Icon) {
			v.rep.addError("ICON_UNKNOWN", "scenario/"+sc.Slug, "icon",
				"未知图标 "+sc.Icon+"，可用："+strings.Join(icons.Names(), " / "))
		}
	}
}

// checkEvidenceBlock 校验「证据类」区块：factory（D6）与 testimonials（D7）。
//
// 这两类区块的共同点是**内容可以写得很像真的**：
// 「已通过 BSCI 审核」「客户很满意」谁都会写。校验能拦的只有一件事——
// 可核验的锚点有没有填：审核报告的编号与日期、证言的署名与公司。
// 没有锚点的证据不是证据，是形容词。
func (v *Validator) checkEvidenceBlock(p *model.Page, idx int) {
	b := p.Blocks[idx]
	where := fmt.Sprintf("blocks[%d]", idx)
	switch b.Type {
	case "factory":
		var in struct {
			Audits []struct {
				Name   any    `yaml:"name"`
				Number string `yaml:"number"`
				Date   string `yaml:"date"`
			} `yaml:"audits"`
		}
		if err := decodeExtra(b.Extra, &in); err != nil {
			return // 结构错误由其它规则报，这里不重复
		}
		for k, a := range in.Audits {
			if !anyText(a.Name) {
				continue
			}
			if strings.TrimSpace(a.Number) == "" || strings.TrimSpace(a.Date) == "" {
				v.rep.addError("FACTORY_AUDIT_INCOMPLETE", "page/"+p.Slug,
					fmt.Sprintf("%s.audits[%d]", where, k),
					"第三方审核缺编号或日期：买家查不到、核不了，等于没写")
			}
		}
	case "testimonials":
		var in struct {
			Items []struct {
				Quote   any    `yaml:"quote"`
				Author  string `yaml:"author"`
				Company string `yaml:"company"`
				Country string `yaml:"country"`
			} `yaml:"items"`
		}
		if err := decodeExtra(b.Extra, &in); err != nil {
			return
		}
		for k, it := range in.Items {
			if !anyText(it.Quote) {
				continue
			}
			if strings.TrimSpace(it.Author) == "" || strings.TrimSpace(it.Company) == "" {
				v.rep.addError("TESTIMONIAL_INCOMPLETE", "page/"+p.Slug,
					fmt.Sprintf("%s.items[%d]", where, k),
					"客户证言缺署名或公司：不署名的好评在 B 端等于自说自话")
			}
		}
	}
	// legal_facts 不需要规则：它至少会渲染制造商（company.legal_name + 地址），
	// 而 company 为空已由 COMPANY_INVALID 拦下。再多一条只会制造噪音。
}

// anyText 判断一个区块字段是否带有非空文本：兼容 i18n map（经 YAML 往返后
// 是 map[string]any）与纯字符串两种写法。
func anyText(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case map[string]any:
		for _, x := range t {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case model.I18n:
		return !t.Empty()
	}
	return false
}

// decodeExtra 把区块的 YAML 原始 map 解到目标结构（与渲染层同一套解码路径）。
func decodeExtra(extra map[string]any, dst any) error {
	raw, err := yaml.Marshal(extra)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(raw, dst)
}

// checkIconFields 递归扫描区块原始字段里的 icon 键。
//
// 区块是 map[string]any（见 model.Block），新增区块只要用 icon: xxx 就自动被
// 这条规则覆盖，不需要每加一个区块就往校验层补一段。
func (v *Validator) checkIconFields(entity, field string, node any) {	switch t := node.(type) {
	case map[string]any:
		for k, val := range t {
			if k == "icon" {
				if name, ok := val.(string); ok && name != "" && !icons.Has(name) {
					v.rep.addError("ICON_UNKNOWN", entity, field+"."+k,
						"未知图标 "+name+"，可用："+strings.Join(icons.Names(), " / "))
				}
				continue
			}
			v.checkIconFields(entity, field+"."+k, val)
		}
	case []any:
		for idx, val := range t {
			v.checkIconFields(entity, fmt.Sprintf("%s[%d]", field, idx), val)
		}
	}
}

// checkBoldPairs 检查 richtext 正文里的 ** 是否成对（M5-7）。
// 落单的 ** 不会被渲染成加粗，只会以字面星号出现在页面上。
func (v *Validator) checkBoldPairs(entity, field string, node any) {
	m, ok := node.(map[string]any)
	if !ok {
		return
	}
	body, ok := m["body"]
	if !ok {
		return
	}
	v.scanBold(entity, field+".body", body)
}

func (v *Validator) scanBold(entity, field string, node any) {
	switch t := node.(type) {
	case map[string]any:
		for k, val := range t {
			v.scanBold(entity, field+"."+k, val)
		}
	case string:
		if strings.Count(t, "**")%2 == 1 {
			v.rep.addWarn("MARKDOWN_BOLD_LITERAL", entity, field,
				"加粗标记 ** 落单，页面上会显示为字面星号，请补齐成对")
		}
	}
}

var (
	// themeColorKeys 是渲染层真正会写进 CSS 变量的色板键。
	// 注意是连字符 primary-fill，不是下划线——两个词拼错一个，颜色就静默失效。
	themeColorKeys = []string{"primary", "primary-fill", "secondary", "ink", "paper", "surface", "muted", "accent"}
	themeFontKeys  = []string{"heading", "body", "mono", "heading-file", "body-file", "mono-file"}
)

// 规则：主题 token 拼写。
//
// theme.colors / theme.fonts 在模型里是自由 map，渲染层只认上面那几个键：
// 拼错的键既不报错、也不会进 CSS，结果是「改了没效果」——整站悄悄沿用默认色，
// 只有客户说「怎么还是绿的」时才会被发现。与拼错区块 type 是同一类问题，
// 因此同样按白名单在构建期失败。
func (v *Validator) checkTheme() {
	th := v.site.Theme
	// 皮肤名不存在 = 整站观感回到中性基线，而且没有任何报错。
	// 皮肤是「观感」的唯一开关，写错就等于换肤没发生——必须构建期拦下。
	// 可用名字来自内置预设 + 站点 skins/ 目录，所以这里走 skins.Resolve 而不是写死列表。
	if _, err := skins.Resolve(v.site.Root, th.SkinName()); err != nil {
		field := "skin"
		if th.Skin == "" && th.Layout != "" {
			field = "layout" // 存量站点用的旧键，提示里点名它，免得去改 skin
		}
		v.rep.addError("SKIN_UNKNOWN", "theme", field, err.Error())
	}
	for k := range th.Colors {
		if !contains(themeColorKeys, k) {
			v.rep.addError("THEME_UNKNOWN_TOKEN", "theme", "colors."+k,
				"未知主题色 "+k+"，可用："+strings.Join(themeColorKeys, " / ")+"（多余键不会进 CSS，写了也不会生效）")
		}
	}
	for k := range th.Fonts {
		if !contains(themeFontKeys, k) {
			v.rep.addError("THEME_UNKNOWN_TOKEN", "theme", "fonts."+k,
				"未知字体槽 "+k+"，可用："+strings.Join(themeFontKeys, " / "))
		}
	}
	// 色板写了但没写 primary：能构建，只是整站主色停在出厂绿。
	if len(th.Colors) > 0 && th.Colors["primary"] == "" {
		v.rep.addWarn("THEME_NO_PRIMARY", "theme", "colors.primary",
			"未定义 primary，将使用内置默认色 #2F6B3F")
	}
	// 自托管字体：文件缺失 = 整站悄悄回落到系统字体，字形与设计稿完全不同，
	// 而浏览器不会报任何错。这类「看起来只是没生效」必须构建期拦下。
	for i, f := range th.FontFaces {
		field := fmt.Sprintf("font_faces[%d]", i)
		if strings.TrimSpace(f.Family) == "" {
			v.rep.addError("FONT_FACE_INVALID", "theme", field, "font_faces 缺少 family")
			continue
		}
		if strings.TrimSpace(f.File) == "" {
			v.rep.addError("FONT_FACE_INVALID", "theme", field,
				"font_faces 缺少 file（字体文件名，如 media/fonts/inter-400.woff2）")
			continue
		}
		if ext := strings.ToLower(filepath.Ext(f.File)); ext != ".woff2" && ext != ".woff" {
			v.rep.addError("FONT_FACE_INVALID", "theme", field,
				"字体只支持 .woff2 / .woff，当前为 "+ext)
		}
		if _, err := os.Stat(filepath.Join(v.site.Root, filepath.FromSlash(f.File))); err != nil {
			v.rep.addError("FONT_FILE_MISSING", "theme", field,
				"字体文件不存在："+f.File+"（先跑 tools/fetch_fonts.py 抓取，或用系统字体）")
		}
	}
}

// 规则：导航可达性。
//
// 导航栏断链是外贸站最常见的低级错误：站内所有页面都带着这个坏链接，
// 客户点到 404 就直接关掉，比少一个页面严重得多。
func (v *Validator) checkNav() {
	known := map[string]bool{"/": v.site.PageBySlug("home") != nil}
	for i := range v.site.Pages {
		p := &v.site.Pages[i]
		if p.Status == "published" {
			known["/"+p.Slug+"/"] = true
		}
	}
	known["/products/"] = len(v.site.PublishedProducts()) > 0
	known["/cases/"] = len(v.site.PublishedCases()) > 0
	// 场景页只有「已发布且有关联产品」的才出，与构建 emit 条件保持一致
	for i := range v.site.Scenarios {
		sc := &v.site.Scenarios[i]
		if sc.Status == "published" && len(v.site.ProductsInScenario(sc.Slug)) > 0 {
			known["/scenarios/"] = true
			known["/scenarios/"+sc.Slug+"/"] = true
		}
	}

	for _, n := range v.site.Settings.Nav {
		href := strings.TrimSpace(n.Href)
		if href == "" {
			v.rep.addError("NAV_HREF_MISSING", "settings", "nav.href", "导航项缺少 href")
			continue
		}
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") ||
			strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
			continue
		}
		if !strings.HasPrefix(href, "/") {
			href = "/" + href
		}
		// 锚点链接（/#inquiry、/contact/#inquiry）取路径部分
		if i := strings.IndexAny(href, "#?"); i >= 0 {
			href = href[:i]
		}
		if href == "" || href == "/" {
			href = "/"
		} else if !strings.HasSuffix(href, "/") {
			href += "/"
		}
		if !known[href] {
			v.rep.addError("NAV_BROKEN", "settings", "nav.href",
				"导航指向不存在的页面："+n.Href)
		}
	}
}

// 规则：同语言标题重复 + 产品缺封面。
//
// 标题重复会直接导致关键词自我竞争（keyword cannibalisation）：
// 两个页面抢同一个词，Google 通常两个都不给好位置。
func (v *Validator) checkDuplicateTitles() {
	for _, lang := range v.site.Langs() {
		seen := map[string]string{}
		for i := range v.site.Products {
			p := &v.site.Products[i]
			if p.Status != "published" {
				continue
			}
			t, _ := p.Title.Resolve(lang, v.site.DefaultLang())
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if first, ok := seen[t]; ok {
				v.rep.addWarn("TITLE_DUPLICATE", "product/"+p.Slug, "title",
					"与 "+first+" 的 "+lang+" 标题完全相同，会造成关键词自我竞争："+t)
			} else {
				seen[t] = p.Slug
			}
		}
	}
}

// 规则 21：案例结果应有数字
func (v *Validator) checkCases() {
	for i := range v.site.Cases {
		c := &v.site.Cases[i]
		ent := "case/" + c.Slug
		for lang, txt := range c.Result {
			if !digitRe.MatchString(txt) {
				v.rep.addWarn("CASE_NO_METRIC", ent, "result", "案例结果缺少数字（"+lang+"），B 端客户更信具体数据")
			}
		}
		if c.Cover != "" {
			if _, err := os.Stat(filepath.Join(v.site.Root, filepath.FromSlash(c.Cover))); err != nil {
				v.rep.addError("MEDIA_NOT_FOUND", ent, "cover", "引用的媒体文件不存在："+c.Cover)
			}
		}
	}
}

func (v *Validator) checkSEOLength(ent string, seo *model.SEO, title, summary model.I18n) {
	for _, l := range v.site.Langs() {
		t := ""
		if seo != nil {
			t, _ = seo.Title.Resolve(l, v.site.DefaultLang())
		}
		if t == "" {
			t, _ = title.Resolve(l, v.site.DefaultLang())
		}
		if n := len([]rune(t)); n > 60 {
			v.rep.addWarn("SEO_TITLE_LONG", ent, "seo.title", fmt.Sprintf("%s 标题 %d 字符，建议 ≤ 60", l, n))
		}
		d := ""
		if seo != nil {
			d, _ = seo.Description.Resolve(l, v.site.DefaultLang())
		}
		if d == "" {
			d, _ = summary.Resolve(l, v.site.DefaultLang())
		}
		if n := len([]rune(d)); n > 160 {
			v.rep.addWarn("SEO_DESC_LONG", ent, "seo.description", fmt.Sprintf("%s 描述 %d 字符，建议 ≤ 160", l, n))
		}
	}
}

func (v *Validator) hasScenario(slug string) bool {
	for i := range v.site.Scenarios {
		if v.site.Scenarios[i].Slug == slug {
			return true
		}
	}
	return false
}

func (v *Validator) hasCertificate(slug string) bool {
	for i := range v.site.Certificates {
		if v.site.Certificates[i].Slug == slug {
			return true
		}
	}
	return false
}

func (v *Validator) hasCase(slug string) bool {
	for i := range v.site.Cases {
		if v.site.Cases[i].Slug == slug {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func validVideoURL(raw string) bool {
	u, err := parseURL(raw)
	if err != nil {
		return false
	}
	return videoHosts[u]
}

func parseURL(raw string) (string, error) {
	// 避免引入 net/url 之外的依赖，这里只需要 host
	raw = strings.TrimSpace(raw)
	idx := strings.Index(raw, "://")
	if idx < 0 {
		return "", fmt.Errorf("not absolute url")
	}
	rest := raw[idx+3:]
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return "", fmt.Errorf("empty host")
	}
	return strings.ToLower(rest), nil
}

var (
	i18nType     = reflect.TypeOf(model.I18n{})
	modelPkgPath = reflect.TypeOf(model.Site{}).PkgPath()
)
