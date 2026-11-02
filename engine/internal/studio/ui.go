package studio

import "html/template"

// studioHTML 是 Web Studio 的全部界面：服务端渲染、零 JS、纯表单可提交。
//
// 与 admin 的取舍不同：admin 面向开发者（单页应用 + JSON 端点），
// studio 面向客户——表单编辑要的是「每一项都能看懂」，不是操作速度。
// 整段是 Go raw string，内部不能出现反引号。
const studioHTML = `{{define "head"}}<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Web Studio</title>
<style>
:root{--bg:#f6f7f5;--panel:#fff;--ink:#1e2b23;--sub:#6b7a70;--line:#e3e7e3;--pri:#2f7d5b;--bad:#c0392b;--warn:#b07d18;--chip:#eef4ef}
*{box-sizing:border-box;margin:0}
body{font:14px/1.65 -apple-system,"Segoe UI","Microsoft YaHei",sans-serif;background:var(--bg);color:var(--ink);min-height:100vh;display:flex;flex-direction:column}
header{display:flex;align-items:center;gap:14px;padding:10px 18px;background:var(--panel);border-bottom:1px solid var(--line);flex-wrap:wrap}
header .brand{font-weight:700;color:var(--pri)}
header .user{margin-left:auto;color:var(--sub);font-size:12px;display:flex;align-items:center;gap:8px}
nav{display:flex;gap:4px;flex-wrap:wrap}
nav a{padding:5px 12px;border-radius:6px;color:var(--ink);text-decoration:none;font-size:13px}
nav a:hover{background:var(--chip)}
nav a.on{background:var(--pri);color:#fff}
main{flex:1;width:min(1060px,100%);margin:0 auto;padding:22px 18px 60px}
h1{font-size:19px;margin-bottom:4px}
h2{font-size:15px;margin:22px 0 10px}
.sub{color:var(--sub);font-size:12px;margin-bottom:16px}
.card{background:var(--panel);border:1px solid var(--line);border-radius:10px;padding:16px 18px;margin-bottom:14px}
.cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(170px,1fr));gap:12px}
.cards a{text-decoration:none;color:var(--ink)}
.stat b{font-size:22px;display:block}
.stat .lbl{color:var(--sub);font-size:12px;display:block}
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{text-align:left;padding:7px 10px;border-bottom:1px solid var(--line)}
th{color:var(--sub);font-weight:500;font-size:12px}
tr:hover td{background:#fafcf9}
a{color:var(--pri)}
code{font-size:12px;background:var(--chip);padding:1px 5px;border-radius:4px}
.badge{display:inline-block;padding:1px 9px;border-radius:99px;font-size:12px}
.b-draft{background:#f0f1f0;color:var(--sub)}
.b-in_review{background:#fdf3e0;color:var(--warn)}
.b-published{background:#e8f3ec;color:var(--pri)}
.msg{background:#e8f3ec;border:1px solid #cfe5d8;color:var(--pri);padding:8px 14px;border-radius:8px;margin-bottom:14px}
.err{background:#fdecea;border:1px solid #f5c9c2;color:var(--bad);padding:8px 14px;border-radius:8px;margin-bottom:14px}
label{display:block;font-size:13px;margin:14px 0 4px;font-weight:600}
label small{color:var(--sub);font-weight:400;margin-left:6px}
input[type=text],input[type=number],input[type=password],textarea,select{width:100%;padding:8px 10px;border:1px solid var(--line);border-radius:8px;font:inherit;background:#fff}
textarea{min-height:90px;resize:vertical}
.langtag{display:inline-block;background:var(--chip);border:1px solid var(--line);border-radius:6px 0 0 6px;padding:7px 9px;color:var(--sub);font-size:12px;white-space:nowrap}
.i18nrow{display:flex;margin-bottom:6px}
.i18nrow input,.i18nrow textarea{border-radius:0 8px 8px 0}
.check{display:flex;align-items:center;gap:8px;margin:12px 0}
.check input,.check select{width:auto}
.btn{display:inline-block;font:inherit;padding:8px 18px;border:1px solid var(--line);background:#fff;border-radius:8px;cursor:pointer;text-decoration:none;color:var(--ink)}
.btn:hover{border-color:var(--pri);color:var(--pri)}
.btn.pri{background:var(--pri);border-color:var(--pri);color:#fff}
.btn.pri:hover{opacity:.9;color:#fff}
.btn.warn{color:var(--bad);border-color:#f5c9c2}
.btnrow{display:flex;gap:8px;margin-top:20px;flex-wrap:wrap;align-items:center}
.locked{background:#fafafa;border:1px dashed var(--line);border-radius:8px;padding:10px 14px;color:var(--sub);font-size:12px;margin-top:14px}
.field-hint{color:var(--sub);font-size:12px;margin-top:2px}
form.inline{display:inline}
.empty{color:var(--sub);padding:18px 0}
</style>
</head>
{{end}}

{{define "login"}}{{template "head"}}
<body>
<main style="max-width:340px;margin-top:12vh">
  <div class="card">
    <h1>Web Studio</h1>
    <p class="sub">内容编辑 · 审核发布</p>
    {{if .Err}}<div class="err">{{.Err}}</div>{{end}}
    <form method="post" action="/studio/login">
      <label>用户名</label>
      <input type="text" name="username" autofocus autocomplete="username">
      <label>口令</label>
      <input type="password" name="password" autocomplete="current-password">
      <div class="btnrow"><button class="btn pri" type="submit">登录</button></div>
    </form>
  </div>
</main>
</body></html>{{end}}

{{define "nav"}}<header>
  <span class="brand">Web Studio</span>
  <nav>
    {{range .Items}}<a href="{{.Href}}"{{if .Active}} class="on"{{end}}>{{.Label}}</a>{{end}}
  </nav>
  <span class="user">{{.Username}}（{{.Role}}）· {{.Version}}
    <form class="inline" method="post" action="/studio/logout">
      <input type="hidden" name="csrf" value="{{.CSRF}}">
      <button class="btn" type="submit">退出</button>
    </form>
  </span>
</header>{{end}}

{{define "dash"}}{{template "head"}}
<body>{{template "nav" .Nav}}
<main>
  <h1>总览</h1>
  <p class="sub">内容改动保存在内容库中；「构建预览」查看效果，engine export 可随时拿回文件。</p>
  {{if .Msg}}<div class="msg">{{.Msg}}</div>{{end}}
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}

  {{if .Pending}}
  <h2>待审核（{{len .Pending}}）</h2>
  <div class="card">
    <table>
      <tr><th>类型</th><th>标题</th><th>slug</th><th></th></tr>
      {{range .Pending}}<tr>
        <td>{{.Title}}</td><td>{{.Name}}</td><td><code>{{.Slug}}</code></td>
        <td><a href="/studio/entities/{{.Table}}/{{.Slug}}">去审核 →</a></td>
      </tr>{{end}}
    </table>
  </div>
  {{end}}

  <h2>内容</h2>
  <div class="cards">
    {{range .Stats}}<a href="/studio/entities/{{.Table}}"><div class="card stat">
      <b>{{.Total}}</b>
      <span class="lbl">{{.Title}}</span>
      <span class="lbl">草稿 {{index .Counts "draft"}} · 待审 {{index .Counts "in_review"}} · 已发布 {{index .Counts "published"}}</span>
    </div></a>{{end}}
  </div>

  <div class="btnrow">
    <form class="inline" method="post" action="/studio/build"><input type="hidden" name="csrf" value="{{.Nav.CSRF}}"><button class="btn pri" type="submit">构建预览</button></form>
    <a class="btn" href="/preview/" target="_blank">打开预览站</a>
  </div>
</main>
</body></html>{{end}}

{{define "list"}}{{template "head"}}
<body>{{template "nav" .Nav}}
<main>
  <h1>{{.Schema.Title}}</h1>
  <p class="sub">
    状态筛选：
    <a href="/studio/entities/{{.Table}}">全部</a> ·
    {{range $st := .Statuses}}<a href="/studio/entities/{{$.Table}}?status={{$st}}">{{index $.StatusName $st}}</a> · {{end}}
  </p>
  {{if .Msg}}<div class="msg">{{.Msg}}</div>{{end}}
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}
  <div class="card">
    {{if .Rows}}<table>
      <tr><th>排序</th><th>标题</th><th>slug</th><th>状态</th><th></th></tr>
      {{range .Rows}}<tr>
        <td>{{.Ord}}</td>
        <td><a href="/studio/entities/{{$.Table}}/{{.Slug}}">{{.Name}}</a></td>
        <td><code>{{.Slug}}</code></td>
        <td><span class="badge b-{{.Status}}">{{.StatusLabel}}</span></td>
        <td>{{if .Locked}}<span class="field-hint">已锁定</span>{{else}}<a href="/studio/entities/{{$.Table}}/{{.Slug}}">编辑</a>{{end}}</td>
      </tr>{{end}}
    </table>{{else}}<p class="empty">暂无内容</p>{{end}}
  </div>
</main>
</body></html>{{end}}

{{define "edit"}}{{template "head"}}
<body>{{template "nav" .Nav}}
<main>
  <h1>{{.Schema.Title}}：<code>{{.Slug}}</code>
    <span class="badge b-{{.Status}}">{{.StatusLabel}}</span></h1>
  <p class="sub">参数表、页面区块、SEO 等不在表单里的字段，保存时原样保留。</p>
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}

  {{if .Editable}}
  <form method="post" action="/studio/entities/{{.Table}}/{{.Slug}}/save">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    {{range $f := .Fields}}
      {{if $f.IsI18n}}
        <label>{{$f.Label}}</label>
        {{range $lang := $f.LangOrder}}
        <div class="i18nrow"><span class="langtag">{{$lang}}{{if eq $lang $.DefaultLang}} ★{{end}}</span>
          {{if eq $f.Kind "i18n_textarea"}}<textarea name="{{$f.Name}}.{{$lang}}">{{index $f.LangValues $lang}}</textarea>
          {{else}}<input type="text" name="{{$f.Name}}.{{$lang}}" value="{{index $f.LangValues $lang}}">{{end}}
        </div>
        {{end}}
      {{else if eq $f.Kind "textarea"}}
        <label>{{$f.Label}}</label>
        <textarea name="{{$f.Name}}">{{$f.Value}}</textarea>
      {{else if eq $f.Kind "list"}}
        <label>{{$f.Label}} <small>每行一项</small></label>
        <textarea name="{{$f.Name}}">{{$f.Value}}</textarea>
      {{else if eq $f.Kind "bool"}}
        <div class="check"><input type="checkbox" name="{{$f.Name}}"{{if $f.Checked}} checked{{end}}> <span>{{$f.Label}}</span></div>
      {{else if eq $f.Kind "num"}}
        <label>{{$f.Label}}{{if $f.Help}} <small>{{$f.Help}}</small>{{end}}</label>
        <input type="number" name="{{$f.Name}}" value="{{$f.Value}}">
      {{else}}
        <label>{{$f.Label}}{{if $f.Help}} <small>{{$f.Help}}</small>{{end}}</label>
        <input type="text" name="{{$f.Name}}" value="{{$f.Value}}">
      {{end}}
    {{end}}
    <div class="btnrow"><button class="btn pri" type="submit">保存</button></div>
  </form>
  {{else}}
  <div class="locked">该内容已发布，编辑不可修改。管理员可在下方「下线」后开放编辑。</div>
  {{end}}

  {{if .Actions}}
  <h2>审核动作</h2>
  <div class="btnrow">
    {{range $a := .Actions}}
    <form class="inline" method="post" action="/studio/entities/{{$.Table}}/{{$.Slug}}/action">
      <input type="hidden" name="csrf" value="{{$.CSRF}}">
      <input type="hidden" name="do" value="{{$a.Do}}">
      <button class="btn{{if eq $a.Do "publish"}} pri{{end}}" type="submit">{{$a.Label}}</button>
    </form>
    {{end}}
  </div>
  {{end}}
</main>
</body></html>{{end}}

{{define "settings"}}{{template "head"}}
<body>{{template "nav" .Nav}}
<main>
  <h1>站点设置</h1>
  <p class="sub">只包含客户会改的字段；询盘收件人、API、部署等配置由开发者在 settings.yaml 维护。</p>
  {{if .Msg}}<div class="msg">{{.Msg}}</div>{{end}}
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}
  {{if .Editable}}
  <form method="post" action="/studio/settings/save">
    <input type="hidden" name="csrf" value="{{.CSRF}}">
    {{range $f := .Fields}}
      {{if $f.IsI18n}}
        <label>{{$f.Label}}</label>
        {{range $lang := $f.LangOrder}}
        <div class="i18nrow"><span class="langtag">{{$lang}}{{if eq $lang $.DefaultLang}} ★{{end}}</span>
          <input type="text" name="{{$f.Name}}.{{$lang}}" value="{{index $f.LangValues $lang}}">
        </div>
        {{end}}
      {{else}}
        <label>{{$f.Label}}{{if $f.Help}} <small>{{$f.Help}}</small>{{end}}</label>
        <input type="text" name="{{$f.Name}}" value="{{$f.Value}}">
      {{end}}
    {{end}}
    <div class="btnrow"><button class="btn pri" type="submit">保存</button></div>
  </form>
  {{else}}<div class="locked">站点设置仅管理员可修改。</div>{{end}}
</main>
</body></html>{{end}}

{{define "users"}}{{template "head"}}
<body>{{template "nav" .Nav}}
<main>
  <h1>用户</h1>
  <p class="sub">editor 可编辑未发布内容并提交审核；admin 另可发布 / 下线 / 管理用户。</p>
  {{if .Msg}}<div class="msg">{{.Msg}}</div>{{end}}
  {{if .Err}}<div class="err">{{.Err}}</div>{{end}}
  <div class="card">
    <table>
      <tr><th>ID</th><th>用户名</th><th>角色</th><th>创建时间</th><th></th></tr>
      {{range .Users}}<tr>
        <td>{{.ID}}</td><td>{{.Username}}</td>
        <td><span class="badge {{if eq .Role "admin"}}b-published{{else}}b-draft{{end}}">{{.Role}}</span></td>
        <td>{{.CreatedAt}}</td>
        <td>{{if ne .Username $.Self}}
          <form class="inline" method="post" action="/studio/users/delete">
            <input type="hidden" name="csrf" value="{{$.CSRF}}">
            <input type="hidden" name="id" value="{{.ID}}">
            <button class="btn warn" type="submit">删除</button>
          </form>{{end}}</td>
      </tr>{{end}}
    </table>
  </div>
  <h2>新增用户</h2>
  <div class="card">
    <form method="post" action="/studio/users/add">
      <input type="hidden" name="csrf" value="{{.CSRF}}">
      <label>用户名</label><input type="text" name="username" required>
      <label>口令</label><input type="password" name="password" required minlength="8">
      <label>角色</label>
      <div class="check">
        <select name="role">
          <option value="editor">editor（编辑，可提交审核）</option>
          <option value="admin">admin（管理员，可发布）</option>
        </select>
      </div>
      <div class="btnrow"><button class="btn pri" type="submit">创建</button></div>
    </form>
  </div>
</main>
</body></html>{{end}}

{{define "audit"}}{{template "head"}}
<body>{{template "nav" .Nav}}
<main>
  <h1>审计日志</h1>
  <p class="sub">最近 200 条：谁在什么时候动了什么。审核流的证据链。</p>
  <div class="card">
    {{if .Entries}}<table>
      <tr><th>时间</th><th>用户</th><th>动作</th><th>对象</th><th>备注</th></tr>
      {{range .Entries}}<tr>
        <td>{{.TS}}</td><td>{{.Username}}</td><td>{{.Action}}</td>
        <td>{{.Entity}}{{if .Slug}}/{{.Slug}}{{end}}</td><td>{{.Detail}}</td>
      </tr>{{end}}
    </table>{{else}}<p class="empty">暂无记录</p>{{end}}
  </div>
</main>
</body></html>{{end}}
`

// parseTemplates 编译全部页面模板。
func parseTemplates() *template.Template {
	return template.Must(template.New("studio").Parse(studioHTML))
}
