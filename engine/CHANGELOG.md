# 更新日志

遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号 `语义化版本-里程碑`。

## [0.13.4] - 2026-10-23

### 新增

- **第九套预置皮肤 `aeronova`（航宇暗色）**，按 `design/aeronova-website.html` 逐段转译，`engine skins` 自动收录。五条设计语法：① 全页暗底 `#0A0E1A` + **固定网格**（60px 青色网格线 rgba(0,212,255,.03)）与三枚辉光球（cyan/indigo/green，radial 多层背景收进 `body::before`，`pointer-events:none`）——网格在所有节之下，与设计稿 z-index 关系一致；② **bg2 `#0D1220` 满宽暗带节奏**：核心技术（`section:has(>.features.numbered)`）/ 认证（`section:has(>.audits)`）/ CTA / 询盘 / 页脚坐在暗带上，询盘带顶部另有 600px 青色 radial 辉光，其余节透出网格底；③ Space Grotesk 全系 600 / -0.02em，hero 56 / h2 40 / 卡片题 20，数字用 JetBrains Mono（编号卡）或 Space Grotesk 700；④ 青色主导：标题渐变词 primary→accent、**渐变按钮 primary→secondary 深字压底**（tech 同款已过 A100 的模式）、渐变 logo 底座（`.brand::before` 34px + 青色辉光）、图标座 cyan/indigo 15% 双色渐变 + 青描边、卡片悬停泛青浮起 + 顶部 2px 渐变线滑入（`.scenario-card::before` 语法）；⑤ indigo 药丸眉题（`.section-tag`：rgba(99,102,241,.1) 底 + .2 边 + 16px 圆角）。
- **kestrel 站切换 `skin: tech → aeronova`**（theme.yaml 的色板/字体/font_faces 本就按该设计稿配齐，零改动）。82 页 0 错 0 警，Lighthouse 四项 **100 / 97 / 100 / 100**。
- 暗色统一转译规则（沿用 tech 口径）：小字 text3 `#64748B`→`#7B8AA3`（3.7–4.05→≥5.06）、小号 indigo `#6366F1`→`#7B7EF3`（3.96–4.31→≥5.16）——浅色小字在暗底上要**提亮**过 AA；大字号与装饰用途保持设计稿原值。cookie 拒绝按钮显式还原幽灵样式（渐变实底特异性压过 `.ck-no` 的坑，ecobike 已记）。

## [0.13.3] - 2026-10-23

### 新增

- **第八套预置皮肤 `zentea`（茶韵东方）**，按 `design/zentea-website.html` 逐段转译，`engine skins` 自动收录（预置皮肤按文件名发现，加皮肤不用改 Go 代码）。五条设计语法：① 米白纸底 `#F5F0E8`（body 非白，浅带 `#FAF6EF`、页脚 `#EDE6D8` 也是浅色——与 ecobike 的深页脚相反）；② 衬线标题（Cormorant Garamond / 宋体栈，weight 500，hero 64px/300 + em 斜体玉绿）；③ 满宽暗带三段：hero「墨黑→茶山绿」135deg 渐变 + jade/gold 辉光 + 山峦剪影 SVG（data-uri 多背景），cta 墨黑 `#1A1A1A`，inquiry 茶山绿渐变 + 半透明表单卡；④ 金色点缀：节标题下 48px 金线（`section-head::after`）、金边金字 badge、暗带数字金色；⑤ 锐利几何：按钮 2px / 卡片 4px 圆角，图标为「圆形墨黑底座 + 纸色 1.5 描边」，流程序号是 2px 玉绿圆环 + 顶部虚线。
- **leafworks 站切换 `skin: zentea`**，并补齐自托管字体（theme.yaml `font_faces` 新增 Cormorant Garamond 300 与 italic 400 两档 woff2，hero 的 300 字重与斜体不再合成）。Lighthouse 四项 **100 / 97 / 100 / 100**，76 页 0 错 0 警。
- 亮色统一转译规则：小字金色 `#82622A`（设计稿金 `#C4A35A` 浅底 2.1:1，压深后三底 ≥4.5）、绿色小字 `#3D5C3D`、次要正文 `#63605C`（muted 往墨混 45%）——与 ecobike 的「同色板内加深过 AA」口径一致。cookie 接受按钮底用 `#3D5C3D`（13px 普通字重在 `#5B7F5B` 上白字 3.99:1 不过）。

