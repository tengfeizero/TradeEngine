package render

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tengfeizero/TradeEngine/internal/model"
)

// m5Fixture 搭一个带临时媒体目录的站点：品牌分享卡（M5-3）需要真实文件存在，
// 其余能力（GSC / 汉堡 / FAQ / 加粗）用同一份站点即可覆盖。
func m5Fixture(t *testing.T) (*model.Site, string) {
	t.Helper()
	root := t.TempDir()
	mediaDir := filepath.Join(root, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 一张 4×3 的纯色 JPEG 当品牌卡——Get 只按扩展名走解码路径，内容无所谓
	f, err := os.Create(filepath.Join(mediaDir, "brand-card.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	f.Close()

	site := &model.Site{
		Root: root,
		Settings: model.Settings{
			Brand:       model.Brand{Name: "Fixture"},
			BaseURL:     "https://fixture.example.com",
			DefaultLang: "en",
			Langs:       []string{"en"},
			Analytics:   model.Analytics{GA4ID: "G-TEST", GSCVerify: "verify-token-123"},
		},
		Pages: []model.Page{{
			Base:  model.Base{Slug: "home", Status: "published"},
			Title: model.I18n{"en": "Home"},
			Blocks: []model.Block{
				{Type: "richtext", Extra: map[string]any{
					"title": model.I18n{"en": "Story"},
					"body":  model.I18n{"en": "Plain text with **bold part** inside."},
				}},
				{Type: "faq", Extra: map[string]any{
					"title": model.I18n{"en": "FAQ"},
					"items": []any{
						map[string]any{"q": model.I18n{"en": "Q one?"}, "a": model.I18n{"en": "A one."}},
					},
				}},
			},
		}},
	}
	return site, root
}

// M5-3：没有 cover 的页面用品牌卡兜底 og:image，并输出 twitter:card；
// M5-4 收尾：GSC verify meta；M5-6：汉堡导航结构；M5-5：FAQPage。
func TestShareCardGSCNavAndFAQ(t *testing.T) {
	site, _ := m5Fixture(t)
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := r.Page(&site.Pages[0], "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, want := range []string{
		`<meta property="og:image" content="https://fixture.example.com/media/brand-card.jpg">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="google-site-verification" content="verify-token-123">`,
		`class="nav-toggle"`, `class="nav-burger"`, // 汉堡结构与折叠开关
		`"FAQPage"`, `"Question"`, `"acceptedAnswer"`,
		`<strong>bold part</strong>`, // M5-7：**x** 转加粗，且不再有字面星号
	} {
		if !strings.Contains(html, want) {
			t.Errorf("页面缺少 %q", want)
		}
	}
	if strings.Contains(html, "**") {
		t.Errorf("加粗语法残留为字面星号")
	}
	if strings.Contains(html, `<p>Plain`) == false && !strings.Contains(html, "Plain text") {
		t.Errorf("富文本正文丢失")
	}
	// Organization 首页 JSON-LD：logo 与 sameAs（M5-5）
	if !strings.Contains(html, `"@type":"Organization"`) {
		t.Errorf("首页缺 Organization JSON-LD")
	}
}

// M5-5：带面包屑的页面（404 也算）输出 BreadcrumbList，且视觉与结构化同源。
func TestBreadcrumbJSONLD(t *testing.T) {
	site, _ := m5Fixture(t)
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := r.NotFound("en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if !strings.Contains(html, `"@type":"BreadcrumbList"`) {
		t.Errorf("404 页缺 BreadcrumbList")
	}
	if !strings.Contains(html, `"item":"https://fixture.example.com/en/"`) {
		t.Errorf("面包屑 item 不是绝对地址")
	}
}
