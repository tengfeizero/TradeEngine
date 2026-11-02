# 外贸交付引擎 (engine)

> 把外贸站交付从 6 周压到 12 小时。
> 不是通用 CMS，是**交付流水线**——输入客户资料，输出一个可上线、合规、多语言、能收询盘的外贸站。

当前版本：**v0.13.4**（新增第九套预置皮肤 `aeronova`，按设计稿 design/aeronova-website.html 逐段转译：全页暗底 + 固定青色网格与三枚辉光球、bg2 暗带节奏、青靛渐变按钮与 logo 底座、mono 编号卡，kestrel 站已切该皮，Lighthouse 四项 100/97/100/100。v0.13.3 新增第八套预置皮肤 `zentea`，按设计稿 design/zentea-website.html 逐段转译：米白纸底 + 衬线标题 + 金色点缀 + 墨绿茶山渐变暗带 + 山峦剪影，圆角 2/4px；leafworks 站已切该皮，Lighthouse 四项 100/97/100/100。v0.13.2 ecobike 皮肤按设计稿整体重写：通栏色带语法、设计稿字阶与图标规格、页脚深色满宽段——配色全取设计稿 `:root` 原值；页脚联系链接去冗余 `aria-label`；皮肤小字统一 `#5D6B5E`。v0.13.1 可访问性修复：`--muted/--primary/--accent` 文字色改用朝 ink 混 45% 的派生变量 `*-text`，汉堡导航 aria 结构修正，ecobike 深色页脚补齐——三站 Lighthouse A 项 88 → 100，F 维回归满分。v0.13.0 标准追平版：B1 v2.0 之后 B3 §17 登记的七项欠账全部做进引擎——新增 `factory` / `testimonials` / `legal_facts` 三个区块（共 20 种），`settings.compliance` 市场准入配置（GPSR / Impressum / EPR），机器可读产物 `llms.txt` + 每产品 `specs.csv` + `/data/sku-list.csv` + `/data/product-images.zip`，以及构建期页面字节预算；规则码 50 → 56。皮肤体系仍是 Zen Garden 模型，九套预置 + 站点自定义，`engine skins` 列出）
> 2026-10-22 补丁：`I18N_UNKNOWN_KEY` 校验规则 + `--primary-fill` 主题变量 + tech 布局可达性修复 + `0w` srcset 防御（0.8.1）；修 admin 文件树字段名错位、新建产品模板非法 YAML、404 双状态头、`hs_code` 空值误判（0.8.0 后补丁，见设计约束 26–27）。
> 0.8.2 之前 `engine version` 打印的是 `0.8.0-m4`——因为版本号常量没跟着 CHANGELOG 走，`scripts/release.sh` 会把 0.8.1 的内容打成 0.8.0-m4 的包。这类「文档说 A、二进制说 B」的偏差，只有把版本号当发布物的一部分来管才不会再犯。

---

## 这是什么

做外贸站时，每个项目都在重复同一批劳动：重写 12 个区块、手写 hreflang、配一遍询盘链路、处理多语言目录。

引擎把这些「每个站都要做一次」的事做掉，只把「每个站都不一样」的事留给人。

**它不是** Strapi 的替代品。它只服务一种站：**询盘型 B2B 外贸站**。
所以它有很多「不该有的限制」——12 个区块固定、hreflang 规则固定、询盘字段固定。
这些限制是刻意的：**灵活性是这个产品的敌人。**

---

## 快速开始

**10 分钟起站**（实测：`new` → `build` 共 1 秒，产出 22 页双语站，0 错误）：

```bash
# 0. 获取 engine：下载 release 包，或源码编译（单二进制，无运行时依赖）
go build -o engine ./cmd/engine

# 1. 创建站点（自带示例产品/页面/询盘配置，开箱即建）
./engine new clients/acme --brand "Acme Industrial"

# 2. 编辑 clients/acme/settings.yaml 与 content/ 下的内容
#    （全新骨架不改一行也能构建——先把站跑起来，再替换内容）

# 3. 构建
cd clients/acme && ../../engine build
#    产物：.build/public
#    报告：.build/report.json

# 4. 开发预览（改内容自动重建 + 静态站 + 询盘 API）
../../engine dev --addr :8080

# 5. 查看收到的询盘
../../engine inquiries

# 6. 每天 09:00 发一封未读汇总（兜底即时通知漏掉的情况）
../../engine digest --cron

# 7. 上线：三条一键部署路线见 docs/deploy.md（VPS / Cloudflare Pages / GitHub Pages）
```

看示例站（茶叶 B2B，双语）：`examples/zentea/`，有自己的 README。