## [0.13.2] - 2026-10-23

### 修复

- **ecobike 皮肤按设计稿整体重写**（`design/ecobike-website.html` 逐段转译）：通栏色带语法（首屏/CTA `#0D2818` 满宽暗带、图文/能力/团队/FAQ/表单 `#F6F8F5` 满宽浅带，`::before 100vw` 实现，几何与底色分两条规则——`:is()` 里含 `:has` 的共享规则特异性 (0,2,2) 会压过单类覆盖 (0,2,1)，混写会让暗带永远画不出来）；设计稿字阶（hero 64 / h2 40 / 卡片题 21 / 眉题 13）；52px 薄绿底座 + 28px 线性图标；页脚 `#061A0C` 满宽三段式。配色全部取设计稿 `:root` 原值，唯一例外：小号绿字与按钮白字用色板内的 `primary-dark #1E7B2C`（`#2D9B3E` 白字 3.4:1 不过 AA）。
- **皮肤小字对比度**：设计稿的次要灰 `#78847A`（steel）在 12px 小字上 3.6–3.9:1，文字用途统一换成色板内的 `#5D6B5E`（slate，≥5:1）；steel 只留给非文本用途。
- **页脚联系链接去掉冗余 `aria-label`**（`label-content-name-mismatch`）：链接自带可见文本（邮箱/电话/WhatsApp），`aria-label="Email us"` 之类反而让可访问名与可见文本不一致；浮层图标按钮无文本，`aria-label` 保留。

## [0.13.1] - 2026-10-23

### 修复

- **可访问性 88 → 100（三站 Lighthouse 实测）**。三个失败项全是引擎模板级：
  - `--muted/--primary/--accent` 直接当正文色用，对比度 3.2–4.5:1。修法：主题层新增派生变量 `--muted-text/--primary-text/--accent-text`（`color-mix` 朝 ink 混 45%），base 模板与全部预置皮肤的**文字色规则**统一改用派生色；填充/边框仍用原 token。浅色主题混后变深、深色主题混后变亮，一套公式两用。实测三套色板最差对 4.7:1。
  - 汉堡导航：`<input aria-hidden>` 含可聚焦（aria-hidden-focus）→ 去掉 aria-hidden，checkbox 本就是可达控件；`<label aria-label>` 禁用属性（aria-prohibited-attr）→ 放 `<span class="sr-only">Menu</span>`（新增 sr-only 工具类）。
  - ecobike 深色页脚：`muted` 朝 ink 混在深底上反向，`.foot-compliance/.foot-addr`（0.13.0 新增类）补进皮肤的页脚浅色清单。
  - cookie 接受按钮 `#fff` on `primary-fill` 在亮色填充上 1.77:1 → 改 `color:var(--paper)`，深色主题自动落成深字。
- **「Lighthouse 四项 100」基线作废**：0.13.0 新增区块曾把 A 项拉到 88。质量基线必须随发版重测。

## [0.13.0] - 2026-10-23

> 标准追平版。B1 升到 v2.0 之后，B3 §17 诚实登记了引擎侧的七项欠账——本版把它们全部做进引擎，**不改一个字的文档口径来「凑分」**。七项里六项是新增能力（C9 商业模式政策、C10 数据可用性、D6 工厂证据、D7 客户证言、E7 AI 可见性、R2 市场准入），第七项（F2 弱网字节预算）此前只有设计没有度量，本版给出真实数字。

### 新增

