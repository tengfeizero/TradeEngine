package render

import (
	"fmt"
	"html/template"
	"path"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/icons"
	"github.com/tengfeizero/TradeEngine/internal/model"
)

// 区块渲染层。
//
// 契约：每个区块有一个 YAML 输入结构（首字母小写的 in 结构）+ 一个视图结构 +
// 一个 blk_<type> 模板。三者同名，新增区块只动这三处。
//
// 这样设计的代价是区块字段没有类型检查（YAML 二次解码），收益是 model 不会
// 被几十个只有某类站才用的字段撑爆。权衡见《11》第六节。

// ---------------------------------------------------------------- 视图模型

// HeroView 是首屏。
type HeroView struct {
	Badge    string
	Kicker   string
	Headline template.HTML // 支持 {高亮词} → <span class="hl">
	Sub      string
	CtaText  string
	CtaHref  string
	Cta2Text string
	Cta2Href string
	Stats    []HeroStat
	Image    *ImageView
}

// HeroStat 是首屏下方那排硬指标（国家数、月产能、良率…）。
//
// 它和 stats 区块不是一回事：stats 是章节里的对比条，hero-stats 是「三秒判断」
// 用的证据，位置必须在首屏内，否则买家要滚动才看得到——滚了就流失了。
type HeroStat struct {
	Value string
	Label string
}

// highlight 把 headline 里的 {词} 转成高亮片段。
//
// 设计稿的首屏标题都有一两个着色词（"Fly Beyond Limits" 的 Limits），
// 那是整屏的视觉落点。用花括号标记而不是让作者写 HTML：内容层不碰标签，
// 换布局时同一个词在 tech 下是渐变、在 editorial 下是衬线斜体，模板决定形态。
func highlight(s string) template.HTML {
	esc := template.HTMLEscapeString(s)
	return template.HTML(hlRe.ReplaceAllString(esc, `<span class="hl">$1</span>`)) //nolint:gosec // 已转义
}

var hlRe = regexp.MustCompile(`\{([^{}]{1,40})\}`)

// RichTextView 是纯文本区块（公司介绍、政策说明）。
type RichTextView struct {
	Title      string
	Kicker     string
	Lead       string
	Paragraphs []template.HTML
}

// ImageTextView 是图文左右排布。
type ImageTextView struct {
	Kicker   string
	Title    string
	Body     string
	Bullets  []string
	// Checks 非空时用勾选图标列表替代普通圆点列表——设计稿里的工厂能力清单
	// 全是这种形态，普通 ul 表达不出「逐条承诺」的语气。
	Checks   []string
	Image    *ImageView
	Flip     bool
	CtaText  string
	CtaHref  string
}

// StatItem 是一个数字指标。
type StatItem struct {
	Value string
	Label string
	Note  string
}

// StatsView 是数据条带。
type StatsView struct {
	Kicker string
	Title  string
	Items  []StatItem
}

// FeatureItem 是 features 区块里的一项能力/卖点。
type FeatureItem struct {
	Icon   template.HTML // <use> 片段；icon 风格卡片可填
	No     string        // 编号（numbered 风格自动生成「01 / tag」）
	Tag    string        // 编号后的小标签（如 Flight Control）
	Title  string
	Body   string
	Checks []string
}

// FeaturesView 是图标/编号能力网格，对应设计稿的
// tech-card（Core Technology）与 scenario-card（Use Cases）那一族版式。
// style: icon（默认）= 图标卡；style: numbered = 「01 / 标签」编号卡。
type FeaturesView struct {
	Kicker string
	Title  string
	Intro  string
	Style  string
	Items  []FeatureItem
}

// GridView 是卡片网格的通用外壳。
type GridView struct {
	Kicker string
	Title string
	Intro string
	Cards []Card
	More  string
	MoreHref string
}

