// Package admin 提供本地内容后台：浏览器里改 YAML、保存即重建、左侧预览。
//
// 边界（刻意的取舍）：
//
//   - 只做 YAML 文本编辑，不做表单化编辑。客户站的内容字段有 40+，
//     表单化意味着为每个字段做控件、校验和迁移——那是 M3 的 Web Studio，
//     现在投入会把「够用」做成「半成品」。文本编辑对开发者交付也更快。
//   - 只允许绑定回环地址。admin 端点能读写任意内容文件，暴露到局域网
//     等于把站点源码交出去；部署期的内容修改走 git，不走 admin。
//   - 保存时先做 YAML 语法预检，再做备份，然后写盘 + 重建。重建结果
//     （错误/警告）直接回显在编辑器下方——「保存了但没生效」是最消耗
//     信任的反馈黑洞。
package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tengfeizero/TradeEngine/internal/server"
	"github.com/tengfeizero/TradeEngine/internal/store"
)

// Options 是后台装配参数。
type Options struct {
	Root      string // 站点根目录
	PublicDir string // 构建产物目录（预览与静态兜底）
	DBPath    string // 询盘库；为空时隐藏询盘页签
	Rebuild   func() (BuildStatus, error)
	Version   string
	Log       *slog.Logger
}

// BuildStatus 是一次重建后回显给编辑器的摘要。
type BuildStatus struct {
	Errors  []Issue `json:"errors"`
	Warns   []Issue `json:"warnings"`
	Pages   int     `json:"pages"`
	OK      bool    `json:"ok"`
	Rebuilt string  `json:"rebuilt"` // 时间戳
}

