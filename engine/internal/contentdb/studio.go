package contentdb

import (
	"database/sql"
	"fmt"

	"gopkg.in/yaml.v3"
)

// 本文件是内容库的「写面」：Web Studio（M3 第二阶段）用它把表单结果
// 写回库。三条纪律：
//
//  1. payload 与提升列**必须在同一事务里更新**——列是查询索引，
//     payload 是构建输入，两边不一致比没有列更糟。
//  2. 表名是白名单，不接受任意字符串拼 SQL。
//  3. status 只有一个真相源：payload。SetStatus 会同时改 payload 与列，
//     保证 `engine build --db` 解析 payload 就能拿到最终状态。

// DB 是内容库句柄（studio 的写面）。
type DB struct {
	db *sql.DB
}

// Open 打开内容库（必须已由 engine import 创建）。
func Open(dbPath string) (*DB, error) {
	d, err := open(dbPath, false)
	if err != nil {
		return nil, err
	}
	return &DB{db: d}, nil
}

// Close 关闭句柄。
func (d *DB) Close() error { return d.db.Close() }

// EntityRow 是一张实体表的一行：提升列 + payload。
type EntityRow struct {
	Slug   string
	Ord    int
	Status string
	Data   string
}

// entityTables 是可编辑实体表的白名单。
// settings 不在表内（单行、无 slug，走专用方法）。
var entityTables = map[string]bool{
	"pages": true, "products": true, "categories": true,
	"scenarios": true, "certificates": true, "cases": true,
}

// ValidTable 判断表名是否可编辑。
func ValidTable(table string) bool { return entityTables[table] }

