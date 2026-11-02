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

// 这一组测试覆盖「让引擎追平 B3」新增的六项能力：
//   C10 参数导出 / SKU 清单；D6 工厂证据；D7 客户证言；
//   E7 llms.txt；R2 市场准入（GPSR / Impressum / EPR）。
//
// 测试的写法刻意偏「产物断言」而不是「函数返回值断言」：这些能力的价值
// 只有在最终 HTML / 字节里才成立（链接是不是死的、转义有没有把 tel: 打成
// #ZgotmplZ、CSV 表头会不会被 Excel 认错），单测只查结构等于没查。

func v2Fixture(t *testing.T, withCover bool) (*model.Site, string) {
	t.Helper()
	root := t.TempDir()
	mediaDir := filepath.Join(root, "media")
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if withCover {
		f, err := os.Create(filepath.Join(mediaDir, "cover.jpg"))
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 4, 3))
		img.Set(0, 0, color.RGBA{G: 255, A: 255})
		if err := jpeg.Encode(f, img, nil); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	moq := 50.0
	site := &model.Site{
		Root: root,
		Settings: model.Settings{
			Brand:       model.Brand{Name: "Fixture", Tagline: model.I18n{"en": "Trail hardware"}},
			BaseURL:     "https://fixture.example.com",
			DefaultLang: "en",
			Langs:       []string{"en"},
			Company: model.Company{
				LegalName:   "Fixture Industries Co., Ltd.",
				FoundedYear: 2011,
				Address: model.Address{
					Street:  "12 Factory Road",
					City:    "Ningbo",
					Zip:     "315000",
					Country: "China",
				},
			},
			Contact: model.Contact{Email: "sales@fixture.example.com", Phone: "+86 574 8888 0000"},
			Markets: []model.Market{
				{Code: "DE", Name: model.I18n{"en": "Germany"}},
				{Code: "FR", Name: model.I18n{"en": "France"}},
			},
			Compliance: model.Compliance{
				EUResponsible: model.Party{
					Name:    "Fixture Europe B.V.",
					Address: "Keizersgracht 1, 1015 Amsterdam, NL",
					Email:   "eu@fixture.example.com",
					Phone:   "+31 20 123 4567",
				},
				LegalNotice: model.LegalNotice{
					RegisterNo:       "KvK 12345678",
					RegisterCourt:    "Amsterdam",
					VATID:            "NL123456789B01",
					ManagingDirector: "A. Tester",
					Email:            "legal@fixture.example.com",
				},
				EPR: model.EPR{DEPackaging: "LUCID DE9999999999999"},
			},
		},
		Categories: []model.Category{
			{Base: model.Base{Slug: "mountain", Status: "published"}, Name: model.I18n{"en": "Mountain"}},
		},
		Products: []model.Product{
			{
				Base:    model.Base{Slug: "m720-trail", Status: "published"},
				Title:   model.I18n{"en": "M-720 Trail"},
				Category: "mountain",
				ModelNo: "FX-M720",
				Summary: model.I18n{"en": "Hardtail frame."},
				Description: model.I18n{"en": "Built for trails."},
				LeadTime:   model.I18n{"en": "35 days"},
				Packaging:  model.I18n{"en": "Carton"},
				HSCode:     "8714910000",
				Moq:        moq,
				MoqUnit:    "pcs",
				Cover:      coverPath(withCover),
				Specs: []model.SpecGroup{{
					Group: model.I18n{"en": "Frame"},
					Items: []model.SpecItem{
						{Label: model.I18n{"en": "Material"}, Value: model.I18n{"en": "6061 alloy"}},
						{Label: model.I18n{"en": "Weight"}, ValueNum: func() *float64 { v := 2.35; return &v }(), Unit: "kg"},
					},
				}},
			},
			{
				// 草稿不该出现在任何导出里——SKU 清单是给买家的，不是内部台账。
				Base:  model.Base{Slug: "draft-one", Status: "draft"},
				Title: model.I18n{"en": "Draft product"},
			},
		},
		Pages: []model.Page{
			{
				Base:  model.Base{Slug: "home", Status: "published"},
				Title: model.I18n{"en": "Home"},
			},
			{
				Base:  model.Base{Slug: "about", Status: "published"},
				Title: model.I18n{"en": "About"},
				Blocks: []model.Block{
					{Type: "factory", Extra: map[string]any{
						"title": model.I18n{"en": "Inside the factory"},
						"equipment": []any{
							map[string]any{"name": model.I18n{"en": "CNC tube cutting line"}, "qty": "4"},
						},
						"audits": []any{
							map[string]any{
								"name":   model.I18n{"en": "BSCI social audit"},
								"number": "BSCI-26-88421",
								"issuer": "TÜV Rheinland",
								"date":   "2026-03-11",
							},
						},
						"retention": model.I18n{"en": "Every lot keeps two reference frames for five years."},
					}},
					{Type: "testimonials", Extra: map[string]any{
						"title": model.I18n{"en": "What buyers say"},
						"items": []any{
							map[string]any{
								"quote":   model.I18n{"en": "Third season on the same frames, no failures."},
								"author":  "Jonas Berg",
								"role":    model.I18n{"en": "Head of Product"},
								"company": "Nordvelo AB",
								"country": "SE",
								"date":    "2026-05",
								"product": "m720-trail",
							},
						},
					}},
					{Type: "legal_facts", Extra: map[string]any{
						"title": model.I18n{"en": "Company details"},
					}},
				},
			},
			{
				Base:  model.Base{Slug: "legal-notice", Status: "published"},
				Title: model.I18n{"en": "Legal notice"},
			},
			{
				Base:  model.Base{Slug: "business-terms", Status: "published"},
				Title: model.I18n{"en": "Business terms"},
			},
		},
	}
	return site, root
}