- **三个新区块（区块类型 17 → 20）**
  - `factory`（D6 工厂真实性）：产能统计 + 设备清单（名称/数量/注解）+ 质检工序 + 第三方审核卡（名称/编号/机构/日期/有效期/报告文件）+ 留样承诺 + 带拍摄日期的车间照片。B 端买家判断「是不是真工厂」靠的就是这一屏：设备型号、审核编号、留样年限——都是能被第三方核对的东西
  - `testimonials`（D7 客户证言）：证言正文 + 署名 + 职位 + 公司 + 国家 + 日期，并可挂到具体产品（`product: <slug>`）上生成回流链接。**能点回产品页的证言才算证据，否则只是口号**
  - `legal_facts`（R2 法定披露）：把 `settings.compliance` 渲染成定义列表，用于 legal-notice 页
  - 校验层配套两条**错误级**规则：`FACTORY_AUDIT_INCOMPLETE`（审核缺编号或日期）、`TESTIMONIAL_INCOMPLETE`（证言缺署名或公司）。**没有可核验锚点的「证据」不是证据，是形容词**——所以是错误不是警告
- **`settings.compliance` 市场准入配置（R2）**：`eu_responsible`（名称/地址/邮箱/电话）、`legal_notice`（注册号/注册法院/VAT/法人/联系方式）、`epr`（DE 包装法 LUCID / FR 包装法 / WEEE）。产品页按 GPSR 只披露制造商 + 欧盟责任方；公司登记信息走 `legal_facts` 区块；EU 责任方另在**全站页脚**露一行（GPSR 要求任何面向欧盟买家的页面都可查）。新增 `model.EUCountries`（27 国），`markets` 写 `EU`/`EEA`/`UK`/`GB` 或任一成员国都能触发
- **机器可读产物（C10）**
  - `llms.txt`：站名 + 摘要 + 实体段（法定名称/成立年/地址/邮箱/电话/目标市场/语言）+ 产品段（型号 · MOQ · 交期 · HS）+ 页面段，全部从模型生成。AI 搜索时代，让模型读到「有出处的结构化事实」比让它猜页面语义可靠
  - `/{lang}/products/{cat}/{slug}/specs.csv`：每个产品每种语言一份，列为 `Group,Label,Value`，开头 `#` 注释行带产品名/SKU/型号/品类/MOQ/交期/包装/HS/箱规/样品政策。采购可以把参数直接丢进比价表
  - `/data/sku-list.csv`：全站已发布 SKU 清单（slug/model_no/title/category/moq/moq_unit/lead_time/hs_code/url），买家做 RFQ 的底表
  - `/data/product-images.zip`：产品封面图打包（含 README.txt 说明用途），构建期从站内媒体生成
  - 页脚新增「Data」出口：SKU 清单常驻；图包**只在真有封面文件时**才给链接（死链比没链接更伤）
- **弱网字节预算（F2）**：构建期统计每页 HTML + 首屏 CSS 字节，超 1 MB 报 `PAGE_BUDGET_EXCEEDED` 警告并打印最差页面。数字进报告，不是口头承诺
- **脚手架补两页**：`business-terms`（商业模式政策：OEM/白标最小量、独家代理、IP 归属、报价有效期——C9）、`legal-notice`（法定披露——R2），均为起草版待客户确认
- **新增校验警告 4 条**：`GPSR_RESPONSIBLE_MISSING`（目标市场含欧盟但没填责任方）、`EPR_MISSING`（含德国市场但没填包装法注册号）、`BUSINESS_TERMS_MISSING`、`LEGAL_NOTICE_MISSING`

### 修复

- **`tel:` 链接在 `legal_facts` 里被转义成 `#ZgotmplZ`**：`Fact.Href` 原为 `string`，html/template 的 URL 过滤器只放行 http/https/mailto。这不只是样式问题——「法规要求可送达的联系方式」直接失效。改为 `template.URL`（0.11.0 联系浮层踩过同一个坑，这次是渲染层测试抓出来的）
- **`Validator.Run()` 不幂等**：同一个 Validator 连续 `Run()` 两次，第二次仍带着第一次的警告（报告未重置），导致「清了问题还是报」。测试用例串联时暴露。改为入口重置 `Report`

### 变更

- 规则码 **50 → 56（39 错 / 17 警）**：新增 6 条（2 错 4 警），无删除。另有 `PAGE_BUDGET_EXCEEDED`（构建层）与 `I18N_MISSING`/`I18N_FALLBACK`（i18n 报告码）不计入此 56
- 区块白名单 17 → 20，`model.BlockTypes` 一并更新

