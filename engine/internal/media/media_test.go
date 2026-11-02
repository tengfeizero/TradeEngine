package media

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, dir, name string, w, h int, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestGetReadsDimensionsAndColor(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "solid.png", 8, 6, color.RGBA{R: 200, G: 40, B: 40, A: 255})

	lib := NewLibrary(dir)
	img := lib.Get("solid.png")
	if img == nil {
		t.Fatal("应读到图片")
	}
	if img.Width != 8 || img.Height != 6 {
		t.Errorf("尺寸错误：%dx%d", img.Width, img.Height)
	}
	if img.Format != "png" {
		t.Errorf("格式错误：%s", img.Format)
	}
	if img.Color == "" {
		t.Error("应算出平均色，用于 LQIP 占位")
	}
	if len(lib.Warnings()) != 0 {
		t.Errorf("不应有警告：%v", lib.Warnings())
	}
}

func TestGetMissingReturnsNil(t *testing.T) {
	lib := NewLibrary(t.TempDir())
	if lib.Get("nope.png") != nil {
		t.Error("缺失文件应返回 nil，让模板整段不渲染")
	}
	if lib.Get("https://cdn.example.com/a.png") != nil {
		t.Error("外链不应被当成站内媒体")
	}
	if lib.Get("../../etc/passwd") != nil {
		t.Error("路径穿越必须被拒绝")
	}
}

func TestVariantsFindsModernFormats(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "pic.png", 100, 75, color.White)
	for _, name := range []string{"pic-640w.webp", "pic-1280w.webp", "pic-640w.avif", "pic-640w.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	vs := NewLibrary(dir).Variants("pic.png")
	if vs.Base == nil {
		t.Fatal("缺少原图")
	}
	if len(vs.WebP) != 2 {
		t.Errorf("应发现 2 个 WebP 变体，实际 %d", len(vs.WebP))
	}
	if len(vs.AVIF) != 1 {
		t.Errorf("应发现 1 个 AVIF 变体，实际 %d", len(vs.AVIF))
	}
	if len(vs.SameFormat) != 1 {
		t.Errorf("应发现 1 个同格式变体，实际 %d", len(vs.SameFormat))
	}
	if vs.WebP[0].Width != 640 {
		t.Errorf("宽度解析错误：%d", vs.WebP[0].Width)
	}
}

func TestSVGSizeFromViewBox(t *testing.T) {
	dir := t.TempDir()
	body := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 60"><rect width="240" height="60"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "logo.svg"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	img := NewLibrary(dir).Get("logo.svg")
	if img == nil || !img.IsVector {
		t.Fatal("应识别为矢量图")
	}
	if img.Width != 240 || img.Height != 60 {
		t.Errorf("viewBox 尺寸解析错误：%dx%d", img.Width, img.Height)
	}
}
