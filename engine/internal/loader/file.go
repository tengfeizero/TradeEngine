// Package loader 负责把磁盘上的内容文件读成内存模型。
//
// 存储可换、模型不换：M3 引入 Web 后台时，只需实现 DBLoader，
// 上层 Validator / Renderer / Build 完全不用改（见《11》第十二节）。
package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/model"
)

// Loader 是内容读取的抽象。v0.1 只有 FileLoader 一个实现。
type Loader interface {
	Load() (*model.Site, error)
}

// FileLoader 从站点目录读取 YAML / Markdown 内容。
type FileLoader struct {
	Root string
}

// NewFileLoader 创建文件加载器。
func NewFileLoader(root string) *FileLoader {
	return &FileLoader{Root: root}
}

// Load 读取整个站点。
func (l *FileLoader) Load() (*model.Site, error) {
	site := &model.Site{Root: l.Root}

	if err := readYAML(filepath.Join(l.Root, "engine.yaml"), &site.Config); err != nil {
		return nil, fmt.Errorf("读取 engine.yaml 失败: %w", err)
	}
	if site.Config.Output == "" {
		site.Config.Output = ".build/public"
	}
	if err := readYAML(filepath.Join(l.Root, "settings.yaml"), &site.Settings); err != nil {
		return nil, fmt.Errorf("读取 settings.yaml 失败: %w", err)
	}
	// theme.yaml 缺失时使用内置默认值，不阻塞构建；
	// 但「存在而解析失败」必须报出——静默回退会让整站悄悄变回默认配色，
	// 页面照常产出，用户几乎不可能发现（2026-10-22 ridgeway 交付时实测踩坑）。
	if err := readYAMLOptional(filepath.Join(l.Root, "theme", "theme.yaml"), &site.Theme); err != nil {
		return nil, fmt.Errorf("theme.yaml: %w", err)
	}

	contentDir := filepath.Join(l.Root, "content")

	var err error
	if site.Pages, err = loadPages(filepath.Join(contentDir, "pages")); err != nil {
		return nil, err
	}
	if site.Products, err = loadProducts(filepath.Join(contentDir, "products")); err != nil {
		return nil, err
	}
	if err = readYAMLOptional(filepath.Join(contentDir, "categories.yaml"), &site.Categories); err != nil {
		return nil, fmt.Errorf("读取 categories.yaml 失败: %w", err)
	}
	if err = readYAMLOptional(filepath.Join(contentDir, "scenarios.yaml"), &site.Scenarios); err != nil {
		return nil, fmt.Errorf("读取 scenarios.yaml 失败: %w", err)
	}
	if err = readYAMLOptional(filepath.Join(contentDir, "certificates.yaml"), &site.Certificates); err != nil {
		return nil, fmt.Errorf("读取 certificates.yaml 失败: %w", err)
	}
	if site.Cases, err = loadCases(filepath.Join(contentDir, "cases")); err != nil {
		return nil, err
	}

	return site, nil
}

func loadPages(dir string) ([]model.Page, error) {
	var out []model.Page
	err := eachYAML(dir, func(path string) error {
		var p model.Page
		if err := readYAML(path, &p); err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, err
}

func loadProducts(dir string) ([]model.Product, error) {
	var out []model.Product
	err := eachYAML(dir, func(path string) error {
		var p model.Product
		if err := readYAML(path, &p); err != nil {
			return err
		}
		// 文件名即 slug，避免两处维护不一致
		if p.Slug == "" {
			p.Slug = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		out = append(out, p)
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Slug < out[j].Slug
	})
	return out, err
}

func loadCases(dir string) ([]model.Case, error) {
	var out []model.Case
	err := eachYAML(dir, func(path string) error {
		var c model.Case
		if err := readYAML(path, &c); err != nil {
			return err
		}
		if c.Slug == "" {
			c.Slug = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		out = append(out, c)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, err
}

func eachYAML(dir string, fn func(path string) error) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取目录 %s 失败: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if err := fn(filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
	}
	return nil
}

func readYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, v); err != nil {
		return fmt.Errorf("YAML 语法错误: %w", err)
	}
	return nil
}

func readYAMLOptional(path string, v any) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	return readYAML(path, v)
}