### 验证

- `go test ./...` 15 包全绿；新增 6 个渲染层用例（CSV 结构 / SKU 清单排除草稿 / llms.txt 事实完整 / 三区块产物断言 / GPSR 只在配置时出现 / 页脚死链防护）与 5 个校验用例
- ridgeway 重建 **74 页 0 错 0 警**：`llms.txt` 2939 B、`data/sku-list.csv` 9 行、`data/product-images.zip` 543 KB、`specs.csv` **16 份**（8 产品 × 2 语言）；页脚数据出口两条；产品页 GPSR 段与 specs.csv 入口就位
- kestrel / leafworks 重建 0 错，各 2–3 条**预期警告**（两站未配置 `compliance` 与隐私页——引擎不编造责任方，缺就报）
- `engine new` 脚手架 + `build` 22 页 0 错 3 警（无封面 / 场景空 / GPSR 未配，均为预期）

## [0.12.1] - 2026-10-23

> 一致性补丁：只改文案，不改行为。起因是一次「引擎和 B3 对齐了吗」的复核——B3 侧发现 12 处过期断言，引擎侧发现 1 处。

### 修复

- **`--help` 的 `--skin` 清单停在六套**：`engine new` 的用法行硬编码 `classic|editorial|tech|industrial|organic|minimal`，0.10.1 加入 `ecobike` 后没同步，用户照帮助敲 `--skin ecobike` 会被以为不支持（实际可用）。改为 `[--skin <name>]` + 「可用皮肤见 engine skins」——**不再在帮助里复制皮肤清单**，清单的唯一出处是 `engine skins`，以后加皮肤不会再漂。

### 验证

- `go test ./...` 15 包全绿；ridgeway 重建 70 页 0 错 0 警；`engine version` = 0.12.1
## [0.12.0] - 2026-10-23

> 分享与收录版。M5 路线图收官：分享卡兜底、generate_lead 转化事件、结构化数据补全、移动导航折叠、富文本加粗、目录/规格书下载——六项一次清完，M5 九项全部落地。

### 新增
- **品牌分享卡 + twitter:card**（M5-3 / F6）：`gen_media.py` 为每站生成 1200×630 品牌卡（主题色深底 + 品牌名 + tagline + 双柔光斑，系统字体渲染）；没有 cover 的页面（列表/法务/致谢）`og:image` 回退到品牌卡——全部页面分享出去都是带品牌色的门面；有 `og:image` 的页面输出 `twitter:card=summary_large_image`。ridgeway 71 页 og/twitter 全覆盖
- **generate_lead 转化事件**（M5-4 / E7）：询盘致谢页在 GA4 已注入（Cookie 同意）的前提下发 `generate_lead` 事件；拒绝追踪的访客不产生任何上报。`analytics.gsc_verify` 配置后输出 Google Search Console 验证 meta
- **结构化数据补全**（M5-5 / E4）：`BreadcrumbList`（产品/列表/案例/场景/404，与视觉面包屑同源生成）、`FAQPage`（由 faq 区块问答生成，JSON-LD 与页面内容永不分叉）、`Organization` 补 `logo` 与 `sameAs`（社媒主页）
- **移动导航折叠**（M5-6 / F3）：≤900px 收为汉堡（checkbox + label，零 JS），菜单变抽屉式下拉，带 focus-visible 焦点环；header 高度 ≤15% 视高
- **富文本加粗**（M5-7 / B5）：`**x**` → `<strong>`（先整体转义再回插白名单标签，`**` 是唯一标记语法）；新增校验警告 `MARKDOWN_BOLD_LITERAL` 拦截落单的 `**`。leafworks quality 页三处字面星号就此修复
- **目录 / 规格书下载**（M5-9 / C7）：`settings.catalog.file` 配置后页脚出现下载入口，链接可带 `?intent=catalog` 让询盘表单预填留言（留资钩子，可编辑不强制）；产品 `datasheet` 字段指向站内 PDF 时 CTA 位出「下载规格书」；`gen_media.py` 为配置位生成占位 PDF（缺失才生成，不覆盖真文件）；`media.Library.Exists` 判别非图片附件，PDF 不再误报「无法读取图片尺寸」
- 皮肤修正：ecobike / industrial 深色页脚补品牌名、栏目标题与正文的浅色规则