// Issue 是一条校验消息（与 validate/BuildReport 的结构对齐但独立定义，
// 避免 admin 依赖 build 内部类型）。
type Issue struct {
	Code    string `json:"code"`
	Entity  string `json:"entity"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Handler 组装后台路由。其余路径全部回落到静态产物服务。
func Handler(opts Options) http.Handler {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin", uiPage)
	mux.HandleFunc("GET /admin/", uiPage)
	mux.HandleFunc("GET /admin/api/files", listFiles(opts))
	mux.HandleFunc("GET /admin/api/file", readFile(opts))
	mux.HandleFunc("POST /admin/api/save", save(opts))
	mux.HandleFunc("POST /admin/api/new", createFile(opts))
	mux.HandleFunc("POST /admin/api/build", rebuild(opts))
	mux.HandleFunc("GET /admin/api/inquiries", inquiries(opts))
	mux.Handle("/", server.Static(opts.PublicDir))
	return mux
}

// ---------------------------------------------------------------- 文件清单

var groupOrder = []string{"配置", "主题", "页面", "产品", "案例", "其他内容"}

type fileEntry struct {
	Path   string `json:"path"`
	Group  string `json:"group"`
	Label  string `json:"label"`
	Status string `json:"status"` // published / draft / ""（配置文件）
}

func listFiles(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var files []fileEntry
		add := func(rel, group, label string) {
			if _, err := os.Stat(filepath.Join(opts.Root, filepath.FromSlash(rel))); err == nil {
				files = append(files, fileEntry{Path: rel, Group: group, Label: label, Status: statusOf(opts.Root, rel)})
			}
		}
		add("settings.yaml", "配置", "站点设置")
		add("engine.yaml", "配置", "引擎配置")
		add("theme/theme.yaml", "主题", "主题 token")
		add("content/categories.yaml", "其他内容", "产品分类")
		add("content/scenarios.yaml", "其他内容", "应用场景")
		add("content/certificates.yaml", "其他内容", "资质证书")

		addDir := func(dir, group, prefix string) {
			entries, err := os.ReadDir(filepath.Join(opts.Root, filepath.FromSlash(dir)))
			if err != nil {
				return
			}
			for _, e := range entries {
				if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
					continue
				}
				rel := path.Join(dir, e.Name())
				files = append(files, fileEntry{
					Path: rel, Group: group,
					Label:  prefix + strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
					Status: statusOf(opts.Root, rel),
				})
			}
		}
		addDir("content/pages", "页面", "")
		addDir("content/products", "产品", "")
		addDir("content/cases", "案例", "")

		// 预览面板需要知道默认语言，从 settings 探一下；读不到就按 en
		def := "en"
		if b, err := os.ReadFile(filepath.Join(opts.Root, "settings.yaml")); err == nil {
			var probe struct {
				DefaultLang string   `yaml:"default_lang"`
				Langs       []string `yaml:"langs"`
			}
			if yaml.Unmarshal(b, &probe) == nil && probe.DefaultLang != "" {
				def = probe.DefaultLang
			}
		}

		sort.Slice(files, func(i, j int) bool {
			gi, gj := groupIndex(files[i].Group), groupIndex(files[j].Group)
			if gi != gj {
				return gi < gj
			}
			return files[i].Path < files[j].Path
		})
		writeJSON(w, map[string]any{"files": files, "root": mustAbs(opts.Root), "default_lang": def})
	}
}

func groupIndex(g string) int {
	for i, x := range groupOrder {
		if x == g {
			return i
		}
	}
	return len(groupOrder)
}

func statusOf(root, rel string) string {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	var probe struct {
		Status string `yaml:"status"`
	}
	_ = yaml.Unmarshal(b, &probe)
	return probe.Status
}

// ---------------------------------------------------------------- 读 / 写

func readFile(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rel, ok := sanitizePath(opts, r.URL.Query().Get("p"))
		if !ok {
			http.Error(w, "非法路径", http.StatusBadRequest)
			return
		}
		b, err := os.ReadFile(filepath.Join(opts.Root, filepath.FromSlash(rel)))
		if err != nil {
			http.Error(w, "读取失败："+err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"path": rel, "content": string(b)})
	}
}

// sanitizePath 校验可编辑路径：必须在白名单目录内、是 YAML、无穿越。
//
// admin 能写文件，路径校验是它唯一的安全边界，必须先于任何磁盘操作。
func sanitizePath(opts Options, p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" || strings.Contains(p, "..") || strings.Contains(p, "\\") {
		return "", false
	}
	p = path.Clean("/" + p)
	p = strings.TrimPrefix(p, "/")
	switch {
	case p == "settings.yaml", p == "engine.yaml", p == "theme/theme.yaml",
		strings.HasPrefix(p, "content/pages/"),
		strings.HasPrefix(p, "content/products/"),
		strings.HasPrefix(p, "content/cases/"),
		p == "content/categories.yaml", p == "content/scenarios.yaml", p == "content/certificates.yaml":
	default:
		return "", false
	}
	if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
		return "", false
	}
	return p, true
}

func save(opts Options) http.HandlerFunc {
	type req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "请求体不是 JSON", http.StatusBadRequest)
			return
		}
		rel, ok := sanitizePath(opts, in.Path)
		if !ok {
			http.Error(w, "非法路径", http.StatusBadRequest)
			return
		}
		// 语法预检：解析失败的 YAML 绝不落盘——半路保存（编辑到一半）是常态，
		// 但客户站点的构建产物不能因为一次误保存而挂掉。
		var node yaml.Node
		if err := yaml.Unmarshal([]byte(in.Content), &node); err != nil {
			writeJSON(w, map[string]any{"ok": false, "syntax": err.Error()})
			return
		}
		if len(node.Content) == 0 {
			writeJSON(w, map[string]any{"ok": false, "syntax": "文件是空的"})
			return
		}

		full := filepath.Join(opts.Root, filepath.FromSlash(rel))
		if err := backup(opts.Root, full, rel); err != nil {
			http.Error(w, "备份失败："+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(full, []byte(in.Content), 0o644); err != nil {
			http.Error(w, "写入失败："+err.Error(), http.StatusInternalServerError)
			return
		}

		st, err := opts.Rebuild()
		if err != nil {
			// 重建失败仍然算保存成功：文件在盘上，用户看得到错误并继续改
			writeJSON(w, map[string]any{"ok": true, "saved": rel, "rebuild_error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "saved": rel, "build": st})
	}
}

// backup 把原文件复制进 <root>/.backup/admin/<时间戳>/。
// 备份集中在站点根而不是各内容目录旁，避免污染 loader 的扫描范围。
func backup(root, full, rel string) error {
	orig, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 新建文件没有可备份的旧版
		}
		return err
	}
	dst := filepath.Join(root, ".backup", "admin", stamp(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, orig, 0o644)
}

func stamp() string {
	return time.Now().Format("20060102-150405")
}

// ---------------------------------------------------------------- 新建

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,60}$`)

