package render

import (
	"strings"

	"github.com/tradeengine/tradeengine/internal/geo"
)

// Option 是下拉框的一个选项。
type Option struct {
	Value string
	Label string
}

// FormField 是一个表单字段。
type FormField struct {
	Key          string
	Label        string
	Type         string // text | email | tel | textarea | select
	Required     bool
	Placeholder  string
	Autocomplete string
	Options      []Option
}

// InquiryForm 是询盘表单的展示模型。
type InquiryForm struct {
	Action        string
	Lang          string
	Fields        []FormField
	PresetProduct string
	TurnstileKey  string
	// 文案
	Title     string
	Intro     string
	Submit    string
	Sending   string
	Thanks    string
	ErrorText string
	Privacy   string
	// CatalogIntent 是 ?intent=catalog 时预填的留言（M5-9）。
	CatalogIntent string
}

// fieldMeta 是字段的展示元数据。
//
// 这些文案属于「引擎内置」，不由客户填：它们在外贸站里几乎从不变化，
// 而让客户每站重写一遍既浪费时间又会写错（比如把 Company 写成 Corp）。
var fieldMeta = map[string]struct {
	EN, ZH     string
	Type       string
	PhEN, PhZH string
	Autocomp   string
}{
	"name":    {"Your name", "您的姓名", "text", "John Smith", "张三", "name"},
	"company": {"Company", "公司名称", "text", "Acme Trading Co., Ltd.", "某某贸易有限公司", "organization"},
	"email":   {"Business email", "企业邮箱", "email", "you@company.com", "you@company.com", "email"},
	"phone":   {"Phone / WhatsApp", "电话 / WhatsApp", "tel", "+1 234 567 8900", "+86 138 0000 0000", "tel"},
	"country": {"Country / Region", "国家 / 地区", "select", "", "", "country-name"},
	"product": {"Product of interest", "感兴趣的产品", "select", "", "", ""},
	"volume":  {"Estimated annual volume", "年采购量估算", "select", "", "", ""},
	"message": {"Your requirement", "您的需求", "textarea",
		"Quantity, target market, certification requirements, and destination port…",
		"请说明采购量、目标市场、认证要求与目的港…", ""},
}

// volumeOptions 是「年采购量」下拉的固定档位。
// 不用件数而用柜量：件/公斤/吨因行业而异，40 尺柜是所有外贸品类通用的单位，
// 买家（和销售）都能秒懂档位含义。值为英文键入库，翻译只在展示层。
var volumeOptions = []struct {
	Value string
	EN    string
	ZH    string
}{
	{"trial", "Trial / sample order", "试单 / 样品单"},
	{"c1", "Under one 40ft container per year", "年均不足一个 40 尺柜"},
	{"c2", "1–5 × 40ft containers per year", "年均 1–5 个 40 尺柜"},
	{"c3", "5–20 × 40ft containers per year", "年均 5–20 个 40 尺柜"},
	{"c4", "More than 20 × 40ft containers per year", "年均 20 个以上 40 尺柜"},
	{"contract", "Annual contract / stock program", "年度合同 / 常备库存计划"},
}

// formLabels 是表单周边文案。
var formLabels = map[string]map[string]string{
	"en": {
		"title":    "Request a quote",
		"intro":    "Tell us your quantity and target market — we reply within 12 hours on business days.",
		"submit":   "Send inquiry",
		"sending":  "Sending…",
		"thanks":   "Thank you. Your inquiry has been received — we will reply within 12 hours.",
		"error":    "Submission failed. Please try again, or email us directly.",
		"privacy":  "We use your information only to answer this inquiry. No newsletters, no sharing.",
		"optional": "optional",
	},
	"zh": {
		"title":    "获取报价",
		"intro":    "告知采购量与目标市场，工作日 12 小时内回复。",
		"submit":   "提交询盘",
		"sending":  "提交中…",
		"thanks":   "已收到您的询盘，我们会在 12 小时内回复。",
		"error":    "提交失败，请重试或直接发邮件给我们。",
		"privacy":  "您的信息仅用于回复本次询盘，不会用于群发或提供给第三方。",
		"optional": "选填",
	},
}

func (r *Renderer) labels(lang string) map[string]string {
	out := map[string]string{}
	for k, v := range formLabels["en"] {
		out[k] = v
	}
	if m, ok := formLabels[lang]; ok {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// inquiryAction 返回表单提交地址。
// 未配置 api_base 时为同源相对路径（引擎同时托管静态页与 API）。
func (r *Renderer) inquiryAction() string {
	base := strings.TrimRight(r.site.Settings.Inquiry.APIBase, "/")
	return base + "/api/inquiry"
}

// form 构造询盘表单模型。presetProduct 为空表示由用户自己选产品。
func (r *Renderer) form(lang, presetProduct string) *InquiryForm {
	cfg := r.site.Settings.Inquiry
	def := r.site.DefaultLang()
	lbl := r.labels(lang)

	f := &InquiryForm{
		Action:        r.inquiryAction(),
		Lang:          lang,
		PresetProduct: presetProduct,
		TurnstileKey:  cfg.TurnstileSiteKey,
		Title:         lbl["title"],
		Intro:         lbl["intro"],
		Submit:        lbl["submit"],
		Sending:       lbl["sending"],
		Thanks:        lbl["thanks"],
		ErrorText:     lbl["error"],
		Privacy:       lbl["privacy"],
		CatalogIntent: lbl["catalog_intent"],
	}

	for _, key := range cfg.EffectiveFields() {
		meta, ok := fieldMeta[key]
		if !ok {
			continue // 配置里写了引擎不认识的字段名，忽略而不是崩掉
		}
		label, ph := meta.EN, meta.PhEN
		if lang == "zh" {
			label, ph = meta.ZH, meta.PhZH
		}
		ff := FormField{
			Key: key, Label: label, Type: meta.Type,
			Required: cfg.IsRequired(key), Placeholder: ph, Autocomplete: meta.Autocomp,
		}
		switch key {
		case "country":
			for _, c := range geo.All {
				name := c.EN
				if lang == "zh" {
					name = c.ZH
				}
				ff.Options = append(ff.Options, Option{Value: c.Code, Label: name})
			}
		case "volume":
			for _, vo := range volumeOptions {
				label := vo.EN
				if lang == "zh" {
					label = vo.ZH
				}
				ff.Options = append(ff.Options, Option{Value: vo.Value, Label: label})
			}
		case "product":
			// 已指定产品时用隐藏字段，不再给下拉——客户在详情页点「询价」，
			// 让他再选一次产品只会制造「选错产品」的误会
			if presetProduct != "" {
				continue
			}
			for i := range r.site.Products {
				p := &r.site.Products[i]
				if p.Status != "published" {
					continue
				}
				title, _ := p.Title.Resolve(lang, def)
				ff.Options = append(ff.Options, Option{Value: p.Slug, Label: title})
			}
			if len(ff.Options) == 0 {
				continue
			}
		}
		f.Fields = append(f.Fields, ff)
	}
	return f
}
