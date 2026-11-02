// Package doctor 做站点目录的「体检 + 迁移」。
//
// 为什么它值得一个独立命令：validate 回答「内容对不对」，doctor 回答
// 「**结构对不对、是不是旧版本格式**」。两者互补：
//
//   - validate：模型层面的规则（缺必译、断链、垃圾评分……）——每次构建都跑
//   - doctor：文件层面的健康（必备文件、拼写错的键、旧字段名、缺失推荐段）
//     ——只在人手动改 YAML 或跨版本升级时跑
//
// 迁移用 yaml.v3 的 Node 树做**往返编辑**：解析成节点、改键名、再序列化。
// 好处是注释、缩进风格和键顺序都能保留——客户 YAML 里的注释是交付资产，
// 重排一版格式会让他们以为「引擎动了我的文件」。
package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tradeengine/tradeengine/internal/model"
)

// Finding 是一条体检结论。
type Finding struct {
	Level string // "error" 必须修 / "fix" 可自动修 / "warn" 建议 / "info" 提示
	File  string // 相对站点根的路径（目录场景为目录名）
	Msg   string
}

// Report 是一次体检（或体检+修复）的产出。
type Report struct {
	Findings []Finding
	Fixed    []string // Apply 实际改写过的文件（相对路径）
	Backups  string   // 备份目录；空表示没做备份
}

// HasError 是否存在必须人工处理的错误。
func (r *Report) HasError() bool {
	for _, f := range r.Findings {
		if f.Level == "error" {
			return true
		}
	}
	return false
}

// rename 把旧字段名映射到新字段名。
//
// scope 限定生效范围，避免同名键在别的实体里含义不同（例如 `lang` 在
// settings 里是列表、在别处可能是别的意思）。这张表是引擎向前兼容的机制：
// 字段改名后在这里登记一条，老站点跑一次 doctor --fix 就能跟上新版本。
type rename struct {
	Scope string // "settings" / "product" / "certificate" / "seo"
	Old   string
	New   string
	Why   string
}

var renames = []rename{
	{"settings", "lang", "langs", "语言列表字段在 v0.2 统一为复数 langs"},
	{"product", "cat", "category", "产品归属字段统一为 category"},
	{"certificate", "expiry", "expires", "有效期字段统一为 expires"},
	{"seo", "no_index", "noindex", "与 html meta 习惯一致"},
}

// requiredFiles 是「没有它连 load 都会失败」的最小文件集。
var requiredFiles = []string{
	"engine.yaml",
	"settings.yaml",
	"theme/theme.yaml",
	"content/pages/home.yaml",
}

// Run 只体检，不修改任何文件。
func Run(root string) (*Report, error) {
	return examine(root, false)
}

// Fix 体检 + 自动修复。修复前把原文件备份到 .backup/<时间戳>/。
func Fix(root string) (*Report, error) {
	return examine(root, true)
}

// Examine 是 CLI 的统一入口（--fix 开关决定是否写盘）。
func Examine(root string, apply bool) (*Report, error) {
	return examine(root, apply)
}