---

## 命令

| 命令 | 作用 |
|---|---|
| `engine new <dir> [--brand "Name"] [--theme ...]` | 生成站点骨架（不覆盖已有文件），可选布局 classic / editorial / tech |
| `engine build [dir] [--db content.db]` | 校验 + 构建静态站（`--db` 时内容读自内容库） |
| `engine validate [dir] [--db content.db]` | 只校验，不产出文件 |
| `engine import [dir] [--db .data/content.db] [--force]` | 站点文件内容 → SQLite 内容库（交给后台/DB 编辑前的单向闸；库已存在须 `--force`） |
| `engine export [dir] --out <dir> [--db .data/content.db] [--force]` | 内容库 → **完整可构建**的站点骨架（含 media/theme 拷贝；「拿回你的内容」） |
| `engine dev [dir] [--addr :8080]` | 监听内容变化自动重建 + 本地预览 |
| `engine serve [dir] [--addr :8080]` | 静态站 + `POST /api/inquiry` |
| `engine inquiries [dir]` | 查看已收到的询盘（本地读库，不暴露 HTTP 端点） |
| `engine digest [dir] [--cron]` | 发送未读询盘汇总；`--cron` 每日 09:00 常驻 |
| `engine deploy [dir] --to <dir> [--manifest] [--dry-run]` | 产物同步：sha256 比对（幂等）、删除目标端孤儿（下架页必须 404）、可选部署清单 |
| `engine doctor [dir] [--fix]` | 站点体检：必备文件 / 拼写错的键 / 旧字段名 / 缺失配置段；`--fix` 先备份到 `.backup/<时间戳>/` 再用 yaml.Node 往返改写（保留注释与键顺序） |
| `engine admin [dir] [--addr 127.0.0.1:9090]` | 本地内容后台：文件树 / YAML 编辑 / 保存即重建并回显校验结果 / 新建产品·页面·案例 / 询盘列表 / 右侧实时预览 |
| `engine studio [dir] [--addr 127.0.0.1:9190] [--db .data/content.db]` | **Web Studio**：表单化编辑内容库 + 多用户（admin/editor，PBKDF2 口令 + 会话 + CSRF）+ 审核流（draft → 待审 → published，发布仅 admin）；构建预览 + `/preview/` 预览站；审计日志全程留痕 |
| `engine version` | 版本信息 |

参数可以和目录参数换序（`engine serve site --addr :9000`），内部会重排后再解析。

退出码：`0` 成功 / `1` 校验失败 / `2` 用法错误。可直接用在 CI 里。

`engine inquiries` 支持 `--status new|spam|replied`、`--limit N`、`--days N`、`--json`。
`engine digest` 支持 `--hours N`、`--at HH:MM`、`--dry-run`（只打印不发信，用来核对内容）。

### 内容后台的边界（刻意的取舍）

- **只做 YAML 文本编辑，不做表单化编辑**。产品 40+ 字段，表单化意味着每个字段都要控件、校验和迁移——那是 M3 Web Studio 的范围。文本编辑对开发者交付更快，对客户也不神秘。
- **只允许绑定回环地址**。`engine admin site --addr :9090` 会静默修正为 `127.0.0.1:9090`——把能写盘的后台暴露到局域网是事故，不是配置项。
- **保存三道闸**：YAML 语法预检（失败不落盘）→ 原文件备份到 `.backup/admin/<时间戳>/` → 写盘 + 重建 + 校验结果回显。重建失败也算保存成功，错误直接列在编辑器下方。
- **路径白名单**：只能读写 `settings.yaml` / `engine.yaml` / `theme/` / `content/` 下的 YAML，穿越与扩展名一律 400。

---

## 页面与 URL 约定

| 页面 | URL | 由什么驱动 |
|---|---|---|
| 首页 | `/{lang}/` | `content/pages/home.yaml` 的 blocks |
| 固定页 | `/{lang}/{slug}/` | `content/pages/*.yaml`（about / factory / certifications / contact） |
| 产品总览 | `/{lang}/products/` | 全部已发布产品 + 品类筛选条 |
| 分类页 | `/{lang}/products/{category}/` | 该品类下的产品 |
| 产品详情 | `/{lang}/products/{category}/{slug}/` | 参数表 / 贸易条款 / 认证 / 案例 / 关联产品 |
| 案例总览 | `/{lang}/cases/` | 已发布案例 |
| 案例详情 | `/{lang}/cases/{slug}/` | 问题 / 方案 / 结果 + 关联产品 |
| 场景总览 | `/{lang}/scenarios/` | 已发布且关联了产品的场景 |
| 场景详情 | `/{lang}/scenarios/{slug}/` | 场景文案 + 该场景下的产品网格 |
| 询盘成功 | `/{lang}/thank-you/` | 无 JS 提交后的 303 目标，带 noindex |
| 404 | `/{lang}/404.html` + `/404.html` | 构建自动生成，任意静态服务器可用 |
| sitemap | `/sitemap.xml` | 每条 URL 带齐 hreflang 组（含自身与 x-default） |
| robots | `/robots.txt` | 屏蔽 thank-you 与 /api/ |

