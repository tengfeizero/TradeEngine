package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) (src, dst string) {
	t.Helper()
	root := t.TempDir()
	src, dst = filepath.Join(root, "public"), filepath.Join(root, "site")
	mk := func(base, rel, content string) {
		p := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk(src, "en/index.html", "hello")
	mk(src, "media/a.11111111.png", "AAA")
	mk(dst, "en/index.html", "old")        // 内容不同 → 更新
	mk(dst, "en/old-page/index.html", "x") // 孤儿 → 删除
	return src, dst
}

func TestSyncAddUpdateDelete(t *testing.T) {
	src, dst := setup(t)

	st, err := Sync(src, dst, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Copied != 1 || st.Updated != 1 || st.Deleted != 1 {
		t.Fatalf("应为 1/1/1，实际 %+v", st)
	}
	if len(st.DeletedF) != 1 || st.DeletedF[0] != "en/old-page/index.html" {
		t.Fatalf("孤儿清单不对：%v", st.DeletedF)
	}

	// 孤儿真的删掉了，新文件真的写进去了
	if _, err := os.Stat(filepath.Join(dst, "en", "old-page", "index.html")); !os.IsNotExist(err) {
		t.Error("孤儿文件应被删除")
	}
	b, _ := os.ReadFile(filepath.Join(dst, "media", "a.11111111.png"))
	if string(b) != "AAA" {
		t.Error("新文件内容不对")
	}

	// 幂等：立即再跑一次，应全部 kept
	st2, err := Sync(src, dst, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st2.Copied != 0 || st2.Updated != 0 || st2.Deleted != 0 || st2.Kept != 2 {
		t.Fatalf("第二次应幂等，实际 %+v", st2)
	}
}

func TestDryRunTouchesNothing(t *testing.T) {
	src, dst := setup(t)
	if _, err := Sync(src, dst, Options{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dst, "en", "index.html"))
	if string(b) != "old" {
		t.Error("dry-run 不应写盘")
	}
	if _, err := os.Stat(filepath.Join(dst, "en", "old-page", "index.html")); err != nil {
		t.Error("dry-run 不应删除孤儿")
	}
}

func TestManifest(t *testing.T) {
	src, dst := setup(t)
	if _, err := Sync(src, dst, Options{Manifest: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "deploy-manifest.json")); err != nil {
		t.Error("清单应写入目标目录")
	}
}
