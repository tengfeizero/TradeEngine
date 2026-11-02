package studio

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tradeengine/tradeengine/internal/contentdb"
	"github.com/tradeengine/tradeengine/internal/server"
)

// Options 是 Web Studio 的装配参数。
type Options struct {
	Root      string // 站点根目录（预览产物位置）
	DBPath    string // 内容库路径
	PublicDir string // 构建产物目录（预览静态服务）
	Version   string
	Log       *slog.Logger

	// Rebuild 由外部注入（cmd 用 build.RunWithLoader(DBLoader) 实现）。
	// 注入而不是 studio 内部 import build：保持本包只依赖 contentdb，可独立测试。
	Rebuild func() (BuildResult, error)
}

// BuildResult 是一次「构建预览」的回显。
type BuildResult struct {
	OK       bool
	Pages    int
	Rebuilt  string
	Errors   []string
	Warnings []string
}

// session 是当前登录态（经 context 传递）。
type session struct {
	Username string
	Role     contentdb.Role
	CSRF     string
}

const sessionCookie = "studio_session"

type studioHandler struct {
	opts Options
	db   *contentdb.DB
	tmpl *template.Template
}

// Server 是装配完成的 Web Studio：路由器 + 可关闭的内容库连接。
// 单独包一层是因为 Close 挂在连接上，而 ServeHTTP 在 mux 上。
type Server struct {
	handler http.Handler
	db      *contentdb.DB
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// Close 释放内容库连接（进程退出时可省略，测试必须调用——
// 否则 Windows 上临时目录删不掉）。
func (s *Server) Close() error { return s.db.Close() }

// Handler 装配 Web Studio。返回错误表示模板或数据库初始化失败。
func Handler(opts Options) (http.Handler, error) {
	db, err := contentdb.Open(opts.DBPath)
	if err != nil {
		return nil, err
	}
	h := &studioHandler{opts: opts, db: db, tmpl: parseTemplates()}
	mux := http.NewServeMux()
	// 认证页（不走会话中间件）
	mux.HandleFunc("GET /studio/login", h.loginPage)
	mux.HandleFunc("POST /studio/login", h.loginSubmit)
	// 主界面
	mux.Handle("GET /studio/{$}", h.authed(h.dash))
	mux.Handle("GET /studio/entities/{table}", h.authed(h.listPage))
	mux.Handle("GET /studio/entities/{table}/{slug}", h.authed(h.editPage))
	mux.Handle("POST /studio/entities/{table}/{slug}/save", h.authed(h.save))
	mux.Handle("POST /studio/entities/{table}/{slug}/action", h.authed(h.action))
	mux.Handle("GET /studio/settings", h.authed(h.settingsPage))
	mux.Handle("POST /studio/settings/save", h.authed(h.settingsSave))
	// 用户与审计（admin only 的逻辑在 handler 内部再判一次，路径误漏不如逻辑闸）
	mux.Handle("GET /studio/users", h.authed(h.usersPage))
	mux.Handle("POST /studio/users/add", h.authed(h.userAdd))
	mux.Handle("POST /studio/users/delete", h.authed(h.userDelete))
	mux.Handle("GET /studio/audit", h.authed(h.auditPage))
	// 构建预览
	mux.Handle("POST /studio/build", h.authed(h.build))
	mux.Handle("/preview/", http.StripPrefix("/preview/", h.authed(h.preview)))

	mux.HandleFunc("GET /studio", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/studio/", http.StatusFound)
	})
	return &Server{handler: mux, db: db}, nil
}

// authed 包一层会话校验 + CSRF（POST）。
func (s *studioHandler) authed(next func(http.ResponseWriter, *http.Request, session)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie(sessionCookie)
		if err == nil {
			if username, role, csrf, ok := s.db.SessionUser(ck.Value); ok {
				sess := session{Username: username, Role: role, CSRF: csrf}
				if r.Method == http.MethodPost {
					if r.FormValue("csrf") != sess.CSRF {
						http.Error(w, "CSRF 校验失败，请刷新页面重试", http.StatusForbidden)
						return
					}
				}
				next(w, r, sess)
				return
			}
		}
		// 未登录一律去登录页
		http.Redirect(w, r, "/studio/login", http.StatusFound)
	})
}