func coverPath(with bool) string {
	if !with {
		return ""
	}
	return "media/cover.jpg"
}

// C10：参数 CSV 的列结构必须稳定，且带可读的元数据头。
func TestSpecsCSV(t *testing.T) {
	site, _ := v2Fixture(t, false)
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	p := &site.Products[0]
	out := string(r.SpecsCSV(p, "en"))
	for _, want := range []string{
		"# Product,M-720 Trail",
		"# Slug,m720-trail",
		"# Model,FX-M720",
		"# HS Code,8714910000",
		"Group,Label,Value",
		"Frame,Material,6061 alloy",
		"Frame,Weight,2.35 kg", // ValueNum 走数字格式化 + 单位，不走 i18n
	} {
		if !strings.Contains(out, want) {
			t.Errorf("参数 CSV 缺少 %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "\r") {
		t.Errorf("CSV 用了 CRLF——csv.Writer 应为 LF")
	}
}

// C10：SKU 清单一行一个已发布 SKU，草稿不得混入。
func TestSKUListCSV(t *testing.T) {
	site, _ := v2Fixture(t, false)
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(r.SKUList("en"))), "\n")
	if len(lines) != 2 {
		t.Fatalf("SKU 清单应为表头 + 1 行，实际 %d 行：%v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "slug,model_no,title,category,moq,moq_unit,lead_time,hs_code,url") {
		t.Errorf("表头不稳定：%s", lines[0])
	}
	for _, want := range []string{"m720-trail", "FX-M720", "M-720 Trail", "8714910000", "/en/products/mountain/m720-trail/"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("SKU 行缺少 %q：%s", want, lines[1])
		}
	}
	if strings.Contains(strings.Join(lines, "\n"), "Draft product") {
		t.Errorf("草稿产品混进了 SKU 清单")
	}
}