一个产品页的完整链路（params → 询盘）都在这条路径上闭环，没有「列表页有、详情页没有」的缺口。

---

## 区块（20 个）

页面 = 区块数组，顺序即版式顺序。区块类型写错会**构建失败**（而不是静默消失）。

| 类型 | 用途 | 关键字段 |
|---|---|---|
| `hero` | 首屏 | `headline` / `sub` / `kicker` / `cta_text` / `cta_href` / `image` |
| `richtext` | 纯文本（政策、说明） | `title` / `lead` / `body` |
| `image_text` | 图文左右 | `title` / `body` / `bullets` / `image` / `flip` |
| `stats` | 数字条 | `items[{value,label,note}]` |
| `product_grid` | 产品卡片 | `category` / `featured` / `limit` |
| `category_grid` | 品类卡片（带产品数） | `title` / `intro` |
| `scenario_grid` | 应用场景 | — |
| `certificate_grid` | 证书（自动剔除过期） | — |
| `case_grid` | 案例卡片 | `limit` |
| `process` | 工厂/流程步骤 | `steps[{title,body}]`，序号自动 |
| `team` | 团队 | `members[{name,role,bio,photo}]` |
| `faq` | 问答（`<details>`，无 JS 可开合） | `items[{q,a}]` |
| `cta` | 行动号召 | `title` / `body` / `cta_text` / `cta_href` |
| `logos` | 客户/伙伴标志墙 | `items[]` |
| `gallery` | 图集 | `images[]` |
| `factory` | 工厂证据链（D6） | `capacity[]` / `equipment[]` / `qc[]` / `audits[{name,number,issuer,date,valid_until,file}]` / `retention` / `photos[]` |
| `testimonials` | 客户证言（D7） | `items[{quote,author,role,company,country,date,product}]` |
| `legal_facts` | 法定披露（R2） | 数据来自 `settings.compliance`，区块只提供 `title` |
| `inquiry` | 询盘表单 | 字段与必填项由 `settings.inquiry` 决定 |

多语言文本一律写成 `{ en: ..., zh: ... }`；缺失必译字段直接构建失败。

---

## 图片管线

构建期做四件事，全部为了「不要让图片拖垮页面」：

1. **读真实像素尺寸**写进 `<img width height>`——没有它，图片加载时会把正文顶下去（CLS）。
2. **取平均色**做占位背景，慢网下不是一块白。
3. **发现现代格式**：同名 `.webp` / `.avif` 或 `name-640w.webp` 这类多宽度变体，自动输出 `<picture>` + `srcset`。
4. **可选转码**：`settings.images.transcode: true` 时调用本地 `cwebp` 生成派生图（放在 `media/.derived/`，文件名带内容指纹）；找不到 `cwebp` 只警告、不失败。

首屏图 `loading=eager` + `fetchpriority=high`，其余全部 `lazy`。媒体文件整目录复制进产物，构建结果自包含。

---

---

## 询盘链路

```
浏览器（静态页）
  │  有 JS：fetch JSON → 201，页内提示
  │  无 JS：原生 POST → 303 跳到 /{lang}/thank-you/
  ▼
POST /api/inquiry
  ① 方法/编码 → ② 32KB 体积限制 → ③ 蜜罐 _hp → ④ 3 秒内提交判机器人
  ⑤ 限流（IP 哈希 10/h，计数查库所以重启不清零）→ ⑥ Turnstile
  ⑦ 字段校验 → ⑧ 垃圾打分 → ⑨ **先落库** → ⑩ 异步邮件+webhook（重试 3 次）
  ▼
SQLite（.data/inquiries.db，WAL）
  ├─ 邮件：Reply-To 直接指向客户，销售点「回复」就能对话
  ├─ webhook：企业微信 / 飞书 / Slack / 通用 JSON
  └─ engine inquiries：兜底查看，通知失败也能补看
```

**反垃圾的取舍**：判为垃圾的询盘**照样入库**，只标记 `status=spam`。
宁可多一条垃圾，不可丢一条真询盘。

### 通知的环境变量

密钥不写进 `settings.yaml`（那会进 Git）：

