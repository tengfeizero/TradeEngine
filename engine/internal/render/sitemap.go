package render

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// SitemapEntry 是 sitemap 里的一条 URL。
type SitemapEntry struct {
	Loc     string
	LastMod string
	// Alts 是同一页面其它语言的绝对地址。
	// sitemap 里的 hreflang 比页面里的 <link> 更权威：Google 会优先信它。
	Alts []Alt
}

// Sitemap 生成 sitemap.xml。
//
// 多语言站最容易被忽略的一点：sitemap 里每个 URL 都要带齐 hreflang 组，
// 包括自身。只手写 <link rel="alternate"> 的站，索引速度明显更慢。
func (r *Renderer) Sitemap(entries []SitemapEntry) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:xhtml="http://www.w3.org/1999/xhtml">` + "\n")
	for _, e := range entries {
		if e.Loc == "" {
			continue
		}
		b.WriteString("  <url>\n    <loc>" + escXML(e.Loc) + "</loc>\n")
		if e.LastMod != "" {
			b.WriteString("    <lastmod>" + escXML(e.LastMod) + "</lastmod>\n")
		}
		for _, a := range e.Alts {
			b.WriteString(fmt.Sprintf(`    <xhtml:link rel="alternate" hreflang="%s" href="%s"/>`+"\n",
				escXML(a.Hreflang), escXML(a.Href)))
		}
		b.WriteString("  </url>\n")
	}
	b.WriteString("</urlset>\n")
	return []byte(b.String())
}

// Robots 生成 robots.txt。
//
// 屏蔽 thank-you：它是转化完成页，没有任何检索价值，
// 被收录后会和产品页抢品牌词排名，还会让「提交成功」页出现在搜索结果里。
func (r *Renderer) Robots() []byte {
	base := strings.TrimRight(r.site.Settings.BaseURL, "/")
	var b strings.Builder
	b.WriteString("User-agent: *\nAllow: /\nDisallow: /*/thank-you/\nDisallow: /api/\n\n")
	b.WriteString("Sitemap: " + base + "/sitemap.xml\n")
	return []byte(b.String())
}

// escXML 转义 XML 文本与属性值。
// 只处理 5 个预定义实体：sitemap 里出现的字符集就这些，多余处理反而会引入 bug。
func escXML(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}