// ListEntity 按表列出实体（ord, slug 排序），status 非空时过滤。
func (d *DB) ListEntity(table, status string) ([]EntityRow, error) {
	if !entityTables[table] {
		return nil, fmt.Errorf("未知的实体表：%s", table)
	}
	query := "SELECT slug, ord, status, data FROM " + table
	args := []any{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY ord, slug"
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", table, err)
	}
	defer rows.Close()
	var out []EntityRow
	for rows.Next() {
		var r EntityRow
		if err := rows.Scan(&r.Slug, &r.Ord, &r.Status, &r.Data); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountByStatus 返回某表各状态的实体数（dashboard 用）。
func (d *DB) CountByStatus(table string) (map[string]int, error) {
	if !entityTables[table] {
		return nil, fmt.Errorf("未知的实体表：%s", table)
	}
	rows, err := d.db.Query("SELECT status, COUNT(*) FROM " + table + " GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// GetEntity 读取单个实体的 payload。
func (d *DB) GetEntity(table, slug string) (*EntityRow, error) {
	if !entityTables[table] {
		return nil, fmt.Errorf("未知的实体表：%s", table)
	}
	var r EntityRow
	err := d.db.QueryRow("SELECT slug, ord, status, data FROM "+table+" WHERE slug = ?", slug).
		Scan(&r.Slug, &r.Ord, &r.Status, &r.Data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%s/%s 不存在", table, slug)
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// PutEntity 把（可能被表单改过的）payload 写回库。
// slug 必须已存在（Studio 不做改名——slug 是 URL，改名是删旧建新）。
// 提升列从 payload 重新探测，永远与 payload 同步。
func (d *DB) PutEntity(table string, data []byte) error {
	if !entityTables[table] {
		return fmt.Errorf("未知的实体表：%s", table)
	}
	probe, err := probeColumns(table, data)
	if err != nil {
		return err
	}
	if probe.slug == "" {
		return fmt.Errorf("payload 缺少 slug")
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec("UPDATE "+table+" SET ord = ?, status = ?, data = ? WHERE slug = ?",
		probe.ord, probe.status, string(data), probe.slug)
	if err != nil {
		return fmt.Errorf("更新 %s/%s 失败: %w", table, probe.slug, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%s/%s 不存在（Studio 不支持改名，slug 是 URL）", table, probe.slug)
	}
	for _, c := range probe.extra {
		if err := setCol(tx, table, probe.slug, c.key, c.val); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetStatus 改实体状态：payload 与列在同一事务里更新。
func (d *DB) SetStatus(table, slug, status string) error {
	if !entityTables[table] {
		return fmt.Errorf("未知的实体表：%s", table)
	}
	row, err := d.GetEntity(table, slug)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(row.Data), &doc); err != nil {
		return fmt.Errorf("解析 %s/%s payload 失败: %w", table, slug, err)
	}
	doc["status"] = status
	// payload 是模型序列化产物（无注释），map 重排键序不影响语义
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	probe, err := probeColumns(table, raw)
	if err != nil {
		return err
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE "+table+" SET status = ?, data = ? WHERE slug = ?",
		status, string(raw), slug); err != nil {
		return err
	}
	for _, c := range probe.extra {
		if err := setCol(tx, table, slug, c.key, c.val); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSettings / PutSettings 是 settings 单行的读写（无 slug，专用方法）。
func (d *DB) GetSettings() (string, error) {
	var data string
	err := d.db.QueryRow("SELECT data FROM settings WHERE id = 1").Scan(&data)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("settings 行不存在")
	}
	return data, err
}

func (d *DB) PutSettings(data []byte) error {
	_, err := d.db.Exec("UPDATE settings SET data = ? WHERE id = 1", string(data))
	return err
}

// Meta 返回导入时留下的元信息（语言列表等）。
func (d *DB) Meta() (langs []string, defaultLang string) {
	if v := getMeta(d.db, "langs"); v != "" {
		_ = yaml.Unmarshal([]byte(v), &langs)
	}
	if langs == nil {
		langs = []string{"en"}
	}
	if defaultLang = getMeta(d.db, "default_lang"); defaultLang == "" {
		defaultLang = langs[0]
	}
	return langs, defaultLang
}

// QueryRowInt 执行返回单整数的查询（测试与诊断用的小助手）。
func (d *DB) QueryRowInt(query string, dst *int) error {
	return d.db.QueryRow(query).Scan(dst)
}

// colPair 是一条提升列更新。
type colPair struct {
	key string
	val any
}

// probeColumns 从 payload 里探测提升列的当前值。
// 与 import 用同一套规则，保证列永远跟着 payload 走。
func probeColumns(table string, data []byte) (struct {
	slug   string
	ord    int
	status string
	extra  []colPair
}, error) {
	type baseProbe struct {
		Slug   string `yaml:"slug"`
		Order  int    `yaml:"order"`
		Status string `yaml:"status"`
	}
	var probe struct {
		slug   string
		ord    int
		status string
		extra  []colPair
	}
	var base baseProbe
	if err := yaml.Unmarshal(data, &base); err != nil {
		return probe, fmt.Errorf("解析 payload 失败: %w", err)
	}
	probe.slug, probe.ord, probe.status = base.Slug, base.Order, base.Status

	switch table {
	case "products":
		var extra struct {
			Category string `yaml:"category"`
			Featured bool   `yaml:"featured"`
		}
		if err := yaml.Unmarshal(data, &extra); err != nil {
			return probe, err
		}
		probe.extra = append(probe.extra,
			colPair{"category", extra.Category},
			colPair{"featured", boolInt(extra.Featured)})
	case "categories":
		var extra struct {
			Parent string `yaml:"parent"`
		}
		if err := yaml.Unmarshal(data, &extra); err != nil {
			return probe, err
		}
		probe.extra = append(probe.extra, colPair{"parent", extra.Parent})
	case "cases":
		var extra struct {
			Country string `yaml:"country"`
		}
		if err := yaml.Unmarshal(data, &extra); err != nil {
			return probe, err
		}
		probe.extra = append(probe.extra, colPair{"country", extra.Country})
	case "certificates":
		var extra struct {
			Expires string `yaml:"expires"`
		}
		if err := yaml.Unmarshal(data, &extra); err != nil {
			return probe, err
		}
		probe.extra = append(probe.extra, colPair{"expires", extra.Expires})
	}
	return probe, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
