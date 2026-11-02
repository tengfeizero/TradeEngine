package render

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/tradeengine/tradeengine/internal/model"
)

// 机器可读的产物：llms.txt 与 CSV 导出（B1 v2.0 E7 / C10）。
//
// 为什么这两件事必须做在引擎里而不是交付时手写：
//   - llms.txt 的内容全部来自已有模型（品牌、产品、页面、联系方式），
//     每站手写一遍等于每站都可能写错、且不可能随内容更新；
//   - 参数 CSV 是 B 端买家的真实动作——采购把 5 个 SKU 的参数丢进自己的
//     比价表，比让他从网页上抄 40 行数字专业得多。
//
// 两者都是纯构建期生成，零内容成本、零配置。

// LLMs 生成 /llms.txt（AI 搜索入口的机器可读声明）。
//
// 格式遵循 llms.txt 约定（H1 站名 + blockquote 摘要 + 分节链接列表），
// 只写「已被内容证明的事实」：产品参数来自 YAML，联系方式来自 settings，
// 页面清单来自实际产物——不是让模型去猜这家公司做什么。
func (r *Renderer) LLMs() []byte {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(def, def); return s }
	base := strings.TrimRight(r.site.Settings.BaseURL, "/")
	s := r.site.Settings

	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s\n\n", s.Brand.Name)
	if tag := tr(s.Brand.Tagline); tag != "" {
		fmt.Fprintf(&b, "> %s\n\n", tag)
	}
	// 实体段：把「谁、在哪、怎么联系」写死，减少模型编造空间（E7 NAP 一致）。
	fmt.Fprintf(&b, "- Legal name: %s\n", s.Company.LegalName)
	if s.Company.FoundedYear > 0 {
		fmt.Fprintf(&b, "- Founded: %d\n", s.Company.FoundedYear)
	}
	fmt.Fprintf(&b, "- Address: %s\n", r.addressLine(def))
	if s.Contact.Email != "" {
		fmt.Fprintf(&b, "- Email: %s\n", s.Contact.Email)
	}
	if s.Contact.Phone != "" {
		fmt.Fprintf(&b, "- Phone: %s\n", s.Contact.Phone)
	}
	if len(s.Markets) > 0 {
		var ms []string
		for _, m := range s.Markets {
			if n := tr(m.Name); n != "" {
				ms = append(ms, n+" ("+m.Code+")")
			} else {
				ms = append(ms, m.Code)
			}
		}
		fmt.Fprintf(&b, "- Markets served: %s\n", strings.Join(ms, ", "))
	}
	fmt.Fprintf(&b, "- Languages: %s\n", strings.Join(r.site.Langs(), ", "))
	b.WriteString("\n")

	// 产品段：一条一行，带 MOQ / 交期 / HS——AI 回答「这家能做 X 吗」时
	// 需要的就是这三项，不是形容词。
	if products := r.site.PublishedProducts(); len(products) > 0 {
		b.WriteString("## Products\n\n")
		for _, p := range products {
			title := tr(p.Title)
			cat := p.Category
			if c := r.site.CategoryBySlug(p.Category); c != nil {
				cat = c.Slug
			}
			url := base + productPath(def, cat, p.Slug)
			var facts []string
			if p.ModelNo != "" {
				facts = append(facts, "model "+p.ModelNo)
			}
			if p.Moq > 0 {
				facts = append(facts, "MOQ "+formatMoq(p.Moq, p.MoqUnit))
			}
			if lt := tr(p.LeadTime); lt != "" {
				facts = append(facts, "lead time "+lt)
			}
			if p.HSCode != "" {
				facts = append(facts, "HS "+p.HSCode)
			}
			if len(facts) == 0 {
				fmt.Fprintf(&b, "- [%s](%s)\n", title, url)
			} else {
				fmt.Fprintf(&b, "- [%s](%s): %s\n", title, url, strings.Join(facts, " · "))
			}
		}
		b.WriteString("\n")
	}

	// 页面段：只列已发布的固定页（法律页也在内——AI 引用政策时要有出处）。
	if pages := r.publishedPages(); len(pages) > 0 {
		b.WriteString("## Pages\n\n")
		for _, p := range pages {
			title := tr(p.Title)
			url := base + "/" + def + "/"
			if p.Slug != "home" {
				url = base + "/" + def + "/" + p.Slug + "/"
			}
			fmt.Fprintf(&b, "- [%s](%s)\n", title, url)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Notes\n\n")
	b.WriteString("- Prices are quoted per order; MOQ and lead times above are per SKU.\n")
	b.WriteString("- Product pages list full specifications, packaging and sample policy.\n")
	return b.Bytes()
}

// publishedPages 按导航顺序返回已发布固定页（首页在前）。
func (r *Renderer) publishedPages() []*model.Page {
	var out []*model.Page
	if home := r.site.PageBySlug("home"); home != nil && home.Status == "published" {
		out = append(out, home)
	}
	for i := range r.site.Pages {
		p := &r.site.Pages[i]
		if p.Slug == "home" || p.Status != "published" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// SpecsCSV 导出一个产品的完整参数（含贸易四件套）。
//
// 列固定为 Group,Label,Value：采购把它导进 Excel 后可以直接按 Group 透视，
// 也可以只取「商务」那几行做比价表。开头用 # 注释行带元数据——Excel /
// Numbers / pandas 都能正常读，人也能一眼看出这是哪个 SKU。
func (r *Renderer) SpecsCSV(p *model.Product, lang string) []byte {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }
	ui := r.ui(lang)

	var b bytes.Buffer
	w := csv.NewWriter(&b)
	meta := func(k, v string) {
		if v == "" {
			return
		}
		_ = w.Write([]string{"# " + k, v})
	}
	meta("Product", tr(p.Title))
	meta("Slug", p.Slug)
	meta("Model", p.ModelNo)
	if p.Category != "" {
		name := p.Category
		if c := r.site.CategoryBySlug(p.Category); c != nil {
			if n := tr(c.Name); n != "" {
				name = n
			}
		}
		meta("Category", name)
	}
	if p.Moq > 0 {
		meta(ui["moq"], formatMoq(p.Moq, r.unitLabel(ui, p.MoqUnit)))
	}
	meta(ui["lead_time"], tr(p.LeadTime))
	meta(ui["packaging"], tr(p.Packaging))
	meta(ui["hs_code"], p.HSCode)
	if p.Carton != nil {
		meta(ui["carton"], fmt.Sprintf("%d pcs · %.3f CBM · %.1f kg",
			p.Carton.QtyPerCarton, p.Carton.CBM, p.Carton.GrossWeight))
	}
	meta(ui["sample"], tr(p.SamplePolicy))
	_ = w.Write([]string{"# Language", lang})
	_ = w.Write(nil)

	_ = w.Write([]string{"Group", "Label", "Value"})
	group := ""
	for _, g := range p.Specs {
		group = tr(g.Group)
		for _, it := range g.Items {
			val := tr(it.Value)
			if it.ValueNum != nil {
				val = formatNumber(*it.ValueNum)
				if u := r.unitLabel(ui, it.Unit); u != "" {
					val += " " + u
				}
			}
			_ = w.Write([]string{group, tr(it.Label), val})
		}
	}
	w.Flush()
	return b.Bytes()
}

// SKUList 导出全站 SKU 清单（B1 C10 的「SKU 编码体系」）。
// 一行一个 SKU，含 MOQ / 交期 / HS / 产品页 URL——这是买家做 RFQ 的底表。
func (r *Renderer) SKUList(lang string) []byte {
	def := r.site.DefaultLang()
	tr := func(m model.I18n) string { s, _ := m.Resolve(lang, def); return s }

	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"slug", "model_no", "title", "category", "moq", "moq_unit", "lead_time", "hs_code", "url"})
	for _, p := range r.site.PublishedProducts() {
		_ = w.Write([]string{
			p.Slug, p.ModelNo, tr(p.Title), p.Category,
			strconv.FormatFloat(p.Moq, 'f', -1, 64), p.MoqUnit,
			tr(p.LeadTime), p.HSCode,
			productPath(lang, p.Category, p.Slug),
		})
	}
	w.Flush()
	return b.Bytes()
}
