package render

import (
	"html/template"
	"strings"

	"github.com/tradeengine/tradeengine/internal/model"
)

// 市场准入信息（B1 v2.0 R2）的渲染层。
//
// 这一组字段的性质与其它内容不同：它不是「写给人看的卖点」，而是**法规要求的
// 法定披露**。欧盟 GPSR（2024/12 起）明确要求在线商品页展示制造商与欧盟责任方；
// 德国 Impressum 要求注册号、注册法院、法人、VAT、可送达联系方式；包装法 EPR
// 要求展示 LUCID 注册号。
//
// 因此这里的取舍是：
//   - 值一律来自 settings.compliance，**不在页面正文里重复一遍**（一处真相）；
//   - 没配就不输出，绝不编造（宁可缺一项被 B2 记 P0，也不印假的责任方）；
//   - 产品页只放 GPSR 要求的两方（制造商 + 欧盟责任方），
//     公司注册信息走 legal-notice 页的 legal_facts 区块——放错位置等于没放。

// Fact 是一条「标签 - 值」的法定事实。
//
// Href 用 template.URL 而不是 string：html/template 的 URL 过滤器只放行
// http/https/mailto，tel: 会被改写成 #ZgotmplZ——电话链接会变成一行灰色
// 乱码。这不是样式问题，是「法规要求可送达联系方式」直接失效。
type Fact struct {
	Label string
	Value string
	Href  template.URL // 邮箱转 mailto、电话转 tel；空则纯文本
}

// complianceView 汇总当前站点已配置的法定信息。
// 返回 nil 表示一条都没配——调用方据此整块不渲染。
func (r *Renderer) complianceView(lang string) []Fact {
	c := r.site.Settings.Compliance
	ui := r.ui(lang)
	var facts []Fact

	add := func(label, value string, href template.URL) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		facts = append(facts, Fact{Label: label, Value: value, Href: href})
	}

	// 制造商 = 站点主体（company.legal_name + 注册地址），这是 GPSR 的第一项。
	add(ui["manufacturer"], r.site.Settings.Company.LegalName, "")
	add("", r.addressLine(lang), "")

	// 欧盟责任方：GPSR 要求「在欧盟境内可送达的主体」，含名称、地址、邮箱。
	if p := c.EUResponsible; strings.TrimSpace(p.Name) != "" {
		add(ui["eu_responsible"], p.Name, "")
		add("", p.Address, "")
		if p.Email != "" {
			add("", p.Email, template.URL("mailto:"+p.Email))
		}
		if p.Phone != "" {
			add("", p.Phone, template.URL(telHref(p.Phone)))
		}
	}

	ln := c.LegalNotice
	if ln.RegisterNo != "" {
		v := ln.RegisterNo
		if ln.RegisterCourt != "" {
			v += " · " + ln.RegisterCourt
		}
		add(ui["commercial_register"], v, "")
	} else if ln.RegisterCourt != "" {
		add(ui["commercial_register"], ln.RegisterCourt, "")
	}
	add(ui["vat_id"], ln.VATID, "")
	add(ui["managing_director"], ln.ManagingDirector, "")
	if ln.Email != "" {
		add(ui["contact"], ln.Email, template.URL("mailto:"+ln.Email))
	}
	if ln.Phone != "" {
		add(ui["contact"], ln.Phone, template.URL(telHref(ln.Phone)))
	}

	epr := c.EPR
	add(ui["epr_de"], epr.DEPackaging, "")
	add(ui["epr_fr"], epr.FRPackaging, "")
	add(ui["epr_weee"], epr.WEEE, "")

	return facts
}

// gpsrFacts 只取产品页必须披露的部分（制造商 + 欧盟责任方）。
// 注册号与法人信息属于公司登记信息，放在 legal-notice 页更合适。
func (r *Renderer) gpsrFacts(lang string) []Fact {
	c := r.site.Settings.Compliance
	ui := r.ui(lang)
	var facts []Fact
	if n := strings.TrimSpace(r.site.Settings.Company.LegalName); n != "" {
		facts = append(facts, Fact{Label: ui["manufacturer"], Value: n})
		if a := r.addressLine(lang); a != "" {
			facts = append(facts, Fact{Label: "", Value: a})
		}
	}
	if p := c.EUResponsible; strings.TrimSpace(p.Name) != "" {
		facts = append(facts, Fact{Label: ui["eu_responsible"], Value: p.Name})
		if p.Address != "" {
			facts = append(facts, Fact{Label: "", Value: p.Address})
		}
		if p.Email != "" {
			facts = append(facts, Fact{Label: "", Value: p.Email, Href: template.URL("mailto:" + p.Email)})
		}
	}
	return facts
}

// marketsIncludeEU 判断目标市场是否触发欧盟法规（GPSR / EPR / Impressum）。
//
// settings.markets 可能写区域（"EU"）也可能写国家（"DE"），两种都算。
// 这个判断只用于**告警**，不用于拦截——引擎无法替客户确认他到底在哪些国家卖，
// 但可以在明显命中时提醒一句「你填了 EU，却没填责任方」。
func (r *Renderer) marketsIncludeEU() bool {
	for _, m := range r.site.Settings.Markets {
		code := strings.ToUpper(strings.TrimSpace(m.Code))
		if code == "EU" || code == "EEA" || code == "UK" || code == "GB" {
			return true
		}
		for _, c := range model.EUCountries {
			if code == c {
				return true
			}
		}
	}
	return false
}

// marketsIncludeDE 单独认德国：Impressum 与 LUCID 是德国特有要求。
func (r *Renderer) marketsIncludeDE() bool {
	for _, m := range r.site.Settings.Markets {
		if strings.EqualFold(strings.TrimSpace(m.Code), "DE") {
			return true
		}
	}
	return false
}
