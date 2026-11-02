// Package store 是运行时唯一写实体（询盘）的持久化层。
//
// D7 决定用 SQLite：单文件、零运维、可直接把 .data 目录备份到对象存储。
// 驱动选 modernc.org/sqlite（纯 Go），因此整个引擎仍然是**单二进制、零运行时依赖**。
//
// 注意：内容（产品/页面/证书…）不走这里，它们存在文件里、由 Git 管版本（ADR-001）。
// 本包只处理「构建期之后才产生」的数据。
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动，注册名 "sqlite"
)

// Inquiry 是落到 inquiry 表的一行。
//
// 字段与《11》第十一节 DDL 一一对应。ProductTitle 是**快照**：
// 产品改名或下架后，历史询盘仍能看出当时客户问的是哪个产品。
type Inquiry struct {
	ID           string
	CreatedAt    time.Time
	Site         string
	Lang         string
	Country      string
	Name         string
	Company      string
	Email        string
	Phone        string
	ProductSlug  string
	ProductTitle string
	Volume       string
	Message      string
	SourcePage   string
	Referrer     string
	UTMSource    string
	UTMMedium    string
	UTMCampaign  string
	IPHash       string
	UserAgent    string
	SpamScore    int
	Status       string
	NotifiedAt   *time.Time
	Note         string
}

