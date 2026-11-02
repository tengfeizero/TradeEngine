package admin

// adminHTML 是内容后台的完整界面：一个文件、零依赖、离线可用。
//
// 不引前端框架的理由和静态站一致：这是本地工具，断网也要能用；
// 功能面（列表/编辑/保存/重建）不足以抵消构建链的复杂度。
// 注意：整段是 Go raw string，内部不能出现反引号，JS 一律用普通字符串。
const adminHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>内容后台</title>
<style>
:root{--bg:#f6f7f5;--panel:#fff;--ink:#1e2b23;--sub:#6b7a70;--line:#e3e7e3;--pri:#2f7d5b;--bad:#c0392b;--warn:#b07d18}
*{box-sizing:border-box;margin:0}
body{font:14px/1.6 -apple-system,"Segoe UI","Microsoft YaHei",sans-serif;background:var(--bg);color:var(--ink);height:100vh;display:flex;flex-direction:column}
header{display:flex;align-items:center;gap:10px;padding:10px 16px;background:var(--panel);border-bottom:1px solid var(--line)}
header h1{font-size:15px;font-weight:600;margin-right:auto}
header .root{color:var(--sub);font-size:12px}
button{font:inherit;padding:6px 14px;border:1px solid var(--line);background:#fff;border-radius:6px;cursor:pointer}
button:hover{border-color:var(--pri);color:var(--pri)}
button.pri{background:var(--pri);border-color:var(--pri);color:#fff}
button.pri:hover{opacity:.9;color:#fff}
button:disabled{opacity:.5;cursor:default}
main{flex:1;display:grid;grid-template-columns:230px 1fr 480px;min-height:0}
aside{background:var(--panel);border-right:1px solid var(--line);overflow:auto;padding:10px 0}
aside h3{font-size:11px;color:var(--sub);text-transform:uppercase;letter-spacing:.08em;padding:10px 16px 2px}
aside a{display:flex;justify-content:space-between;align-items:center;padding:5px 16px;color:var(--ink);text-decoration:none;font-size:13px;border-left:2px solid transparent}
aside a:hover{background:#f0f4f0}
aside a.on{border-left-color:var(--pri);background:#eef5f0;font-weight:600}
aside .tag{font-size:10px;color:var(--sub);border:1px solid var(--line);border-radius:8px;padding:0 6px}
aside .tag.pub{color:var(--pri);border-color:#bcd9c9}
#editorWrap{display:flex;flex-direction:column;min-width:0}
#editor{flex:1;width:100%;border:0;resize:none;outline:none;padding:16px 20px;font:13px/1.7 Consolas,"JetBrains Mono",monospace;background:var(--panel);color:var(--ink);tab-size:2}
#status{border-top:1px solid var(--line);background:var(--panel);padding:8px 20px;max-height:220px;overflow:auto}
#status .ok{color:var(--pri)}
#status .err{color:var(--bad)}
#status .warn{color:var(--warn)}
#status li{margin:2px 0}
#right{display:flex;flex-direction:column;border-left:1px solid var(--line);background:var(--panel)}
#right .bar{display:flex;gap:8px;align-items:center;padding:8px 12px;border-bottom:1px solid var(--line)}
#right .bar span{font-size:12px;color:var(--sub);margin-right:auto}
#preview{flex:1;border:0;width:100%;background:#fff}
#inqView{display:none;flex:1;overflow:auto;padding:12px}
table{width:100%;border-collapse:collapse;font-size:12px}
th,td{text-align:left;padding:6px 8px;border-bottom:1px solid var(--line);vertical-align:top}
th{color:var(--sub);font-weight:600}
.badge{font-size:10px;border-radius:8px;padding:1px 6px;border:1px solid}
.badge.spam{color:var(--bad);border-color:var(--bad)}
.badge.new{color:var(--pri);border-color:var(--pri)}
dialog{border:1px solid var(--line);border-radius:10px;padding:20px;min-width:320px}
dialog::backdrop{background:rgba(0,0,0,.3)}
dialog h2{font-size:15px;margin-bottom:12px}
dialog label{display:block;font-size:12px;color:var(--sub);margin:10px 0 4px}
dialog input,dialog select{font:inherit;width:100%;padding:6px 10px;border:1px solid var(--line);border-radius:6px}
dialog .row{display:flex;gap:10px;justify-content:flex-end;margin-top:16px}
</style>
</head>
<body>
<header>
  <h1>内容后台</h1>
  <span class="root" id="rootPath"></span>
  <button id="btnNew">＋ 新建</button>
  <button id="btnRebuild">重建</button>
  <button id="btnSave" class="pri" disabled>保存 (Ctrl+S)</button>
</header>
<main>
  <aside id="tree"></aside>
  <div id="editorWrap">
    <textarea id="editor" spellcheck="false" placeholder="← 从左侧选择一个文件开始编辑"></textarea>
    <div id="status"><span class="ok" id="hint">改动会先备份到 .backup/admin/ 再写盘；保存后自动重建并回显结果。</span></div>
  </div>
  <div id="right">
    <div class="bar">
      <span>站点预览（保存后自动刷新）</span>
      <button id="btnInq">询盘</button>
      <button id="btnPrev">预览</button>
    </div>
    <iframe id="preview"></iframe>
    <div id="inqView"></div>
  </div>
</main>
<dialog id="dlg">
  <h2>新建内容文件</h2>
  <label>类型</label>
  <select id="dlgKind">
    <option value="product">产品（content/products/）</option>
    <option value="page">页面（content/pages/）</option>
    <option value="case">案例（content/cases/）</option>
  </select>
  <label>slug（URL 标识，小写字母 / 数字 / 中划线）</label>
  <input id="dlgSlug" placeholder=" 例如：sencha-green-tea">
  <div class="row">
    <button onclick="dlg.close()">取消</button>
    <button class="pri" id="dlgOk">创建</button>
  </div>
</dialog>
<script>
var cur=null, dirty=false;
var $=function(id){return document.getElementById(id)};
var editor=$('editor'), status=$('status'), tree=$('tree');

function say(html){status.innerHTML=html}
function esc(s){return (s||'').replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')}

function loadTree(){
  fetch('/admin/api/files').then(function(r){return r.json()}).then(function(d){
    $('rootPath').textContent=d.default_lang?('语言：'+d.default_lang):'';
    var groups={},order=[];
    (d.files||[]).forEach(function(f){
      if(!groups[f.group]){groups[f.group]=[];order.push(f.group)}
      groups[f.group].push(f);
    });
    var html='';
    order.forEach(function(g){
      html+='<h3>'+g+'</h3>';
      groups[g].forEach(function(f){
        var tag=f.status==='published'?'<span class="tag pub">已发布</span>':
                (f.status==='draft'?'<span class="tag">草稿</span>':'');
        html+='<a href="#" data-p="'+f.path+'">'
             +'<span>'+esc(f.label)+'</span>'+tag+'</a>';
      });
    });
    tree.innerHTML=html;
    tree.querySelectorAll('a').forEach(function(a){
      a.onclick=function(e){e.preventDefault();openFile(a.getAttribute('data-p'))};
    });
    if(d.default_lang){previewLang=d.default_lang;showPreview()}
  });
}
var previewLang='en';
function showPreview(){
  $('preview').style.display='block';$('inqView').style.display='none';
  $('preview').src='/'+previewLang+'/?t='+Date.now();
}
function showInq(){
  $('preview').style.display='none';$('inqView').style.display='block';
  fetch('/admin/api/inquiries').then(function(r){return r.json()}).then(function(d){
    var rows=(d.items||[]);
    if(!rows.length){$('inqView').innerHTML='<p style="color:var(--sub)">还没有询盘。</p>';return}
    var h='<table><tr><th>时间</th><th>客户</th><th>产品</th><th>留言</th><th>状态</th></tr>';
    rows.forEach(function(q){
      h+='<tr><td>'+esc(q.time)+'</td><td>'+esc(q.name)+(q.company?'<br><span style="color:var(--sub)">'+esc(q.company)+'</span>':'')
        +'<br><span style="color:var(--sub)">'+esc(q.email)+'</span></td><td>'+esc(q.product)+'</td>'
        +'<td style="max-width:220px">'+esc(q.message.slice(0,120))+'</td>'
        +'<td>'+(q.status==='spam'?'<span class="badge spam">垃圾</span>':'<span class="badge new">'+esc(q.status)+'</span>')+' <span style="color:var(--sub)">分 '+q.spam_score+'</span></td></tr>';
    });
    $('inqView').innerHTML=h+'</table>';
  });
}

function openFile(p){
  if(dirty&&!confirm('当前文件未保存，放弃修改？'))return;
  fetch('/admin/api/file?p='+encodeURIComponent(p)).then(function(r){return r.json()}).then(function(d){
    cur=d.path;editor.value=d.content;dirty=false;$('btnSave').disabled=true;
    tree.querySelectorAll('a').forEach(function(a){a.classList.toggle('on',a.getAttribute('data-p')===p)});
    say('<span class="ok">'+esc(p)+'</span>');
  });
}
function save(){
  if(!cur)return;
  $('btnSave').disabled=true;
  fetch('/admin/api/save',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({path:cur,content:editor.value})})
  .then(function(r){return r.json()}).then(function(d){
    dirty=false;
    if(d.syntax){say('<span class="err">YAML 语法错误（未保存）：</span><br>'+esc(d.syntax));$('btnSave').disabled=false;return}
    if(d.rebuild_error){say('<span class="err">已保存，但重建异常：</span><br>'+esc(d.rebuild_error));loadTree();showPreview();return}
    var b=d.build||{};
    var h='<span class="ok">✓ 已保存并重建'+(b.pages?'（'+b.pages+' 页）':'')+'</span>';
    (b.errors||[]).forEach(function(e){h+='<li class="err">✗ ['+esc(e.code)+'] '+esc(e.entity)+'：'+esc(e.message)+'</li>'});
    (b.warnings||[]).forEach(function(x){h+='<li class="warn">⚠ ['+esc(x.code)+'] '+esc(x.entity)+'：'+esc(x.message)+'</li>'});
    say('<ul style="padding-left:18px">'+h+'</ul>');
    loadTree();showPreview();
  });
}
$('btnSave').onclick=save;
$('btnPrev').onclick=showPreview;
$('btnInq').onclick=showInq;
$('btnRebuild').onclick=function(){
  fetch('/admin/api/build',{method:'POST'}).then(function(r){return r.json()}).then(function(d){
    if(d.rebuild_error){say('<span class="err">重建异常：</span>'+esc(d.rebuild_error));return}
    var b=d.build||{};
    var h='<span class="'+(b.ok?'ok':'err')+'">'+(b.ok?'✓ 构建通过':'✗ 构建失败')+(b.pages?'（'+b.pages+' 页）':'')+'</span>';
    (b.errors||[]).forEach(function(e){h+='<li class="err">✗ ['+esc(e.code)+'] '+esc(e.entity)+'：'+esc(e.message)+'</li>'});
    (b.warnings||[]).forEach(function(x){h+='<li class="warn">⚠ ['+esc(x.code)+'] '+esc(x.entity)+'：'+esc(x.message)+'</li>'});
    say('<ul style="padding-left:18px">'+h+'</ul>');showPreview();
  });
};
$('btnNew').onclick=function(){$('dlg').showModal()};
$('dlgOk').onclick=function(){
  var kind=$('dlgKind').value,slug=$('dlgSlug').value.trim();
  fetch('/admin/api/new',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({kind:kind,slug:slug})})
  .then(function(r){return r.json().then(function(d){return{ok:r.ok,d:d}})})
  .then(function(res){
    if(!res.ok){alert(res.d&&res.d.error||'创建失败');return}
    dlg.close();$('dlgSlug').value='';loadTree();openFile(res.d.path);
  });
};
editor.addEventListener('input',function(){dirty=true;$('btnSave').disabled=false});
editor.addEventListener('keydown',function(e){
  if((e.ctrlKey||e.metaKey)&&e.key==='s'){e.preventDefault();save();return}
  if(e.key==='Tab'){e.preventDefault();
    var s=editor.selectionStart;editor.setRangeText('  ',s,editor.selectionEnd,'end');dirty=true;$('btnSave').disabled=false}
});
showPreview();
loadTree();
</script>
</body>
</html>
`