// ---------------------------------------------------------------- 认证

func (s *studioHandler) loginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login", map[string]any{
		"Err": r.URL.Query().Get("err"),
	})
}

func (s *studioHandler) loginSubmit(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	id, _, err := s.db.Authenticate(username, password)
	if err != nil {
		time.Sleep(400 * time.Millisecond) // 抹平暴力尝试的时间信号
		s.db.Audit(username, "login_fail", "", "", err.Error())
		http.Redirect(w, r, "/studio/login?err="+url.QueryEscape("用户名或口令不正确"), http.StatusFound)
		return
	}
	sess, err := s.db.CreateSession(id)
	if err != nil {
		http.Error(w, "创建会话失败", http.StatusInternalServerError)
		return
	}
	s.db.PurgeSessions()
	s.db.Audit(username, "login", "", "", "")
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: sess.Token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Expires: sess.ExpiresAt,
	})
	http.Redirect(w, r, "/studio/", http.StatusFound)
}

func (s *studioHandler) logout(w http.ResponseWriter, r *http.Request, sess session) {
	if ck, err := r.Cookie(sessionCookie); err == nil {
		s.db.DeleteSession(ck.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/studio/login", http.StatusFound)
}

func (s *studioHandler) isAdmin(sess session) bool { return sess.Role == contentdb.RoleAdmin }

// canEdit 判断角色能否保存某状态实体的内容。
// editor：草稿与待审核可改（改完仍要过审）；已发布内容不能动——
// 「上线的东西被随手改掉」比「改动慢半天」贵得多。
func canEdit(role contentdb.Role, status string) bool {
	if role == contentdb.RoleAdmin {
		return true
	}
	return status == StatusDraft || status == StatusInReview
}

// ---------------------------------------------------------------- 页面

func (s *studioHandler) dash(w http.ResponseWriter, r *http.Request, sess session) {
	type tableStat struct {
		Table, Title string
		Counts       map[string]int
		Total        int
	}
	var stats []tableStat
	for _, t := range tableOrder {
		counts, err := s.db.CountByStatus(t)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		total := 0
		for _, n := range counts {
			total += n
		}
		stats = append(stats, tableStat{Table: t, Title: schemas[t].Title, Counts: counts, Total: total})
	}
	// 待审核清单：跨实体表收集，admin 一眼看到该审什么
	var pending []map[string]string
	for _, t := range tableOrder {
		rows, err := s.db.ListEntity(t, StatusInReview)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, row := range rows {
			pending = append(pending, map[string]string{
				"Table": t, "Title": schemas[t].Title, "Slug": row.Slug,
				"Name": displayTitle(row.Data, s.defaultLang()),
			})
		}
	}
	s.render(w, "dash", map[string]any{
		"Stats":   stats,
		"Pending": pending,
		"Nav":     s.nav(sess, ""),
		"Msg":     r.URL.Query().Get("msg"),
	})
}

// tableOrder 固定实体表顺序（map 遍历无序，界面需要稳定）。
var tableOrder = []string{"products", "pages", "cases", "categories", "scenarios", "certificates"}

func (s *studioHandler) defaultLang() string {
	_, dl := s.db.Meta()
	return dl
}

func (s *studioHandler) listPage(w http.ResponseWriter, r *http.Request, sess session) {
	table := r.PathValue("table")
	schema, ok := SchemaOf(table)
	if !ok {
		http.NotFound(w, r)
		return
	}
	statusFilter := r.URL.Query().Get("status")
	rows, err := s.db.ListEntity(table, statusFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dl := s.defaultLang()
	type rowView struct {
		Slug, Status, StatusLabel, Name string
		Ord                             int
		Locked                          bool
	}
	var rowsView []rowView
	for _, row := range rows {
		rowsView = append(rowsView, rowView{
			Slug: row.Slug, Status: row.Status, StatusLabel: statusLabel[row.Status],
			Name:   displayTitle(row.Data, dl),
			Ord:    row.Ord,
			Locked: !canEdit(sess.Role, row.Status),
		})
	}
	s.render(w, "list", map[string]any{
		"Nav": s.nav(sess, table), "Schema": schema, "Table": table,
		"Rows": rowsView, "StatusFilter": statusFilter,
		"Statuses":   []string{StatusDraft, StatusInReview, StatusPublished},
		"StatusName": statusLabel,
		"Msg":        r.URL.Query().Get("msg"),
		"Err":        r.URL.Query().Get("err"),
	})
}

func (s *studioHandler) editPage(w http.ResponseWriter, r *http.Request, sess session) {
	table, slug := r.PathValue("table"), r.PathValue("slug")
	schema, ok := SchemaOf(table)
	if !ok {
		http.NotFound(w, r)
		return
	}
	row, err := s.db.GetEntity(table, slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	langs, dl := s.db.Meta()
	fields := buildFieldViews(schema, row.Data, langs)
	editable := canEdit(sess.Role, row.Status)
	s.render(w, "edit", map[string]any{
		"Nav": s.nav(sess, table), "Schema": schema, "Table": table, "Slug": slug,
		"Fields": fields, "Langs": langs, "DefaultLang": dl,
		"Status": row.Status, "StatusLabel": statusLabel[row.Status],
		"Editable": editable,
		"Actions":   s.actionsFor(sess, row.Status),
		"CSRF":      sess.CSRF,
		"Err":       r.URL.Query().Get("err"),
	})
}

// actionsFor 返回当前角色在当前状态下可用的审核动作。
func (s *studioHandler) actionsFor(sess session, status string) []map[string]string {
	admin := s.isAdmin(sess)
	var out []map[string]string
	switch status {
	case StatusDraft:
		out = append(out, map[string]string{"Do": "submit", "Label": "提交审核"})
	case StatusInReview:
		if admin {
			out = append(out,
				map[string]string{"Do": "publish", "Label": "通过并发布"},
				map[string]string{"Do": "reject", "Label": "退回草稿"})
		}
	case StatusPublished:
		if admin {
			out = append(out, map[string]string{"Do": "unpublish", "Label": "下线（转草稿）"})
		}
	}
	return out
}

func (s *studioHandler) settingsPage(w http.ResponseWriter, r *http.Request, sess session) {
	data, err := s.db.GetSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	langs, dl := s.db.Meta()
	fields := buildSettingsViews(data, langs)
	s.render(w, "settings", map[string]any{
		"Nav": s.nav(sess, "settings"), "Fields": fields,
		"Langs": langs, "DefaultLang": dl,
		"Editable": s.isAdmin(sess), "CSRF": sess.CSRF,
		"Msg": r.URL.Query().Get("msg"), "Err": r.URL.Query().Get("err"),
	})
}

// ---------------------------------------------------------------- 写操作

func (s *studioHandler) save(w http.ResponseWriter, r *http.Request, sess session) {
	table, slug := r.PathValue("table"), r.PathValue("slug")
	if _, ok := SchemaOf(table); !ok {
		http.NotFound(w, r)
		return
	}
	row, err := s.db.GetEntity(table, slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if !canEdit(sess.Role, row.Status) {
		s.db.Audit(sess.Username, "save_denied", table, slug, "已发布内容仅管理员可改")
		http.Error(w, "已发布内容仅管理员可修改（先让管理员下线，或走新的草稿）", http.StatusForbidden)
		return
	}
	langs, _ := s.db.Meta()
	payload, err := applyForm(table, []byte(row.Data), r.Form, langs)
	if err != nil {
		http.Redirect(w, r,
			fmt.Sprintf("/studio/entities/%s/%s?err=%s", table, slug, url.QueryEscape(err.Error())),
			http.StatusFound)
		return
	}
	if err := s.db.PutEntity(table, payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.db.Audit(sess.Username, "save", table, slug, "")
	http.Redirect(w, r,
		fmt.Sprintf("/studio/entities/%s/%s?msg=%s", table, slug, url.QueryEscape("已保存")),
		http.StatusFound)
}

func (s *studioHandler) action(w http.ResponseWriter, r *http.Request, sess session) {
	table, slug, do := r.PathValue("table"), r.PathValue("slug"), r.FormValue("do")
	row, err := s.db.GetEntity(table, slug)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	cur := row.Status
	admin := s.isAdmin(sess)

	// 状态机：合法性 + 角色权限在这里集中裁决
	var next, need string
	var adminOnly bool
	switch do {
	case "submit":
		need, next = StatusDraft, StatusInReview
	case "publish":
		need, next, adminOnly = StatusInReview, StatusPublished, true
	case "reject":
		need, next, adminOnly = StatusInReview, StatusDraft, true
	case "unpublish":
		need, next, adminOnly = StatusPublished, StatusDraft, true
	default:
		http.Error(w, "未知动作", http.StatusBadRequest)
		return
	}
	if cur != need {
		http.Redirect(w, r,
			fmt.Sprintf("/studio/entities/%s/%s?err=%s", table, slug,
				url.QueryEscape(fmt.Sprintf("当前状态是 %s，不能执行该动作", statusLabel[cur]))),
			http.StatusFound)
		return
	}
	if adminOnly && !admin {
		s.db.Audit(sess.Username, do+"_denied", table, slug, "需要管理员")
		http.Error(w, "该动作需要管理员角色", http.StatusForbidden)
		return
	}
	if err := s.db.SetStatus(table, slug, next); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.db.Audit(sess.Username, do, table, slug, cur+" → "+next)
	http.Redirect(w, r,
		fmt.Sprintf("/studio/entities/%s/%s?msg=%s", table, slug,
			url.QueryEscape(fmt.Sprintf("%s：已变为「%s」", do, statusLabel[next]))),
		http.StatusFound)
}

func (s *studioHandler) settingsSave(w http.ResponseWriter, r *http.Request, sess session) {
	if !s.isAdmin(sess) {
		http.Error(w, "站点设置仅管理员可修改", http.StatusForbidden)
		return
	}
	data, err := s.db.GetSettings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	langs, _ := s.db.Meta()
	payload, err := applySettingsForm([]byte(data), r.Form, langs)
	if err != nil {
		http.Redirect(w, r, "/studio/settings?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	if err := s.db.PutSettings(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.db.Audit(sess.Username, "save", "settings", "", "")
	http.Redirect(w, r, "/studio/settings?msg="+url.QueryEscape("已保存"), http.StatusFound)
}

// ---------------------------------------------------------------- 用户与审计

func (s *studioHandler) usersPage(w http.ResponseWriter, r *http.Request, sess session) {
	if !s.isAdmin(sess) {
		http.Error(w, "仅管理员可管理用户", http.StatusForbidden)
		return
	}
	users, err := s.db.ListUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "users", map[string]any{
		"Nav": s.nav(sess, "users"), "Users": users, "CSRF": sess.CSRF,
		"Self": sess.Username,
		"Msg":  r.URL.Query().Get("msg"), "Err": r.URL.Query().Get("err"),
	})
}

func (s *studioHandler) userAdd(w http.ResponseWriter, r *http.Request, sess session) {
	if !s.isAdmin(sess) {
		http.Error(w, "仅管理员可管理用户", http.StatusForbidden)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	role := r.FormValue("role")
	if err := s.db.CreateUser(username, r.FormValue("password"), role); err != nil {
		http.Redirect(w, r, "/studio/users?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	s.db.Audit(sess.Username, "user_add", "users", username, role)
	http.Redirect(w, r, "/studio/users?msg="+url.QueryEscape("已创建 "+username), http.StatusFound)
}

func (s *studioHandler) userDelete(w http.ResponseWriter, r *http.Request, sess session) {
	if !s.isAdmin(sess) {
		http.Error(w, "仅管理员可管理用户", http.StatusForbidden)
		return
	}
	var id int64
	if _, err := fmt.Sscanf(r.FormValue("id"), "%d", &id); err != nil {
		http.Redirect(w, r, "/studio/users?err="+url.QueryEscape("参数错误"), http.StatusFound)
		return
	}
	users, _ := s.db.ListUsers()
	name := ""
	for _, u := range users {
		if u.ID == id {
			name = u.Username
		}
	}
	if name == sess.Username {
		http.Redirect(w, r, "/studio/users?err="+url.QueryEscape("不能删除自己的账号"), http.StatusFound)
		return
	}
	if err := s.db.DeleteUser(id); err != nil {
		http.Redirect(w, r, "/studio/users?err="+url.QueryEscape(err.Error()), http.StatusFound)
		return
	}
	s.db.Audit(sess.Username, "user_del", "users", name, "")
	http.Redirect(w, r, "/studio/users?msg="+url.QueryEscape("已删除 "+name), http.StatusFound)
}

func (s *studioHandler) auditPage(w http.ResponseWriter, r *http.Request, sess session) {
	entries, err := s.db.ListAudit(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "audit", map[string]any{
		"Nav": s.nav(sess, "audit"), "Entries": entries,
	})
}

// ---------------------------------------------------------------- 构建预览

func (s *studioHandler) build(w http.ResponseWriter, r *http.Request, sess session) {
	if s.opts.Rebuild == nil {
		http.Error(w, "未配置构建", http.StatusInternalServerError)
		return
	}
	res, err := s.opts.Rebuild()
	if err != nil {
		s.db.Audit(sess.Username, "build", "", "", err.Error())
		http.Redirect(w, r, "/studio/?err="+url.QueryEscape("构建异常："+err.Error()), http.StatusFound)
		return
	}
	action := "build_ok"
	if !res.OK {
		action = "build_fail"
	}
	s.db.Audit(sess.Username, action, "", "", fmt.Sprintf("pages=%d errs=%d", res.Pages, len(res.Errors)))
	q := url.Values{}
	if res.OK {
		q.Set("msg", fmt.Sprintf("构建成功：%d 页，%s，预览见 /preview/", res.Pages, res.Rebuilt))
	} else {
		q.Set("err", fmt.Sprintf("构建失败（%d 个错误，第一个：%s）", len(res.Errors), strings.Join(res.Errors, "；")))
	}
	http.Redirect(w, r, "/studio/?"+q.Encode(), http.StatusFound)
}

func (s *studioHandler) preview(w http.ResponseWriter, r *http.Request, _ session) {
	if s.opts.PublicDir == "" {
		http.Error(w, "预览未配置", http.StatusNotFound)
		return
	}
	server.Static(s.opts.PublicDir).ServeHTTP(w, r)
}

// ---------------------------------------------------------------- 辅助

type navItem struct {
	Href, Label string
	Active      bool
}

func (s *studioHandler) nav(sess session, active string) map[string]any {
	items := []navItem{{Href: "/studio/", Label: "总览", Active: active == ""}}
	for _, t := range tableOrder {
		items = append(items, navItem{
			Href: "/studio/entities/" + t, Label: schemas[t].Title,
			Active: active == t,
		})
	}
	items = append(items, navItem{Href: "/studio/settings", Label: "站点设置", Active: active == "settings"})
	if s.isAdmin(sess) {
		items = append(items, navItem{Href: "/studio/users", Label: "用户", Active: active == "users"})
	}
	items = append(items, navItem{Href: "/studio/audit", Label: "审计日志", Active: active == "audit"})
	return map[string]any{
		"Items":    items,
		"Username": sess.Username,
		"Role":     string(sess.Role),
		"Admin":    s.isAdmin(sess),
		"CSRF":     sess.CSRF,
		"Version":  s.opts.Version,
	}
}

// displayTitle 从 payload 里取展示标题（默认语言，缺失回退任意非空）。
func displayTitle(payload, defaultLang string) string {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(payload), &doc); err != nil {
		return ""
	}
	return i18nOf(doc, "title", defaultLang)
}

// i18nOf 取 doc[key]（map[string]any）的指定语言值，带回退。
func i18nOf(doc map[string]any, key, lang string) string {
	m, ok := doc[key].(map[string]any)
	if !ok {
		if s, ok := doc[key].(string); ok {
			return s
		}
		return ""
	}
	if v, _ := m[lang].(string); v != "" {
		return v
	}
	// 回退：任意非空值
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if v, _ := m[k].(string); v != "" {
			return v
		}
	}
	return ""
}

func (s *studioHandler) render(w http.ResponseWriter, page string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, page, data); err != nil {
		// 此时响应头已发出，改不了状态码；把错误大声写进日志与响应尾部，
		// 绝不能让模板错误静默截断页面——那是最难排查的反馈黑洞
		s.opts.Log.Error("模板渲染失败", "page", page, "err", err)
		fmt.Fprintf(w, "\n<!-- 渲染出错（见服务端日志）：%s -->", template.HTMLEscapeString(err.Error()))
	}
}