### 验证
- 15 个测试包全绿（新增分享卡/GSC/汉堡/FAQPage/加粗 与 BreadcrumbList、MARKDOWN_BOLD_LITERAL 两组用例）；三站 0 错 0 警（kestrel/leafworks 各 1 条预期的 `PRIVACY_MISSING`）；ridgeway 71 页 og:image / twitter:card 100% 覆盖；JSON-LD 逐块可解析；1280 / 390 截图核对，390px 无横向溢出，折叠/展开两态与暗色皮肤（tech）均正常

## [0.11.0] - 2026-10-22

> 联系与合规版。B1 评审（ridgeway 75 vs 三份设计稿 22–27）指出的引擎侧欠账一次补齐：联系浮层、页脚直达链接、年采购量表单字段、隐私/条款脚手架、Cookie 同意 + GA4。设计稿赢了观感，这版把它们值得抄的转化设施抄进引擎。

### 新增
- **联系浮层**（A1/A6）：全站常驻右下角，WhatsApp / mailto / tel 三条直达通道，由 `settings.yaml` 的 `contact` 配置驱动，一条都没配就不渲染。新增 `chat` 图标；`tel:` 用 `template.URL` 承载（html/template 默认会把它转义成 `#ZgotmplZ`）
- **页脚升级**（A2/C7/G5）：三列结构——品牌 + tagline + 实体地址（含邮编与国名）、联系直达（含社媒）、法律页出口（`privacy-policy` / `trade-terms` 只列**已发布**页面，设计稿那种 `#` 死链在这里不可能出现）
- **表单 `volume` 字段**（A4）：年采购量估算下拉，档位用 40 尺柜计数（试单 / <1 柜 / 1–5 / 5–20 / 20+ / 年度合同）——件/公斤因行业而异，柜量通用。进入 `DefaultInquiryFields`，B1 A4 的「国家/年采购量/目标市场/用途」四项由字段 + 留言框共同覆盖；`Input`/`store`/邮件/webhook 全链路透传，老库 `ALTER TABLE` 自动迁移
- **Cookie 同意 + GA4**（D2/E7）：配了 `analytics.ga4_id` 才出 Cookie 条；拒绝则完全不注入 gtag（默认不追踪），同意后动态加载并带 `anonymize_ip`
- **隐私政策 / 贸易条款脚手架**（G5/D2/C6）：`engine new` 直接生成可发布的起草版（占位信息方括号标出）；新增校验警告 `PRIVACY_MISSING`（缺 privacy-policy 页时提醒，交付门禁 B6 拦截）
- ridgeway 内容接入：`privacy-policy` 与 `trade-terms` 两页（与既有 FOB 宁波/上海、5 年车架质保、索赔条款一致）；about 页新增 `team` 区块（4 名成员：焊接主管/出口销售/品控认证/单证发运，占位头像由 `gen_media` 生成）

### 验证
- 115 测试全绿；ridgeway 70 页 0 错 0 警（+4 = 两法律页 × 两语言）；kestrel/leafworks 各触发 1 条 `PRIVACY_MISSING` 警告（符合预期）；1280 / 390 截图核对，移动端 Cookie 条与浮层做了让位联动（`.cookie-bar:not([hidden]) ~ .float-contact`）

## [0.10.1] - 2026-10-22

> 第七套预置皮肤 `ecobike`（生态骑行）。按设计稿 `design/ecobike-website.html` 逐段转译：深林绿暗色首屏面板（圆角、径向绿光晕）+ 白底细线卡片 + 薄荷绿图标底座 + 暗色 CTA 与页脚，全站居中版式。

### 新增
- **`ecobike` 预置皮肤**（`internal/skins/presets/ecobike.css`，103 条规则全部带 `html[data-skin=ecobike]` 前缀）：适用自行车 / 户外出行 / 环保制造。语言是「森林里的展厅」——首屏、CTA、页脚是三段深林绿暗色带，页面本体是白底展厅；标题着色词用绿色渐变裁切，卡片悬停浮起带绿影，序号坐薄荷绿圆章
- ridgeway 示例站 `theme.yaml` 切到 `skin: ecobike`（色板本就取自该设计稿，无需改动）

