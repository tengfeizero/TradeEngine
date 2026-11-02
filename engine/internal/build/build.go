// Package build 编排构建流水线的 7 个阶段：
// Load → Validate → Resolve → i18n → Render → Assets → Emit。
//
// 关键约束：**有错误就一个页面都不产出**。
// 半个站部署上去比没部署更糟——客户看到的是 404 和空列表，而你以为上线了。
package build

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tengfeizero/TradeEngine/internal/loader"
	"github.com/tengfeizero/TradeEngine/internal/model"
	"github.com/tengfeizero/TradeEngine/internal/render"
	"github.com/tengfeizero/TradeEngine/internal/validate"
)

// Report 是写入 .build/report.json 的完整构建报告，也是交付凭证。
type Report struct {
	Site     string           `json:"site"`
	BuildAt  string           `json:"build_at"`
	Pages    int              `json:"pages"`
	Langs    []string         `json:"langs"`
	Errors   []validate.Issue `json:"errors"`
	Warnings []validate.Issue `json:"warnings"`
	SEO      SEOStats         `json:"seo"`
	Assets   AssetStats       `json:"assets"`
	Budget   BudgetStats      `json:"budget"`
	Output   string           `json:"output"`
}

// BudgetStats 是首屏字节预算（B1 v2.0 F2）：
// 买家在拉各斯/雅加达的 3G 上看站，不在我们的机房旁边。
// Lighthouse 的模拟网络测不出「首屏 3 MB」这种量级问题，只能自己算。
type BudgetStats struct {
	BudgetKB int    `json:"budget_kb"`
	WorstURL string `json:"worst_url"`
	WorstKB  int    `json:"worst_kb"`
	Over     int    `json:"over"`
}

// pageBudgetBytes 取 1 MB（B1 F2 的「首屏字节 ≤1MB」）。
// 计量口径：HTML 全文 + 首屏第一张图（hero/cover）+ 内联 CSS/JS（已在 HTML 内）。
const pageBudgetBytes = 1024 * 1024

// SEOStats 汇总 SEO 相关的结构性指标。
type SEOStats struct {
	HreflangSets   int `json:"hreflang_sets"`
	HreflangBroken int `json:"hreflang_broken"`
	SitemapURLs    int `json:"sitemap_urls"`
	LinkBroken     int `json:"link_broken"`
}

// AssetStats 汇总图片管线结果。
type AssetStats struct {
	Images     int   `json:"images"`
	MediaFiles int   `json:"media_files"`
	MediaBytes int64 `json:"media_bytes"`
	Oversized  int   `json:"oversized"`
	Transcoded int   `json:"transcoded"`
}

// Result 是构建结果。
type Result struct {
	Report    *Report
	OutputDir string
}

var (
	hreflangRe = regexp.MustCompile(`hreflang="([^"]+)"\s+href="([^"]+)"`)
	linkRe     = regexp.MustCompile(`(?:href|src)="(/[^"#?]*)"`)
)

// pageGroup 是一组「同一实体在不同语言下的产物」，用于生成 sitemap 与 hreflang。
type pageGroup struct {
	Paths   map[string]string // lang -> 相对输出路径
	LastMod string
	NoIndex bool
}

// Options 是构建的临时覆写项，只影响本次构建，不写回站点配置。
type Options struct {
	// Skin 非空时临时换皮（engine build --skin xxx）：设计比稿、客户看样都用它，
	// 不必为了看一眼效果去改 theme.yaml。
	Skin string
}

// Run 执行一次完整构建。
func Run(root string) (*Result, error) {
	return RunWithLoader(loader.NewFileLoader(root))
}

// RunWithOptions 用临时覆写项构建站点目录。
func RunWithOptions(root string, opts Options) (*Result, error) {
	return RunWithLoaderOpts(loader.NewFileLoader(root), opts)
}

// RunWithLoader 用指定 Loader 加载内容并构建——「存储可换、模型不换」（M3）：
// 文件与内容库（contentdb.DBLoader）走同一条校验 → 渲染 → 产物流水线。
func RunWithLoader(l loader.Loader) (*Result, error) {
	return RunWithLoaderOpts(l, Options{})
}

