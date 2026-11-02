package contentdb

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// 本文件是多用户 + 审核流的存储部分：users / sessions / audit 三张表。
//
// 口令散列用标准库 crypto/pbkdf2（Go 1.24+，RFC 8018），不引第三方依赖——
// 单二进制承诺比「用 bcrypt 更时髦」重要。
// 会话是服务端表（不是自包含 JWT）：撤销 = 删一行，简单可靠。

const (
	pbkdf2Iter      = 210000 // OWASP 2023 对 SHA-256 的建议值
	sessionLifetime = 12 * time.Hour
)

// Role 是用户角色。admin 全权；editor 只能编辑未发布内容并提交审核。
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
)

// User 是用户信息（不含口令散列）。
type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

const userSchema = `
CREATE TABLE IF NOT EXISTS users (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  username   TEXT UNIQUE NOT NULL,
  pass_hash  TEXT NOT NULL,
  role       TEXT NOT NULL DEFAULT 'editor',
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  token      TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  csrf       TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS audit (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  ts      TEXT NOT NULL,
  username TEXT NOT NULL,
  action  TEXT NOT NULL,
  entity  TEXT NOT NULL DEFAULT '',
  slug    TEXT NOT NULL DEFAULT '',
  detail  TEXT NOT NULL DEFAULT ''
);
`

// UserCount 返回用户数（首次启动引导管理员的判断依据）。
func (d *DB) UserCount() (int, error) {
	var n int
	err := d.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// CreateUser 创建用户。角色必须是 admin / editor。
func (d *DB) CreateUser(username, password, role string) error {
	if username == "" || password == "" {
		return fmt.Errorf("用户名与口令不能为空")
	}
	if role != string(RoleAdmin) && role != string(RoleEditor) {
		return fmt.Errorf("角色必须是 admin 或 editor，当前为 %q", role)
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	_, err = d.db.Exec("INSERT INTO users(username, pass_hash, role, created_at) VALUES(?, ?, ?, ?)",
		username, hash, role, time.Now().UTC().Format(time.RFC3339))
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return fmt.Errorf("用户 %q 已存在", username)
	}
	return err
}

// ListUsers 列出用户（不含口令散列）。
func (d *DB) ListUsers() ([]User, error) {
	rows, err := d.db.Query("SELECT id, username, role, created_at FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteUser 删除用户。禁止删掉最后一个 admin——把自己锁在门外是运维事故。
func (d *DB) DeleteUser(id int64) error {
	var role string
	err := d.db.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&role)
	if err == sql.ErrNoRows {
		return fmt.Errorf("用户不存在")
	}
	if err != nil {
		return err
	}
	if role == string(RoleAdmin) {
		var admins int
		if err := d.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return fmt.Errorf("不能删除最后一个管理员")
		}
	}
	if _, err := d.db.Exec("DELETE FROM users WHERE id = ?", id); err != nil {
		return err
	}
	// 会话有 ON DELETE CASCADE，随用户一起消失
	return nil
}

// Authenticate 校验用户名口令，返回用户 ID 与角色。
// 失败时返回一致的错误信息（不区分「无此用户」与「口令错」，不给枚举口令的余地）。
func (d *DB) Authenticate(username, password string) (int64, Role, error) {
	var id int64
	var hash, role string
	err := d.db.QueryRow("SELECT id, pass_hash, role FROM users WHERE username = ?", username).
		Scan(&id, &hash, &role)
	if err == sql.ErrNoRows {
		// 仍执行一次散列，抹平「用户不存在」与「口令错误」的时间差
		_, _ = hashPassword(password)
		return 0, "", fmt.Errorf("用户名或口令不正确")
	}
	if err != nil {
		return 0, "", err
	}
	ok, err := verifyPassword(hash, password)
	if err != nil || !ok {
		return 0, "", fmt.Errorf("用户名或口令不正确")
	}
	return id, Role(role), nil
}

// Session 是一次登录的凭据。
type Session struct {
	Token     string
	CSRF      string
	Username  string
	Role      Role
	ExpiresAt time.Time
}

// CreateSession 建立会话（12 小时有效）。
func (d *DB) CreateSession(userID int64) (*Session, error) {
	token, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(sessionLifetime)
	if _, err := d.db.Exec("INSERT INTO sessions(token, user_id, csrf, created_at, expires_at) VALUES(?, ?, ?, ?, ?)",
		token, userID, csrf, time.Now().UTC().Format(time.RFC3339), exp.UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	return &Session{Token: token, CSRF: csrf, ExpiresAt: exp}, nil
}

// SessionUser 校验会话令牌，返回绑定用户。过期会话视为不存在。
func (d *DB) SessionUser(token string) (username string, role Role, csrf string, ok bool) {
	if token == "" {
		return "", "", "", false
	}
	var u struct{ username, role, csrf string }
	err := d.db.QueryRow(`SELECT u.username, u.role, s.csrf FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ?`, token, time.Now().UTC().Format(time.RFC3339)).
		Scan(&u.username, &u.role, &u.csrf)
	if err != nil {
		return "", "", "", false
	}
	return u.username, Role(u.role), u.csrf, true
}

// DeleteSession 注销（撤销 = 删一行，这就是用服务端会话表的理由）。
func (d *DB) DeleteSession(token string) {
	_, _ = d.db.Exec("DELETE FROM sessions WHERE token = ?", token)
}

// PurgeSessions 清理过期会话。
func (d *DB) PurgeSessions() {
	_, _ = d.db.Exec("DELETE FROM sessions WHERE expires_at <= ?", time.Now().UTC().Format(time.RFC3339))
}

// Audit 记一条审计日志。审计是审核流的证据链：谁在什么时候改了什么。
func (d *DB) Audit(username, action, entity, slug, detail string) {
	_, _ = d.db.Exec("INSERT INTO audit(ts, username, action, entity, slug, detail) VALUES(?, ?, ?, ?, ?, ?)",
		time.Now().UTC().Format(time.RFC3339), username, action, entity, slug, detail)
}

// AuditEntry 是一条审计记录。
type AuditEntry struct {
	TS       string `json:"ts"`
	Username string `json:"username"`
	Action   string `json:"action"`
	Entity   string `json:"entity"`
	Slug     string `json:"slug"`
	Detail   string `json:"detail"`
}

// ListAudit 倒序返回最近 N 条审计记录。
func (d *DB) ListAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := d.db.Query("SELECT ts, username, action, entity, slug, detail FROM audit ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.TS, &e.Username, &e.Action, &e.Entity, &e.Slug, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- 口令散列

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, pbkdf2Iter, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iter, hex.EncodeToString(salt), hex.EncodeToString(key)), nil
}

func verifyPassword(stored, password string) (bool, error) {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false, fmt.Errorf("口令散列格式无法识别")
	}
	var iter int
	if _, err := fmt.Sscanf(parts[1], "%d", &iter); err != nil {
		return false, err
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false, err
	}
	want, err := hex.DecodeString(parts[3])
	if err != nil {
		return false, err
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false, err
	}
	return hmac.Equal(got, want), nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