// Card 是通用卡片：产品 / 品类 / 场景 / 认证 / 案例都复用它。
// 模板只关心「有图没图、几行字、链到哪」，不需要知道数据来自哪个实体。
type Card struct {
	Href  string
	Title string
	Sub   string
	Meta  string
	Tag   string
	// Icon 是已展开的 <svg><use> 片段（template.HTML，注册表输出，无注入面）。
	// 设置后若没有图片，卡片渲染成「图标卡」——设计稿的场景/服务网格
	// 就是这种无图、图标置顶的形态。
	Icon          template.HTML
	Image         *ImageView
	Download      string
	DownloadLabel string
	Count         int
}

// Step 是流程里的一步。
type Step struct {
	No    string
	Title string
	Body  string
}

// ---------------------------------------------------------------- 工厂证据链

// EquipItem 是一台设备。qty 用字符串是为了容纳「2 条 · 2021 年投产」这种写法。
type EquipItem struct {
	Name string
	Qty  string
	Note string
}

// AuditItem 是第三方审核报告（BSCI / Sedex / ISO 9001 …）。
// Number 与 Date 是这一项的意义所在：没有编号与日期的「已通过 BSCI 审核」
// 是一句谁都可以写的话，买家查不到也核不了。
type AuditItem struct {
	Name   string
	Number string
	Issuer string
	Date   string
	Valid  string
	File   string
}

// FactoryView 是「工厂证据链」区块（B1 v2.0 D6）。
//
// 设计立场：买家不信「我们有自己的工厂」，只信「这是产线、这是产能数字、
// 这是 QC 的第五道工序、这是带编号的审核报告」。所以这个区块的每个字段
// 都要求可核验的具体值，而不是形容词。
type FactoryView struct {
	Kicker    string
	Title     string
	Intro     string
	Capacity  []StatItem
	Equipment []EquipItem
	QC        []Step
	Audits    []AuditItem
	Retention string
	Photos    []*ImageView
	// 小标题文案由引擎给死（与 UI 词典同源），内容层只写值。
	LabelCapacity  string
	LabelEquipment string
	LabelQC        string
	LabelAudits    string
	LabelRetention string
	LabelPhotos    string
	LabelNumber    string
	LabelValid     string
}

// Testimonial 是一条客户证言（B1 v2.0 D7）。
type Testimonial struct {
	Quote   string
	Author  string
	Role    string
	Company string
	Country string
	Date    string
	Product *NavLink
}

// TestimonialView 是客户证言区块。
type TestimonialView struct {
	Kicker string
	Title  string
	Intro  string
	Items  []Testimonial
}

// LegalFactsView 是法定披露区块（B1 v2.0 R2）。
// 它的字段全部来自 settings.compliance，页面不重复写一遍值——
// 一处真相，改 settings 全站同步。
type LegalFactsView struct {
	Title string
	Facts []Fact
}

// ProcessView 是工厂/生产流程。
type ProcessView struct {
	Kicker string
	Title  string
	Intro  string
	Steps  []Step
}

// Member 是团队成员。
type Member struct {
	Name  string
	Role  string
	Bio   string
	Photo *ImageView
}

// TeamView 是团队区块。
type TeamView struct {
	Kicker  string
	Title   string
	Intro   string
	Members []Member
}

// QA 是一条问答。
type QA struct {
	Q string
	A []string
}

// FAQView 是常见问题。
type FAQView struct {
	Kicker string
	Title  string
	Items  []QA
}

// CTAView 是行动号召。
type CTAView struct {
	Kicker  string
	Title   string
	Body    string
	CtaText string
	CtaHref string
}

// LogosView 是客户/合作伙伴标志墙。
type LogosView struct {
	Kicker string
	Title  string
	Logos  []*ImageView
}

// GalleryView 是图集。
type GalleryView struct {
	Kicker string
	Title  string
	Images []*ImageView
}