// RunWithLoaderOpts 是构建的真正入口。
func RunWithLoaderOpts(l loader.Loader, opts Options) (*Result, error) {
	site, err := l.Load()
	if err != nil {
		return nil, err
	}
	// Root 统一取自 Site：FileLoader 与 DBLoader 都会填
	root := site.Root

	rep := validate.New(site).Run()

	outRel := site.Config.Output
	outDir := filepath.Join(root, filepath.FromSlash(outRel))
	reportDir := filepath.Dir(outDir)
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return nil, err
	}

	br := &Report{
		Site:     filepath.Base(absPath(root)),
		BuildAt:  time.Now().Format(time.RFC3339),
		Langs:    site.Langs(),
		Errors:   append([]validate.Issue{}, rep.Errors...),
		Warnings: append([]validate.Issue{}, rep.Warnings...),
		Output:   outRel,
	}

	// 有错误则不产出任何页面，避免把有问题的站部署上去
	if !rep.OK() {
		writeReport(reportDir, br)
		return &Result{Report: br, OutputDir: outDir}, nil
	}

	r, err := render.New(site, opts.Skin)
	if err != nil {
		return nil, err
	}

	if err := os.RemoveAll(outDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	emitted := map[string]bool{}
	var htmlFiles []string
	// writeFile 只落文件与登记路径；write 额外计入页面数。
	writeFile := func(rel string, content []byte) error {
		full := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			return err
		}
		emitted["/"+strings.TrimPrefix(filepath.ToSlash(rel), "/")] = true
		return nil
	}
	write := func(rel string, content []byte) error {
		if err := writeFile(rel, content); err != nil {
			return err
		}
		br.Pages++
		if strings.HasSuffix(rel, ".html") {
			htmlFiles = append(htmlFiles, rel)
		}
		return nil
	}

	// 静态资产：表单增强脚本。放在 /assets 下便于单独设缓存策略。
	if err := writeFile("assets/inquiry.js", render.InquiryJS()); err != nil {
		return nil, err
	}

	// favicon：用主题主色生成 16x16。没有它会 404 一次——Lighthouse 的
	// best-practices 会因为 console 里的网络报错扣分，浏览器也会多一个请求。
	if fav, err := favicon(site.Theme.Colors["primary"]); err == nil {
		if err := writeFile("favicon.ico", fav); err != nil {
			return nil, err
		}
	}

	var groups []pageGroup
	// emit 渲染同一实体的所有语言版本，并登记成一组用于 sitemap。
	emit := func(lastmod string, noindex bool, fn func(lang string) ([]byte, string, error)) error {
		g := pageGroup{Paths: map[string]string{}, LastMod: lastmod, NoIndex: noindex}
		for _, lang := range site.Langs() {
			html, rel, err := fn(lang)
			if err != nil {
				return err
			}
			if err := write(rel, html); err != nil {
				return err
			}
			g.Paths[lang] = rel
		}
		groups = append(groups, g)
		return nil
	}

	// 1. 固定页（首页 / 关于 / 工厂 / 认证 / 联系）
	for i := range site.Pages {
		p := &site.Pages[i]
		if p.Status != "published" {
			continue
		}
		noindex := p.SEO != nil && p.SEO.NoIndex
		if err := emit(p.UpdatedAt, noindex, func(lang string) ([]byte, string, error) {
			return r.Page(p, lang)
		}); err != nil {
			return nil, err
		}
	}

	// 2. 产品总览
	if len(site.PublishedProducts()) > 0 {
		if err := emit("", false, r.ProductIndex); err != nil {
			return nil, err
		}
	}

	// 3. 分类页
	for i := range site.Categories {
		c := &site.Categories[i]
		if c.Status != "published" || len(site.ProductsInCategory(c.Slug)) == 0 {
			continue
		}
		if err := emit(c.UpdatedAt, false, func(lang string) ([]byte, string, error) {
			return r.Category(c, lang)
		}); err != nil {
			return nil, err
		}
	}

	// 4. 产品详情
	for i := range site.Products {
		p := &site.Products[i]
		if p.Status != "published" {
			continue
		}
		noindex := p.SEO != nil && p.SEO.NoIndex
		if err := emit(p.UpdatedAt, noindex, func(lang string) ([]byte, string, error) {
			return r.Product(p, lang)
		}); err != nil {
			return nil, err
		}
	}

	// 4.5 每个产品的参数 CSV（C10）：与产品页同目录，链接天然有效。
	for i := range site.Products {
		p := &site.Products[i]
		if p.Status != "published" {
			continue
		}
		for _, lang := range site.Langs() {
			rel := path.Join(lang, "products", p.Category, p.Slug, "specs.csv")
			if err := writeFile(rel, r.SpecsCSV(p, lang)); err != nil {
				return nil, err
			}
		}
	}

	// 5. 案例
	if len(site.PublishedCases()) > 0 {
		if err := emit("", false, r.CaseIndex); err != nil {
			return nil, err
		}
		for i := range site.Cases {
			c := &site.Cases[i]
			if c.Status != "published" {
				continue
			}
			if err := emit(c.UpdatedAt, false, func(lang string) ([]byte, string, error) {
				return r.CaseDetail(c, lang)
			}); err != nil {
				return nil, err
			}
		}
	}

	// 5.5 场景总览与详情（有关联产品时才出，避免空页进 sitemap）
	if len(site.Scenarios) > 0 {
		hasScenarioPage := false
		for i := range site.Scenarios {
			sc := &site.Scenarios[i]
			if sc.Status != "published" || len(site.ProductsInScenario(sc.Slug)) == 0 {
				continue
			}
			hasScenarioPage = true
			if err := emit(sc.UpdatedAt, false, func(lang string) ([]byte, string, error) {
				return r.ScenarioDetail(sc, lang)
			}); err != nil {
				return nil, err
			}
		}
		if hasScenarioPage {
			if err := emit("", false, r.ScenarioIndex); err != nil {
				return nil, err
			}
		}
	}

	// 6. 询盘成功页与 404（都不进 sitemap）
	if err := emit("", true, r.Thanks); err != nil {
		return nil, err
	}
	for _, lang := range site.Langs() {
		html, rel, err := r.NotFound(lang)
		if err != nil {
			return nil, err
		}
		if err := write(rel, html); err != nil {
			return nil, err
		}
	}
	// 根目录 404.html 给任意静态服务器兜底（Nginx / Pages 都认这个路径）
	if def := site.DefaultLang(); def != "" {
		src := filepath.Join(outDir, def, "404.html")
		if b, err := os.ReadFile(src); err == nil {
			if err := writeFile("404.html", b); err != nil {
				return nil, err
			}
		}
	}

	// 7. 媒体文件：复制并附加内容哈希，构建产物必须自包含
	assets, mediaMap, err := copyMedia(site.Root, outDir)
	if err != nil {
		return nil, err
	}
	br.Assets = assets

	// 7.5 把 HTML 里的 /media/... 引用改写成带哈希的文件名。
	// 必须在 verifyLinks 之前做：改写后的引用要能通过可达性检查。
	if len(mediaMap) > 0 {
		if err := rewriteMediaRefs(outDir, emitted, mediaMap); err != nil {
			return nil, err
		}
	}

	// 8. 机器可读产物（E7 / C10）
	// llms.txt：AI 搜索的入口声明，内容全部取自模型，不手写。
	if err := writeFile("llms.txt", r.LLMs()); err != nil {
		return nil, err
	}
	products := site.PublishedProducts()
	if len(products) > 0 {
		// SKU 清单：买家做 RFQ 的底表（一行一个 SKU）。
		if err := writeFile("data/sku-list.csv", r.SKUList(site.DefaultLang())); err != nil {
			return nil, err
		}
		// 产品图包：买家写提案要用你的图，让他一次下完。
		if pack, n, err := mediaPack(root, products); err != nil {
			return nil, err
		} else if n > 0 {
			if err := writeFile("data/product-images.zip", pack); err != nil {
				return nil, err
			}
		}
	}

	// 8. sitemap.xml / robots.txt
	entries := sitemapEntries(site, groups)
	if err := writeFile("sitemap.xml", r.Sitemap(entries)); err != nil {
		return nil, err
	}
	if err := writeFile("robots.txt", r.Robots()); err != nil {
		return nil, err
	}
	br.SEO.SitemapURLs = len(entries)

	// 9. 结构性校验：hreflang 互链 + 站内链接可达
	broken, sets, err := verifyHreflang(outDir, emitted)
	if err != nil {
		return nil, err
	}
	br.SEO.HreflangSets = sets
	br.SEO.HreflangBroken = broken
	if broken > 0 {
		br.Errors = append(br.Errors, validate.Issue{
			Code:    "HREFLANG_BROKEN",
			Message: fmt.Sprintf("%d 条 hreflang 指向的输出文件不存在", broken),
		})
	}
	var overPages []string
	lbroken, samples, err := verifyLinks(outDir, emitted)
	if err != nil {
		return nil, err
	}
	br.SEO.LinkBroken = lbroken
	if lbroken > 0 {
		br.Errors = append(br.Errors, validate.Issue{
			Code:    "LINK_BROKEN",
			Message: fmt.Sprintf("%d 条站内链接指向不存在的页面：%s", lbroken, strings.Join(samples, "、")),
		})
	}

	// 首屏字节预算（F2）：HTML + 首屏图 + 脚本，超过 1 MB 记警告。
	br.Budget, overPages = budgetReport(outDir, htmlFiles)
	if len(overPages) > 0 {
		br.Warnings = append(br.Warnings, validate.Issue{
			Code:    "PAGE_BUDGET_EXCEEDED",
			Entity:  "pages",
			Message: fmt.Sprintf("%d 页首屏超过 %d KB（最重 %s %d KB）：%s", len(overPages), pageBudgetBytes/1024, br.Budget.WorstURL, br.Budget.WorstKB, strings.Join(overPages, "、")),
		})
	}

	// 图片管线的警告并入报告：缺图、超大图、转码失败都在这里浮出来
	for _, w := range r.MediaWarnings() {
		br.Warnings = append(br.Warnings, validate.Issue{Code: "MEDIA_PIPELINE", Entity: "media", Message: w})
	}

	writeReport(reportDir, br)
	return &Result{Report: br, OutputDir: outDir}, nil
}

