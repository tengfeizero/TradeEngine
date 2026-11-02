package contentdb

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/loader"
	"github.com/tengfeizero/TradeEngine/internal/model"
)

// newFixture 造一个覆盖全部实体类型的最小站点。
// stats 区块里故意放数字字段：payload 用 YAML 往返，就是要验证
// map[string]any 里的 int 不会像 JSON 那样退化成 float64。
func newFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("engine.yaml", "engine_version: 1\noutput: .build/public\n")
	write("settings.yaml", `base_url: https://example.test
schema_version: 1
default_lang: en
langs: [en, zh]
theme: t

brand:
  name: Test Brand
  tagline: { en: Test tagline, zh: 测试标语 }

company:
  legal_name: Test Co., Ltd.
  address: { country: CN, region: Zhejiang, city: Yiwu }

contact:
  email: sales@example.test

markets:
  - code: EU
    name: { en: Europe, zh: 欧洲 }

nav:
  - label: { en: Home, zh: 首页 }
    href: /{lang}/
  - label: { en: Products, zh: 产品 }
    href: /{lang}/products/
`)
	write("theme/theme.yaml", `name: t
layout: editorial
colors:
  primary: "#4A6B4A"
  accent: "#755A21"
`)
	write("content/pages/home.yaml", `slug: home
status: published
title: { en: Home, zh: 首页 }
blocks:
  - type: stats
    title: { en: Numbers, zh: 数字 }
    items:
      - value: 80
        label: { en: Countries, zh: 国家 }
      - value: 1.5
        label: { en: Ratio, zh: 比例 }
  - type: richtext
    body: { en: "Hello **world**", zh: "你好**世界**" }
`)
	write("content/products/sencha.yaml", `slug: sencha
status: published
title: { en: Sencha, zh: 煎茶 }
category: green-tea
scenarios: [daily-drink]
summary: { en: Tasty, zh: 好喝 }
description: { en: Very tasty., zh: 非常好喝。 }
lead_time: { en: 15-20 days, zh: 15-20 天 }
packaging: { en: 20kg bag, zh: 20 公斤袋 }
specs:
  - group: { en: Basic, zh: 基础 }
    items:
      - key: altitude
        label: { en: Altitude, zh: 海拔 }
        value_num: 1200
        unit: m
        filterable: true
featured: true
cover: tea.png
`)
	write("content/categories.yaml", `- slug: green-tea
  status: published
  name: { en: Green Tea, zh: 绿茶 }
  desc: { en: Unoxidized, zh: 未发酵 }
`)
	write("content/scenarios.yaml", `- slug: daily-drink
  status: published
  name: { en: Daily Drink, zh: 日常饮用 }
`)
	write("content/certificates.yaml", `- slug: usda
  status: published
  name: { en: USDA Organic, zh: USDA 有机 }
  issuer: USDA
  expires: "2030-01-01"
  markets: [EU]
`)
	write("content/cases/osaka.yaml", `slug: osaka
status: published
title: { en: Osaka Chain, zh: 大阪连锁 }
country: JP
industry: { en: Retail, zh: 零售 }
challenge: { en: Need supply, zh: 需要稳定供货 }
solution: { en: Annual contract, zh: 年度合约 }
result: { en: 3 years running, zh: 合作三年 }
products: [sencha]
`)
	// media 二进制文件：导出必须原样带回来
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "tea.png"), []byte{0x89, 'P', 'N', 'G', 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// canon 是模型级规范化比较：把 Site 重新序列化成 YAML 再比对。
// 直接 reflect.DeepEqual 会被 nil map / nil slice 卡住（YAML 往返后
// nil 与空容器在语义上等价，下游 Resolve / Empty 也都能正确处理），
// 规范化序列化只比「模型承载了什么」。Root 是目录路径，导出目录必然
// 不同，剔除。
func canon(t *testing.T, s *model.Site) string {
	t.Helper()
	s.Root = ""
	b, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("序列化 Site 失败: %v", err)
	}
	return string(b)
}

func TestImportExportRoundTrip(t *testing.T) {
	root := newFixture(t)
	db := filepath.Join(root, ".data", "content.db")

	st, err := Import(root, db, false)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if st.Pages != 1 || st.Products != 1 || st.Categories != 1 ||
		st.Scenarios != 1 || st.Certificates != 1 || st.Cases != 1 {
		t.Fatalf("导入数量不对: %+v", st)
	}

	siteFile, err := loader.NewFileLoader(root).Load()
	if err != nil {
		t.Fatalf("FileLoader: %v", err)
	}
	siteDB, err := NewDBLoader(root, db).Load()
	if err != nil {
		t.Fatalf("DBLoader: %v", err)
	}
	if got, want := canon(t, siteDB), canon(t, siteFile); got != want {
		t.Fatalf("DBLoader 与 FileLoader 的 Site 不一致:\n--- DB ---\n%s\n--- 文件 ---\n%s", got, want)
	}

	// 数字字段必须是 int（YAML 往返），不能退化成 float64（JSON 往返会这样）
	blk := siteDB.Pages[0].Blocks[0]
	items := blk.Extra["items"].([]any)
	first := items[0].(map[string]any)
	if _, ok := first["value"].(int); !ok {
		t.Fatalf("stats value 类型错误：期望 int，得到 %T", first["value"])
	}

	// 导出 → 重新用文件加载 → 与原站模型一致
	out := filepath.Join(t.TempDir(), "restored")
	if _, err := Export(root, db, out, false); err != nil {
		t.Fatalf("Export: %v", err)
	}
	siteExp, err := loader.NewFileLoader(out).Load()
	if err != nil {
		t.Fatalf("加载导出目录: %v", err)
	}
	if got, want := canon(t, siteExp), canon(t, siteFile); got != want {
		t.Fatalf("导出站点与原站不一致:\n--- 导出 ---\n%s\n--- 原站 ---\n%s", got, want)
	}

	// media 必须原样拷贝，导出目录才能独立构建
	orig, err := os.ReadFile(filepath.Join(root, "media", "tea.png"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "media", "tea.png"))
	if err != nil {
		t.Fatalf("导出目录缺少 media: %v", err)
	}
	if string(got) != string(orig) {
		t.Fatal("media 文件内容与原图不一致")
	}
}

func TestImportRefusesOverwriteWithoutForce(t *testing.T) {
	root := newFixture(t)
	db := filepath.Join(root, ".data", "content.db")

	if _, err := Import(root, db, false); err != nil {
		t.Fatalf("首次 Import: %v", err)
	}
	// 后台可能已在库上积累编辑：无 --force 的二次导入必须被拒绝
	if _, err := Import(root, db, false); err == nil {
		t.Fatal("对已存在内容库的 import 未被拒绝")
	}
	if _, err := Import(root, db, true); err != nil {
		t.Fatalf("--force 导入: %v", err)
	}
}

func TestExportRefusesNonEmptyDirWithoutForce(t *testing.T) {
	root := newFixture(t)
	db := filepath.Join(root, ".data", "content.db")
	if _, err := Import(root, db, false); err != nil {
		t.Fatalf("Import: %v", err)
	}

	out := t.TempDir()
	stray := filepath.Join(out, "手工文件.txt")
	if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(root, db, out, false); err == nil {
		t.Fatal("对非空目录的 export 未被拒绝")
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatal("被拒绝的导出不应触碰目录内已有文件")
	}
}

func TestDBLoaderRequiresExistingDB(t *testing.T) {
	root := newFixture(t)
	if _, err := NewDBLoader(root, filepath.Join(root, "nope.db")).Load(); err == nil {
		t.Fatal("打开不存在的内容库应报错")
	}
}