```bash
export ENGINE_SMTP_HOST=smtp.example.com
export ENGINE_SMTP_PORT=587            # 465 走隐式 TLS，587 走 STARTTLS
export ENGINE_SMTP_USER=notify@example.com
export ENGINE_SMTP_PASS=...
export ENGINE_MAIL_FROM=notify@example.com
export ENGINE_TURNSTILE_SECRET=...     # 留空则跳过人机校验
```

还没配 SMTP 时服务会打一条警告——**询盘只入库、不发信**，不会静默丢单。

### 静态站与 API 分开部署

静态站放 Cloudflare Pages、API 放 VPS 时：

```yaml
inquiry:
  api_base: https://api.example.com     # 表单 action 指向这里
  allowed_origins: [https://example.com] # 跨源白名单，不支持 "*"
```

---

## 站点结构

```
client/
├── engine.yaml              # 引擎版本锁、输出目录
├── settings.yaml            # 品牌、公司、市场、语言、询盘配置
├── content/
│   ├── pages/*.yaml         # 固定区块页面（home / about / factory …）
│   ├── products/*.yaml      # 产品（参数表 + 商务条款 + 认证 + SEO）
│   ├── cases/*.yaml         # 客户案例
│   ├── categories.yaml
│   ├── scenarios.yaml
│   └── certificates.yaml
├── media/                   # 图片与证书文件
├── theme/theme.yaml         # 只有设计 token，不能改结构
└── .build/
    ├── public/              # 静态产物
    └── report.json          # 构建报告（交付凭证）
```

**内容即文件，Git 即版本库**。数据库只存询盘。

---

## 主题与皮肤

主题 = 设计 token（颜色 / 字体 / 圆角）+ **皮肤**（`skin`）。皮肤是 CSS Zen Garden 模型：结构由引擎给，观感全在皮肤 CSS 里，`html[data-skin=<name>]` 前缀作用域。

- **预置 9 套，清单只在 `engine skins` 里**（本文件不复制，避免又一处过期）：覆盖通用工业品、消费品出版感、暗色科技、重型工业、自然有机、极简、绿色出行、茶韵东方（zentea）、航宇暗色（aeronova）
- 站点可自带 `skins/<name>.css` 覆盖或新增，**免编译**；`engine build --skin <name>` 可临时换皮看效果
- 换皮肤不会把询盘表单换没——皮肤只动观感，不动结构；深色皮肤靠 `@tone dark` 兜底

---

## 当前能力（权威数字）

下面里程碑清单里出现的「16 个区块」「36 条规则」「69 个用例」都是**当时的值**，只作演进记录，不作为现状依据。现状只有这一处：

| 维度 | 当前值 |
|---|---|
| 命令 | 14 个 + `version`（`new/build/validate/dev/serve/...`、`skins`） |
| 区块类型 | **20 种**（白名单，拼错即构建失败） |
| 校验规则码 | **56 个：39 错 / 17 警**（`addError`/`addWarn` 字面量，脚本枚举）；另有构建层 `PAGE_BUDGET_EXCEEDED` 与 i18n 报告码 `I18N_MISSING` / `I18N_FALLBACK` |
| 预置皮肤 | **9 套**（`engine skins` 列出，含站点 `skins/` 目录自定义） |
| 测试 | `go test ./...` **15 个包**全绿 |
| 交付验证站 | 4 个行业站（茶叶 / 自行车 / 无人机 / 茶叶独立站），ridgeway 74 页 0 错 0 警 |

## 已完成（M0）

- [x] CLI：`new` / `build` / `validate` / `version`
- [x] 文件 Loader（`Loader` 接口已抽出，M3 可换 `DBLoader`）
- [x] 模型层：settings / page / product / category / scenario / certificate / case
- [x] 校验器：19 条规则，分「错误 / 警告」两级，14 个单元测试覆盖
- [x] 字段级翻译 + fallback，必译字段缺失**直接构建失败**
- [x] 产品页与首页渲染（en + zh）
- [x] canonical + hreflang（含 x-default）+ 构建期断链校验
- [x] JSON-LD（Organization / Product，按 D9 不输出 offers）
- [x] 主题 token → CSS 变量；界面文案（标签/单位）由引擎内置并本地化
- [x] `report.json` 构建报告

## M1 已完成

### 询盘链路（M1 第一阶段）

