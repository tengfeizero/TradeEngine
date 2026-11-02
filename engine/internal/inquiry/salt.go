package inquiry

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LoadSalt 读取 IP 哈希盐；不存在时生成并落盘。
//
// 为什么必须持久化：IP 哈希用于限流计数。如果盐每次重启都变，
// 重启后旧记录的 ip_hash 全部对不上，限流就被清零了。
// 也不接受「留空用默认值」——那样等于不哈希，配合彩虹表能还原出 IP。
func LoadSalt(path string) ([]byte, error) {
	if path == "" {
		path = filepath.Join(".data", "ip_salt")
	}
	if b, err := os.ReadFile(path); err == nil {
		if raw, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b))); derr == nil && len(raw) == 32 {
			return raw, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("生成 IP 哈希盐失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建盐目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(buf)), 0o600); err != nil {
		return nil, fmt.Errorf("写入盐文件失败: %w", err)
	}
	return buf, nil
}

// HashIP 返回 sha256(ip + salt) 的十六进制串。
// 库里从不存明文 IP：一是隐私（GDPR 里 IP 属个人数据），二是没必要。
func HashIP(ip string, salt []byte) string {
	h := sha256.New()
	h.Write([]byte(ip))
	h.Write(salt)
	return hex.EncodeToString(h.Sum(nil))
}

// ClientIP 从请求中取客户端 IP。
//
// trustProxy 为 false 时只用 RemoteAddr——直接信任 X-Forwarded-For 会让攻击者
// 随便伪造 IP 绕过限流。只有确实部署在 Cloudflare/Nginx 之后才该打开。
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// 取最左侧：那是客户端真实 IP，其余是代理链
			if i := strings.Index(xff, ","); i >= 0 {
				xff = xff[:i]
			}
			if ip := strings.TrimSpace(xff); ip != "" {
				return ip
			}
		}
		if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
