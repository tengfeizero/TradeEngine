// Package skins 管理「审美皮肤」——同一套 HTML，换一张 CSS 就换一种设计。
//
// 这是 CSS Zen Garden 的模型：内容与区块序列决定页面「说什么」，
// 皮肤决定页面「给人什么感觉」。两者完全解耦，所以同一份内容可以
// 为了行业审美差异反复换皮，而不改一行内容。
//
// 皮肤来源有两处，站点优先：
//
//	<site>/skins/<name>.css   客户或设计自己加的皮肤（不重编译）
//	engine 内置 presets/*.css  预置审美（六个大类）
//
// 皮肤文件用头部注释自我描述：
//
//	/*!
//	 * @skin organic
//	 * @label 有机自然（食品 / 农产品 / 茶）
//	 * @category 食品农业
//	 * @tone light
//	 */
package skins

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed presets/*.css
var presetFS embed.FS

// Default 是缺省皮肤名：没有显式配置时用它，保证永远有皮可穿。
const Default = "classic"

// Skin 是一套审美皮肤。
type Skin struct {
	Name     string // 文件名即皮肤名，构建产物里写进 data-skin
	Label    string // 给人看的中文名
	Category string // 适用行业，用于 engine skins 分组
	Tone     string // light / dark
	CSS      string
	Preset   bool // true = 引擎内置；false = 站点 skins/ 目录
}

// Dark 表示暗底皮肤。渲染层据此决定在主题没给 paper/ink 时兜底成深色基线。
func (s Skin) Dark() bool { return s.Tone == "dark" }

// meta 从 CSS 头部注释里读 @skin / @label / @category / @tone。
// 皮肤文件把自己描述清楚，就不需要额外的清单文件来同步——
// 加一套皮肤只要丢一个 .css 进目录，不用改 Go 代码。
func meta(css string) (name, label, category, tone string) {
	head := css
	if i := strings.Index(head, "*/"); i >= 0 {
		head = head[:i]
	}
	for _, line := range strings.Split(head, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "*!/*"))
		if !strings.HasPrefix(line, "@") {
			continue
		}
		kv := strings.SplitN(line[1:], " ", 2)
		if len(kv) != 2 {
			continue
		}
		val := strings.TrimSpace(kv[1])
		switch kv[0] {
		case "skin":
			name = val
		case "label":
			label = val
		case "category":
			category = val
		case "tone":
			tone = val
		}
	}
	if tone != "dark" {
		tone = "light"
	}
	return name, label, category, tone
}

func preset(name string) (Skin, bool) {
	if !ValidName(name) {
		return Skin{}, false
	}
	raw, err := presetFS.ReadFile("presets/" + name + ".css")
	if err != nil {
		return Skin{}, false
	}
	css := string(raw)
	_, label, category, tone := meta(css)
	if label == "" {
		label = name
	}
	if category == "" {
		category = "未分类"
	}
	return Skin{Name: name, Label: label, Category: category, Tone: tone, CSS: css, Preset: true}, true
}

// Presets 返回全部内置皮肤，按行业类别、再按名字排序，便于 CLI 展示。
func Presets() []Skin {
	entries, err := presetFS.ReadDir("presets")
	if err != nil {
		return nil
	}
	out := make([]Skin, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".css")
		if s, ok := preset(name); ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Names 返回全部内置皮肤名（有序）。
func Names() []string {
	ps := Presets()
	out := make([]string, 0, len(ps))
	for _, s := range ps {
		out = append(out, s.Name)
	}
	return out
}

// ValidName 挡住路径穿越：皮肤名只允许小写字母、数字、连字符与下划线。
// 站点皮肤是客户可写的目录，不做这一步就可能被 ../ 读到任意文件。
func ValidName(name string) bool {
	if name == "" || len(name) > 48 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// Resolve 决定这个站点该穿哪套皮。
//
//	root 非空时先看 <root>/skins/<name>.css —— 客户加的皮肤不用重编译引擎
//	再看内置预置
//	都没有则报错，并给出最接近的候选名（拼错时不要让用户去猜）
func Resolve(root, name string) (Skin, error) {
	if name == "" {
		name = Default
	}
	if !ValidName(name) {
		return Skin{}, fmt.Errorf("皮肤名只允许小写字母、数字、连字符与下划线，当前为 %q", name)
	}
	if root != "" {
		p := filepath.Join(root, "skins", name+".css")
		if raw, err := os.ReadFile(p); err == nil {
			css := string(raw)
			_, label, category, tone := meta(css)
			if label == "" {
				label = name
			}
			if category == "" {
				category = "站点自定义"
			}
			return Skin{Name: name, Label: label, Category: category, Tone: tone, CSS: css}, nil
		}
	}
	if s, ok := preset(name); ok {
		return s, nil
	}
	all := append(Names(), OverrideNames(root)...)
	msg := "皮肤 " + name + " 不存在，可用：" + strings.Join(all, " / ")
	if near := nearest(name, all); near != "" {
		msg += "（是想写 " + near + " 吗？）"
	}
	return Skin{}, fmt.Errorf("%s", msg)
}

// OverrideNames 列出站点 skins/ 目录里自定义的皮肤名。
func OverrideNames(root string) []string {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "skins"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".css") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".css")
		if ValidName(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// All 返回「内置 + 站点自定义」，同名时站点版本覆盖内置。
func All(root string) []Skin {
	out := Presets()
	idx := map[string]int{}
	for i, s := range out {
		idx[s.Name] = i
	}
	for _, name := range OverrideNames(root) {
		s, err := Resolve(root, name)
		if err != nil {
			continue
		}
		if i, ok := idx[name]; ok {
			out[i] = s
		} else {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// nearest 找一个编辑距离最近的候选名，只用于拼错时的提示。
func nearest(name string, candidates []string) string {
	best, bestD := "", 1<<30
	for _, c := range candidates {
		if d := distance(name, c); d < bestD {
			best, bestD = c, d
		}
	}
	// 差太多就别乱猜，免得提示反而误导
	if bestD > len(name)/2+1 {
		return ""
	}
	return best
}

func distance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