### 验证
- 115 测试全绿；ridgeway 66 页 0 错 0 警；1280 / 390 双宽度截图核对，390px 无横向溢出

## [0.10.0] - 2026-10-22

> 皮肤版（CSS Zen Garden 模型）。同一套 HTML，换一张 CSS 就换一种设计：六套行业预置审美 + 站点自定义皮肤，内容与观感彻底解耦。

### 新增
- **皮肤系统**（`internal/skins`）：皮肤 = 自描述的 CSS 文件（头部 `@skin/@label/@category/@tone` 注释）。内置预设 embed 进二进制；站点 `skins/<名字>.css` 免编译即生效，同名覆盖内置
- **六套预置审美**：`classic`（通用制造基线）/ `editorial`（出版文化）/ `tech`（消费电子，深色）/ `industrial`（机械工业）/ `organic`（食品农业）/ `minimal`（家居设计）
- **`engine skins`**：按行业分组列出全部皮肤，标注当前站点在用哪套
- **`engine build --skin`**：临时换皮构建预览，不写回 theme.yaml——比稿利器
- **`engine new --skin`**（旧 `--theme` 仍可用）：六套皮肤各配一套出厂品牌色板
- **`SKIN_UNKNOWN` 校验**（替换 `THEME_LAYOUT_INVALID`）：皮肤名不存在构建即失败，报错附可用清单与最接近的候选名
- **皮肤包测试**：头部解析、预置盘完整性（作用域前缀 + 900px 断点）、未知名建议、路径穿越拒绝、站点覆盖优先

### 变更
- `base.tmpl` 删除全部布局变体选择器（editorial 33 条 + tech 49 条），基础层收敛为「功能 + 响应式」；`data-layout` → `data-skin`；皮肤 CSS 由 `{{.SkinCSS}}` 追加在后，后写覆盖、不靠 `!important`
- 深色色板兜底判据从 `layout == "tech"` 改为皮肤自述 `@tone dark`——暗色自定义皮肤同样拿到兜底
- `Theme` 新增 `skin` 键；旧键 `layout` 等同 `skin`（存量站点零改动）；`render.New(site, skinOverride)` 增加覆写参数
- 三站配置迁移：kestrel→`tech`、ridgeway→`editorial`、leafworks→`organic`
- **实测**：同一份 ridgeway 内容 × 六套皮肤，六版构建均 0 错 0 警、390px 宽零溢出；115 个测试全绿

## [0.9.0] - 2026-10-22

> 设计对齐版。目标是「引擎产出的站点离设计稿更近」：图标体系、能力网格区块、章节眉题、可复现的媒体生成管线。

### 新增
- **内联图标体系**（`internal/icons`）：40 个内置 24×24 stroke 图标（feather/lucide 风格简化版），按页收集、以 `<symbol>` sprite 插在 `<body>` 开头，正文 `<use>` 引用；未知图标构建期报 `ICON_UNKNOWN`（区块字段与 `scenarios.icon` 都查），渲染层对未知名静默丢弃，绝不产出悬空引用
- **`features` 区块**：图标卡（`style: icon`，对应设计稿的 Use Cases / 服务承诺网格）与编号卡（`style: numbered`，输出「01 / 标签」编号，对应 Core Technology 网格），支持 `kicker`/`title`/`intro`/`items[].{icon,tag,title,body,checks}`
- **章节眉题 `kicker`**：设计稿每个 section 标题上方的小标签（Product Lines / Core Technology…），全部区块（grid ×5、features、image_text、richtext、stats、process、team、faq、cta、logos、gallery）支持
- **`image_text` 的 `checks`**：勾选清单形态（设计稿工厂能力清单的样式），`bullets` 仍是普通圆点列表
- **场景卡图标化**：`scenarios[].icon` 命中注册表时场景卡走「图标置顶卡」，未配置才退回封面图；场景总览页同规则
- **媒体生成管线**（`tools/gen_media.py`）：`media/sources.yaml` 声明「原图 → 槽位、焦点、缩放」，工具裁成 4:3 / 1600px / JPEG q86（默认切掉底部 13% 去生成图水印）；无原图的槽位按主题色板 + 版式生成品牌图案占位图（种子取自路径：同路径永远同一张，不同槽位互不相同）。三站共 60+ 张图已入库，构建不依赖本工具

