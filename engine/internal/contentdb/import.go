package contentdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tradeengine/tradeengine/internal/loader"
	"github.com/tradeengine/tradeengine/internal/model"
)

// Import 把站点文件内容灌入 SQLite 内容库。
//
// 这是「文件 → DB」方向：交付期内容在文件里（Git 管版本），
// 交给后台编辑前先灌入库；后台的所有写操作都发生在 DB 上。
//
// 安全闸：dbPath 已存在时拒绝执行——后台可能已经在库上积累了编辑，
// 用文件覆盖它必须是显式决定（--force）。
func Import(root, dbPath string, force bool) (*Stats, error) {
	if _, err := os.Stat(dbPath); err == nil && !force {
		return nil, fmt.Errorf("内容库已存在：%s\n"+
			"  若确认以文件内容为准，加 --force 覆盖（后台在库上的未导出编辑会丢失）", dbPath)
	}
	if force {
		for _, suf := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(dbPath + suf)
		}
	}

	site, err := loader.NewFileLoader(root).Load()
	if err != nil {
		return nil, fmt.Errorf("读取站点内容失败: %w", err)
	}

	db, err := open(dbPath, true)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	st := &Stats{}
	err = transact(db, func(tx *sql.Tx) error {
		if err := writeSettings(tx, &site.Settings); err != nil {
			return err
		}
		st.Settings = 1
		if err := writeTheme(tx, &site.Theme); err != nil {
			return err
		}
		st.Theme = 1

		if st.Pages, err = writeEntities(tx, "pages", site.Pages); err != nil {
			return err
		}
		if st.Products, err = writeEntities(tx, "products", site.Products); err != nil {
			return err
		}
		if st.Categories, err = writeEntities(tx, "categories", site.Categories); err != nil {
			return err
		}
		if st.Scenarios, err = writeEntities(tx, "scenarios", site.Scenarios); err != nil {
			return err
		}
		if st.Certificates, err = writeEntities(tx, "certificates", site.Certificates); err != nil {
			return err
		}
		if st.Cases, err = writeEntities(tx, "cases", site.Cases); err != nil {
			return err
		}

		for _, e := range site.Products {
			if err := setCol(tx, "products", e.Slug, "category", e.Category); err != nil {
				return err
			}
			if e.Featured {
				if err := setCol(tx, "products", e.Slug, "featured", 1); err != nil {
					return err
				}
			}
		}
		for _, c := range site.Cases {
			if err := setCol(tx, "cases", c.Slug, "country", c.Country); err != nil {
				return err
			}
		}
		for _, c := range site.Certificates {
			if err := setCol(tx, "certificates", c.Slug, "expires", c.Expires); err != nil {
				return err
			}
		}
		for _, c := range site.Categories {
			if err := setCol(tx, "categories", c.Slug, "parent", c.Parent); err != nil {
				return err
			}
		}

		if err := setMeta(tx, "schema_version", fmt.Sprint(SchemaVersion)); err != nil {
			return err
		}
		if err := setMeta(tx, "default_lang", site.DefaultLang()); err != nil {
			return err
		}
		if err := setMeta(tx, "langs", marshalJSON(site.Langs())); err != nil {
			return err
		}
		return setMeta(tx, "imported_at", time.Now().UTC().Format(time.RFC3339))
	})
	if err != nil {
		return nil, err
	}
	return st, nil
}

// writeEntities 把一组实体写进对应表。payload 是整个 struct 的 YAML 序列化
// （不是原始文件文本——经过模型意味着库里的内容永远与模型同构）。
// slug / order / status 从序列化结果里探回，保证列与 payload 永远一致。
func writeEntities[T any](tx *sql.Tx, table string, items []T) (int, error) {
	n := 0
	for i := range items {
		raw, err := yaml.Marshal(items[i])
		if err != nil {
			return n, fmt.Errorf("序列化 %s #%d 失败: %w", table, i, err)
		}
		var probe struct {
			Slug   string `yaml:"slug"`
			Order  int    `yaml:"order"`
			Status string `yaml:"status"`
		}
		if err := yaml.Unmarshal(raw, &probe); err != nil {
			return n, fmt.Errorf("解析 %s #%d 的 Base 字段失败: %w", table, i, err)
		}
		if probe.Slug == "" {
			return n, fmt.Errorf("%s #%d 缺少 slug，无法入库", table, i)
		}
		if _, err := tx.Exec(
			"INSERT INTO "+table+"(slug, ord, status, data) VALUES(?, ?, ?, ?)",
			probe.Slug, probe.Order, probe.Status, string(raw)); err != nil {
			return n, fmt.Errorf("写入 %s/%s 失败: %w", table, probe.Slug, err)
		}
		n++
	}
	return n, nil
}

// setCol 更新提升列（category / country / expires / parent / featured）。
func setCol(tx *sql.Tx, table, slug, col string, val any) error {
	// col 来自代码内常量调用，非用户输入，白名单拼接是安全的
	if _, err := tx.Exec("UPDATE "+table+" SET "+col+" = ? WHERE slug = ?", val, slug); err != nil {
		return fmt.Errorf("更新 %s/%s 的 %s 失败: %w", table, slug, col, err)
	}
	return nil
}

func writeSettings(tx *sql.Tx, s *model.Settings) error {
	raw, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("序列化 settings 失败: %w", err)
	}
	_, err = tx.Exec("INSERT INTO settings(id, data) VALUES(1, ?)", string(raw))
	return err
}

func writeTheme(tx *sql.Tx, t *model.Theme) error {
	raw, err := yaml.Marshal(t)
	if err != nil {
		return fmt.Errorf("序列化 theme 失败: %w", err)
	}
	_, err = tx.Exec("INSERT INTO theme(id, data) VALUES(1, ?)", string(raw))
	return err
}

func transact(db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}
