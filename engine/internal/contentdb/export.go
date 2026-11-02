package contentdb

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Export 把内容库反向导出成一个**完整可构建**的站点骨架。
//
// 这是「客户随时拿回自己的内容」的兑现：不再依赖引擎版本、后台账号或
// 平台接口，一个 SQLite 文件 + 一条命令，产出的目录 engine build 直接能跑。
//
// 输出包含：
//   - engine.yaml（重新生成，output 固定 .build/public）
//   - settings.yaml / theme/theme.yaml（来自 DB）
//   - content/ 下全部实体 YAML（来自 DB）
//   - media/ 与 theme/ 的其余文件（从 root 原样拷贝；图片二进制不入库）
//
// 安全闸：outDir 非空时拒绝执行——导出覆盖人工文件必须是显式决定（--force）。
func Export(root, dbPath, outDir string, force bool) (*Stats, error) {
	if outDir == "" {
		return nil, fmt.Errorf("缺少 --out 导出目录")
	}
	if empty, err := dirEmpty(outDir); err != nil {
		return nil, err
	} else if !empty && !force {
		return nil, fmt.Errorf("导出目录 %s 非空（防止覆盖手工内容）；确认后加 --force", outDir)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	site, err := NewDBLoader(root, dbPath).Load()
	if err != nil {
		return nil, err
	}

	st := &Stats{}

	// engine.yaml：引擎配置不属于内容，导出时按当前约定重新生成
	write := func(rel string, data []byte) error {
		full := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		return os.WriteFile(full, data, 0o644)
	}
	if err := write("engine.yaml", []byte("engine_version: 1\noutput: .build/public\n")); err != nil {
		return nil, err
	}

	if raw, err := yaml.Marshal(site.Settings); err != nil {
		return nil, err
	} else if err = write("settings.yaml", raw); err != nil {
		return nil, err
	}
	if raw, err := yaml.Marshal(site.Theme); err != nil {
		return nil, err
	} else if err = write("theme/theme.yaml", raw); err != nil {
		return nil, err
	}

	for _, p := range site.Pages {
		if err := writeEntity(outDir, "content/pages", p.Slug, p); err != nil {
			return nil, err
		}
		st.Pages++
	}
	for _, p := range site.Products {
		if err := writeEntity(outDir, "content/products", p.Slug, p); err != nil {
			return nil, err
		}
		st.Products++
	}
	for _, c := range site.Cases {
		if err := writeEntity(outDir, "content/cases", c.Slug, c); err != nil {
			return nil, err
		}
		st.Cases++
	}
	if err := writeList(outDir, "content/categories.yaml", site.Categories); err != nil {
		return nil, err
	}
	st.Categories = len(site.Categories)
	if err := writeList(outDir, "content/scenarios.yaml", site.Scenarios); err != nil {
		return nil, err
	}
	st.Scenarios = len(site.Scenarios)
	if err := writeList(outDir, "content/certificates.yaml", site.Certificates); err != nil {
		return nil, err
	}
	st.Certificates = len(site.Certificates)

	// media/ 与 theme/ 的其余文件原样拷贝——图片与主题静态资源是二进制/资产，
	// 从不入内容库；导出目录要能独立构建，必须带上它们
	if n, err := copyTree(filepath.Join(root, "media"), filepath.Join(outDir, "media")); err != nil {
		return nil, fmt.Errorf("拷贝 media 失败: %w", err)
	} else {
		st.MediaFiles = n
	}
	if err := copyThemeExtras(root, outDir); err != nil {
		return nil, err
	}

	return st, nil
}

// writeEntity 写单实体文件（pages / products / cases）。
func writeEntity[T any](outDir, subdir, slug string, item T) error {
	if slug == "" {
		return fmt.Errorf("%s 中存在空 slug，无法导出", subdir)
	}
	raw, err := yaml.Marshal(item)
	if err != nil {
		return fmt.Errorf("序列化 %s/%s 失败: %w", subdir, slug, err)
	}
	full := filepath.Join(outDir, filepath.FromSlash(subdir), slug+".yaml")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, raw, 0o644)
}

// writeList 写列表文件（categories / scenarios / certificates）。
func writeList[T any](outDir, rel string, items []T) error {
	raw, err := yaml.Marshal(items)
	if err != nil {
		return fmt.Errorf("序列化 %s 失败: %w", rel, err)
	}
	full := filepath.Join(outDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, raw, 0o644)
}

// copyThemeExtras 拷贝 theme/ 下除 theme.yaml 以外的文件（静态资源等）。
func copyThemeExtras(root, outDir string) error {
	src := filepath.Join(root, "theme")
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil // theme 目录不存在不算错误
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "theme.yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "theme", e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// copyTree 递归拷贝目录，返回拷贝的文件数。
func copyTree(src, dst string) (int, error) {
	var count int
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // 源目录不存在不算错误
			}
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

func dirEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}