func examine(root string, apply bool) (*Report, error) {
	rep := &Report{}
	abs := func(rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

	// ---- 1. 必备文件 --------------------------------------------------
	for _, rel := range requiredFiles {
		if _, err := os.Stat(abs(rel)); err != nil {
			rep.Findings = append(rep.Findings, Finding{
				Level: "error", File: rel,
				Msg: "缺少必备文件（engine new 可以生成骨架）",
			})
		}
	}

	// ---- 2. base_url 格式 --------------------------------------------
	if b, err := os.ReadFile(abs("settings.yaml")); err == nil {
		var node yaml.Node
		if yaml.Unmarshal(b, &node) == nil && len(node.Content) > 0 {
			if v := mapGet(node.Content[0], "base_url"); v != nil {
				u := v.Value
				switch {
				case u == "":
					rep.Findings = append(rep.Findings, Finding{
						Level: "error", File: "settings.yaml",
						Msg: "base_url 为空：canonical/hreflang/sitemap 全部依赖它"})
				case !strings.HasPrefix(u, "https://"):
					rep.Findings = append(rep.Findings, Finding{
						Level: "warn", File: "settings.yaml",
						Msg: "base_url 不是 https://：搜索引擎与部分客户网络会降权或拦截 http 站点"})
				case strings.HasSuffix(strings.Trim(u, "\"'"), "/"):
					rep.Findings = append(rep.Findings, Finding{
						Level: "fix", File: "settings.yaml",
						Msg: "base_url 以 / 结尾：会拼出双斜杠 URL"})
					if apply {
						// 引号可能来自 YAML 风格标记而非值本身，Trim 后再写回
						v.Value = strings.TrimRight(u, "/")
						if err := backupAndWrite(root, "settings.yaml", &node, rep); err != nil {
							return rep, err
						}
					}
				}
			}
		}
	}

	// ---- 3. 未知顶层键（拼写错误检测）---------------------------------
	// blocks 内部的自由字段不做这项检查——那是渲染层契约（规则 25）的事。
	checks := []struct {
		pattern string // 相对路径，支持目录通配
		kind    string
		typ     reflect.Type
	}{
		{"settings.yaml", "settings", reflect.TypeOf(model.Settings{})},
		{"theme/theme.yaml", "theme", reflect.TypeOf(model.Theme{})},
	}
	eachYAMLFile(abs("content/products"), func(p string) {
		checks = append(checks, struct {
			pattern, kind string
			typ           reflect.Type
		}{relOf(root, p), "product", reflect.TypeOf(model.Product{})})
	})
	eachYAMLFile(abs("content/cases"), func(p string) {
		checks = append(checks, struct {
			pattern, kind string
			typ           reflect.Type
		}{relOf(root, p), "case", reflect.TypeOf(model.Case{})})
	})
	eachYAMLFile(abs("content/pages"), func(p string) {
		checks = append(checks, struct {
			pattern, kind string
			typ           reflect.Type
		}{relOf(root, p), "page", reflect.TypeOf(model.Page{})})
	})

	for _, c := range checks {
		b, err := os.ReadFile(abs(c.pattern))
		if err != nil {
			continue
		}
		var node yaml.Node
		if err := yaml.Unmarshal(b, &node); err != nil {
			rep.Findings = append(rep.Findings, Finding{
				Level: "error", File: c.pattern, Msg: "YAML 解析失败：" + err.Error()})
			continue
		}
		if len(node.Content) == 0 {
			continue
		}
		doc := node.Content[0]
		// 列表文件（categories.yaml 等）第一层是 sequence，逐项检查
		items := []*yaml.Node{doc}
		if doc.Kind == yaml.SequenceNode {
			items = doc.Content
		}
		known := structKeys(c.typ)
		for _, item := range items {
			if item.Kind != yaml.MappingNode {
				continue
			}
			for i := 0; i+1 < len(item.Content); i += 2 {
				k := item.Content[i].Value
				if !known[k] {
					rep.Findings = append(rep.Findings, Finding{
						Level: "warn", File: c.pattern,
						Msg: fmt.Sprintf("未知字段 %q（拼写错误会被静默忽略），合法键：%s",
							k, joinKeys(known))})
				}
			}
		}
	}

	// ---- 4. 推荐但非必需的配置段 ---------------------------------------
	if b, err := os.ReadFile(abs("settings.yaml")); err == nil {
		var node yaml.Node
		if yaml.Unmarshal(b, &node) == nil && len(node.Content) > 0 &&
			node.Content[0].Kind == yaml.MappingNode {
			m := node.Content[0]
			if mapGet(m, "images") == nil {
				rep.Findings = append(rep.Findings, Finding{
					Level: "fix", File: "settings.yaml",
					Msg: "缺少 images 段：图片管线将按默认值运行（可不填，但显式声明便于团队协作）"})
				if apply {
					addImages(m)
					if err := backupAndWrite(root, "settings.yaml", &node, rep); err != nil {
						return rep, err
					}
				}
			}
		}
	}
	if b, err := os.ReadFile(abs("theme/theme.yaml")); err == nil {
		var node yaml.Node
		if yaml.Unmarshal(b, &node) == nil && len(node.Content) > 0 &&
			node.Content[0].Kind == yaml.MappingNode {
			m := node.Content[0]
			if mapGet(m, "layout") == nil {
				rep.Findings = append(rep.Findings, Finding{
					Level: "fix", File: "theme/theme.yaml",
					Msg: "缺少 layout：将回退 classic（可选 classic / editorial / tech）"})
				if apply {
					addLayout(m)
					if err := backupAndWrite(root, "theme/theme.yaml", &node, rep); err != nil {
						return rep, err
					}
				}
			}
		}
	}

	// ---- 5. 字段重命名迁移 ---------------------------------------------
	kinds := map[string]string{
		"settings.yaml":    "settings",
		"theme/theme.yaml": "theme",
	}
	eachYAMLFile(abs("content/products"), func(p string) { kinds[relOf(root, p)] = "product" })
	eachYAMLFile(abs("content/pages"), func(p string) { kinds[relOf(root, p)] = "page" })
	eachYAMLFile(abs("content/cases"), func(p string) { kinds[relOf(root, p)] = "case" })
	eachYAMLFile(abs("content"), func(p string) {
		switch filepath.Base(p) {
		case "certificates.yaml":
			kinds[relOf(root, p)] = "certificate"
		case "categories.yaml":
			kinds[relOf(root, p)] = "category"
		case "scenarios.yaml":
			kinds[relOf(root, p)] = "scenario"
		}
	})

	files := make([]string, 0, len(kinds))
	for f := range kinds {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, rel := range files {
		scope := kinds[rel]
		b, err := os.ReadFile(abs(rel))
		if err != nil {
			continue
		}
		var node yaml.Node
		if err := yaml.Unmarshal(b, &node); err != nil {
			continue // 解析错误已在上面的检查里报告过
		}
		var changed []string
		walkRenames(node.Content[0], scope, &changed)
		if len(changed) > 0 {
			rep.Findings = append(rep.Findings, Finding{
				Level: "fix", File: rel, Msg: strings.Join(changed, "；")})
			if apply {
				if err := backupAndWrite(root, rel, &node, rep); err != nil {
					return rep, err
				}
			}
		}
	}

	sortFindings(rep)
	return rep, nil
}

// walkRenames 递归遍历映射/序列节点，按 scope 改键名。
// scope 传 "list:certificates" 这类前缀时按列表项实体的规则匹配。
func walkRenames(n *yaml.Node, scope string, changed *[]string) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.SequenceNode:
		itemScope := strings.TrimPrefix(scope, "list:")
		if itemScope == scope { // 不是 list: 前缀，items 当作同 scope
			itemScope = scope
		}
		for _, c := range n.Content {
			walkRenames(c, itemScope, changed)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			// seo 是嵌套结构，单独一个 scope
			childScope := scope
			if key.Value == "seo" && val.Kind == yaml.MappingNode {
				childScope = "seo"
			}
			for _, r := range renames {
				if r.Scope == scope && key.Value == r.Old {
					old := key.Value
					key.Value = r.New
					*changed = append(*changed,
						fmt.Sprintf("%s → %s（%s）", old, r.New, r.Why))
					break
				}
			}
			walkRenames(val, childScope, changed)
		}
	}
}

