// Package media 是构建期图片管线。
//
// 它解决三件事，顺序就是危害程度：
//
//  1. **防 CLS**：没有 width/height 的图片会在加载时把正文顶下去，
//     移动端 Core Web Vitals 直接判负。这里读真实像素尺寸写进 img 标签。
//  2. **首屏空窗**：取图平均色作为占位背景，慢网下不是一块白。
//  3. **体积**：发现同名 .webp / .avif 兄弟文件或多宽度变体就输出 srcset；
//     配置开启时调用本地 cwebp 生成 WebP 派生图。
//
// 刻意**不引入图片编解码依赖**：整个引擎保持单二进制、无 cgo。
// 转码交给可选的 cwebp，缺失时退化为「只用原图 + 警告」，绝不让构建失败。
package media

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"  // 注册解码器
	_ "image/jpeg" // 注册解码器
	_ "image/png"  // 注册解码器
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Image 是一张图片的元数据。
type Image struct {
	Path     string // 相对站点根目录，如 media/products/sencha-cover.png
	Width    int
	Height   int
	Format   string // png / jpeg / gif / svg / webp / avif
	Color    string // 平均色 #rrggbb，用作 LQIP 占位
	Bytes    int64
	IsVector bool
}

// Candidate 是一个候选图源。
type Candidate struct {
	Path  string
	Width int
}

// VariantSet 是某张原图可用的全部图源。
type VariantSet struct {
	Base       *Image
	WebP       []Candidate
	AVIF       []Candidate
	SameFormat []Candidate // 同格式多宽度变体（手工或前期工具生成）
}

// Library 缓存一次构建内的图片元数据，避免同一张图被解多次。
type Library struct {
	root     string
	images   map[string]*Image
	warnings []string
	cwebp    string
	cwebpOK  bool
	checked  bool
}

// NewLibrary 创建图片库。
func NewLibrary(root string) *Library {
	return &Library{root: root, images: map[string]*Image{}}
}

// Warnings 返回图片管线发现的问题（缺失、转码失败、体积超标等）。
func (l *Library) Warnings() []string { return l.warnings }

func (l *Library) warn(format string, args ...any) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, args...))
}

// Get 读取图片元数据。路径为空或文件不存在时返回 nil——
// 调用方据此退化为「不输出图片」，而不是渲染一个坏链接。
func (l *Library) Get(rel string) *Image {
	rel = strings.TrimSpace(rel)
	if rel == "" || strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		return nil
	}
	rel = strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(rel)), "/")
	if rel == "" || strings.HasPrefix(rel, "..") {
		return nil
	}
	if img, ok := l.images[rel]; ok {
		return img
	}
	full := filepath.Join(l.root, filepath.FromSlash(rel))
	st, err := os.Stat(full)
	if err != nil || st.IsDir() {
		l.images[rel] = nil
		return nil
	}

	img := &Image{Path: rel, Bytes: st.Size()}
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".svg":
		img.Format, img.IsVector = "svg", true
		img.Width, img.Height = svgSize(full)
	case ".webp":
		img.Format = "webp"
	case ".avif":
		img.Format = "avif"
	default:
		f, err := os.Open(full)
		if err != nil {
			l.images[rel] = nil
			return nil
		}
		defer f.Close()
		cfg, format, err := image.DecodeConfig(f)
		if err != nil {
			l.warn("无法读取图片尺寸（可能不是有效图片）：%s", rel)
			l.images[rel] = nil
			return nil
		}
		img.Format = format
		img.Width, img.Height = cfg.Width, cfg.Height
		// 主色只在「要当封面用」的图上算，且限制像素总量：
		// 一张 6000×4000 的图解码出来 96MB，构建机不该为占位色付出这个代价。
		if int64(cfg.Width)*int64(cfg.Height) <= 16_000_000 {
			if _, err := f.Seek(0, 0); err == nil {
				if decoded, _, err := image.Decode(f); err == nil {
					img.Color = averageColor(decoded)
				}
			}
		}
	}
	l.images[rel] = img
	return img
}

// Exists 判断站内文件是否存在（不限图片）。
//
// PDF 这类「可下载附件」不走解码路径：用 Get 会把它们当坏图报警，
// 而一条 PDF 报「无法读取图片尺寸」的警告会把真正该看的警告淹掉。
func (l *Library) Exists(rel string) bool {
	rel = strings.TrimSpace(rel)
	if rel == "" || strings.HasPrefix(rel, "http://") || strings.HasPrefix(rel, "https://") {
		return false
	}
	rel = strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(rel)), "/")
	if rel == "" || strings.HasPrefix(rel, "..") {
		return false
	}
	st, err := os.Stat(filepath.Join(l.root, filepath.FromSlash(rel)))
	return err == nil && !st.IsDir()
}