// Blocks 渲染整页的区块序列。
func (r *Renderer) blocks(lang string, blocks []model.Block) (template.HTML, error) {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type == "" {
			continue
		}
		view, err := r.blockView(lang, blk)
		if err != nil {
			return "", err
		}
		if view == nil {
			continue // 数据不足（例如该品类下没有产品），整块不渲染
		}
		var buf strings.Builder
		if err := r.tmpl.ExecuteTemplate(&buf, "blk_"+blk.Type, view); err != nil {
			return "", fmt.Errorf("渲染区块 %s 失败: %w", blk.Type, err)
		}
		b.WriteString(buf.String())
	}
	return template.HTML(b.String()), nil //nolint:gosec // 内容已在区块模板里转义
}

func (r *Renderer) blockView(lang string, blk model.Block) (any, error) {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }
	trs := func(list []model.I18n) []string {
		out := make([]string, 0, len(list))
		for _, it := range list {
			if s := tr(it); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	// ic 把图标名变成 <use> 片段并登记进本页 sprite。
	// 未知名字返回空串——校验层已报 ICON_UNKNOWN，这里只保证不产出悬空引用。
	ic := func(name string) string {
		r.useIcon(name)
		return icons.Use(name)
	}

	switch blk.Type {
	case "features":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Intro  model.I18n `yaml:"intro"`
			Style  string     `yaml:"style"`
			Items  []struct {
				Icon   string       `yaml:"icon"`
				Tag    model.I18n   `yaml:"tag"`
				Title  model.I18n   `yaml:"title"`
				Body   model.I18n   `yaml:"body"`
				Checks []model.I18n `yaml:"checks"`
			} `yaml:"items"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		if in.Style != "numbered" {
			in.Style = "icon"
		}
		v := &FeaturesView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro), Style: in.Style}
		for i, it := range in.Items {
			item := FeatureItem{Icon: template.HTML(ic(it.Icon)), Title: tr(it.Title), Body: tr(it.Body), Checks: trs(it.Checks)}
			if len(item.Checks) > 0 {
				r.useIcon("check")
			}
			if in.Style == "numbered" {
				// 设计稿的编号是「01 / Flight Control」：序号引擎给死，标签可选。
				item.No = fmt.Sprintf("%02d", i+1)
				item.Tag = tr(it.Tag)
			}
			if item.Title == "" {
				continue
			}
			v.Items = append(v.Items, item)
		}
		if len(v.Items) == 0 {
			return nil, nil
		}
		return v, nil

	case "hero":
		var in struct {
			Badge    model.I18n `yaml:"badge"`
			Kicker   model.I18n `yaml:"kicker"`
			Headline model.I18n `yaml:"headline"`
			Sub      model.I18n `yaml:"sub"`
			CtaText  model.I18n `yaml:"cta_text"`
			CtaHref  string     `yaml:"cta_href"`
			Cta2Text model.I18n `yaml:"cta2_text"`
			Cta2Href string     `yaml:"cta2_href"`
			Stats    []struct {
				Value string     `yaml:"value"`
				Label model.I18n `yaml:"label"`
			} `yaml:"stats"`
			Image string `yaml:"image"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		headline := tr(in.Headline)
		v := &HeroView{
			Badge:    tr(in.Badge),
			Kicker:   tr(in.Kicker),
			Headline: highlight(headline),
			Sub:      tr(in.Sub),
			CtaText:  tr(in.CtaText),
			CtaHref:  r.localHref(lang, in.CtaHref),
			Cta2Text: tr(in.Cta2Text),
			Cta2Href: r.localHref(lang, in.Cta2Href),
			Image:    r.heroImage(in.Image, headline),
		}
		for _, s := range in.Stats {
			if s.Value == "" {
				continue
			}
			v.Stats = append(v.Stats, HeroStat{Value: s.Value, Label: tr(s.Label)})
		}
		if headline == "" {
			return nil, nil
		}
		return v, nil

	case "richtext":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Lead   model.I18n `yaml:"lead"`
			Body   model.I18n `yaml:"body"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &RichTextView{Kicker: tr(in.Kicker), Title: tr(in.Title), Lead: tr(in.Lead), Paragraphs: boldParagraphs(tr(in.Body))}
		if v.Title == "" && len(v.Paragraphs) == 0 {
			return nil, nil
		}
		return v, nil

	case "image_text":
		var in struct {
			Kicker  model.I18n   `yaml:"kicker"`
			Title   model.I18n   `yaml:"title"`
			Body    model.I18n   `yaml:"body"`
			Bullets []model.I18n `yaml:"bullets"`
			Checks  []model.I18n `yaml:"checks"`
			Image   string     `yaml:"image"`
			Flip    bool       `yaml:"flip"`
			CtaText model.I18n `yaml:"cta_text"`
			CtaHref string   `yaml:"cta_href"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &ImageTextView{
			Kicker: tr(in.Kicker), Title: tr(in.Title), Body: tr(in.Body), Bullets: trs(in.Bullets), Checks: trs(in.Checks),
			Image: r.image(in.Image, tr(in.Title), "media-img", "(max-width: 860px) 100vw, 480px", "lazy"),
			Flip:  in.Flip, CtaText: tr(in.CtaText), CtaHref: r.localHref(lang, in.CtaHref),
		}
		if len(v.Checks) > 0 {
			r.useIcon("check")
		}
		if v.Title == "" && v.Body == "" {
			return nil, nil
		}
		return v, nil

	case "stats":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Items  []struct {
				Value string   `yaml:"value"`
				Label model.I18n `yaml:"label"`
				Note  model.I18n `yaml:"note"`
			} `yaml:"items"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &StatsView{Kicker: tr(in.Kicker), Title: tr(in.Title)}
		for _, it := range in.Items {
			if it.Value == "" {
				continue
			}
			v.Items = append(v.Items, StatItem{Value: it.Value, Label: tr(it.Label), Note: tr(it.Note)})
		}
		if len(v.Items) == 0 {
			return nil, nil
		}
		return v, nil

	case "product_grid":
		var in struct {
			Kicker   model.I18n `yaml:"kicker"`
			Title    model.I18n `yaml:"title"`
			Intro    model.I18n `yaml:"intro"`
			Category string   `yaml:"category"`
			Featured bool     `yaml:"featured"`
			Limit    int      `yaml:"limit"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		products := r.site.PublishedProducts()
		var cards []Card
		for _, p := range products {
			if in.Category != "" && p.Category != in.Category {
				continue
			}
			if in.Featured && !p.Featured {
				continue
			}
			cards = append(cards, r.productCard(p, lang))
			if in.Limit > 0 && len(cards) >= in.Limit {
				break
			}
		}
		if len(cards) == 0 {
			return nil, nil
		}
		v := &GridView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro), Cards: cards}
		if in.Category == "" {
			v.More, v.MoreHref = r.ui(lang)["all_products"], "/"+lang+"/products/"
		}
		return v, nil

	case "category_grid":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Intro  model.I18n `yaml:"intro"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		var cards []Card
		for i := range r.site.Categories {
			c := &r.site.Categories[i]
			if c.Status != "published" {
				continue
			}
			n := len(r.site.ProductsInCategory(c.Slug))
			if n == 0 {
				continue
			}
			cards = append(cards, Card{
				Href:  path.Join("/", lang, "products", c.Slug) + "/",
				Title: tr(c.Name), Sub: tr(c.Desc), Count: n,
				Image: r.cardImage(c.Cover, tr(c.Name)),
			})
		}
		if len(cards) == 0 {
			return nil, nil
		}
		return &GridView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro), Cards: cards}, nil

	case "scenario_grid":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Intro  model.I18n `yaml:"intro"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		var cards []Card
		for i := range r.site.Scenarios {
			s := &r.site.Scenarios[i]
			if s.Status != "published" {
				continue
			}
			// 场景卡优先走图标（设计稿的 Use Cases 网格无图、图标置顶）；
			// 没配图标才退回封面图。icon 名的合法性由校验层把关。
			card := Card{Title: tr(s.Name), Sub: tr(s.Summary), Tag: s.Icon}
			if s.Icon != "" && icons.Has(s.Icon) {
				card.Icon = template.HTML(ic(s.Icon))
			} else {
				card.Image = r.cardImage(s.Cover, tr(s.Name))
			}
			cards = append(cards, card)
		}
		if len(cards) == 0 {
			return nil, nil
		}
		return &GridView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro), Cards: cards}, nil

	case "certificate_grid":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Intro  model.I18n `yaml:"intro"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		today := time.Now().Format("2006-01-02")
		var cards []Card
		for _, c := range r.site.PublishedCertificates(today) {
			meta := c.Issuer
			if c.Number != "" {
				meta = strings.TrimSpace(meta + " · " + c.Number)
			}
			cards = append(cards, Card{
				Title: tr(c.Name), Sub: meta, Meta: c.Expires,
				Download: c.File, DownloadLabel: r.ui(lang)["download"],
				Image: r.cardImage(c.Logo, tr(c.Name)),
			})
		}
		if len(cards) == 0 {
			return nil, nil
		}
		return &GridView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro), Cards: cards}, nil

	case "case_grid":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Intro  model.I18n `yaml:"intro"`
			Limit  int        `yaml:"limit"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		var cards []Card
		for _, c := range r.site.PublishedCases() {
			cards = append(cards, Card{
				Href:  path.Join("/", lang, "cases", c.Slug) + "/",
				Title: tr(c.Title), Sub: tr(c.Industry), Meta: c.Country,
				Image: r.cardImage(c.Cover, tr(c.Title)),
			})
			if in.Limit > 0 && len(cards) >= in.Limit {
				break
			}
		}
		if len(cards) == 0 {
			return nil, nil
		}
		v := &GridView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro), Cards: cards}
		v.More, v.MoreHref = r.ui(lang)["all_cases"], "/"+lang+"/cases/"
		return v, nil

	case "process":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Intro  model.I18n `yaml:"intro"`
			Steps  []struct {
				Title model.I18n `yaml:"title"`
				Body  model.I18n `yaml:"body"`
			} `yaml:"steps"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &ProcessView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro)}
		for i, s := range in.Steps {
			v.Steps = append(v.Steps, Step{No: fmt.Sprintf("%02d", i+1), Title: tr(s.Title), Body: tr(s.Body)})
		}
		if len(v.Steps) == 0 {
			return nil, nil
		}
		return v, nil

	case "team":
		var in struct {
			Kicker  model.I18n `yaml:"kicker"`
			Title   model.I18n `yaml:"title"`
			Intro   model.I18n `yaml:"intro"`
			Members []struct {
				Name  string   `yaml:"name"`
				Role  model.I18n `yaml:"role"`
				Bio   model.I18n `yaml:"bio"`
				Photo string   `yaml:"photo"`
			} `yaml:"members"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &TeamView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro)}
		for _, m := range in.Members {
			v.Members = append(v.Members, Member{
				Name: m.Name, Role: tr(m.Role), Bio: tr(m.Bio),
				Photo: r.cardImage(m.Photo, m.Name),
			})
		}
		if len(v.Members) == 0 {
			return nil, nil
		}
		return v, nil

	case "faq":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Items  []struct {
				Q model.I18n `yaml:"q"`
				A model.I18n `yaml:"a"`
			} `yaml:"items"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &FAQView{Kicker: tr(in.Kicker), Title: tr(in.Title)}
		for _, it := range in.Items {
			q := tr(it.Q)
			if q == "" {
				continue
			}
			a := splitParagraphs(tr(it.A))
			v.Items = append(v.Items, QA{Q: q, A: a})
			// 同一份问答喂给 FAQPage 结构化数据（M5-5），视觉与 JSON-LD 同源。
			r.pageFAQ = append(r.pageFAQ, QA{Q: q, A: a})
		}
		if len(v.Items) == 0 {
			return nil, nil
		}
		return v, nil


	case "factory":
		var in struct {
			Kicker    model.I18n `yaml:"kicker"`
			Title     model.I18n `yaml:"title"`
			Intro     model.I18n `yaml:"intro"`
			Capacity  []struct {
				Item  model.I18n `yaml:"item"`
				Value string     `yaml:"value"`
				Note  model.I18n `yaml:"note"`
			} `yaml:"capacity"`
			Equipment []struct {
				Name model.I18n `yaml:"name"`
				Qty  string     `yaml:"qty"`
				Note model.I18n `yaml:"note"`
			} `yaml:"equipment"`
			QC []struct {
				Title model.I18n `yaml:"title"`
				Body  model.I18n `yaml:"body"`
			} `yaml:"qc"`
			Audits []struct {
				Name       model.I18n `yaml:"name"`
				Number     string     `yaml:"number"`
				Issuer     string     `yaml:"issuer"`
				Date       string     `yaml:"date"`
				ValidUntil string     `yaml:"valid_until"`
				File       string     `yaml:"file"`
			} `yaml:"audits"`
			Retention model.I18n `yaml:"retention"`
			Photos    []struct {
				Src     string     `yaml:"src"`
				Caption model.I18n `yaml:"caption"`
				Date    string     `yaml:"date"`
			} `yaml:"photos"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		ui := r.ui(lang)
		v := &FactoryView{
			Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro),
			Retention:      tr(in.Retention),
			LabelCapacity:  ui["factory_capacity"],
			LabelEquipment: ui["factory_equipment"],
			LabelQC:        ui["factory_qc"],
			LabelAudits:    ui["factory_audits"],
			LabelRetention: ui["factory_retention"],
			LabelPhotos:    ui["factory_photos"],
			LabelNumber:    ui["audit_number"],
			LabelValid:     ui["audit_valid"],
		}
		for _, c := range in.Capacity {
			if c.Value == "" {
				continue
			}
			v.Capacity = append(v.Capacity, StatItem{Value: c.Value, Label: tr(c.Item), Note: tr(c.Note)})
		}
		for _, e := range in.Equipment {
			if n := tr(e.Name); n != "" {
				v.Equipment = append(v.Equipment, EquipItem{Name: n, Qty: e.Qty, Note: tr(e.Note)})
			}
		}
		for i, q := range in.QC {
			if t := tr(q.Title); t != "" {
				v.QC = append(v.QC, Step{No: fmt.Sprintf("%02d", i+1), Title: t, Body: tr(q.Body)})
			}
		}
		for _, a := range in.Audits {
			item := AuditItem{
				Name: tr(a.Name), Number: a.Number, Issuer: a.Issuer,
				Date: a.Date, Valid: a.ValidUntil,
			}
			if a.File != "" && r.media.Exists(a.File) {
				item.File = "/" + strings.TrimPrefix(a.File, "/")
			}
			if item.Name != "" {
				v.Audits = append(v.Audits, item)
			}
		}
		for _, ph := range in.Photos {
			if img := r.cardImage(ph.Src, tr(ph.Caption)); img != nil {
				v.Photos = append(v.Photos, img)
			}
		}
		if len(v.Capacity)+len(v.Equipment)+len(v.QC)+len(v.Audits) == 0 {
			return nil, nil
		}
		return v, nil

	case "testimonials":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Intro  model.I18n `yaml:"intro"`
			Items  []struct {
				Quote   model.I18n `yaml:"quote"`
				Author  string     `yaml:"author"`
				Role    model.I18n `yaml:"role"`
				Company string     `yaml:"company"`
				Country string     `yaml:"country"`
				Date    string     `yaml:"date"`
				Product string     `yaml:"product"`
			} `yaml:"items"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &TestimonialView{Kicker: tr(in.Kicker), Title: tr(in.Title), Intro: tr(in.Intro)}
		for _, it := range in.Items {
			q := tr(it.Quote)
			if q == "" || it.Author == "" {
				continue
			}
			t := Testimonial{
				Quote: q, Author: it.Author, Role: tr(it.Role),
				Company: it.Company, Country: it.Country, Date: it.Date,
			}
			// 证言挂到具体产品上，买家能顺着点回去看规格——这是「证言」与
			// 「口号」的区别：可核对。
			if it.Product != "" {
				if p := r.site.ProductBySlug(it.Product); p != nil && p.Status == "published" {
					title, _ := p.Title.Resolve(lang, def)
					t.Product = &NavLink{
						Label: title,
						Href:  productPath(lang, p.Category, p.Slug),
					}
				}
			}
			v.Items = append(v.Items, t)
		}
		if len(v.Items) == 0 {
			return nil, nil
		}
		return v, nil

	case "legal_facts":
		var in struct {
			Title model.I18n `yaml:"title"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		facts := r.complianceView(lang)
		if len(facts) == 0 {
			return nil, nil
		}
		title := tr(in.Title)
		if title == "" {
			title = r.ui(lang)["compliance_title"]
		}
		return &LegalFactsView{Title: title, Facts: facts}, nil

	case "cta":
		var in struct {
			Kicker  model.I18n `yaml:"kicker"`
			Title   model.I18n `yaml:"title"`
			Body    model.I18n `yaml:"body"`
			CtaText model.I18n `yaml:"cta_text"`
			CtaHref string   `yaml:"cta_href"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		v := &CTAView{Kicker: tr(in.Kicker), Title: tr(in.Title), Body: tr(in.Body), CtaText: tr(in.CtaText), CtaHref: r.localHref(lang, in.CtaHref)}
		if v.Title == "" {
			return nil, nil
		}
		if v.CtaHref == "" {
			v.CtaHref = "#inquiry"
		}
		return v, nil

	case "logos":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Items  []string   `yaml:"items"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		var logos []*ImageView
		for _, it := range in.Items {
			if v := r.image(it, "", "logo-img", "160px", "lazy"); v != nil {
				logos = append(logos, v)
			}
		}
		if len(logos) == 0 {
			return nil, nil
		}
		return &LogosView{Kicker: tr(in.Kicker), Title: tr(in.Title), Logos: logos}, nil

	case "gallery":
		var in struct {
			Kicker model.I18n `yaml:"kicker"`
			Title  model.I18n `yaml:"title"`
			Images []string   `yaml:"images"`
		}
		if err := decodeBlock(blk, &in); err != nil {
			return nil, err
		}
		g := r.gallery(in.Images, tr(in.Title))
		if len(g) == 0 {
			return nil, nil
		}
		return &GalleryView{Kicker: tr(in.Kicker), Title: tr(in.Title), Images: g}, nil

	case "inquiry":
		return r.form(lang, ""), nil

	default:
		// 未知区块在 validate 阶段已经报错；这里静默跳过，避免渲染期 panic
		return nil, nil
	}
}

// decodeBlock 把区块的原始 map 二次解码成强类型。
// 这条路径比直接 Unmarshal 慢，但构建期只跑一次，换来的是区块字段可以自由演进。
func decodeBlock(b model.Block, v any) error {
	raw, err := yaml.Marshal(b.Extra)
	if err != nil {
		return fmt.Errorf("区块 %s 解码失败: %w", b.Type, err)
	}
	if err := yaml.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("区块 %s 字段类型错误: %w", b.Type, err)
	}
	return nil
}

// localHref 给站内绝对路径补上语言前缀。
// 外链（http/https/mailto/tel/锚点）原样返回，不与语言混在一起。
func (r *Renderer) localHref(lang, href string) string {
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "#") || strings.Contains(href, "://") ||
		strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return href
	}
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}
	return "/" + lang + href
}

// productCard 把产品转成卡片。
func (r *Renderer) productCard(p *model.Product, lang string) Card {
	def := r.site.DefaultLang()
	title, _ := p.Title.Resolve(lang, def)
	sub, _ := p.Subtitle.Resolve(lang, def)
	return Card{
		Href:  productPath(lang, p.Category, p.Slug),
		Title: title, Sub: sub,
		Image: r.cardImage(p.Cover, title),
	}
}