// structKeys 用反射收集 struct 的合法 yaml 顶层键（含 inline 展开）。
func structKeys(t reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		parts := strings.Split(f.Tag.Get("yaml"), ",")
		tag := parts[0]
		// inline 判断必须在「空 tag 跳过」之前：Base `yaml:",inline"` 的
		// 键名部分是空的，但它的字段全部属于本实体（slug/status/seo…）。
		// 漏了它，公共字段会被误报成「未知字段」。
		inline := false
		for _, p := range parts[1:] {
			if p == "inline" {
				inline = true
			}
		}
		if inline {
			ft := f.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			for k := range structKeys(ft) {
				keys[k] = true
			}
			continue
		}
		if tag == "-" || tag == "" {
			continue
		}
		keys[tag] = true
	}
	return keys
}

func joinKeys(m map[string]bool) string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// mapGet 从映射节点里取值节点；找不到返回 nil。
func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// addImages 在 settings 顶层补 images 段（默认值与 model.Images 的回退一致）。
func addImages(m *yaml.Node) {
	img := &yaml.Node{Kind: yaml.MappingNode}
	pair := func(k, v string) {
		img.Content = append(img.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k},
			&yaml.Node{Kind: yaml.ScalarNode, Value: v})
	}
	pair("transcode", "false")
	pair("quality", "82")
	pair("max_kb", "400")
	widths := &yaml.Node{Kind: yaml.SequenceNode}
	for _, w := range []string{"640", "960", "1280"} {
		widths.Content = append(widths.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: w})
	}
	img.Content = append(img.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "widths"}, widths)

	key := &yaml.Node{Kind: yaml.ScalarNode, Value: "images"}
	// 追加到末尾即可：键顺序无语义，保持现有内容不动是第一原则
	m.Content = append(m.Content, key, img)
}

// addLayout 在 theme 顶层补 layout 键。
func addLayout(m *yaml.Node) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "layout"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: "classic"})
}

// backupAndWrite 把 node 序列化写回 rel；原文件先复制到 .backup/<ts>/。
func backupAndWrite(root, rel string, node *yaml.Node, rep *Report) error {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	orig, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	if rep.Backups == "" {
		rep.Backups = filepath.ToSlash(filepath.Join(".backup", time.Now().Format("20060102-150405")))
	}
	bdir := filepath.Join(root, filepath.FromSlash(rep.Backups))
	bfile := filepath.Join(bdir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(bfile), 0o755); err != nil {
		return err
	}
	// 同一文件可能被多条 fix 触达（比如 settings 的 base_url 和 images 段）：
	// 备份只落第一次，保证 .backup 里永远是最原始的版本。
	if _, err := os.Stat(bfile); err != nil {
		if err := os.WriteFile(bfile, orig, 0o644); err != nil {
			return err
		}
	}

	var buf strings.Builder
	enc := yaml.NewEncoder(stringWriter{&buf})
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := os.WriteFile(abs, []byte(buf.String()), 0o644); err != nil {
		return err
	}
	// Fixed 去重：报告里一个文件只出现一次，无论它被几条 fix 修改
	for _, f := range rep.Fixed {
		if f == rel {
			return nil
		}
	}
	rep.Fixed = append(rep.Fixed, rel)
	return nil
}

func eachYAMLFile(dir string, fn func(path string)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
			fn(filepath.Join(dir, e.Name()))
		}
	}
}

func relOf(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

func sortFindings(rep *Report) {
	order := map[string]int{"error": 0, "fix": 1, "warn": 2, "info": 3}
	sort.SliceStable(rep.Findings, func(i, j int) bool {
		if order[rep.Findings[i].Level] != order[rep.Findings[j].Level] {
			return order[rep.Findings[i].Level] < order[rep.Findings[j].Level]
		}
		return rep.Findings[i].File < rep.Findings[j].File
	})
}

type stringWriter struct{ b *strings.Builder }

func (w stringWriter) Write(p []byte) (int, error) { return w.b.Write(p) }

var _ io.Writer = stringWriter{}