// Variants 收集一张图可用的全部图源。
func (l *Library) Variants(rel string) VariantSet {
	base := l.Get(rel)
	if base == nil {
		return VariantSet{}
	}
	vs := VariantSet{Base: base}
	dir := path.Dir(rel)
	name := strings.TrimSuffix(path.Base(rel), path.Ext(rel))

	entries, err := os.ReadDir(filepath.Join(l.root, filepath.FromSlash(dir)))
	if err != nil {
		return vs
	}
	// 命名约定：<name>-<width>w.<ext>，例如 sencha-cover-640w.webp
	wRe := regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(\d+)w$`)
	for _, e := range entries {
		baseName := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		m := wRe.FindStringSubmatch(baseName)
		if m == nil {
			continue
		}
		w, _ := strconv.Atoi(m[1])
		if w <= 0 {
			continue
		}
		cand := Candidate{Path: path.Join(dir, e.Name()), Width: w}
		switch strings.ToLower(path.Ext(e.Name())) {
		case ".webp":
			vs.WebP = append(vs.WebP, cand)
		case ".avif":
			vs.AVIF = append(vs.AVIF, cand)
		default:
			vs.SameFormat = append(vs.SameFormat, cand)
		}
	}
	// 同目录下的同名现代格式（常见于设计师另存一份）。
	// 候选宽度沿用 base——base 尺寸未知（如 webp 无解码器可读）时宁可不进
	// srcset，也不能产出 `0w` 候选（见 render.srcset 的防御）。
	if base.Width > 0 {
		if l.Get(path.Join(dir, name+".webp")) != nil {
			if len(vs.WebP) == 0 {
				vs.WebP = append(vs.WebP, Candidate{Path: path.Join(dir, name+".webp"), Width: base.Width})
			}
		}
		if l.Get(path.Join(dir, name+".avif")) != nil {
			if len(vs.AVIF) == 0 {
				vs.AVIF = append(vs.AVIF, Candidate{Path: path.Join(dir, name+".avif"), Width: base.Width})
			}
		}
	}
	// 排序保证 srcset 顺序稳定：构建产物必须可复现，否则每次提交都在 diff
	sortCands(vs.WebP)
	sortCands(vs.AVIF)
	sortCands(vs.SameFormat)
	return vs
}

func sortCands(c []Candidate) {
	sort.Slice(c, func(i, j int) bool { return c[i].Width < c[j].Width })
}

// cwebpPath 探测本地 cwebp。结果缓存：一次构建只找一次。
func (l *Library) cwebpPath() string {
	if l.checked {
		return l.cwebp
	}
	l.checked = true
	if p, err := exec.LookPath("cwebp"); err == nil {
		l.cwebp, l.cwebpOK = p, true
	}
	return l.cwebp
}

// Transcode 按需生成 WebP 派生图，返回可用候选。
//
// 派生文件放在 media/.derived/ 下，文件名带内容指纹：
// 原图一改指纹就变，CDN 不会把旧图缓存给新价格。
func (l *Library) Transcode(rel string, widths []int, quality int) []Candidate {
	base := l.Get(rel)
	if base == nil || base.IsVector || base.Format == "webp" || base.Format == "avif" {
		return nil
	}
	bin := l.cwebpPath()
	if bin == "" {
		l.warn("已开启图片转码但未找到 cwebp，跳过派生（安装 webp 工具或关闭 images.transcode）")
		return nil
	}
	full := filepath.Join(l.root, filepath.FromSlash(rel))
	st, err := os.Stat(full)
	if err != nil {
		return nil
	}

	var out []Candidate
	for _, w := range widths {
		if w > 0 && base.Width > 0 && w > base.Width {
			continue // 不放大：放大只增加体积，不增加信息
		}
		sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d", rel, st.ModTime().Unix(), w, quality)))
		name := hex.EncodeToString(sum[:6]) + "-" + strconv.Itoa(w) + "w.webp"
		relOut := path.Join("media", ".derived", name)
		fullOut := filepath.Join(l.root, filepath.FromSlash(relOut))

		if err := os.MkdirAll(filepath.Dir(fullOut), 0o755); err != nil {
			l.warn("创建派生目录失败：%v", err)
			return out
		}
		if _, err := os.Stat(fullOut); err != nil {
			args := []string{"-quiet", "-q", strconv.Itoa(quality), "-resize", strconv.Itoa(w), "0", full, "-o", fullOut}
			if err := exec.Command(bin, args...).Run(); err != nil {
				l.warn("cwebp 转码失败 %s @%dw：%v", rel, w, err)
				continue
			}
		}
		out = append(out, Candidate{Path: relOut, Width: w})
	}
	return out
}

// svgSize 从 SVG 头部的 width/height 或 viewBox 推出像素尺寸。
// 拿不到时返回 0，渲染层会退化成不带宽高的 img（少数情况可接受）。
func svgSize(full string) (int, int) {
	b, err := os.ReadFile(full)
	if err != nil {
		return 0, 0
	}
	head := string(b)
	if len(head) > 4096 {
		head = head[:4096]
	}
	attr := func(key string) int {
		re := regexp.MustCompile(`(?i)\b` + key + `\s*=\s*["']\s*([0-9.]+)`)
		m := re.FindStringSubmatch(head)
		if m == nil {
			return 0
		}
		v, _ := strconv.ParseFloat(m[1], 64)
		return int(v)
	}
	if w, h := attr("width"), attr("height"); w > 0 && h > 0 {
		return w, h
	}
	re := regexp.MustCompile(`(?i)viewBox\s*=\s*["']\s*[0-9.+-]+\s+[0-9.+-]+\s+([0-9.]+)\s+([0-9.]+)`)
	if m := re.FindStringSubmatch(head); m != nil {
		w, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		return int(w), int(h)
	}
	return 0, 0
}

// averageColor 采样求平均色。步长按面积自适应：大图不逐像素扫。
func averageColor(img image.Image) string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return ""
	}
	step := 1
	if total := w * h; total > 40_000 {
		step = total / 40_000
		if step < 1 {
			step = 1
		}
	}
	var r, g, bl, n uint64
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			r += uint64(cr >> 8)
			g += uint64(cg >> 8)
			bl += uint64(cb >> 8)
			n++
		}
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r/n, g/n, bl/n)
}