- [x] `POST /api/inquiry`：落库 → 邮件 → webhook，**先落库后发信**
- [x] 五层反垃圾：蜜罐 / 提交速度 / 限流 / Turnstile / 内容启发式（只标记不删单）
- [x] 询盘表单进页面，**有 JS 无刷新、无 JS 走原生 POST + 303** 两条路径都通
- [x] 提交成功页 `/{lang}/thank-you/` 由构建自动生成（带 noindex）
- [x] SQLite（纯 Go 驱动，仍是单二进制、无 cgo）、WAL、ULID 主键
- [x] `engine serve`：静态站 + 3 个端点、优雅关闭、路径穿越防护、CORS 白名单
- [x] `engine inquiries`：本地查看 + `--status` / `--json` 过滤
- [x] 并发压测：100 并发 **0 丢单**，并修掉「限流可被并发绕过」缺陷（数据与结论见 `../B3外贸站交付引擎设计实现.md` 9.5）

### 内容与交付（M1 第二阶段）

- [x] 区块系统：**16 个区块**，YAML 二次解码到强类型，区块拼错直接构建失败
- [x] 页面路由：固定页 / 产品总览 / 分类页 / 案例总览 / 案例详情 / 404 / thank-you
- [x] 首页改为区块驱动（不再有独立的 home 模板，首页与内页共用一套语义）
- [x] 图片管线：真实宽高防 CLS、平均色占位、`<picture>` + srcset、可选 cwebp 转码
- [x] `sitemap.xml`（每 URL 带 hreflang 组）+ `robots.txt`
- [x] 构建期站内断链检查：导航、区块 CTA、卡片链接、手写路径全部兜住
- [x] 校验规则 19 → **36 条（45 个错误码）**：新增区块白名单、导航可达、缺封面、图片超限、标题重复
- [x] 布局变体 3 套（classic / editorial / tech），主题仍然只能改 token
- [x] `engine dev`：600ms 轮询内容指纹自动重建（不用 fsnotify，Windows 下更可靠）
- [x] `engine digest`：未读询盘汇总 + `--cron` 每日 09:00，作为即时通知的兜底
- [x] 测试：**69 个用例**，新增构建集成测试（产物路径 / sitemap / 断链 / 未知区块）与图片管线测试

### 性能与交付（M2 第二阶段）

- [x] **图片内容哈希文件名**：`tea.png → tea.09a446a8.png`，HTML 引用构建期统一改写（渲染层与 YAML 不感知）；文件名变 = 内容变，CDN 可放心给一年 immutable
- [x] 三级缓存：HTML `no-cache` / 带哈希媒体 `max-age=31536000, immutable` / 其余 1 小时
- [x] 场景独立页 `/{lang}/scenarios/` 与 `/{lang}/scenarios/{slug}/`（有关联产品才出，避免空页进 sitemap）
- [x] `engine deploy`：sha256 比对的目录同步（幂等），孤儿清理（下架产品页必须从线上消失），`--manifest` 输出部署清单
- [x] 构建自动生成 favicon（主题主色 16×16）——没有它 console 报 404，Lighthouse best-practices 扣分
- [x] **Lighthouse 实测（本地 Edge 无头，移动端模拟）**：Performance / Accessibility / Best Practices / SEO 全部 **100**，LCP 1.3s、CLS 0、TBT 0ms（报告见 `docs-lighthouse-en-home.json`；本地无真实网络与 CDN 延迟，线上 LCP 会略高，但架构余量充足）
- [x] 测试：**82 个测试函数**（新增 deploy 3 个、构建哈希断言）

### 内容运营（M2 第一阶段）

- [x] `engine doctor`：必备文件检查、未知键拼写检测（反射收集合法键，含 inline 展开）、字段重命名迁移（`cat→category` / `expiry→expires` / `lang→langs` / `no_index→noindex`）、缺失配置段补全（`images` / `layout`）、base_url 规范化
- [x] `engine admin`：浏览器内容后台——文件树、YAML 编辑（Tab 缩进 / Ctrl+S）、保存即重建并回显错误警告、新建产品/页面/案例模板、询盘列表、实时预览 iframe
- [x] 测试：**79 个测试函数**（新增 doctor 4 个、admin 6 个）

### 存储双轨（M3 第一阶段）