func createFile(opts Options) http.HandlerFunc {
	type req struct {
		Kind string `json:"kind"` // product / page / case
		Slug string `json:"slug"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var in req
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "请求体不是 JSON", http.StatusBadRequest)
			return
		}
		if !slugRe.MatchString(in.Slug) {
			http.Error(w, "slug 只允许小写字母、数字和中划线（2–61 位）", http.StatusBadRequest)
			return
		}
		var dir, tmpl string
		switch in.Kind {
		case "product":
			dir, tmpl = "content/products", productTemplate
		case "page":
			dir, tmpl = "content/pages", pageTemplate
		case "case":
			dir, tmpl = "content/cases", caseTemplate
		default:
			http.Error(w, "kind 必须是 product / page / case", http.StatusBadRequest)
			return
		}
		rel := path.Join(dir, in.Slug+".yaml")
		full := filepath.Join(opts.Root, filepath.FromSlash(rel))
		if _, err := os.Stat(full); err == nil {
			http.Error(w, rel+" 已存在", http.StatusConflict)
			return
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			http.Error(w, "创建目录失败", http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(full, []byte(fmt.Sprintf(tmpl, in.Slug)), 0o644); err != nil {
			http.Error(w, "写入失败", http.StatusInternalServerError)
			return
		}
		_, _ = opts.Rebuild()
		writeJSON(w, map[string]any{"ok": true, "path": rel})
	}
}

// 三个模板刻意只放**必填**字段：生成的文件必须能直接构建通过，
// 占位文案提示用户替换，而不是一堆注释掉的选填项。
const productTemplate = `slug: %s
status: draft
category: main

title:
  en: New product
  zh: 新产品
summary:
  en: Replace this one-line summary (what it is, key benefit).
  zh: 替换这句一句话简介（是什么、核心卖点）。
description:
  en: |
    Replace with the full description: material, workmanship, use.
  zh: |
    替换为完整描述：材质、工艺、用途。
lead_time:
  en: 15-20 days after deposit
  zh: 定金后 15-20 天
packaging:
  en: 1 pc / carton
  zh: 1 台 / 纸箱

moq: 100
moq_unit: pcs

specs:
  - group:
      en: Product data
      zh: 产品参数
    items:
      - key: material
        label:
          en: Material
          zh: 材质
        value:
          en: "-"
          zh: "-"
`

const pageTemplate = `slug: %s
status: draft

title:
  en: New page
  zh: 新页面

blocks:
  - type: richtext
    title:
      en: New page
      zh: 新页面
    body:
      en: |
        Write the page content here.
      zh: |
        在这里写页面正文。
`

const caseTemplate = `slug: %s
status: draft

title:
  en: New case study
  zh: 新案例
country: ""
industry:
  en: Industry
  zh: 行业
challenge:
  en: What problem did the customer face?
  zh: 客户当时面对什么问题？
solution:
  en: What did we supply and how was it deployed?
  zh: 我们供了什么、怎么落地？
result:
  en: What measurable outcome followed?
  zh: 带来了什么可量化的结果？
products: []
`

// ---------------------------------------------------------------- 重建 / 询盘

func rebuild(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := opts.Rebuild()
		if err != nil {
			writeJSON(w, map[string]any{"ok": false, "rebuild_error": err.Error()})
			return
		}
		writeJSON(w, map[string]any{"ok": st.OK, "build": st})
	}
}

func inquiries(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if opts.DBPath == "" {
			writeJSON(w, map[string]any{"items": []any{}})
			return
		}
		st, err := store.Open(opts.DBPath)
		if err != nil {
			// 库还没建（一条询盘都没有）不是错误
			writeJSON(w, map[string]any{"items": []any{}})
			return
		}
		defer st.Close()
		rows, err := st.List(store.ListOptions{Limit: 30})
		if err != nil {
			http.Error(w, "查询失败："+err.Error(), http.StatusInternalServerError)
			return
		}
		type row struct {
			ID        string `json:"id"`
			Time      string `json:"time"`
			Lang      string `json:"lang"`
			Name      string `json:"name"`
			Company   string `json:"company"`
			Email     string `json:"email"`
			Product   string `json:"product"`
			Message   string `json:"message"`
			SpamScore int    `json:"spam_score"`
			Status    string `json:"status"`
		}
		items := make([]row, 0, len(rows))
		for _, q := range rows {
			// IPHash / UserAgent / UTM 不出给浏览器：admin 的页面权限
			// 只有「回环地址」这一道防线，数据面要按最小暴露原则裁剪。
			items = append(items, row{
				ID: q.ID, Time: q.CreatedAt.Format("2006-01-02 15:04"),
				Lang: q.Lang, Name: q.Name, Company: q.Company, Email: q.Email,
				Product: q.ProductTitle, Message: q.Message,
				SpamScore: q.SpamScore, Status: q.Status,
			})
		}
		writeJSON(w, map[string]any{"items": items})
	}
}

// ---------------------------------------------------------------- 工具

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func uiPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(adminHTML))
}