// sitemapEntries 从页面组生成 sitemap 条目，跳过 noindex 与草稿。
func sitemapEntries(site *model.Site, groups []pageGroup) []render.SitemapEntry {
	base := strings.TrimRight(site.Settings.BaseURL, "/")
	var entries []render.SitemapEntry
	for _, g := range groups {
		if g.NoIndex || len(g.Paths) == 0 {
			continue
		}
		for _, lang := range site.Langs() {
			rel, ok := g.Paths[lang]
			if !ok {
				continue
			}
			loc := urlPath(rel)
			if loc == "" {
				continue
			}
			e := render.SitemapEntry{Loc: base + loc, LastMod: g.LastMod}
			for _, l := range site.Langs() {
				if p, ok := g.Paths[l]; ok {
					e.Alts = append(e.Alts, render.Alt{Hreflang: l, Href: base + urlPath(p)})
				}
			}
			if def, ok := g.Paths[site.DefaultLang()]; ok {
				e.Alts = append(e.Alts, render.Alt{Hreflang: "x-default", Href: base + urlPath(def)})
			}
			entries = append(entries, e)
		}
	}
	return entries
}

// urlPath 把输出文件路径转成站点路径：en/products/a/index.html → /en/products/a/
func urlPath(rel string) string {
	p := "/" + strings.TrimPrefix(filepath.ToSlash(rel), "/")
	p = strings.TrimSuffix(p, "index.html")
	if p == "" {
		return "/"
	}
	return p
}

// verifyHreflang 检查每个产出页面的 alternate 链接是否都能落到真实文件。
// 这一条把「70% 的 hreflang 错误来自缺返回链接」变成构建期不可能犯的错。
func verifyHreflang(outDir string, emitted map[string]bool) (broken, sets int, err error) {
	err = filepath.Walk(outDir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		matches := hreflangRe.FindAllStringSubmatch(string(data), -1)
		if len(matches) > 0 {
			sets++
		}
		for _, m := range matches {
			href := m[2]
			if idx := strings.Index(href, "://"); idx >= 0 {
				rest := href[idx+3:]
				if i := strings.Index(rest, "/"); i >= 0 {
					href = rest[i:]
				} else {
					href = "/"
				}
			}
			// 目录式 URL（以 / 结尾）对应 index.html
			if strings.HasSuffix(href, "/") {
				href += "index.html"
			}
			if !emitted[href] {
				broken++
			}
		}
		return nil
	})
	return broken, sets, err
}