- [x] `internal/contentdb`：SQLite 内容库（与询盘库同为纯 Go 驱动，单二进制承诺不变）
- [x] `engine import`：站点文件 → 内容库（单向闸：库已存在拒绝覆盖，`--force` 才放行）
- [x] `engine export`：内容库 → 完整可构建骨架（engine.yaml + settings + theme + content + media/theme 原样拷贝）
- [x] `DBLoader` 实现 `Loader` 接口：`engine build --db` / `engine validate --db` 与文件构建走同一条校验 → 渲染 → 产物流水线
- [x] payload 存 **YAML**（不是 JSON）：`Block.Extra` 里的 `map[string]any` 经 JSON 往返会把 int 变 float64，污染区块二次解码；YAML 往返类型零损耗
- [x] 查询列提升：slug / order / status / category / country / expires / parent / featured 独立成列，供后台列表与表单查询，不必在 payload 里翻找
- [x] 往返实测（zentea）：文件构建与库构建统计逐项一致（32 页 / sitemap 28 / hreflang 30 组断 0）；导出目录独立构建结果一致
- [x] 测试：**86 个测试函数**（新增 contentdb 4 个：往返相等 / 拒绝覆盖 / 拒绝非空导出 / 缺库报错）

### Web Studio（M3 第二阶段）

- [x] `engine studio`：表单化编辑内容库——列表读提升列（slug/状态/排序），编辑页按字段 schema 渲染，**schema 之外的字段（参数表/区块/SEO）原样保留**（payload 经模型 struct 往返）
- [x] 审核流状态机：`draft →(submit)→ in_review →(publish)→ published`，退回 / 下线；**发布与下线仅 admin**，editor 不能改已发布内容（先下线再改）
- [x] 多用户：PBKDF2-SHA256（210k 迭代，标准库 `crypto/pbkdf2`，零新依赖）、服务端会话表（撤销=删一行）、CSRF 令牌、登录失败进审计；首次启动引导 admin 并打印一次性口令
- [x] 审计日志：每次保存 / 状态变更 / 用户操作 / 构建都记「谁在何时动了什么」
- [x] 构建预览：`/studio/build` 用 DBLoader 构建（错误回显到界面），`/preview/` 提供预览站——表单改完立刻能看到效果，构建期校验兜底（表单放不进的非法值会被 validate 拦住）
- [x] 站点设置表单：只暴露客户会改的字段（品牌 / 联系方式 / base_url），询盘与部署配置留给 YAML
- [x] 端到端实测（zentea）：登录 → 表单改产品 → 构建 32 页 → `/preview/` 显示新标题 → `engine export` 把改动拿回文件
- [x] 测试：**95 个测试函数**（新增 studio 9 个：权限矩阵 / 状态机 / CSRF / 会话失效 / 完整渲染守卫）

### 开源准备（M4）

- [x] MIT LICENSE、CHANGELOG（Keep a Changelog 格式，全里程碑回填）
- [x] CI：ubuntu + windows 测试矩阵 + `new → build` 冒烟（起站底线进 CI）
- [x] `scripts/release.sh`：linux amd64/arm64、darwin arm64、windows amd64 四包交叉编译（CGO_ENABLED=0、-trimpath、含 README/LICENSE）
- [x] `docs/deploy.md`：三条一键部署路线——VPS（rsync + systemd + nginx 样例）、Cloudflare Pages（wrangler 一条命令）、GitHub Pages（Actions 样例）
- [x] `examples/zentea/README.md`：示例站三步跑通 / 看点索引 / 改造起点
- [x] **10 分钟起站实测**：干净目录 `new → build` 计时 **1 秒**（14 页双语、0 错误）；serve 探测 home/product/sitemap/favicon 全 200；询盘 API 反垃圾拒短消息、长消息正常落库

---

## 设计约束（改代码前先读）

1. **内容存文件，不存数据库**（ADR-001）。模型不随存储变，只换 Loader。
2. **参数必须结构化**：`filterable: true` 必须有 `value_num`，否则构建失败。
3. **主题只能改 token**，不能改结构——否则复用率归零。
4. **校验分两级**：会导致客户看到错误信息的才算错误；只影响质量的算警告。
   规则太严，人就会加 `--skip-check`，整套校验就废了。
