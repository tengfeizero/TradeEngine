package render

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/tradeengine/tradeengine/internal/geo"
	"github.com/tradeengine/tradeengine/internal/icons"
	"github.com/tradeengine/tradeengine/internal/model"
)

// 联系浮层与页脚：B1 A1/A2/A6/C7/D2 的引擎侧承载。
//
// 设计稿评审（ecobike/aeronova/zentea 对 ridgeway）的结论之一：
// 联系通道藏在正文文本里（「邮箱 — xxx@xx.com」是纯文字），买家想联系时
// 没有任何可点击的直达路径。这两个组件把它变成引擎默认能力：
//   - 联系浮层：全站常驻右下角，WhatsApp / 邮箱 / 电话一击直达（A1/A6）；
//   - 页脚：地址 + 直达链接 + 法律页出口（隐私政策/贸易条款，G5/D2/C6）。
// 两者都由 settings.yaml 的 contact / company / 社媒配置驱动，
// 没配就不渲染——引擎不假装一个不存在的联系方式。

// ContactLink 是浮层与页脚共用的一个联系通道。
type ContactLink struct {
	Icon template.HTML
	// Href 用 template.URL 承载：html/template 只放行 http/https/mailto，
	// tel: 会被转义成 #ZgotmplZ。这里的值全部由 settings 联系配置构造，
	// 不是用户输入，安全标注是恰当的。
	Href  template.URL
	Label string // 展示文本（页脚用）
	Aria  string // 无障碍标签（浮层用）
	Class string // 通道类型：whatsapp | mail | phone，供皮肤着色
}

// FooterView 是页脚的展示模型。
type FooterView struct {
	// BrandCol 是品牌列：tagline + 实体地址（A2）。
	Tagline   string
	Address   string
	LegalName string
	// ContactCol 是联系列：可直点的邮箱/电话/WhatsApp（A6）。
	Contact []ContactLink
	Social  []NavLink
	// LegalCol 是法律页出口：privacy-policy / trade-terms 等已发布页（G5/D2/C6）。
	Legal []NavLink
	// Catalog 是目录下载入口（M5-9），settings.catalog 配置了站内 PDF 才有。
	Catalog *NavLink
	// Data 是机器可读产物入口（C10）：SKU 清单 CSV、产品图包 ZIP。
	Data []NavLink
	// Compliance 是欧盟责任方的一句话披露（R2）：页脚常驻，不进二级页。
	Compliance string
	Year       int
}

// waDigits 只留数字：wa.me 要求国际格式纯数字（86138…），不能带 +、空格、横线。
func waDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// telHref 电话链接：去掉空格与连字符，保留 +。
func telHref(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '+' || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return "tel:" + b.String()
}

// contactLinks 从 settings 联系配置装配三条直达通道。
// 顺序刻意为 WhatsApp → 邮箱 → 电话：B1 的洞察是中东买家先加 WhatsApp、
// 欧洲买家先看邮箱，电话是兜底；浮层与页脚共用这一份装配。
func (r *Renderer) contactLinks() []ContactLink {
	c := r.site.Settings.Contact
	var out []ContactLink
	if wa := waDigits(c.WhatsApp); wa != "" {
		r.useIcon("chat")
		out = append(out, ContactLink{
			Icon:  template.HTML(icons.Use("chat")),
			Href:  template.URL("https://wa.me/" + wa),
			Label: "WhatsApp",
			Aria:  "Chat on WhatsApp",
			Class: "whatsapp",
		})
	}
	if c.Email != "" {
		r.useIcon("mail")
		out = append(out, ContactLink{
			Icon:  template.HTML(icons.Use("mail")),
			Href:  template.URL("mailto:" + c.Email),
			Label: c.Email,
			Aria:  "Email us",
			Class: "mail",
		})
	}
	if c.Phone != "" {
		r.useIcon("phone")
		out = append(out, ContactLink{
			Icon:  template.HTML(icons.Use("phone")),
			Href:  template.URL(telHref(c.Phone)),
			Label: c.Phone,
			Aria:  "Call us",
			Class: "phone",
		})
	}
	return out
}