### 变更
- 布局旋钮扩充：editorial 的能力网格走「报头式」（顶线分栏、无卡片边框），tech 三列紧凑 + 图标辉光 + 编号卡两列；900px 断点对全部新网格单列重压
- 场景总览页 / `scenario_grid` 的卡片数据结构 `Card` 增加 `Icon`

### 测试
- 109 个测试函数（0.8.2 事后 104 + 本轮 5：sprite 收集与配对 3、图标白名单校验 2）；kestrel / leafworks / ridgeway 重建均 0 错 0 警；390px 宽三站横向溢出 0px

## [0.8.2] - 2026-10-22

> 文档-代码一致性审查版。审查口径：文档里每一条可验证的事实性描述，都要能在源码或产物里找到证据。

### 修复
- **版本号常量漂移**：`cmd/engine/main.go` 的 `version` 仍为 `0.8.0-m4`，而 CHANGELOG/README 已是 0.8.1；`scripts/release.sh` 从该常量取版本，会把 0.8.1 的内容打成 0.8.0-m4 的包。归位为 `0.8.2`
- **补齐文档承诺的日闸**：文档写「IP 10/h，24h 30 次」，实现只有小时闸（单 IP 一天可灌 240 条）。新增 `settings.inquiry.rate_limit_per_day`（默认 30），两道闸都在落库前的 IP 锁内复查；`Retry-After` 分窗口给 3600 / 86400
- **主题 token 白名单**：`THEME_UNKNOWN_TOKEN`（色键/字体槽拼错）、`THEME_LAYOUT_INVALID`（未知 layout 原本静默回落 classic）、`THEME_NO_PRIMARY`（警告）。文档里写错的 `primary_fill` 即此类静默失效
- 删除死字段 `density`（模型与脚手架里有，渲染层从未读取）
- **`engine new --theme tech|editorial` 生成的 theme.yaml 非法**：`heading: "Inter", system-ui, sans-serif` 引号只包住首个字体名，YAML 第 8 行解析失败；`classic` 无引号故一直正常，CI 冒烟也只跑 classic。整个字体值改为单引号包裹

### 文档
- 产品模板改块式 i18n（原模板自身用了流式映射）
- 主题键 `primary_fill` → `primary-fill`（连字符才是真实键名）

### 测试
- 100 个测试函数（+5：日闸 1、主题 3、三布局骨架 new→build 1）；ridgeway/kestrel/leafworks 重建均 0 错 0 警；`new --theme tech` 实测 14 页 0 错 2 警

## [0.8.1] - 2026-10-22

### 新增
- 主题变量 `--primary-fill`：暗色主题中「深底文字色」与「按钮填充色」分离（文字要亮、按钮要深才能同时满足对比度），未设置时回落 `--primary`，浅色主题行为不变
- 校验规则 `I18N_UNKNOWN_KEY`：i18n 字段出现非语言代码的键即报错。拦的是 YAML 流式映射 `{ en: A, 20 MP, zh: B }` 的静默截断——逗号后的裸值成为「无值键」，en 被截到第一个逗号且零报错。上线当天抓出 kestrel 104 处、ridgeway 存量 41 处

### 修复
- `tech` 布局漏覆盖 `.image-text ul`：图文区块项目符号仍是浅色主题的 `rgba(0,0,0,.75)`，深色底上对比度 1.06:1，整段不可见。全局改 `var(--muted)`
- `process` / `team` 区块标题 h4 → h3（h2 → h4 跳级违反标题层级审计）
- `tech` 布局 hero 氛围光晕由硬编码绿色改为 `color-mix(var(--primary))`，跟随主题色
- `.skip:focus` 补文字色（白底浅字，键盘用户不可见）
- **`0w` srcset 导致图片整批不加载**：以 webp 为引用主体时（标准库无 webp 解码器）宽度探测为 0，`srcset="... 0w"` 使浏览器判定全部候选无效，页面成片「图丢了」却零报错。`srcset` 跳过宽度未知候选；同名 webp/avif 候选仅在 base 宽度已知时加入。正确用法：内容引用 PNG 原图，同名 WebP 并存由引擎自动进 `<picture>`