5. **语言代码用 BCP 47**（`zh` 可以，`zh_CN` 不行）。
6. **询盘先落库再发信**——邮件服务挂了不能丢单。
7. **垃圾询盘只标记不删除**——漏掉一条真询盘的代价远大于多存一条垃圾。
8. **不信任提交上来的 lang**：它会拼进 303 跳转地址，必须收敛到 `settings.langs` 白名单。
9. **限流的「复查 + 落库」必须在同一把 IP 锁内**。分成两步查，100 并发会让阈值 10 放过 18 条。
10. **`synchronous=FULL` 不要随手改成 NORMAL**。实测能从 600 rps 提到 1479 rps，但断电时会丢掉最后几条询盘——这个交换在询盘场景不划算。
11. **任何图片都必须带宽高**。渲染层统一走 `image` 子模板，不允许在模板里手写 `<img src>`——CLS 靠纪律守不住。
12. **区块类型是白名单**。拼错的 `type` 会让整块内容静默消失，页面看起来还「正常」，因此必须构建期失败。
13. **图片转码是可选增强，不是构建前提**。`cwebp` 缺失只降级为警告；把构建成败绑在外部二进制上是交付事故的常见来源。
14. **命令行参数必须能和目录参数换序**。Go 的 `flag` 遇到第一个位置参数就停止解析，`engine serve site --addr :9000` 会静默忽略 `--addr`——这类「没报错但配置没生效」的问题最难查。
15. **迁移必须保留注释**。客户的 YAML 注释是交付资产，用 yaml.Node 往返编辑而不是重新序列化 map——「引擎动了我的文件格式」是信任杀手。
16. **编辑器保存的第一道闸是语法，不是语义**。半路保存（编辑到一半）是常态，语法坏的 YAML 绝不能落盘；语义交给重建后的校验回显，别在保存接口里做第二次 validate。
17. **doctor 与 validate 分工**：validate 管「内容对不对」（每次构建跑），doctor 管「结构对不对、格式旧不旧」（人改了文件或跨版本升级时跑）。混在一起两边都会变得难用。
18. **缓存时长跟着文件名走**。文件名里有内容哈希才配 immutable；没有哈希兜底的长缓存等于把「改图不生效」埋成定时炸弹。
19. **下架即 404**。deploy 的孤儿清理不是可选优化——旧产品页留在服务器上会被搜索引擎继续收录，客户点进来看到的是你不再卖的东西。
20. **小字号的颜色必须按对比度选**。12px 的 kicker / 语言切换链接用浅色 accent，好看但过不了 WCAG AA（4.5:1）——Lighthouse 一跑就现形，交付前别指望客户不跑。
21. **内容库是单向闸，不是双向同步**。import 后内容以库为准，再 import 必须 `--force`；后台编辑只写库，回文件走 export。双向自动同步看似贴心，实则制造「哪边是新的」这个永远答不清的问题。
22. **导出必须产出可独立构建的完整骨架**。「拿回内容」若还要人肉补 media、补 theme，就不是拿回内容，是换一种被锁死的方式。
23. **表单编辑的保留语义靠 struct 往返**：schema 只描述客户该动的字段，其余字段因「解码进模型 → 整体重序列化」而原样保留。新增表单字段必须同时改 schema 与 setter——两边不一致会让「看起来能改」的字段静默失效。
24. **已发布内容对 editor 锁死**。「上线的东西被随手改掉」比「改动慢半天」贵得多；editor 要改，先让 admin 下线，或作为新草稿重新走审核。
25. **模板渲染错误必须大声失败**：错误写日志 + 响应尾部，绝不静默截断——截断的表单页是最难排查的反馈黑洞（页面看起来正常，字段全没了）。
26. **测试不得拿示例站当夹具**。`examples/zentea` 是会演进的内容，不是契约：`validate` 的 13 个结构测试曾直接加载它，示例内容一改（新增一个 `abc.yaml`）就集体误报 + panic。结构测试必须用 `t.TempDir()` 里的确定性夹具，示例站只留「基线零错误」一条。
27. **前端读 JSON 一律用小写键**。Go 的 json tag 是小写，JS 写 `f.Group` 会静默得到 `undefined`——不报错，只是列表全空。这类「字段名大小写错位」在无类型的前后端边界上，只能靠约定 + 肉眼审查，写完就去页面确认列表非空。
28. **i18n 值禁止写成 YAML 流式映射**。`label: { en: A, 20 MP, zh: B }` 是合法 YAML——逗号后的裸值成为「无值键」，`en` 被静默截到第一个逗号，页面零报错。多语言值一律用块式映射（`label:` 换行缩进）。校验器以 `I18N_UNKNOWN_KEY` 兜底，但规则兜不住「碰巧是单词」的截断，写的时候就要写对。
29. **宽度未知的图源禁止进 srcset**。`srcset="url 0w"` 会让浏览器判定全部候选无效，整张图不加载且零报错。引用图一律用能读出尺寸的原始格式（PNG/JPG），同名 `.webp`/`.avif` 并存即可，引擎会自动组装 `<picture>`；以 webp 为引用主体时标准库读不出尺寸，曾经整站图片静默消失。
30. **主题 token 键名是白名单**。色板只认 `primary / primary-fill / ink / paper / surface / muted / accent`，字体只认 `heading / body`，布局只认 `classic / editorial / tech`。写错一个键既不报错也不进 CSS，只表现为「改了没效果」——因此校验器以 `THEME_UNKNOWN_TOKEN` 在构建期失败。**是 `primary-fill`（连字符），不是 `primary_fill`。**