// verifyLinks 检查所有产物的站内链接。
//
// 这是「交付前最后一道闸」：内容里的 nav、区块 CTA、卡片链接、
// 甚至手写的相对路径，最终都要落到一个真实文件上。
func verifyLinks(outDir string, emitted map[string]bool) (broken int, samples []string, err error) {
	seen := map[string]bool{}
	err = filepath.Walk(outDir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(p, ".html") {
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		for _, m := range linkRe.FindAllStringSubmatch(string(data), -1) {
			href := m[1]
			if seen[href] {
				continue
			}
			seen[href] = true
			target := href
			if strings.HasSuffix(target, "/") {
				target += "index.html"
			}
			if emitted[target] {
				continue
			}
			// 媒体与静态资产不登记在 emitted 里，直接看磁盘
			disk := filepath.Join(outDir, filepath.FromSlash(strings.TrimPrefix(target, "/")))
			if st, err := os.Stat(disk); err == nil && !st.IsDir() {
				continue
			}
			broken++
			if len(samples) < 5 {
				samples = append(samples, href)
			}
		}
		return nil
	})
	return broken, samples, err
}

// copyMedia 把 media/ 原样复制到产物目录。
//
// 不做内容哈希文件名：内容目录里的引用是人工写的（media/products/x.png），
// 哈希化会让 YAML 与实际文件名脱节。缓存策略交给 CDN 的 cache-purge。
// copyMedia 把 media/ 复制进产物，文件名附加内容哈希：
// media/products/tea.png → media/products/tea.3fa2b1c9.png。
//
// 为什么值得这一步：文件名不变时，CDN/浏览器只敢给 1 小时缓存；
// 内容一变文件名就变，缓存可以放心给一年 immutable——重复访问的外贸客户
// 二次打开几乎零流量。HTML 引用由 rewriteMediaRefs 统一改写，
// 渲染层与 YAML 内容都不感知哈希的存在。
//
// media/.derived/（cwebp 派生图）跳过：文件名里已带内容指纹。
func copyMedia(root, outDir string) (AssetStats, map[string]string, error) {
	var st AssetStats
	mapping := map[string]string{}
	src := filepath.Join(root, "media")
	if _, err := os.Stat(src); err != nil {
		return st, mapping, nil
	}
	err := filepath.Walk(src, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		outRel := rel
		// 已指纹化的派生图与 system 目录文件不改名
		if !strings.Contains(rel, "/.derived/") {
			outRel = hashedName(rel, b)
		}
		mapping[rel] = outRel

		dst := filepath.Join(outDir, filepath.FromSlash(outRel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		st.MediaFiles++
		st.MediaBytes += info.Size()
		switch strings.ToLower(path.Ext(p)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg":
			st.Images++
		}
		if info.Size() > 400*1024 && !strings.HasSuffix(strings.ToLower(p), ".pdf") {
			st.Oversized++
		}
		return nil
	})
	return st, mapping, err
}

// hashedName 在文件名与扩展名之间插入 8 位内容哈希。
func hashedName(rel string, content []byte) string {
	sum := sha1.Sum(content)
	dir, name := path.Split(rel)
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return dir + base + "." + hex.EncodeToString(sum[:4]) + ext
}

// rewriteMediaRefs 把已产出 HTML 里的 /media/... 引用改写成哈希文件名。
//
// 逐 token 做边界检查而不是无脑 ReplaceAll：srcset 里同目录多张图用逗号+
// 空格分隔，中间的 URL 两边都不是引号；前缀碰撞（a.png 与 a.png.bak）
// 靠「先长后短」排序兜住。
func rewriteMediaRefs(outDir string, emitted map[string]bool, mapping map[string]string) error {
	// 收集成切片并按 token 长度降序，防止短路径替换破坏长路径
	type pair struct{ from, to string }
	pairs := make([]pair, 0, len(mapping))
	for orig, hashed := range mapping {
		if orig == hashed {
			continue
		}
		pairs = append(pairs, pair{"/" + orig, "/" + hashed})
	}
	sort.Slice(pairs, func(i, j int) bool { return len(pairs[i].from) > len(pairs[j].from) })
	if len(pairs) == 0 {
		return nil
	}

	boundary := func(b byte) bool {
		switch b {
		case '"', '\'', ',', ' ', '\t', '(', ')':
			return true
		}
		return false
	}
	replace := func(src []byte, from, to string) []byte {
		var out []byte
		for i := 0; i < len(src); {
			if bytes.HasPrefix(src[i:], []byte(from)) {
				before := byte(0)
				if i > 0 {
					before = src[i-1]
				}
				after := byte(0)
				if i+len(from) < len(src) {
					after = src[i+len(from)]
				}
				if boundary(before) && boundary(after) {
					out = append(out, []byte(to)...)
					i += len(from)
					continue
				}
			}
			out = append(out, src[i])
			i++
		}
		return out
	}

	for rel := range emitted {
		if !strings.HasSuffix(rel, ".html") {
			continue
		}
		full := filepath.Join(outDir, filepath.FromSlash(rel))
		b, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		for _, p := range pairs {
			b = replace(b, p.from, p.to)
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// favicon 生成主题主色的 16x16 PNG（favicon.ico 位置放 PNG 字节，
// 所有现代浏览器都按内容嗅探，不再需要真正的 ICO 编码器）。
// imgSrcRe 匹配页面里第一张图的 src。首屏图（hero / cover）一定在正文最前，
// 所以「第一个 <img>」就是首屏的主要字节来源。
var imgSrcRe = regexp.MustCompile(`<img[^>]+src="(/[^"]+)"`)

// budgetReport 计算每页的首屏字节数（HTML + 首屏图）。
//
// 为什么要自己算：Lighthouse 的手机模拟是 4G 降速，1 MB 与 3 MB 的差别
// 在那里只差几百分之一秒；但买家在 3G 上，那就是 2 秒与 8 秒的差别。
// 这个检查不看分数，只看字节。
func budgetReport(outDir string, files []string) (BudgetStats, []string) {
	st := BudgetStats{BudgetKB: pageBudgetBytes / 1024}
	var over []string
	for _, rel := range files {
		raw, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		total := int64(len(raw))
		if m := imgSrcRe.FindSubmatch(raw); m != nil {
			img := filepath.Join(outDir, filepath.FromSlash(string(m[1])))
			if fi, err := os.Stat(img); err == nil {
				total += fi.Size()
			}
		}
		kb := int(total / 1024)
		if kb > st.WorstKB {
			st.WorstKB, st.WorstURL = kb, "/"+filepath.ToSlash(rel)
		}
		if total > pageBudgetBytes {
			st.Over++
			if len(over) < 5 {
				over = append(over, fmt.Sprintf("%s(%dKB)", rel, kb))
			}
		}
	}
	return st, over
}

// mediaPack 把已发布产品的封面图打成一个 zip（C10：图片素材包）。
// 买家写内部提案时要放供应商的产品图，让他一张张右键保存是在浪费他的时间。
// 只打包原图、不做转码：这是给他做 PPT 用的，不是给网页用的。
func mediaPack(root string, products []*model.Product) ([]byte, int, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	n := 0
	for _, p := range products {
		if p.Cover == "" {
			continue
		}
		src := filepath.Join(root, filepath.FromSlash(p.Cover))
		f, err := os.Open(src)
		if err != nil {
			continue // 缺图由媒体管线报警，这里不重复报
		}
		w, err := zw.Create("images/" + p.Slug + filepath.Ext(src))
		if err == nil {
			if _, cerr := io.Copy(w, f); cerr == nil {
				n++
			}
		}
		f.Close()
	}
	if n > 0 {
		if w, err := zw.Create("README.txt"); err == nil {
			_, _ = io.WriteString(w, "Product images for buyer proposals. Please keep the brand name and model numbers intact.\n")
		}
	}
	if err := zw.Close(); err != nil {
		return nil, 0, err
	}
	if n == 0 {
		return nil, 0, nil
	}
	return buf.Bytes(), n, nil
}
func favicon(primaryHex string) ([]byte, error) {
	c := color.RGBA{R: 0x5B, G: 0x7F, B: 0x5B, A: 0xff}
	if strings.HasPrefix(primaryHex, "#") && len(primaryHex) == 7 {
		if v, err := strconv.ParseUint(primaryHex[1:], 16, 32); err == nil {
			c = color.RGBA{
				R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff,
			}
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func absPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

func writeReport(dir string, br *Report) {
	sort.SliceStable(br.Warnings, func(i, j int) bool {
		if br.Warnings[i].Entity != br.Warnings[j].Entity {
			return br.Warnings[i].Entity < br.Warnings[j].Entity
		}
		return br.Warnings[i].Field < br.Warnings[j].Field
	})
	b, err := json.MarshalIndent(br, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "report.json"), b, 0o644)
}

// Scaffold 生成一个新的站点目录（engine new）。
func Scaffold(dir, brand, theme string) error {
	files := scaffoldFiles(brand, theme)
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(full); err == nil {
			continue // 不覆盖已有文件
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return err
		}
	}
	for _, d := range []string{"media", "theme", "content/products", "content/pages", "content/cases"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(d)), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func scaffoldFiles(brand, skin string) map[string]string {
	// 皮肤决定观感，色板决定品牌。脚手架给每个预置皮肤配一套「它的典型品牌色」，
	// 让客户建站第一眼看到的就是「这套皮肤应该长什么样」，而不是六套皮肤配同一个绿。
	colors := map[string]string{
		"classic":    `  primary: "#2F6B3F"` + "\n" + `  ink: "#1A1A1A"` + "\n" + `  paper: "#F7F4EC"` + "\n" + `  accent: "#B08A3E"`,
		"editorial":  `  primary: "#1F3A5F"` + "\n" + `  ink: "#121212"` + "\n" + `  paper: "#FBFAF7"` + "\n" + `  accent: "#8C1D18"`,
		"tech":       `  primary: "#3DDC97"` + "\n" + `  ink: "#E8EDF2"` + "\n" + `  paper: "#0B0F14"` + "\n" + `  accent: "#4EA8FF"`,
		"industrial": `  primary: "#C2410C"` + "\n" + `  ink: "#14181C"` + "\n" + `  paper: "#F2F3F4"` + "\n" + `  accent: "#1D4ED8"`,
		"organic":    `  primary: "#5B7F5B"` + "\n" + `  ink: "#2A2620"` + "\n" + `  paper: "#FAF6EE"` + "\n" + `  accent: "#8A6A2C"`,
		"minimal":    `  primary: "#1C1C1C"` + "\n" + `  ink: "#1C1C1C"` + "\n" + `  paper: "#FCFCFB"` + "\n" + `  accent: "#8A8A8A"`,
	}
	if _, ok := colors[skin]; !ok {
		skin = "classic"
	}
	return map[string]string{
		"engine.yaml": `engine_version: 1
output: .build/public
`,
		"settings.yaml": fmt.Sprintf(`base_url: https://example.com
schema_version: 1
default_lang: en
langs: [en, zh]
theme: default

brand:
  name: %s
  logo: ""
  tagline:
    en: Your tagline here
    zh: 这里写一句品牌定位

company:
  legal_name: Your Company Co., Ltd.
  founded_year: 2010
  address:
    country: CN
    region: Zhejiang
    city: Yiwu
    street: ""
    zip: "322000"

contact:
  email: sales@example.com
  phone: "+86 000 0000 0000"
  # whatsapp: "+8613800000000"    # 填了才有浮层与页脚直达链接

analytics:
  ga4_id: ""                      # 填 G-XXXXXXX 即启用（Cookie 同意后才加载）
  gsc_verify: ""                  # Search Console 的 HTML 标签验证码

# catalog:
#   file: media/catalog.pdf       # 放好 PDF 后取消注释，页脚出现下载入口

# 市场准入信息（B1 R2）：目标市场含 EU/UK 时必填，否则构建会提醒 GPSR_RESPONSIBLE_MISSING。
# 值会渲染到产品页「制造商与市场合规」段、页脚常驻行，以及 legal-notice 页的 legal_facts 区块。
# compliance:
#   eu_responsible:               # GPSR 要求的欧盟境内责任主体
#     name: "[EU importer / authorised representative]"
#     address: "[street, city, postal code, country]"
#     email: "[eu@example.com]"
#     phone: ""
#   legal_notice:                 # 德国 Impressum 要素
#     register_no: "[HRB 00000]"
#     register_court: "[Amtsgericht ...]"
#     vat_id: "[DE000000000]"
#     managing_director: "[Name]"
#     email: ""
#     phone: ""
#   epr:                          # 包装法/电子废弃物注册号
#     de_packaging: "[LUCID DE0000000000000]"
#     fr_packaging: ""
#     weee: ""

markets:
  - code: EU
    name: { en: Europe, zh: 欧洲 }
    langs: [en]
  - code: US
    name: { en: North America, zh: 北美 }
    langs: [en]

nav:
  - label: { en: Home, zh: 首页 }
    href: "/"
  - label: { en: Products, zh: 产品 }
    href: "/products/"
  - label: { en: Contact, zh: 联系 }
    href: "/contact/"

images:
  transcode: false
  quality: 82
  widths: [640, 960, 1280]
  max_kb: 400

inquiry:
  recipients: [sales@example.com]
  fields: [name, company, email, country, product, volume, message]
  required: [name, email, message]
  success_url: /{lang}/thank-you/
  api_base: ""
  allowed_origins: []
  rate_limit_per_hour: 10

store:
  driver: sqlite
  dsn: .data/inquiries.db
`, brand),
		"theme/theme.yaml": fmt.Sprintf(`name: %s
# 皮肤决定观感（配色/版式/装饰都归它管）：engine skins 可列出全部可用皮肤
skin: %s
colors:
%s
fonts:
  heading: '%s'
  body: system-ui, sans-serif
radius: %s
`, brand, skin, colors[skin], fontFor(skin), radiusFor(skin)),
		// 商业模式政策（B1 C9）：B 端第一轮谈判必问的四件事。
		// 没有这一页，买家只能发邮件问「能做白标吗」——这一问就可能问丢。
		"content/pages/business-terms.yaml": `slug: business-terms
status: published
title:
  en: Business Terms
  zh: 商业模式政策
summary:
  en: OEM and private-label minimums, exclusivity, IP ownership and quotation validity — answered before you ask.
  zh: OEM/白标最小量、独家代理、知识产权归属与报价有效期——在你开口之前先答清楚。
blocks:
  - type: richtext
    title:
      en: OEM and ODM programmes
      zh: OEM 与 ODM 合作方式
    body:
      en: |
        **OEM.** We build to your drawings, your BOM and your brand. Minimum order for a first run is [1,000] pcs per model; repeat orders [500] pcs. Tooling, samples and the first-article approval run are quoted separately and itemised in the proforma invoice.

        **ODM.** If you want our existing platform under your brand, minimum is [300] pcs per model and colourway, with your logo on the frame, packaging and manual. Lead time for a first ODM run is [45] days after artwork approval.
      zh: |
        **OEM。** 按你的图纸、BOM 与品牌生产。首次量产最小单款 [1,000] 件，返单 [500] 件。模具、样品与首件确认另行报价，并在形式发票中逐项列出。

        **ODM。** 用我们的成熟平台贴你的品牌：单款单色最小 [300] 件，车架、包装与说明书均可印你的标识。首次 ODM 量产在稿件确认后 [45] 天。
  - type: richtext
    title:
      en: Private label and exclusivity
      zh: 白标与独家代理
    body:
      en: |
        **Private label.** Your logo, packaging and manuals; we do not reference our own brand on the product or in the carton. A private-label agreement covers artwork ownership and reorder obligations.

        **Exclusivity.** Granted territory by territory, never globally. It requires a committed annual volume of [10,000] pcs, a 12-month review and a 90-day notice to withdraw. Exclusivity is a commitment on both sides — if we cannot supply, you are free immediately.
      zh: |
        **白标。** 你的标识、包装与说明书；产品与外箱上不出现我方品牌。白标协议会明确稿件归属与返单义务。

        **独家代理。** 按区域授予，从不给全球独家。条件：年度承诺量 [10,000] 件、每 12 个月复核一次、退出需提前 90 天通知。独家是双向承诺——我方供不上货，你可以立即解除。
  - type: richtext
    title:
      en: IP, tooling and quotation validity
      zh: 知识产权、模具与报价有效期
    body:
      en: |
        **IP.** Your drawings, trademarks and moulds remain yours. We sign an NDA before quoting and never produce your design for another buyer. Our platform patents remain ours.

        **Tooling.** Moulds paid for by you are yours; we store them free of charge for [24] months after the last order and ship them on request.

        **Quotation validity.** Prices are valid for [30] days unless the quote states otherwise, because raw material and freight move. Volume breaks apply per order, not per year.
      zh: |
        **知识产权。** 你的图纸、商标与模具归你所有。报价前可签保密协议，我们不会把你的设计做给其他买家；我方平台专利归我方。

        **模具。** 你付费的模具归你所有；最后一次订单后我们免费保管 [24] 个月，可应要求发回。

        **报价有效期。** 除报价单另有说明，价格 [30] 天内有效——原材料与运费会动。阶梯价按单次订单计算，不按年度累计。
  - type: faq
    title:
      en: Questions buyers ask first
      zh: 买家最常问的几个问题
    items:
      - q:
          en: Can you produce under our brand without any reference to yours?
          zh: 能不能完全贴我们的品牌、不出现你们的任何信息？
        a:
          en: Yes. Private label means the product, packaging, manual and carton carry only your marks. The only exception is what the law requires — for example the manufacturer's name and address on the product for EU GPSR compliance.
          zh: 可以。白标意味着产品、包装、说明书与外箱只出现你的标识。唯一例外是法规要求的部分——例如为满足欧盟 GPSR，产品上必须标注制造商名称与地址。
      - q:
          en: Who owns the design if we co-develop it?
          zh: 共同开发的款式，设计归谁？
        a:
          en: You own what you paid to develop and what is unique to your brief. We keep the right to use generic improvements (a stronger joint, a cheaper finish) across our platform.
          zh: 你出资开发、且专属于你的需求的部分归你。通用性改进（更牢的接头、更省的表面处理）我方保留用于平台其它款式的权利。
      - q:
          en: How long is a quotation valid?
          zh: 报价有效期多久？
        a:
          en: 30 days from the date of issue, printed on every quotation. After that we re-quote against current material and freight costs.
          zh: 自开出日起 [30] 天，每份报价单都会写明。过期后按当时的材料与运费重新报价。
`,
		// 法律声明（B1 R2）：页面本身只有一段说明，法定信息全部来自
		// settings.compliance 的 legal_facts 区块——一处真相，改配置全站同步。
		"content/pages/legal-notice.yaml": `slug: legal-notice
status: published
title:
  en: Legal Notice
  zh: 法律声明
summary:
  en: Company registration, VAT and contact details required by EU and German law.
  zh: 欧盟与德国法律要求的公司登记、增值税与联系方式。
blocks:
  - type: legal_facts
    title:
      en: Company information
      zh: 公司信息
  - type: richtext
    title:
      en: Responsible for content
      zh: 内容责任人
    body:
      en: |
        Information on this site is provided by the company named above. Despite careful checking we accept no liability for the content of external links; responsibility for those lies with their operators.

        For legal inquiries, write to [legal@example.com] with the subject line "Legal".
      zh: |
        本站信息由上述公司提供。外部链接内容经仔细核对，但我们不承担其责任，责任归属各链接的运营方。

        法律相关事务请发邮件至 [legal@example.com]，主题注明「法律」。
`,
		// 隐私政策与贸易条款：B1 G5/D2/C6 的门禁项，脚手架直接给出可发布的
		// 起草版——占位信息用方括号标出，客户照着自己公司信息替换即可。
		"content/pages/privacy-policy.yaml": fmt.Sprintf(`slug: privacy-policy
status: published
title:
  en: Privacy Policy
  zh: 隐私政策
summary:
  en: How we handle the personal information you submit through this site.
  zh: 我们如何处理您通过本站提交的个人信息。
blocks:
  - type: richtext
    title:
      en: Who we are
      zh: 我们是谁
    body:
      en: |
        %s (your registered legal entity, address [registered address]) is the data controller for the information collected on this site. Contact: [sales@example.com].

        We collect only what the inquiry form needs: your name, company, email, country, product interest and estimated annual volume. We use this information for one purpose — replying to your inquiry and preparing a quotation.
      zh: |
        %s（您的注册法律实体，注册地址 [注册地址]），是本站所收集信息的数据控制者。联系方式：[sales@example.com]。

        我们只收集询盘表单所需的信息：姓名、公司、邮箱、国家/地区、感兴趣的产品与年采购量估算。这些信息只用于一个目的——回复您的询盘并准备报价。
  - type: richtext
    title:
      en: Retention, sharing and your rights
      zh: 保存期限、共享与您的权利
    body:
      en: |
        **Retention.** Inquiry records are kept for up to 24 months, then deleted. **Sharing.** We do not sell or share your data with third parties. Our hosting and analytics providers process data on our behalf under contract.

        **Cookies.** This site uses analytics cookies only after you accept the cookie notice; declining leaves analytics disabled. **Your rights.** You may request access, correction or deletion of your data at any time by emailing [sales@example.com]. We respond within 30 days.
      zh: |
        **保存期限。** 询盘记录最长保存 24 个月后删除。**共享。** 我们不出售、不与第三方共享您的数据；托管与统计服务商仅按合同约定代为处理。

        **Cookie。** 本站仅在您同意 Cookie 提示后才启用统计；拒绝则统计保持关闭。**您的权利。** 您随时可以发邮件至 [sales@example.com] 要求查阅、更正或删除您的数据，我们会在 30 天内回复。
`, brand, brand),
		"content/pages/trade-terms.yaml": `slug: trade-terms
status: published
title:
  en: Trade Terms
  zh: 贸易条款
summary:
  en: Payment, incoterms, lead time, packaging and claims — the commercial baseline for every order.
  zh: 付款方式、贸易术语、交期、包装与索赔——每张订单的商业基线。
blocks:
  - type: richtext
    title:
      en: Payment and incoterms
      zh: 付款与贸易术语
    body:
      en: |
        **Payment.** T/T, 30% deposit with order and 70% against copy of B/L. L/C at sight is accepted for orders above [amount]. **Incoterms.** Default FOB [port]; CIF and EXW available on request.

        **Lead time.** [XX] days for repeat orders after deposit; custom development per agreed schedule. **Packaging.** Export cartons; palletized loading plans provided before shipment.
      zh: |
        **付款方式。** 电汇 T/T：下单付 30% 定金，见提单副本付 70% 尾款。[金额] 以上订单可接受即期信用证 L/C at sight。**贸易术语。** 默认 FOB [港口]；可按需 CIF 或 EXW。

        **交期。** 定金到账后常规件 [XX] 天；定制开发按双方确认的排期。**包装。** 出口纸箱，发货前提供托盘装柜方案。
  - type: richtext
    title:
      en: Claims and disputes
      zh: 索赔与争议
    body:
      en: |
        Quality claims must be raised within 30 days of arrival with photos and batch numbers; we reply with a corrective plan within 7 business days. Disputes are first settled by negotiation, otherwise under [arbitration body] arbitration.
      zh: |
        质量索赔需在货物到港后 30 天内提出，附照片与批号；我们将在 7 个工作日内给出处理方案。争议优先协商解决，协商不成按 [仲裁机构] 仲裁。
`,
		"content/pages/home.yaml": `slug: home
status: published
title:
  en: Home
  zh: 首页
summary:
  en: B2B supplier with export experience.
  zh: 有出口经验的 B 端供应商。
blocks:
  - type: hero
    headline:
      en: Built for global buyers
      zh: 为全球买家而生
    sub:
      en: Tell us your market and volume — we reply within 12 hours.
      zh: 告知市场与采购量，工作日 12 小时内回复。
    cta_text:
      en: Request a quote
      zh: 获取报价
    cta_href: "#inquiry"
  - type: product_grid
    title:
      en: Products
      zh: 产品
    limit: 6
  - type: inquiry
`,
		"content/categories.yaml": `- slug: main
  status: published
  order: 1
  name: { en: Main range, zh: 主营产品 }
  desc:
    en: Replace this with your real product lines.
    zh: 替换成你真实的产品线。
`,
		"content/products/sample-product.yaml": `slug: sample-product
status: published
order: 1
featured: true
category: main
model_no: SAMPLE-01
moq: 100
moq_unit: pcs
hs_code: "0000000000"

title:
  en: Sample product
  zh: 示例产品
subtitle:
  en: Replace this file with a real product
  zh: 用真实产品替换这个文件
summary:
  en: One published product is enough to make the whole site build — product list, category page and inquiry form all work.
  zh: 只要有一个已发布产品，整个站就能构建：产品总览、分类页与询盘表单都会正常工作。
description:
  en: |
    这是引擎生成的示例产品，用来验证构建链路。

    Delete it and add your own files under content/products/.
  zh: |
    这是引擎生成的示例产品，用来验证构建链路。

    删掉它，把真实产品放到 content/products/ 下。
lead_time:
  en: 15–20 days after deposit
  zh: 定金后 15–20 天
packaging:
  en: 25kg carton, 20 per pallet
  zh: 25 公斤纸箱，每托 20 箱

specs:
  - group:
      en: Product data
      zh: 产品参数
    items:
      - key: material
        label:
          en: Material
          zh: 材质
        value:
          en: Replace me
          zh: 请替换
      - key: size
        label:
          en: Size
          zh: 尺寸
        value_num: 100
        unit: pcs
        filterable: true
`,
		"content/pages/contact.yaml": `slug: contact
status: published
title:
  en: Contact
  zh: 联系我们
summary:
  en: Send your requirement — we reply within 12 hours on business days.
  zh: 发送需求，工作日 12 小时内回复。
blocks:
  - type: inquiry
`,
	}
}

func fontFor(skin string) string {
	switch skin {
	case "tech":
		return `"Inter", system-ui, sans-serif`
	case "industrial":
		return `"Helvetica Neue", Arial, sans-serif`
	case "minimal":
		return `"Helvetica Neue", Arial, sans-serif`
	case "editorial", "organic":
		return `"Iowan Old Style", Georgia, serif`
	}
	return "Georgia, serif"
}

func radiusFor(skin string) string {
	switch skin {
	case "tech":
		return "6px"
	case "industrial", "minimal":
		return "0"
	case "organic":
		return "10px"
	}
	return "2px"
}