// E7：llms.txt 只写有出处的事实（NAP、产品贸易四件套、页面清单）。
func TestLLMsTxt(t *testing.T) {
	site, _ := v2Fixture(t, false)
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	out := string(r.LLMs())
	for _, want := range []string{
		"# Fixture",
		"> Trail hardware",
		"- Legal name: Fixture Industries Co., Ltd.",
		"- Founded: 2011",
		"- Email: sales@fixture.example.com",
		"- Markets served: Germany (DE), France (FR)",
		"## Products",
		"model FX-M720",
		"MOQ 50 pcs",
		"HS 8714910000",
		"https://fixture.example.com/en/products/mountain/m720-trail/",
		"## Pages",
		"https://fixture.example.com/en/about/",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("llms.txt 缺少 %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "Draft product") {
		t.Errorf("llms.txt 混入了草稿产品")
	}
}

// D6 / D7 / R2：三个新区块的产物断言。
func TestEvidenceAndComplianceBlocks(t *testing.T) {
	site, _ := v2Fixture(t, false)
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := r.Page(&site.Pages[1], "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, want := range []string{
		// D6 工厂：设备、审核编号、留样承诺
		"Inside the factory",
		"CNC tube cutting line",
		"BSCI-26-88421",
		"TÜV Rheinland",
		"Every lot keeps two reference frames for five years.",
		// D7 证言：署名 + 公司 + 国家 + 回流产品链接
		"What buyers say",
		"Jonas Berg",
		"Nordvelo AB",
		"Third season on the same frames, no failures.",
		"/en/products/mountain/m720-trail/",
		// R2 法定披露：注册号、VAT、法人、EPR
		"Company details",
		"KvK 12345678",
		"NL123456789B01",
		"A. Tester",
		"LUCID DE9999999999999",
		"Fixture Europe B.V.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("about 页缺少 %q", want)
		}
	}
	// tel: / mailto: 必须原样落地，不能被 html/template 打成 #ZgotmplZ
	if strings.Contains(html, "ZgotmplZ") {
		t.Errorf("出现 #ZgotmplZ：URL 未走 template.URL")
	}
	if !strings.Contains(html, `href="mailto:eu@fixture.example.com"`) {
		t.Errorf("欧盟责任方邮箱未转为 mailto 链接")
	}
}

// R2：产品页只披露 GPSR 要求的两方，且未配置时整块不出现。
func TestProductGPSRFacts(t *testing.T) {
	site, _ := v2Fixture(t, false)
	r, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	p := &site.Products[0]
	out, _, err := r.Product(p, "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, want := range []string{
		"Manufacturer", "EU responsible person",
		"Fixture Europe B.V.",
		"specs.csv", // C10 下载入口跟着产品页走
	} {
		if !strings.Contains(html, want) {
			t.Errorf("产品页缺少 %q", want)
		}
	}
	// 注册号属于公司登记信息，不该出现在产品页
	if strings.Contains(html, "KvK 12345678") {
		t.Errorf("产品页混入了公司登记信息（应只在 legal-notice）")
	}

	site.Settings.Compliance = model.Compliance{}
	r2, err := New(site, "")
	if err != nil {
		t.Fatal(err)
	}
	out2, _, err := r2.Product(p, "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out2), "EU responsible person") {
		t.Errorf("未配置合规信息时仍渲染了 GPSR 声明")
	}
}

// C10 + R2：页脚的数据出口与责任方行——图包链接必须跟着真实文件走。
func TestFooterDataLinks(t *testing.T) {
	site, _ := v2Fixture(t, true)
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
		`href="/data/sku-list.csv"`,
		`href="/data/product-images.zip"`,
		`EU responsible person: Fixture Europe B.V.`,
		`href="/en/legal-notice/"`,
		`href="/en/business-terms/"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("页脚缺少 %q", want)
		}
	}

	// 没有封面文件时不能给图包链接——死链比没链接更伤（E5）。
	bare, _ := v2Fixture(t, false)
	rb, err := New(bare, "")
	if err != nil {
		t.Fatal(err)
	}
	outB, _, err := rb.Page(&bare.Pages[0], "en")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(outB), "/data/product-images.zip") {
		t.Errorf("无封面时仍输出了图包链接")
	}
}
