package contentdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/loader"
	"github.com/tengfeizero/TradeEngine/internal/model"
)

// DBLoader 从内容库读出 model.Site，实现 loader.Loader。
//
// 「存储可换、模型不换」的另一半：上层 Validator / Renderer / Build 不感知
// 内容来自文件还是 DB（见《11 数据模型与 API 设计》第十二节）。
//
// 边界：engine.yaml 仍从站点目录读取——它是引擎配置（输出目录等），
// 不是站点内容；theme.yaml 与全部内容实体来自 DB。
type DBLoader struct {
	Root   string
	DBPath string
}

// 编译期保证 DBLoader 满足 Loader 接口。
var _ loader.Loader = (*DBLoader)(nil)

// NewDBLoader 创建内容库加载器。
func NewDBLoader(root, dbPath string) *DBLoader {
	return &DBLoader{Root: root, DBPath: dbPath}
}

// Load 读取整个站点（内容库版本）。
func (l *DBLoader) Load() (*model.Site, error) {
	site := &model.Site{Root: l.Root}

	if err := readYAMLFile(filepath.Join(l.Root, "engine.yaml"), &site.Config); err != nil {
		return nil, fmt.Errorf("读取 engine.yaml 失败: %w", err)
	}
	if site.Config.Output == "" {
		site.Config.Output = ".build/public"
	}

	db, err := open(l.DBPath, false)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if v := getMeta(db, "schema_version"); v != "" && v != fmt.Sprint(SchemaVersion) {
		return nil, fmt.Errorf("内容库版本过旧（schema %s，引擎要求 %d）：请重新 engine import",
			v, SchemaVersion)
	}

	if err := scanRow(db, "SELECT data FROM settings WHERE id = 1", &site.Settings); err != nil {
		return nil, fmt.Errorf("读取 settings 失败: %w", err)
	}
	// theme 行理论上必有（import 时写入）；异常缺失时退回文件，不阻塞构建
	if err := scanRow(db, "SELECT data FROM theme WHERE id = 1", &site.Theme); err != nil {
		_ = readYAMLFile(filepath.Join(l.Root, "theme", "theme.yaml"), &site.Theme)
	}

	if site.Pages, err = loadEntities[model.Page](db, "pages", "slug"); err != nil {
		return nil, err
	}
	if site.Products, err = loadEntities[model.Product](db, "products", "ord, slug"); err != nil {
		return nil, err
	}
	if site.Categories, err = loadEntities[model.Category](db, "categories", "slug"); err != nil {
		return nil, err
	}
	if site.Scenarios, err = loadEntities[model.Scenario](db, "scenarios", "slug"); err != nil {
		return nil, err
	}
	if site.Certificates, err = loadEntities[model.Certificate](db, "certificates", "slug"); err != nil {
		return nil, err
	}
	if site.Cases, err = loadEntities[model.Case](db, "cases", "slug"); err != nil {
		return nil, err
	}
	return site, nil
}

// loadEntities 读一张实体表并反序列化 payload。
// 排序与 FileLoader 保持一致（产品按 order+slug，其余按 slug），
// 保证两种 Loader 产出的 Site 连顺序都相等。
func loadEntities[T any](db *sql.DB, table, orderBy string) ([]T, error) {
	rows, err := db.Query("SELECT data FROM " + table + " ORDER BY " + orderBy)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", table, err)
	}
	defer rows.Close()

	var out []T
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("读取 %s 行失败: %w", table, err)
		}
		var item T
		if err := yaml.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("解析 %s payload 失败: %w", table, err)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanRow(db *sql.DB, query string, v any) error {
	var raw string
	if err := db.QueryRow(query).Scan(&raw); err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(raw), v)
}

func readYAMLFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, v)
}
