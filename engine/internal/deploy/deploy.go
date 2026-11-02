// Package deploy 做产物目录到目标目录的单向同步（rsync --delete 语义）。
//
// 为什么先做本地目标：SFTP/对象存储都要引入外部依赖或密钥管理，而
// 「产物目录已经自包含」——任何静态托管（Nginx、Cloudflare Pages、
// Vercel）都能直接吃这个目录。本地同步 + 部署清单覆盖了 80% 的交付场景：
// 一条命令把新版本推到服务器挂载点，剩下的交给 CDN。
//
// 正确性基于内容哈希而不是 mtime：构建产物是全新写出的，mtime 永远是新的，
// 按 mtime 比较会把「内容没变」也判成「要复制」。sha256 一次读盘的成本
// 换来幂等——连续跑两次 deploy，第二次应该是 0 copied。
package deploy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Stats 是一次同步的结果摘要。
type Stats struct {
	Copied   int      `json:"copied"`  // 新增文件
	Updated  int      `json:"updated"` // 内容有变化的文件
	Deleted  int      `json:"deleted"` // 目标端多出来的孤儿文件
	Kept     int      `json:"kept"`    // 内容一致、跳过
	Bytes    int64    `json:"bytes"`   // 实际写入字节
	DeletedF []string `json:"deleted_files,omitempty"`
}

// File 是清单里的一条。
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Options 控制同步行为。
type Options struct {
	DryRun   bool // 只算差异不写盘
	Manifest bool // 在目标端写 deploy-manifest.json
}

// Sync 把 srcDir 的内容同步到 dstDir：
//
//   - src 有 / dst 没有 或 内容不同 → 复制
//   - dst 有 / src 没有 → 删除（孤儿文件：上一版产物里的旧哈希图、下架的产品页）
//
// 孤儿删除是刻意的：静态托管的缓存是按 URL 算的，旧哈希文件留着不碍事，
// 但旧**页面**留着会被搜索引擎继续收录——下架一个产品就必须让它 404。
func Sync(srcDir, dstDir string, opt Options) (*Stats, error) {
	srcSet, err := scan(srcDir)
	if err != nil {
		return nil, fmt.Errorf("扫描源目录失败: %w", err)
	}
	dstSet, err := scan(dstDir)
	if err != nil {
		return nil, fmt.Errorf("扫描目标目录失败: %w", err)
	}

	st := &Stats{DeletedF: []string{}}
	// 按（相对路径 → 源文件）处理新增与更新
	rels := make([]string, 0, len(srcSet))
	for rel := range srcSet {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		src := srcSet[rel]
		dst, ok := dstSet[rel]
		switch {
		case !ok:
			st.Copied++
			st.Bytes += src.Bytes
		case dst.SHA256 != src.SHA256:
			st.Updated++
			st.Bytes += src.Bytes
		default:
			st.Kept++
			continue
		}
		if opt.DryRun {
			continue
		}
		if err := copyFile(filepath.Join(srcDir, rel), filepath.Join(dstDir, rel)); err != nil {
			return st, err
		}
	}
	// 目标端孤儿（清单文件是自己生成的，不算孤儿）
	for rel := range dstSet {
		if _, ok := srcSet[rel]; !ok {
			if rel == "deploy-manifest.json" && opt.Manifest {
				continue
			}
			st.Deleted++
			st.DeletedF = append(st.DeletedF, rel)
			if opt.DryRun {
				continue
			}
			if err := os.Remove(filepath.Join(dstDir, rel)); err != nil {
				return st, err
			}
		}
	}

	if !opt.DryRun && opt.Manifest {
		files := make([]File, 0, len(srcSet))
		for rel, f := range srcSet {
			files = append(files, File{Path: rel, SHA256: f.SHA256, Bytes: f.Bytes})
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		b, err := json.MarshalIndent(files, "", "  ")
		if err == nil {
			_ = os.WriteFile(filepath.Join(dstDir, "deploy-manifest.json"), b, 0o644)
		}
	}
	sort.Strings(st.DeletedF)
	return st, nil
}

type fileInfo struct {
	SHA256 string
	Bytes  int64
}

// scan 遍历目录，返回 相对路径 → 内容指纹。
func scan(dir string) (map[string]fileInfo, error) {
	out := map[string]fileInfo{}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		out[rel] = fileInfo{SHA256: hex.EncodeToString(sum[:]), Bytes: info.Size()}
		return nil
	})
	return out, err
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// Summary 输出人话摘要。
func (s *Stats) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "新增 %d · 更新 %d · 删除 %d · 未变 %d（写入 %.1fMB）",
		s.Copied, s.Updated, s.Deleted, s.Kept, float64(s.Bytes)/1024/1024)
	return b.String()
}