// Store 包装 *sql.DB。
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS inquiry (
  id              TEXT PRIMARY KEY,
  created_at      TEXT NOT NULL,
  site            TEXT NOT NULL DEFAULT '',
  lang            TEXT NOT NULL DEFAULT 'en',
  country         TEXT,
  name            TEXT NOT NULL,
  company         TEXT,
  email           TEXT NOT NULL,
  phone           TEXT,
  product_slug    TEXT,
  product_title   TEXT,
  volume          TEXT,
  message         TEXT NOT NULL,
  source_page     TEXT,
  referrer        TEXT,
  utm_source      TEXT,
  utm_medium      TEXT,
  utm_campaign    TEXT,
  ip_hash         TEXT,
  user_agent      TEXT,
  spam_score      INTEGER NOT NULL DEFAULT 0,
  status          TEXT NOT NULL DEFAULT 'new',
  notified_at     TEXT,
  note            TEXT
);
CREATE INDEX IF NOT EXISTS idx_inquiry_created ON inquiry(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inquiry_status  ON inquiry(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inquiry_site    ON inquiry(site, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_inquiry_email   ON inquiry(email);
CREATE INDEX IF NOT EXISTS idx_inquiry_ip      ON inquiry(ip_hash, created_at DESC);
`

// Open 打开（并初始化）数据库。dsn 为空时用默认路径。
//
// 用 WAL 而不是默认的 rollback journal：静态站 + 询盘 API 是「一写多读」场景，
// WAL 允许读不阻塞写，且崩溃恢复更快。busy_timeout 避免并发提交时立即报 SQLITE_BUSY。
func Open(dsn string) (*Store, error) {
	if dsn == "" {
		dsn = filepath.Join(".data", "inquiries.db")
	}
	// 内存库或已带参数的 DSN 不再追加 pragma
	connStr := dsn
	if !strings.Contains(dsn, "?") {
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
		connStr = "file:" + filepath.ToSlash(dsn) +
			"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}

	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 单写者：连接数不宜过大，否则容易出现写锁竞争
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化表结构失败: %w", err)
	}
	// 0.11.0 新增 volume 列。CREATE TABLE IF NOT EXISTS 对老库不起作用，
	// 用 ALTER 补列；列已存在时 SQLite 报 duplicate column，那正是我们要的终态。
	if _, err := db.Exec(`ALTER TABLE inquiry ADD COLUMN volume TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		_ = db.Close()
		return nil, fmt.Errorf("迁移询盘表失败: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Insert 写入一条询盘。
//
// 这是整条链路上最关键的一次调用：它成功，询盘就不会丢；
// 之后的邮件/webhook 全部是「尽力而为」的通知，失败只影响提醒速度。
func (s *Store) Insert(in *Inquiry) error {
	if in.ID == "" {
		return errors.New("询盘缺少 ID")
	}
	if in.Status == "" {
		in.Status = "new"
	}
	_, err := s.db.Exec(`
INSERT INTO inquiry (
  id, created_at, site, lang, country, name, company, email, phone,
  product_slug, product_title, volume, message, source_page, referrer,
  utm_source, utm_medium, utm_campaign, ip_hash, user_agent,
  spam_score, status, notified_at, note
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.CreatedAt.UTC().Format(time.RFC3339),
		in.Site, in.Lang, in.Country, in.Name, in.Company, in.Email, in.Phone,
		in.ProductSlug, in.ProductTitle, in.Volume, in.Message, in.SourcePage, in.Referrer,
		in.UTMSource, in.UTMMedium, in.UTMCampaign, in.IPHash, in.UserAgent,
		in.SpamScore, in.Status, nilIfZero(in.NotifiedAt), in.Note,
	)
	if err != nil {
		return fmt.Errorf("写入询盘失败: %w", err)
	}
	return nil
}

// MarkNotified 记录通知已发出（用于「日报兜底」时筛出漏看或未通知的询盘）。
func (s *Store) MarkNotified(id string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE inquiry SET notified_at=? WHERE id=?`, at.UTC().Format(time.RFC3339), id)
	return err
}

// CountByIPSince 统计某 IP 哈希在指定时间之后的提交数，用于限流。
//
// 为什么查库而不是只用内存计数器：引擎重启后内存计数会清零，
// 攻击者只要等你重启就能绕过限流。库里的计数是持久的。
func (s *Store) CountByIPSince(ipHash string, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM inquiry WHERE ip_hash=? AND created_at>=?`,
		ipHash, since.UTC().Format(time.RFC3339),
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("统计限流计数失败: %w", err)
	}
	return n, nil
}

// ListOptions 是查询条件。
type ListOptions struct {
	Status string
	Limit  int
	Since  time.Time
}

// List 按时间倒序返回询盘，供 CLI 查看。
func (s *Store) List(opt ListOptions) ([]Inquiry, error) {
	if opt.Limit <= 0 {
		opt.Limit = 20
	}
	q := `SELECT id, created_at, site, lang, COALESCE(country,''), name, COALESCE(company,''),
	             email, COALESCE(phone,''), COALESCE(product_slug,''), COALESCE(product_title,''),
	             COALESCE(volume,''), message, COALESCE(source_page,''), COALESCE(referrer,''),
	             COALESCE(utm_source,''), COALESCE(utm_medium,''), COALESCE(utm_campaign,''),
	             COALESCE(ip_hash,''), COALESCE(user_agent,''), spam_score, status,
	             notified_at, COALESCE(note,'')
	        FROM inquiry`
	var args []any
	var where []string
	if opt.Status != "" {
		where = append(where, "status=?")
		args = append(args, opt.Status)
	}
	if !opt.Since.IsZero() {
		where = append(where, "created_at>=?")
		args = append(args, opt.Since.UTC().Format(time.RFC3339))
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, opt.Limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("查询询盘失败: %w", err)
	}
	defer rows.Close()

	var out []Inquiry
	for rows.Next() {
		var it Inquiry
		var created string
		var notified sql.NullString
		if err := rows.Scan(
			&it.ID, &created, &it.Site, &it.Lang, &it.Country, &it.Name, &it.Company,
			&it.Email, &it.Phone, &it.ProductSlug, &it.ProductTitle,
			&it.Volume, &it.Message, &it.SourcePage, &it.Referrer,
			&it.UTMSource, &it.UTMMedium, &it.UTMCampaign,
			&it.IPHash, &it.UserAgent, &it.SpamScore, &it.Status,
			&notified, &it.Note,
		); err != nil {
			return nil, fmt.Errorf("读取询盘失败: %w", err)
		}
		it.CreatedAt, _ = time.Parse(time.RFC3339, created)
		if notified.Valid {
			if t, perr := time.Parse(time.RFC3339, notified.String); perr == nil {
				it.NotifiedAt = &t
			}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// CountUnnotified 返回未成功通知的询盘数，用于日报兜底提醒。
func (s *Store) CountUnnotified() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM inquiry WHERE notified_at IS NULL AND status<>'spam'`).Scan(&n)
	return n, err
}

func nilIfZero(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