---

## 压测

```bash
go build ./tools/loadtest/
cd examples/zentea && ../../engine.exe serve --addr :18090 --trust-proxy
# 另开终端
./loadtest.exe -mode distinct -n 100 -c 100 -db examples/zentea/.data/inquiries.db
./loadtest.exe -mode sameip   -n 100 -c 100 -db examples/zentea/.data/inquiries.db
```

它不只统计状态码，还会**回库逐条核对**：返回的每个 ULID 是否真的在库里、`name/message/email/lang` 是否逐字节相同、主键有无重复。退出码 `1` 表示有丢单或内容不一致，可直接进 CI。

`-mode sameip` 检验限流：同一 IP 打 100 并发，放行数必须**精确等于** `rate_limit_per_hour`。

实测结论（i5-10400F / Windows 10 / 同机回环）：100 并发 **0 丢单**；吞吐天花板约 **600 次/秒**（SQLite 单写者 + 每次提交 fsync），而典型 B2B 外贸站的需求是 0.2 次/秒。完整数据、`synchronous` 取舍与「单实例前提」见 `../B3外贸站交付引擎设计实现.md` 9.5。

---

## 文档

| 文档 | 内容 |
|---|---|
| `docs/deploy.md` | 部署指南：VPS（nginx + systemd）/ Cloudflare Pages / GitHub Pages 三条路线 |
| `CHANGELOG.md` | 版本历史 |
| `../B3外贸站交付引擎设计实现.md` | 定位、边界、三种交付模式、数据模型、流水线、API、里程碑 |
| `../B4外贸站交付引擎使用说明.md` | 安装、13 命令、内容写作铁律、排错 |
| `../B5外贸站交付引擎变更记录.md` | 版本演进、缺陷档案、迁移对照 |
| `../B6外贸站承接交付SOP.md` | 一单活的作业顺序（S0–S6）、交付门禁（Veto 9 + 七维 50 条评分）、验收清单 |
| `../B1优秀的外贸站的标准.md` / `../B2外贸站评分SOP.md` | 评什么（七维 50 条 × 2 分 + 9 条否决项 + Distinction 晋级）/ 怎么取证计分 |

---

## 目录

```
cmd/engine/          CLI 入口
internal/model/      内容模型（struct tag 即契约）
internal/loader/     文件加载（Loader 接口 + FileLoader）
internal/contentdb/  内容库：import/export 双向迁移 + DBLoader（存储可换、模型不换）
internal/studio/     Web Studio：表单化编辑 + 多用户 + 审核流（建在内容库上）
internal/validate/   36 条构建期规则 + 单元测试
internal/render/     页面与区块渲染、内置界面文案、询盘表单模型、sitemap/robots、表单脚本
internal/admin/      本地内容后台：文件树 / 编辑 / 保存三道闸 / 预览 / 询盘列表
internal/build/      流水线编排、hreflang 与站内链接校验、媒体哈希复制、报告生成
internal/deploy/     产物同步：sha256 比对、孤儿清理、部署清单
internal/doctor/     站点体检与字段迁移：必备文件 / 拼写检测 / 重命名表 / yaml.Node 往返改写
internal/media/      图片管线：尺寸探测、平均色、现代格式变体、cwebp 转码
internal/inquiry/    询盘领域：解析/校验/打分/限流/Turnstile/ULID/Handler
internal/store/      SQLite 持久化（现代 Go 纯驱动，无需 cgo）
internal/notify/     邮件 + webhook 异步投递，带重试
internal/server/     静态服务 + 路由 + CORS + 安全头
internal/geo/        表单国家清单
tools/loadtest/      并发压测 + 回库核对（独立程序，不计入 engine 二进制）
examples/zentea/     示例站（茶叶 B2B，双语，内容完整，带 README）
examples/nginx.conf  询盘型站点 nginx 配置样例
examples/deploy-pages.yml  GitHub Pages Actions 样例
scripts/release.sh   三平台交叉编译发布脚本
docs/deploy.md       部署指南（VPS / Cloudflare Pages / GitHub Pages）
.github/workflows/   CI（ubuntu + windows 测试矩阵 + new→build 冒烟）
tools/               占位媒体生成脚本
```

---

## 贡献

改代码前先读「设计约束」一节——这个项目的很多限制是刻意的，PR 若放宽边界（更多区块、更多布局自由度）大概率不会被接受；修 bug、补校验规则、改文案、完善文档则非常欢迎。

```bash
go test ./...          # 全部测试必须绿
go vet ./...
```

---

MIT License。详见 [LICENSE](LICENSE)。