### 交付
- `clients/kestrel`：无人机站（Kestrel Aero，`tech` 暗色布局），74 页双语，Lighthouse P/A/BP/SEO 四项 100，LCP 1.1s · CLS 0 · TBT 0ms；媒体以 WebP 交付（占位图每张约 4KB）
- ridgeway 存量内容 186 处流式 i18n 映射转为块式，修复规格文案静默截断

## [0.8.0-m4] - 2026-10-21

### 新增
- 开源基础：MIT LICENSE、CI 工作流（ubuntu + windows 测试矩阵）、发布脚本（三平台交叉编译）
- 一键部署配方：VPS（rsync + systemd）、Cloudflare Pages、GitHub Pages（Actions workflow）
- `examples/zentea` 示例站 README（跑通 / 改造 / 部署三步）
- 「10 分钟起站」实测流程与文档（陌生人从 clone 到可上线站点的验收路径）

## [0.7.0-m3] - 2026-10-21

### 新增
- `engine studio`：表单化内容编辑 + 多用户（admin/editor，PBKDF2）+ 审核流（draft→待审→published，发布仅 admin）+ CSRF + 审计日志
- `/studio/build` 构建预览与 `/preview/` 预览站
- contentdb：users / sessions / audit 表；`PutEntity`/`SetStatus` 单事务同步 payload 与提升列

### 修复
- Studio Handler 返回 mux 导致 DB 连接永不关闭（Windows 上测试临时目录删不掉）
- 编辑页模板字段笔误导致表单静默截断（补「渲染到末字段」守卫测试）

## [0.6.0-m3] - 2026-10-21

### 新增
- `internal/contentdb`：SQLite 内容库（纯 Go 驱动）
- `engine import` / `engine export`：文件 ↔ 内容库双向迁移，导出为可独立构建的完整骨架
- `engine build --db` / `engine validate --db`：内容读自库，与文件构建同一条流水线

## [0.5.0-m2] - 2026-10-21

### 新增
- 图片内容哈希文件名 + immutable 缓存；主题色自动生成 favicon
- 场景独立页（ScenarioIndex / ScenarioDetail）
- `engine deploy`：sha256 比对同步、孤儿清理、`--manifest` / `--dry-run`
- Lighthouse 移动端 P/A/BP/SEO 四项 100（本地实测，报告入库）

### 修复
- favicon 404（best-practices 96 → 100）
- 主色 / accent 对比度不足 WCAG AA（accessibility 96 → 100）

## [0.4.0-m2] - 2026-10-21

### 新增
- `engine doctor --fix`：YAML 往返式字段迁移（cat→category 等），备份到 `.backup/`
- `engine admin`：本地内容后台（文件树 / YAML 编辑 / 保存三道闸 / 询盘列表 / 实时预览）

## [0.3.0-m1] - 2026-10-21

### 新增
- 16 个内容区块；图片管线（真实宽高 / LQIP / WebP 派生）；页面路由（固定页 / 产品总览 / 分类 / 案例 / 404）
- sitemap（hreflang 完整组）与站内断链校验；校验规则 19→36 条
- `engine dev`（600ms 防抖重建）、`engine digest`（每日询盘汇总）
- 询盘链路压测：1910 次并发提交 0 丢失

## [0.2.0-m1] - 2026-10-21

### 新增
- 询盘链路：落库 → 邮件/webhook 通知 → 五层反垃圾 → 无 JS 降级；`engine serve` / `engine inquiries`

## [0.1.0-m0] - 2026-10-21

### 新增
- 骨架 + 数据模型（40+ 字段）+ 多语言 fallback + `engine new / build / validate`
- ZenTea 茶叶示例站（双语）