// contactBar 是全站常驻的联系浮层。任何一条通道都没配时返回 nil，
// 模板就不输出——宁可没有浮层，不可有死按钮。
func (r *Renderer) contactBar() []ContactLink {
	return r.contactLinks()
}

// addressLine 把 company.address 拼成一行可读地址，带邮编与国名（A2 的「可核验」要求）。
func (r *Renderer) addressLine(lang string) string {
	a := r.site.Settings.Company.Address
	if a.Street == "" && a.City == "" {
		return ""
	}
	country := geo.Name(a.Country, lang)
	parts := []string{a.Street, a.City, a.Region}
	if a.Zip != "" {
		parts = append(parts, a.Zip)
	}
	parts = append(parts, country)
	var b strings.Builder
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p)
	}
	return b.String()
}

// footerView 装配页脚。法律页只列**真实存在且已发布**的页面，
// 设计稿那种「页脚五个链接全是 #」在这里不可能出现。
func (r *Renderer) footerView(lang string) *FooterView {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }

	f := &FooterView{
		Tagline:   tr(r.site.Settings.Brand.Tagline),
		Address:   r.addressLine(lang),
		LegalName: r.site.Settings.Company.LegalName,
		Contact:   r.contactLinks(),
		Year:      time.Now().Year(),
	}
	for name, href := range r.site.Settings.Social {
		if href == "" {
			continue
		}
		label := name
		if label != "" {
			label = strings.ToUpper(label[:1]) + label[1:]
		}
		f.Social = append(f.Social, NavLink{Label: label, Href: href})
	}
	// 法律页出口按「法规要求的顺序」排：隐私 → 条款 → 商业模式政策 → 法律声明。
	for _, slug := range []string{"privacy-policy", "trade-terms", "business-terms", "legal-notice"} {
		if p := r.site.PageBySlug(slug); p != nil && p.Status == "published" {
			title, _ := p.Title.Resolve(lang, def)
			if title == "" {
				title = slug
			}
			f.Legal = append(f.Legal, NavLink{
				Label: title,
				Href:  fmt.Sprintf("/%s/%s/", lang, slug),
			})
		}
	}
	// 欧盟责任方（R2）：GPSR 要求在任何面向欧盟买家的页面可查到责任主体。
	// 页脚是全站唯一「每页都有」的位置，放这里比放法律页更符合法规意图。
	if p := r.site.Settings.Compliance.EUResponsible; strings.TrimSpace(p.Name) != "" {
		parts := []string{r.ui(lang)["eu_responsible"] + ": " + p.Name}
		if p.Address != "" {
			parts = append(parts, p.Address)
		}
		if p.Email != "" {
			parts = append(parts, p.Email)
		}
		f.Compliance = strings.Join(parts, " · ")
	}
	// 机器可读产物（C10）：只要站上有产品，这两个文件就一定在产物里
	// （构建期同批生成），所以链接不会死。
	if products := r.site.PublishedProducts(); len(products) > 0 {
		ui := r.ui(lang)
		f.Data = append(f.Data, NavLink{Label: ui["sku_list"], Href: "/data/sku-list.csv"})
		// 图包只在真的有图时给链接：构建期一个封面都没打进 zip，链接就会 404，
		// 而死链比没有链接更伤（E5）。
		for _, p := range products {
			if p.Cover != "" && r.media.Exists(p.Cover) {
				f.Data = append(f.Data, NavLink{Label: ui["media_pack"], Href: "/data/product-images.zip"})
				break
			}
		}
	}
	// 目录下载（M5-9）：settings.catalog.file 指向站内 PDF 即出现入口。
	if c := r.site.Settings.Catalog; c.File != "" && r.media.Exists(c.File) {
		label := tr(c.Label)
		if label == "" {
			label = r.ui(lang)["catalog"]
		}
		f.Catalog = &NavLink{Label: label, Href: "/" + strings.TrimPrefix(c.File, "/")}
	}
	return f
}
