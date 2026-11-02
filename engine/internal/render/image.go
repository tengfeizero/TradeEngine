package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tradeengine/tradeengine/internal/media"
)

// ImageView 是模板里的图片模型。
//
// 它存在的理由是「任何一张图都必须带宽高」：模板里直接写 <img src> 是 CLS 的
// 头号来源，而这件事靠人工纪律守不住——只能在渲染层强制。
type ImageView struct {
	Src      string
	Width    int
	Height   int
	Alt      string
	Color    string
	Class    string
	Loading  string
	Priority bool

	WebPSrcset  string
	AVIFSrcset  string
	PlainSrcset string
	Sizes       string
}

// HasSrcset 供模板判断是否需要 <picture> 包裹。
func (v ImageView) HasSrcset() bool {
	return v.WebPSrcset != "" || v.AVIFSrcset != "" || v.PlainSrcset != ""
}

func srcset(cands []media.Candidate) string {
	if len(cands) == 0 {
		return ""
	}
	sorted := append([]media.Candidate(nil), cands...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Width < sorted[j].Width })
	parts := make([]string, 0, len(sorted))
	for _, c := range sorted {
		if c.Width <= 0 {
			// 宽度未知的候选不能进 srcset：`url 0w` 会让浏览器判定全部候选
			// 无效，整张图不加载——页面看起来像「图丢了」却零报错。
			continue
		}
		parts = append(parts, fmt.Sprintf("/%s %dw", strings.TrimPrefix(c.Path, "/"), c.Width))
	}
	return strings.Join(parts, ", ")
}

// image 构造图片视图。rel 为空或文件缺失时返回 nil，模板据此整段不渲染。
//
// loading 传 eager 表示首屏图（LCP 元素），其余都应该懒加载：
// 一个 20 个产品的列表页若全部 eager，首屏要抢 20 张图的带宽。
func (r *Renderer) image(rel, alt, class, sizes, loading string) *ImageView {
	vs := r.media.Variants(rel)
	if vs.Base == nil {
		return nil
	}
	v := &ImageView{
		Src:     "/" + strings.TrimPrefix(vs.Base.Path, "/"),
		Width:   vs.Base.Width,
		Height:  vs.Base.Height,
		Alt:     alt,
		Color:   vs.Base.Color,
		Class:   class,
		Loading: loading,
		Sizes:   sizes,
	}
	if v.Loading == "" {
		v.Loading = "lazy"
	}
	if v.Loading == "eager" {
		v.Priority = true
	}

	// 配置开启时补一组 WebP 派生图；cwebp 缺失会退化为空，不影响出图。
	if cfg := r.site.Settings.Images; cfg.Transcode && !vs.Base.IsVector {
		if derived := r.media.Transcode(vs.Base.Path, cfg.WidthSet(), cfg.QualityValue()); len(derived) > 0 {
			vs.WebP = append(vs.WebP, derived...)
		}
	}

	v.WebPSrcset = srcset(vs.WebP)
	v.AVIFSrcset = srcset(vs.AVIF)
	// 有现代格式时，原图 srcset 只在没有 WebP/AVIF 时才用，避免浏览器选错格式回退
	if v.WebPSrcset == "" && v.AVIFSrcset == "" {
		v.PlainSrcset = srcset(vs.SameFormat)
	}
	return v
}

// cardImage 是列表卡片的常用尺寸约定（4:3 封面）。
func (r *Renderer) cardImage(rel, alt string) *ImageView {
	return r.image(rel, alt, "", "(max-width: 720px) 100vw, 320px", "lazy")
}

// heroImage 是首屏大图：eager + 高优先级，且不限 sizes（铺满视口）。
func (r *Renderer) heroImage(rel, alt string) *ImageView {
	return r.image(rel, alt, "hero-img", "100vw", "eager")
}

// Gallery 返回一组图片视图，跳过缺失文件。
func (r *Renderer) gallery(rels []string, alt string) []*ImageView {
	out := make([]*ImageView, 0, len(rels))
	for i, rel := range rels {
		if v := r.image(rel, fmt.Sprintf("%s %d", alt, i+1), "", "(max-width: 720px) 100vw, 480px", "lazy"); v != nil {
			out = append(out, v)
		}
	}
	return out
}
