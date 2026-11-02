// Package contentdb 是 M3「存储可换、模型不换」的内容库层。
//
// ADR-001 决定内容存文件（Git 管版本）；本包不推翻它，而是给模式 B 提供
// 另一条等价的存储路径：同一份 model.Site，可以住在文件里，也可以住在
// 单个 SQLite 文件里。双向迁移本身是卖点——客户随时能「拿回自己的内容」。
//
// 三个入口：
//
//	Import(root, dbPath)        文件 → DB（engine import）
//	Export(dbPath, outDir)      DB → 完整可构建的站点骨架（engine export）
//	NewDBLoader(root, dbPath)   DB → model.Site（engine build --db）
//
// 存储格式的一个关键取舍：payload 存 **YAML 文本**而不是 JSON。
// Block.Extra 是 map[string]any，JSON 往返会把 int 变 float64，
// 污染渲染层的区块二次解码；YAML 往返类型零损耗，在 DB 里同样可读。
// 供后台列表/表单查询的字段（slug / order / status / category / country /
// expires）提升为独立列，不在 payload 里翻找。
package contentdb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，注册名 "sqlite"（与 store 包同款，保住单二进制承诺）
)

// SchemaVersion 是内容库结构版本。结构变更时递增，并在 meta 表留痕。
const SchemaVersion = 1

const schema = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS settings (
  id   INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS theme (
  id   INTEGER PRIMARY KEY CHECK (id = 1),
  data TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pages (
  slug   TEXT PRIMARY KEY,
  ord    INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT '',
  data   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS products (
  slug     TEXT PRIMARY KEY,
  ord      INTEGER NOT NULL DEFAULT 0,
  status   TEXT NOT NULL DEFAULT '',
  category TEXT NOT NULL DEFAULT '',
  featured INTEGER NOT NULL DEFAULT 0,
  data     TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS categories (
  slug   TEXT PRIMARY KEY,
  ord    INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT '',
  parent TEXT NOT NULL DEFAULT '',
  data   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS scenarios (
  slug   TEXT PRIMARY KEY,
  ord    INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT '',
  data   TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS certificates (
  slug    TEXT PRIMARY KEY,
  ord     INTEGER NOT NULL DEFAULT 0,
  status  TEXT NOT NULL DEFAULT '',
  expires TEXT NOT NULL DEFAULT '',
  data    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS cases (
  slug    TEXT PRIMARY KEY,
  ord     INTEGER NOT NULL DEFAULT 0,
  status  TEXT NOT NULL DEFAULT '',
  country TEXT NOT NULL DEFAULT '',
  data    TEXT NOT NULL
);
`

// Stats 记录一次导入 / 导出的数量摘要。
type Stats struct {
	Settings     int
	Theme        int
	Pages        int
	Products     int
	Categories   int
	Scenarios    int
	Certificates int
	Cases        int
	MediaFiles   int
}

// open 打开（并按需创建）内容库，初始化表结构。
func open(dbPath string, create bool) (*sql.DB, error) {
	if !create {
		if _, err := os.Stat(dbPath); err != nil {
			return nil, fmt.Errorf("内容库不存在：%s（先运行 engine import）", dbPath)
		}
	} else if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("创建内容库目录失败: %w", err)
	}

	connStr := dbPath
	if !strings.Contains(dbPath, "?") {
		connStr = "file:" + filepath.ToSlash(dbPath) +
			"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("打开内容库失败: %w", err)
	}
	// 内容库是单用户编辑场景，一个写连接足够，避免 SQLite 写锁竞争
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接内容库失败: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化内容库表结构失败: %w", err)
	}
	// 用户 / 会话 / 审计表是 M3 第二阶段追加的；CREATE IF NOT EXISTS
	// 让旧内容库（v0.6 创建）打开时自动补齐，无需迁移
	if _, err := db.Exec(userSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化用户表结构失败: %w", err)
	}
	return db, nil
}

// setMeta 在事务里写一条 meta 记录。
func setMeta(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec("INSERT INTO meta(key, value) VALUES(?, ?) "+
		"ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}

// getMeta 读一条 meta 记录，缺失返回空串。
func getMeta(db *sql.DB, key string) string {
	var v string
	_ = db.QueryRow("SELECT value FROM meta WHERE key = ?", key).Scan(&v)
	return v
}
