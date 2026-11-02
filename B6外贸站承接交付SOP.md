# B6 外贸站承接交付 SOP

> **文档性质**：作业指导书（SOP）。回答三个问题：**怎么干、按什么顺序干、干到什么标准算完**。
>
> **在文档族里的位置**：`B1` 定标准（干到什么算好）｜`B2` 定评分（怎么算分、怎么处理分歧）｜`B3` 定设计（为什么这么造、不做什么）｜`B4` 定操作（命令怎么敲）｜`B5` 定变更（版本与缺陷档案）｜**`B6` 就是本篇：一单活从头到尾的作业顺序**。
>
> **版本**：v2.7 ｜ 2026-10-23 ｜ 对应 engine **v0.13.0**（标准追平版：七项欠账落地，用法增加 `factory`/`testimonials`/`legal_facts` 三区块与 `settings.compliance`），对齐 **B1 v2.0**（v2.0 对应 0.8.2、v2.1 对应 0.9.0、v2.2 对应 0.10.0、v2.3 对应 0.10.1、v2.4 对应 0.11.0：门禁七条、资料单 14 项、分数账严格口径均沿用）
>
> **v2.6 改动**：B1 升 v2.0（50 条 / 七维 / Veto 9 条 / 每条 2 分 / Distinction），本篇门禁与分数账同步——9.1 由「Gate 7 条」改为「Veto 9 条」并清掉停在 0.11.0 之前的引擎现状描述；9.2 明确旧分数（72.5/81）只在 v1.0 口径下有效，v2.0 复算登记为待办；第九节流程与文末检查单同步。
>
> **v2.0 相对旧 `13` 的改动**（旧 `13` 已并入本篇并删除）：
> 1. **接入 B2 门禁**：新增第九节 S5.5，交付前必须跑门禁 + 全维评分，`≥75` 才许交（v2.0 时为 Gate 7 条 + 六维，**v2.6 起为 Veto 9 条 + 七维 50 条**）；旧版只有「0 错 0 警 + Lighthouse 四项」，那是技术合格线，不是交付线。
> 2. **把引擎欠账写成作业动作**：页脚无地址、无浮层、无隐私页、无 GA4 出口、移动导航不折叠——v2.0 时这五项引擎都不管，交付期必须由人补；**0.11.0（v2.4）已落地前四项**（页脚标准件、浮层联系、隐私/条款脚手架、GA4+Cookie 同意），移动导航折叠也已由 **0.12.0（v2.5）落地**（≤900px 自动汉堡折叠，无需配置）——**引擎侧五项欠账至此全部清零，M5 九项收官**（6.2 / 9.3）。
> 3. **修正 7 处与 v0.8.2 不符的描述**：`INQUIRY_SMTP_*`→`ENGINE_SMTP_*`（还漏了 `ENGINE_MAIL_FROM`）、`primary_fill`→`primary-fill`、测试 95→100、缺失的 `--trust-proxy`、失效的「10/11 文档」引用、`brand` 被放进 `engine.yaml`（应是 `settings.yaml`，且 `brand.name` 是单语字符串）、`schema_version` 与 `address` 键名（`zip` 不是 `postal`）。
> 4. **工时重排**：把 B2 的 1.5h 评分放进 S5，总预算仍守 ≤12h（B3 §12.3 的指标不破）。
> 5. **新增实测结论与分数账**：390px 下导航折行使 header 占 19–23% 视高，F3 在现引擎下必然失分；并算出「只做技术活 = 72.5 分（合格），补完 D 维 = 81 分（优秀）」——把 75 分线拆成可执行的内容清单（9.2）。

---

## 一、适用范围与前置条件

**适用**：基于外贸交付引擎承接的 B2B 外贸展示 + 询盘站；默认双语（en/zh 起步，`langs` 可扩）。

**不适用**（客户提了就直接回绝，话术见 B3 §12.2）：

| 不做 | 原因 |
| --- | --- |
| 在线支付 / 购物车 / 会员 | 那是电商引擎的活 |
| 可视化 page builder / 通用内容建模器 | 无底洞；外贸站结构本来就该固定 |
| 客户在后台自定义页面结构（模式 C） | 成本 4 倍，交付期用不上 |
| 非技术用户完全自助建站 | 那是 Wix 的地盘 |

**交付模式先定性再报价**（B3 §3.4）：

| 模式 | 形态 | 适用客户 | 报价必须讲清 |
| --- | --- | --- | --- |
| A 纯静态 + 文件 | 改 YAML 重建 | 内容一年不动的展示站 | 客户不能自己改，改一次找你一次 |
| **B 静态 + 薄后台**（默认） | `engine admin` / `engine studio` | 80% 的外贸客户 | 两套后台互不相通，只能选一个 |
| C 全动态 + 通用后台 | — | **永不做** | — |

**开工前置检查**：

| 项 | 检查方式 | 缺失时 |
| --- | --- | --- |
| engine 可执行文件 | `engine version`（应输出 `0.13.0`） | 按 B4 第二节构建 |
| 审美皮肤 | `engine skins` 按行业挑一套；比稿用 `engine build --skin`，不改配置 | B4 六、6.1 |
| Python + Pillow | `python -c "import PIL"` | 装进隔离 venv，不污染系统 |
| Edge / Chrome（Lighthouse + 截图） | 确认可执行文件路径 | 至少一个 Chromium 系 |
| playwright-core | `NODE_PATH=... node -e "require('playwright-core')"` | 装到 `~/.workbuddy/binaries/node/workspace` |
| wget（爬取死链） | `wget --version` | Git Bash 自带；不足则用 `curl` 逐页 |

**本机环境三条**：curl 一律加 `--noproxy '*'`（否则走代理返回空）；Go 构建需便携版 PATH 与 `GOCACHE/GOPATH/GOPROXY`；端口按站递增（8090 ridgeway / 8091 kestrel / 8092 leafworks / **8093 起给新站**）。

---

## 二、流程总览

七个阶段，总预算 **12 小时**（其中 S5 = QA 1.0h + 门禁评分 1.5h）。

| 阶段 | 干什么 | 主要买哪个维度 | 预算 | 关键产出 |
| --- | --- | --- | --- | --- |
| S0 | 承接与信息收集 | D1 / A2（实体与联系） | 1.0h | 资料单、假设清单、报价口径 |
| S1 | 定位与信息架构 | B1（主张）·E5（内链） | 1.0h | 人设句、集合规划、HS 预判、导航 |
| S2 | 脚手架 | 地基（不直接得分） | 0.5h | 可构建骨架 + 主题 token |
| S3 | 内容生产 | **B 20 + C 20 = 40 分** | 5.0h | 全部 YAML（含条款页、隐私页、FAQ） |
| S4 | 媒体生产 | F4 / F6 | 1.5h | 占位媒体 + 每页 cover |
| S5 | 校验、构建与 QA | E 15 + F 10 | 2.5h<br>（QA 1.0 + 门禁 1.5） | 0 错 0 警、四项分、截图、评分报告 |
| S6 | 交付与移交 | —（决定客户能不能自己维护） | 0.5h | 交付 README、上线、培训 |

**工时去向就是分数去向**：S3 的 5 小时买的是 B/C 两维共 40 分，占权重 40%；S5 的 2.5 小时买的是 E/F 共 25 分。把时间花在配色微调上，最多影响 F 维的部分观感；花在「每个 SKU 三个可测数字」上，直接换 B2 的 20 分。

**五条顺序纪律**（违反其中任一条，返工成本都会超过整单利润）：

1. **S1 定不下来不进 S3**。内容返工是最大的浪费——改 8 个产品的定位，等于重写全部 YAML。
2. **S2 建完先 `build` 一次**。v0.8.2 之前 `--theme tech|editorial` 的骨架本身就构建失败（B5 缺陷 17），现在修好了，但仍要第一时间验证，别等写完 8 个产品才发现地基是坏的。
3. **媒体必须在 S5 之前落地**。`validate` 对缺失图片是**错误不是警告**，媒体没做完根本进不了 QA。
4. **S5.5 没过不进 S6**。对客户可以说「还差一项」，但不能把没打分的站当成交付物。
5. **门禁不过时先分类再动手**：是**内容错**（改 YAML）、**引擎缺陷**（改代码，见 10.6）还是**客户资料缺**（写进待确认清单）。三者修法完全不同，混着改会把缺陷藏进内容里。

---

## 三、S0 承接与信息收集

### 3.1 客户资料收集单（18 项，每项都对应一条 B1 标准）

共 **18 项**（0.13.0 新增 15–18：法定实体信息、商业模式口径、工厂证据、客户证言——这四项都是「引擎有位置、客户没给料」的典型）。**缺第 1/3/5 项不许开工**——Gate G1/G2 会直接不过；其余缺失记入「待确认清单」，先写合理占位。

| # | 要什么 | B1 条目 | 缺失的代价 |
| --- | --- | --- | --- |
| 1 | 公司注册全称 / 英文名 / 成立年份 / **街道级地址** / 统一信用代码 | D1、A2 | G1 不过（不评级） |
| 2 | 办公地址与工厂地址（分开）、地图定位 | D1 | D 维扣分，买家不敢核验 |
| 3 | 固定电话（含国际区号）+ 业务邮箱 + **WhatsApp 号** | A1、A2、A6 | G1 不过；中东客户直接流失 |
| 4 | 产品线清单一览（每个 SKU 至少 3 个可测数字） | B2、C1 | B 维直接塌，全站变形容词 |
| 5 | MOQ（**带单位**）、现货交期、定制交期、装箱量、样品政策 | C2 | G2 不过 |
| 6 | 付款方式（T/T 比例、是否 L/C）、贸易术语与港口、装运单证、索赔流程 | C6 | C 维失分，条款页写不出来 |
| 7 | 产能上限（月产能 / 月柜数）、旺季排队情况 | 十问 10 | 大客户的第一道筛子 |
| 8 | 目标市场（国家/地区） | D4、R2、V5 | EU/UK 缺隐私政策 = V5 不过；缺 GPSR 责任方 = R2 不过 |
| 9 | 证书编号 / 发证机构 / 有效期 / 覆盖市场 / **PDF 原件** | C5、D3、G7 | G7 不过；截图证书等于负分 |
| 10 | HS 编码或报关品名 | C3 | C 维失分（可预判后核对） |
| 11 | 语言（默认 en/zh） | E2 | — |
| 12 | 现有站 URL + 2 家竞品站 | S1 定位、B2 对标 | 人设容易撞车 |
| 13 | 实拍图 / LOGO / **团队照（人名 + 职位）** / 客户 logo 授权 | D4、D7、F4 | D 维扣 2 项（「团队是不是人」） |
| 14 | 产品目录 PDF（可公开下载） | C7 | 少一个转化钩子 |
| 15 | **法定实体信息**：欧盟责任方（名称/地址/邮箱）、商业登记号与法院、VAT、法人姓名、包装法 LUCID / WEEE 注册号 | R2 | 面向 EU 销售时**法规违规**（不是扣分）；引擎会在 `markets` 命中欧盟时告警 |
| 16 | **商业模式政策的口径**：OEM/白标最小起订量、是否给独家、模具与设计 IP 归属、报价有效期 | C9 | 政策页只能写空话，买家最关心的四个问题没有答案 |
| 17 | **工厂证据素材**：产线/设备照片（带拍摄日期）、第三方审核报告编号与有效期、留样与追溯做法 | D6 | `factory` 区块填不满；没有编号的「已通过审核」会被校验拦下 |
| 18 | **客户证言**：原文（可截取）+ 客户公司名 + 国家 + 联系人职务 + 使用产品 + 时间 | D7 | `testimonials` 空着，D 维「别人怎么说」这一项无解 |

### 3.2 案例演示：辣研社（示例填写，实际交付以客户确认为准）

> 客户：**辣研社 Layanshi Foods**（成都，2013 年成立），川味辣酱出口与品牌代工。

| 项 | 示例填写 |
| --- | --- |
| 公司 | 成都辣研食品有限公司 / Layanshi Foods Co., Ltd.，2013，成都市郫都区某某路 88 号 |
| 联系 | +86 28 8888 0000 ／ sales@layanshi.example ／ WhatsApp +86 138 0000 0000 |
| 产品线 | 经典油泼辣子（210g 瓶 / 5kg 餐饮桶）、蒜蓉辣椒酱、魔鬼椒超辣酱、辣椒脆（即食）、辣椒面（餐饮散装）、发酵豆瓣辣酱 |
| 目标市场 | US、UK、AE、SG（**UK 触发 V5 与 R2**） |
| 证书 | ISO 22000、HACCP、FDA 食品设施注册、Halal、Kosher |
| MOQ | 瓶装 3000 pcs / SKU；餐饮散装 500 kg |
| 交期 | 现货 15 天；OEM 定制 45 天 |
| 样品 | 3 瓶以内免费，运费到付 |
| 付款与术语 | 30% T/T 定金 + 70% 见提单副本；FOB 宁波 / CIF 视目的港；接受即期 L/C（≥USD 30k） |
| 产能 | 月产 120 万瓶，旺季（8–10 月）需提前 30 天下单 |
| 目录 | 2026 产品目录 PDF（英文，18 页） |

### 3.3 假设纪律：不许留空，只许写默认值

客户没给 HS 编码 → 按 4.3 预判填上，同时进「待确认清单」；客户没给交期 → 填行业常见值并在 README 标注「待客户确认」。**留空是最差的选择**：空值不会报错，但会让产品页在 Gate G2 上失分，而且没人记得回来补。

### 3.4 报价与承诺口径（不要在销售阶段埋雷）

**对外只承诺「优秀（≥75）」，不承诺「一等（≥90）」**——一等需要真实运营数据（GSC 收录、CrUX、GA4 转化），上线当天无法证明（B2 第九节）。

以下五项属于**引擎当前不具备**，客户问到必须如实说明，不许口头答应：

| 客户会问 | 现状 | 回复 |
| --- | --- | --- |
| 手机上要有汉堡菜单 | ✅ 0.12.0 自动折叠（≤900px 汉堡，零 JS） | 无需配置，交付前 390px 截图确认即可 |
| 要接 GA4 看哪个页面带来询盘 | ✅ 0.12.0 全链路：Cookie 同意门 + 致谢页 `generate_lead` 事件 | 填 `analytics.ga4_id` 即生效；广告转化目标选 generate_lead |
| 分享到 WhatsApp 要有缩略图 | 有 cover 的页有 `og:image`（已实现） | 给每页配 cover 即可 |
| 要在 Google 显示面包屑/FAQ | ✅ 0.12.0 自动输出 BreadcrumbList / FAQPage / Organization（logo+sameAs） | 无需配置；Rich Results Test 复验一次 |
| 我自己能在后台改所有页面 | 只能改内容库里的产品/页面内容 | 模式 B 的边界，签合同前讲清 |

---

## 四、S1 定位与信息架构

### 4.1 品牌人设一句话

模板：`我们不是 ___，我们是 ___。` 没有取舍就没有记忆点，行业通用叙事（百年秘方 / 源头工厂 / 质优价廉）一律不用。

> **辣研社：`Hot sauce, engineered.`（辣酱，是研发出来的。）**
>
> 不是「百年老灶」——是一家做**辣酱配方开发与品牌代工**的工厂：你的品牌、你的辣度、你的包装，我们的配方档案、菌落与水分活度数据、以及一条 3000 瓶起订的产线。

已验证结论（Leafworks）：**买手型 / 研发型人设比「源头工厂」叙事能承载更硬的贸易内容**，而硬内容才是 B2B 询盘的转化点（B1 B1/B4）。

### 4.2 集合规划基准

| 集合 | 数量基准 | 辣研社示例 |
| --- | --- | --- |
| 分类 | 5–6 | retail-jars、food-service、oem-private-label、chili-snacks、bulk-ingredients |
| 产品 | 6–8 | 见 4.4 |
| 场景 | 4–6 | 商超自有品牌 / 连锁餐饮 / 食品厂复配 / 电商代发 |
| 认证 | 5–6 | ISO 22000、HACCP、FDA、Halal、Kosher（+ BRCGS 如有） |
| 案例 | 4–5 | US 超市品牌 / 迪拜餐饮集团 / 新加坡电商 / UK 精品店 |
| 页面 | 7–9 | home、products（自动）、oem、quality、certifications、about、contact、**trade-terms**、**privacy** |

> 页面基准比旧版多 2 个：条款页与隐私页是 B1 v2.0 的 C6 与 R1/V5。**0.11.0 起 `engine new` 自带两页起草版**（占位方括号标出），替换占位信息即可；改造存量站点时手写或从脚手架复制（见 6.2）。

### 4.3 HS 编码与合规预判（食品类必做）

**必须 6 位**（B2 口径：写 4 位算不过），且与客户/货代核对后锁定：

| 产品 | HS（6 位） | 依据 |
| --- | --- | --- |
| 油泼辣子 / 蒜蓉辣酱 / 豆瓣辣酱 | **210390** | 调味汁及其制品 |
| 辣椒面（干、碎/粉） | **090422** | 辣椒（干）碎或碾磨 |
| 辣椒脆（即食零食） | **200599** | 其他加工蔬菜（预判，风险高，必须核对） |

合规预判写进 Quality 页：US 市场 FDA 食品设施注册 + 营养成分标签；EU/UK 关注**黄曲霉毒素**限量（辣椒类高频退单原因，与茶叶的蒽醌同类）；清真/犹太认证按目标市场配齐。

### 4.4 产品清单示例（8 个 SKU）

`chili-crisp-210g`（经典油泼辣子）、`garlic-chili-210g`（蒜蓉）、`ghost-pepper-150g`（魔鬼椒）、`chili-crisp-snack-80g`（辣椒脆）、`chili-powder-25kg`（餐饮辣椒面）、`douban-fermented-500g`（豆瓣辣酱）、`oem-recipe-dev`（配方开发服务，归 oem 分类）、`gift-box-3pack`（礼盒）。

### 4.5 导航与 URL

URL 由引擎按 `/en/products/<category>/<slug>/` 自动生成，无需规划；**slug 一旦定稿不改**（改了就断外链与 SEO）。

**导航项数不要为分数妥协——实测证明妥协也没用。** 390×844 实测（leafworks 版式，editorial）：

| 导航项数 | header 高度 | 占视高 | B2 F3 判定（>15% 即不过） |
| --- | --- | --- | --- |
| 4–5 | 158px | 19% | ❌ 不过 |
| 6–8 | 192px | 23% | ❌ 不过 |

窄屏下 0.12.0 起导航自动折叠为汉堡（≤900px，零 JS），F3 的失分理由已从引擎账上划掉。导航项数仍建议 **6–8 项**——折叠只解决「放得下」，不解决「找得到」。

---

## 五、S2 脚手架

### 5.1 布局选择

```bash
engine new clients/layanshi --brand "Layanshi Foods" --theme editorial
cd clients/layanshi && engine build      # 先确认骨架能构建，再写内容
```

| 布局 | 适合 | 反例 |
| --- | --- | --- |
| `classic` | 通用机械、工具、建材等浅色商务 | — |
| `editorial` | **食品、农产品、茶叶**等讲究质感与克制的品类 | 无人机等科技品 |
| `tech` | 深色科技感（3C、无人机、电子） | 食品（深底显脏） |

辣酱是食品 → **editorial**。四行业站实测：布局 + 主题 token 足以完成全部视觉定制，**不要改模板**（改了复用率归零，升级必冲突）。

### 5.2 主题 token：7 个色键、2 个字体槽，一个都不能拼错

```yaml
colors:
  primary: "#8C2F1B"        # 主色：干椒红（导航、链接）
  accent: "#9A5B21"         # 强调色：kicker、编号、高亮
  ink: "#2B211B"            # 正文
  paper: "#FAF5EC"          # 底色：暖纸
  # 其余可用：surface、muted；暗色主题另加 primary-fill
fonts:
  heading: "Iowan Old Style, Georgia, serif"
  body: "Helvetica Neue, Arial, sans-serif"
radius: 2px
layout: editorial
```

三条硬规则：

1. **键名是 `primary-fill`（连字符）**，不是 `primary_fill`。v0.8.2 起拼错直接构建失败（`THEME_UNKNOWN_TOKEN`）——这是**故意的**：旧版静默无效，改完零反应比报错贵得多。
2. **`accent` 用小字之前先算对比度**（≥4.5:1）。`#A87C3F` 在暖纸底上只有 3.28:1，Leafworks 被 Lighthouse accessibility 抓过。
3. **字体值带逗号就整体加引号**。`heading: "Inter", system-ui` 只引了第一个字体名，YAML 直接断（缺陷 17）。

### 5.3 settings.yaml 要点

```yaml
base_url: https://www.layanshi.example
schema_version: 1                    # 当前引擎只支持 1；写大了 build 直接失败
default_lang: en
langs: [en, zh]
theme: layanshi
brand:
  name: Layanshi Foods             # 单语字符串，别写成 {en, zh}
  tagline:
    en: "Hot sauce, engineered."
    zh: 辣酱，是研发出来的。
company:
  legal_name: Chengdu Layanshi Foods Co., Ltd.
  founded_year: 2013
  address: {country: CN, region: Sichuan, city: Chengdu, street: 郫都区某某路 88 号, zip: "611700"}
contact:
  email: sales@layanshi.example
  phone: "+86 28 8888 0000"
  whatsapp: "+8613800000000"     # 0.11.0 起：浮层 + 页脚自动渲染直达链接
inquiry:
  recipients: [sales@layanshi.example]
  required: [name, email, message]   # 必填 ≤5（B2 口径 A4）
  success_url: /{lang}/thank-you/
  rate_limit_per_hour: 10
  rate_limit_per_day: 30
```

- `nav` 的 label 用**块式映射**（见 6.1）；`required` 加字段可以，**不要加公司网址/电话/职位**（A4 判不过）。
- **SMTP 与收件人绝不写进 `settings.yaml`**（会进 Git），一律环境变量（见 8.5）。
- `analytics.ga4_id` 0.11.0 起有渲染出口（Cookie 同意后才注入，拒绝不追踪），可以如实向客户承诺基础流量看板；0.12.0 起致谢页自动发 `generate_lead` 转化事件，广告平台的目标事件直接用它。

### 5.4 建完立刻体检

```bash
engine doctor      # 结构体检 + 旧字段迁移提示（--fix 会先备份再改写）
engine build       # 骨架必须 0 错
```

---

## 六、S3 内容生产（本阶段决定一半分数）

### 6.1 四条 YAML 铁律（每条都对应一次真实事故）

```yaml
# ① i18n 一律块式，禁止流式
title:
  en: Chili Crisp 210 g
  zh: 油泼辣子 210 g
# ❌ title: { en: Chili Crisp, 210 g, zh: 油泼辣子 }
#    合法 YAML：逗号后的裸值成为「无值键」，en 被静默截到第一个逗号，零报错（缺陷 4）

# ② 值里禁止半角冒号+空格
summary:
  en: The spec that matters — 18% chili solids      # ✅ 破折号
# ❌ en: The spec that matters: 18% chili solids     # 断解析

# ③ 列表项 en/zh 同级同缩进
highlights:
  - en: Batch SHU tested and printed on every label
    zh: 每批实测辣度并印标

# ④ 带逗号/引号的值整体加引号（字体、含逗号的标语）
brand: {tagline: "Hot sauce, engineered."}
```

校验器能用 `I18N_UNKNOWN_KEY` 兜住「键不在 langs 白名单」的流式映射，但**兜不住「逗号后碰巧是合法语言码」的截断**——写的时候就要写对。

### 6.2 六个「引擎不管、但 B1 必查」的内容件

这是本篇最重要的一节：**引擎的 A/D 维欠账（B3 §13：A ~50%、D ~45%）只能靠内容补**。少写一个，交付时就少一个分数或直接卡 Gate。

| 内容件 | B1 条目 | 放哪 | 引擎现状 | 写到什么程度算完 |
| --- | --- | --- | --- | --- |
| **实体信息块** | A2 / **G1** | contact 页（`image_text` 或 `richtext`），About 页再放一次 | **页脚只输出「品牌 · 标语 · 语言」**，不含地址电话邮箱 | 街道地址 + 带区号电话 + 业务邮箱 + 到厂指引/地图链接，双语齐 |
| **回复承诺** | A3 | contact 页 lead 或 hero | 无 | 「工作日 12 小时内回复（GMT+8）」，写清工作日口径 |
| **贸易条款页** | C6 | 新增 `pages/trade-terms.yaml` | `engine new` 自带起草版（M5-8） | 付款比例、Incoterm + 港口、装运与单证、退换与索赔四段，每段带数字 |
| **隐私 / Cookie 页** | D2 / **G5** | 新增 `pages/privacy-policy.yaml` | `engine new` 自带起草版（M5-8） | 收集哪些字段、用途、保留期、第三方（邮件/统计）、联系邮箱；面向 EU/UK 必做 |
| **FAQ 十问** | B1·B6 | 每产品页 + 一个总 FAQ 页 | `faq` 区块有，`FAQPage` schema 无（M5-5） | 十问中 ≥8 问有**带数字**的答案（B2 口径） |
| **每页 CTA** | B7 / A6 | 每页最后一个 `cta` 区块 | `cta` 支持 `mailto:`/`tel:`/`https` | 「要样品 / 要报价 / 要目录」三选一，`cta_href` 可点 |

两个连带细节：

- **隐私页与条款页不在导航里**，从 contact 页底部用 `cta` 链过去（`richtext` 正文不能放链接，见 6.4）。否则成孤岛页，E5 扣分。
- **每页都要配 `cover`**：`og:image` 只在有 cover 时输出（`seo.og_image` → `cover`）。about/quality/contact 没 cover，分享到 WhatsApp 就是纯文字链接（F6）。

### 6.3 数字纪律（B2 口径，别自己发明）

| 写法 | 判 |
| --- | --- |
| 「ISO 22000 认证」 | **不算数字**。B2 口径：数字指可测量指标 |
| 「辣椒固形物 18%、酸价 ≤2 mg KOH/g、批间 SHU 漂移 ±8%」 | 算 |
| 「MOQ 3000」 | 不过——必须 `3000 pcs`（单位在 `moq_unit`） |
| 「价格请询」 | 不过——除非页面同时解释了「为什么不定价」（原料随行就市） |
| 「质量第一、服务至上」 | 全部删除。这类句子在 B 维是负分 |

辣研社可用素材：油泼辣子固形物 **18%**、酸价 ≤2 mg KOH/g；蒜蓉辣酱蒜椒比 **3:2**（写成 three-to-two，躲开冒号坑）、水分活度 Aw ≤0.85；辣椒脆出厂菌落 ≤1000 CFU/g、保质期 12 个月、充氮残氧 ≤2%；OEM 页写配方档案、小样 3 轮迭代、标签合规审核（FDA 营养标签 / EU 过敏原加粗）。

**产地与链路敏感信息写明而不是回避**（阿萨姆茶案例：客户会问，单证对不上才丢客户）。

### 6.4 富文本的真实能力（别把 Markdown 写进去）

`richtext` 只做一件事：`title` + `lead` + `body` 按空行切段。**不支持 Markdown，也不支持链接**：

- `**加粗**` **自 0.12.0 起可用**：`**x**` 会渲染成 `<strong>`（先整体转义再回插白名单标签）。**这是唯一的标记语法**——`*斜体*`、`# 标题`、`- 列表` 写了都只是字面字符。落单的 `**` 会报 `MARKDOWN_BOLD_LITERAL` 警告（曾把 M5-7 落地前的状态写在这里，2026-10-23 更正）。
- 正文里写 `[下载目录](...)` 不会变成链接。要链接 → 用 `cta` 区块的 `cta_href`；要强调 → 用短句、独立成段、或 `stats` 区块的大数字。

### 6.5 页面写作基准

每页 1 个 `hero` + 2–4 个区块（`image_text` / `process` / `stats` / `faq` / `cta`）。区块类型共 **20 种**（0.13.0 新增 `factory` / `testimonials` / `legal_facts`，用于工厂证据、客户证言与法定披露，写法见 B4 5.5），**写错 type 构建就失败**（白名单）——这是好事，比静默消失强。

- `process` / `team` 的标题模板已修为 h3，直接用，**不要自创 h4**（heading-order 会挂）。
- FAQ 放采购方真会问的（见 6.6），不要放「你们公司在哪里」这种自己编的问题。
- 案例三段式：**国家 + 行业 + 带数字的结果**（"德国巴伐利亚果园，40 台，9 个月减少 32% 打药量"），`case.result` 无数字会告警。

### 6.6 十问自查表（内容完整性的验收口径）

交付前逐条在站内找答案，找不到就是 B1 的 B6 条不过（「B6」在这里是标准编号，不是本篇文号）：

| # | 采购方会问 | 辣研社的答案落在哪 |
| --- | --- | --- |
| 1 | MOQ 多少？能降吗？ | 产品页 `moq`；OEM 页写「3000 起，5000 以上单价降 6%」 |
| 2 | 打样多久、收费吗、运费谁付？ | `sample_policy` + FAQ |
| 3 | 现货 / 定制交期？ | `lead_time` 分两行写 |
| 4 | 付款方式？首付比例？接受 L/C 吗？ | trade-terms 页 |
| 5 | 走哪个港口？FOB 还是 CIF？ | trade-terms 页 |
| 6 | 有哪些认证？覆盖我的市场吗？ | certifications 页 + 产品页证书区 |
| 7 | 包装什么样？一柜装多少？ | `packaging` + `carton` |
| 8 | HS 编码？目的国清关注意什么？ | `hs_code` + quality 页合规段 |
| 9 | 质量争议怎么处理？有索赔先例吗？ | trade-terms 页「退换与索赔」 |
| 10 | 产能上限？下 3 个柜要排队吗？ | quality 或 about 页（月产 120 万瓶） |

**十问都有答案 = B/C 维基本合格；十问都有带数字的答案 = 一等水平。**

---

## 七、S4 媒体生产

1. 复制上一站的 `tools/gen_media.py`（`engine new` **不生成**该文件），改底色渐变与品类水印（辣研社用辣椒轮廓/火焰纹）。
2. 产出 `media/products/*.png`、`media/hero/*.png`、`media/cases/*.png`、`media/certs/*`，**PNG 与同名 `.webp` 双格式并存**——内容引用 PNG，引擎自动组装 `<picture>` 取用 WebP。
3. 单图 **<1MB**（`MEDIA_TOO_LARGE` 阈值）。纸纹/噪点用**低分辨率噪点放大**——细颗粒噪点会把 PNG 撑过 1MB（Leafworks 实测压到 376KB 才过）。
4. **每个页面都给 `cover`**：它同时是卡片图与 `og:image`。深色布局（tech）媒体必须做深色版，浅色图在深底上是灾难。
5. 客户实拍图**同名替换** PNG 并重跑脚本（或直接替换 + `build`），零代码改动。

---

## 八、S5 校验、构建与 QA

### 8.1 构建链

```bash
cd engine && go build -o /tmp/engine.exe ./cmd/engine
cd clients/layanshi
/tmp/engine.exe doctor                    # 结构体检
/tmp/engine.exe validate                  # 必须 0 错 0 警，一条都不带过
/tmp/engine.exe build                     # 产物在 .build/public
/tmp/engine.exe serve -addr :8093 --trust-proxy   # 本地预览 + 询盘 API
```

`serve` 不带 `--trust-proxy` 也能本地跑，但**上线走反代时必须带**——否则所有询盘都顶着 Nginx 的 IP，限流把全站客户当成同一个人，第 11 个真实客户就吃 429（B4 第四节）。

`validate` 报错时的第一反应：**先查「合法但不正确」类问题**——流式映射、`: `、缩进错位。它们零报错穿透到页面才最贵。

### 8.2 Lighthouse（移动模拟，四项 ≥95，目标 100）

```bash
CHROME_PATH='C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe' \
npx -y lighthouse@12 http://localhost:8093/en/ \
  --output=json --output-path=lighthouse-en-home.json \
  --chrome-flags="--headless=new --no-sandbox --user-data-dir=<系统临时目录>/lhprof" \
  --only-categories=performance,accessibility,best-practices,seo --quiet
```

首页 + 1 个产品页 + 1 个列表页各跑一次，JSON 存进客户目录（这是 B2 的 E-01 证据）。

| 翻车点 | 修法 |
| --- | --- |
| kicker / 步骤编号对比度不足 | `accent` 加深到 ≥4.5:1（装饰小字是高频翻车点） |
| heading-order（h2→h4 跳级） | 用模板现成 h3，不自创 h4 |
| `0w` srcset（整批图不加载） | 引用 PNG 原图 + 同名 WebP（缺陷 5） |
| 图片过大 | 见第七节 1MB 上限与低分辨率噪点技巧 |
| skip link 不可见 | v0.8.1 已修；若自加样式，记得给 `.skip:focus` 文字色 |

### 8.3 结构 QA

- sitemap 条数 = 双语页面总数；hreflang 每组 en/zh 互链 **0 断**（构建期就会拦，断裂直接 build 失败）。
- 抽 5 组页面查**语言互串**：英文页不得出现大段中文（G6 判的是这个，不只是 hreflang）。
- robots 排除后台与 thank-you；`/en/thank-you/` 可访问且被 noindex。
- 结构化数据自检：`grep -o '"@type":"[^"]*"'` 看现有类型（Organization 只在首页、Product 在详情页；Breadcrumb/FAQ 尚无，不要以为是 bug）。

### 8.4 视觉验证（桌面 + 移动，别只截桌面）

headless `--virtual-time-budget` 下 **lazy 图片会随机不解码**（整页「图丢了」的假象，不是真 bug）。可靠流程：滚动触发 lazy + `networkidle` + `fullPage`。

```bash
NODE_PATH="<lighthouse 所在的全局 node_modules 目录，npm root -g 可查>" \
node -e "
(async () => {
  const pw = require('playwright-core');
  const b = await pw.chromium.launch({ headless: true,
    executablePath: 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
    args: ['--no-sandbox'] });
  for (const [w,h,tag] of [[1440,1000,'desktop'],[390,844,'mobile']]) {
    const page = await b.newPage({ viewport: { width: w, height: h } });
    await page.goto('http://localhost:8093/en/', { waitUntil: 'networkidle' });
    await page.evaluate(async () => { for (let i=0;i<document.body.scrollHeight;i+=700){window.scrollTo(0,i);await new Promise(r=>setTimeout(r,150));} window.scrollTo(0,0); });
    await page.waitForLoadState('networkidle'); await page.waitForTimeout(600);
    const m = await page.evaluate(() => ({
      header: Math.round(document.querySelector('.site-header').getBoundingClientRect().height),
      overflow: document.documentElement.scrollWidth > window.innerWidth }));
    console.log(tag, JSON.stringify(m));
    await page.screenshot({ path: 'qa-'+tag+'-home.png', fullPage: true });
    await page.close();
  }
  await b.close();
})();
"
```

截首页 + 一个产品页 + 中文首页，肉眼确认：图片全加载、列表可读、无跳级标题、无截断文案（截断常是流式 i18n 的残留）。移动端三量顺带记下来（header 高度、是否横向溢出、导航可点）——它们是 B2 的 G3 与 F3 证据。

### 8.5 询盘链路实测（三通道都要通）

```bash
export ENGINE_SMTP_HOST=smtp.example.com
export ENGINE_SMTP_PORT=587
export ENGINE_SMTP_USER=robot@example.com
export ENGINE_SMTP_PASS='******'
export ENGINE_MAIL_FROM='Layanshi <robot@example.com>'
export ENGINE_TURNSTILE_SECRET=...      # 可选，缺失即跳过该层
```

1. 前台提交一条真实格式的询盘 → 页面进 thank-you。
2. `engine inquiries` 能看到，`engine inquiries --json` 能看到来源页与 UTM。
3. 邮件到达（查垃圾箱！）→ 没到先查 SPF/DKIM/DMARC，这三条在引擎之外，却是「询盘进垃圾箱」的头号原因。
4. `.data/ip_salt` 与 `.data/inquiries.db` 一起备份；**迁移漏了 salt，历史 ip_hash 全部对不上，限流计数当场归零**。

### 8.6 冻结被审版本

评分前记录：`build_at`、页面数、`git rev-parse --short HEAD`（如有）。**审错版本是最常见的评分事故**——本地产物、预发、线上三者的分数可以差 10 分。

---

## 九、S5.5 交付门禁（B2 评分，2h）

顺序不可颠倒：

```
build → validate 0 错 0 警 → Veto 9 条（12min）→ Lighthouse → B2 七维全量 50 条（110min）
     → 总分 ≥75 且 Veto 9/9 且 七维每维 ≥60% → 允许交付
     → 否则按缺口清单回修，只复测改动项
```

### 9.1 Veto 9 条自查（任一不过 → 不评级，不许交付）

| Veto | 判定 | 本单怎么过 | 引擎现状提示（0.12.0） |
| --- | --- | --- | --- |
| V1 实体联系方式 | 地址/电话/邮箱三者皆无 → 不过 | contact 页三件齐 + 页脚标准件（6.2 第一行） | **0.11.0 起页脚自动输出**地址/邮箱/电话/WA，不再靠联系页单点兜 |
| V2 产品页贸易参数 | 抽 3 个 SKU 全无 MOQ/交期/规格 → 不过 | 产品 YAML 必填四件套 | 字段齐，靠写 |
| V3 移动端可用 | 横滚 / 导航不可展开 / 文字重叠 / **表单提交不了** | 390px 实测三量 + 键盘实填一次 | **0.12.0 起 ≤900px 汉堡折叠**，零 JS |
| V4 表单反馈 | 无成功页且 24h 无邮件无落库 → 不过 | thank-you + SMTP | 需配 `ENGINE_SMTP_*` |
| V5 隐私政策 | 目标含 EU/UK 且无隐私页 → 不过 | `pages/privacy-policy.yaml` | **0.11.0 起 `engine new` 自带起草版**；未发布则 `PRIVACY_MISSING` 警告 |
| V6 多语言互串 | hreflang 断裂或英文页大段中文 | 构建期已拦；抽查语言 | 天然过，抽查防内容串 |
| V7 证书真实性 | 编号查不到 / 过期仍标有效 | 抽 2 张官网核验；PDF 换实件 | 占位 PDF（870B）视为未过 |
| **V8 站点被入侵** | 暗链 / 挂马 / 被搜索引擎标记 | `site:` 抽查 + 首页外链白名单比对 | 产物不引外部脚本，风险低；**但模板外挂与客户加装统计要查** |
| **V9 询盘通道不可达** | 48h 两处皆无，或联系邮箱退信 | 双向验证（落库 + 收件箱）+ `dig` 三记录 | 落库自动；**SPF/DKIM/DMARC 属部署侧，必须手工配**，缺 DMARC 按不过 |

> **V9 是新增的门禁，也是最容易被漏的一条**：0.12.0 之前本单只验「表单提交成功」，而 Gmail/Yahoo 从 2024 年起对批量发信强制 SPF+DKIM+DMARC。询盘通知从免费邮箱发出、域名没有 DMARC，就直接进垃圾箱——站在线、询盘为零。

### 9.2 分数账：75 分靠什么买到

> **口径先说，别混用**：本节 9.2 的表是 **B1 v1.0 口径**（42 条 / 六维 / 加权）的历史账，保留它是因为「技术活 72.5、补 D 维 81」这个结论解释过很多次。B1 v2.0 改了题目（50 条、七维、每条 2 分、加 Distinction），**这些分数不可沿用、也不可换算**。v2.0 复算见 9.2.1。

**v1.0 历史账**（下表「引擎欠账」项已于 0.11.0 / 0.12.0 全部清零，故 72.5 在 0.12.0 产物上已不再成立）：

| 维度 | 第一遍（只做完 S3 的骨架内容） | 补完 D 维后 | 差额从哪来 |
| --- | --- | --- | --- |
| A 可达性 15 | 4/6 = 10.0 | 4/6 = 10.0 | A1 浮层（M5-2，已清零） |
| B 内容与主张 20 | 7/7 = 20.0 | 7/7 = 20.0 | 纯内容，**唯一能靠人拿满的维度** |
| C 产品与贸易 20 | 7/8 = 17.5 | 7/8 = 17.5 | 缺 C7 目录 PDF（0.12.0 已补下载位，PDF 内容仍要客户给） |
| D 信任与合规 20 | 3/7 = 8.6 | **6/7 = 17.1** | 隐私页、团队页、第三方背书、合规声明——全是内容活 |
| E 技术与 SEO 15 | 5/7 = 10.7 | 5/7 = 10.7 | E4/E7（M5-4/5，已清零） |
| F 性能与移动 10 | 4/7 = 5.7 | 4/7 = 5.7 | F3 移动导航（M5-6，已清零） |
| **合计** | **72.5** | **81.0** | 差 8.5 分全在 D 维 |

结论仍然成立：**技术层在现引擎下已经封顶，剩下的分全在内容、信任与合规**。花 1 小时写隐私页与团队页，比花 1 小时调间距值 8 分。

#### 9.2.1 v2.0 复算（**2026-10-23 已完成**——产物可证子集，详见《三站B1v2.0评分报告-2026-10-23.md》）

三个问题全部有了答案，另加两条此前没预料到的发现：

| # | 要回答 | 答案（2026-10-23 实测） |
| --- | --- | --- |
| 1 | 三站在 50 条下各得多少 | **kestrel 82（41 条）/ leafworks 84（42 条）/ ridgeway 84（42 条）**；同日 0.13.1 修复可访问性后复测 **90 / 92 / 92**（F1/F4 转过，F 维满分）；再同日补内容缺口后复算 **96 / 94 / 94**（kestrel 补 trade-terms 页 + C4 定价策略说明 + 十三问 Q4/Q5/Q13 → C4/C6/B6 转过；leafworks 补 trade-terms 页 → C6 转过；ridgeway 补 C4 定价策略说明）。E6 因未部署记 N/A（上限 98）。剩余失分：R4/R5/R6（部署与运维）+ D3（第三方背书，需客户素材） |
| 2 | R 维会不会挂 | **会，但挂法变了**：R2 已全过（0.13.0 的 `settings.compliance` 三站都配了）；挂的是 R4（邮件三记录未部署）、R5（gsc_verify 未配）、R6（无运维）。**V9 邮件通道未部署 → 按 B2 口径三站均「不评级」**——落库通道实测通过，差的是部署侧 SMTP |
| 3 | Distinction 拿到几条 | **0 条**，与预计一致。「一等」不是引擎单独能交付的目标，此条由实测坐实 |

**推演 vs 实测的对账**：ridgeway 推演 ≈96，实测 84，差 12 分。差值来源：①推演按「引擎有字段位」给分，实测按「内容立不立得住」；②F1/F4 的可访问性 88 是推演时没料到的（此前 Lighthouse 四项 100 是旧版模板的成绩，0.13.0 新增区块带入了对比度与 aria 问题）；③R4/R5/R6 推演标「属部署侧」未扣，实测按 B2 口径扣。**教训：推演数永远偏乐观，且偏在引擎管不到的地方——这正是复算必须真跑的理由。**

#### 9.2.2 v2.0 新增的「本单必做」六件事

这六件事不是引擎欠账，是**每单交付都要做的动作**，写清是因为 B1 v2.0 之后它们直接影响能不能过 Veto：

| 动作 | 对应条目 | 谁来做 | 耗时 |
| --- | --- | --- | --- |
| 配 SPF / DKIM / DMARC 三条记录，并发一封测试询盘到 Gmail 与 Outlook | R4 / V9 | 部署方（客户 DNS 权限） | 30 min |
| 按目标市场查法定信息：EU 的 GPSR 责任人、德国 Impressum 与 LUCID、英国 UKCA | R2 | 交付 + 客户确认（**引擎已给位置**：填 `settings.compliance` 即三段出口；不填构建告警） | 30 min |
| 统一 NAP：站内、Google 商家、LinkedIn、平台店铺的名称/地址/电话 | E7 | 客户 + 交付（站内一侧自 0.13.0 多一个载体：`llms.txt` 的实体段，改一处要同时改那里） | 20 min |
| 填 OEM/白标/独家/报价有效期政策页 | C9 | 客户（商业决策；**引擎已给 `business-terms` 骨架**，只缺数字） | 由客户定 |
| 补工厂证据链：产线图带日期、月产能、QC 流程、验厂报告编号 | D6 | 客户提供素材（**引擎已给 `factory` 区块**；编号或日期缺失直接构建失败） | 由客户定 |
| 微信/WA 分享一次，确认品牌卡与标题正确 | F5 | 交付 | 5 min |

> 前三项是**技术活**，我们自己能干；后两项是**客户决策与素材**，接单时就要进资料单（6.2），否则交付期一定卡住。

**引擎侧欠账已全部清零**（M5 五项 + B1 v2.0 新增的七项），本单不再需要临时改模板。**这一栏清空的含义是：交付时剩下的缺口全是「客户没给」而不是「引擎没有」**——两者的处置完全不同（前者进资料单，后者要发版）。

| 原欠账 | 对应条目 | 状态 |
| --- | --- | --- |
| 页脚标准件（地址/电话/邮箱/隐私链接） | A2、A1 部分 | ✅ 0.11.0 |
| 浮层联系方式（WhatsApp/电话/邮件） | A1 | ✅ 0.11.0 |
| GA4 渲染出口 + 转化事件 | R5（原 E7） | ✅ 0.12.0（含 `generate_lead`） |
| BreadcrumbList / FAQPage schema | E4 | ✅ 0.12.0 |
| 移动导航折叠 | F3 | ✅ 0.12.0 |
| 商业模式政策页骨架 | C9 | ✅ 0.13.0（`business-terms` 脚手架 + 缺页告警） |
| 参数导出 / SKU 清单 / 图包 | C10 | ✅ 0.13.0（`specs.csv` / `sku-list.csv` / `product-images.zip`） |
| 工厂证据链字段 | D6 | ✅ 0.13.0（`factory` 区块 + 审核锚点校验） |
| 客户证言字段 | D7 | ✅ 0.13.0（`testimonials` 区块 + 署名校验） |
| `llms.txt` | E7 | ✅ 0.13.0 |
| 法定信息字段（GPSR / Impressum / LUCID） | R2 | ✅ 0.13.0（`settings.compliance` + 三处出口 + 3 条告警） |
| 弱网字节预算 | F2 | ✅ 0.13.0（`PAGE_BUDGET_EXCEEDED` + `report.json` 的 `budget`） |

### 9.3 阈值与处置

| 结果 | 处置 |
| --- | --- |
| ≥75 且 Veto 9/9 且 七维每维 ≥60% | 进入 S6 |
| ≥90 但 Distinction <3 | 只按「优秀」交付，**不宣称一等**；X 项属范围外，报价时说清 |
| 60–74 | 修 P0 缺口后**只复测改动项**（30min），再判 |
| <60 或任一 Veto 不过 | 退回 S3/S5，不许带病交付 |
| 任一维 <60%（维度塌陷） | 按不合格处理，不许用别的维度高分掩盖 |
| P0 缺口 ≤3 | 允许「带缺口交付」，但缺口必须写进交付 README 并在交付说明里点名（客户知情） |
| P0 >3 | 不交付 |

**不许自评过线**：≥90（一等）必须两人独立评分、差值 ≤5 才算；差值 >5 说明口径有漏洞，当场补 B2 第五节口径表重算，**禁止取平均分**。

### 9.4 报告归档

评分报告（结论 + 评分卡 + 缺口清单 + E-xx 证据索引）存 `clients/<brand>/qa/report.md`，证据文件存 `qa/`。它是**唯一能回答「你说优秀，凭什么」的东西**，也是下一单的对比基线。

---

## 十、S6 交付与移交

### 10.1 交付 README 必含 8 项

1. 品牌设定与定位一句话（值不值得复用，看这句）
2. 页面 / 内容统计（双语页数、产品数、证书数）
3. 质量指标：`validate` 0 错 0 警、Lighthouse 四项分、LCP、评分总分与等级
4. 预览端口与常用命令（`build` / `serve` / `inquiries` / `digest`）
5. 媒体替换说明（同名替换 + 重跑脚本）
6. **待客户确认清单**（S0 的假设全部列出，含 HS 编码、交期、证书 PDF）
7. **已知缺口披露**：Veto 边缘项（尤其 V9 邮件送达）、R 维缺口（DMARC / 法定信息）、占位件（证书 PDF、目录 PDF、产线图）
8. 通知与数据（SMTP 环境变量名、`.data/` 两个文件必须一起备份）

### 10.2 通知与数据三条前置

- 发件域 **SPF + DKIM + DMARC** 配齐（引擎之外，却是「询盘进了库没人知道」的头号原因）。
- 通知正文带齐：客户信息 + **来源页** + **UTM** + 提交语言与时间（按客户时区）。少了来源，渠道归因事后无法补。
- 三通道互补：邮件（留档）→ webhook（企业微信/Slack，12 小时内响应是 B 端成交第一杠杆）→ `digest`（兜底防漏看）。

### 10.3 两个后台二选一（边界必须讲清）

| 客户能力 | 用哪个 | 一句话 |
| --- | --- | --- |
| 会一点文本编辑 | `engine admin` | 改**文件**，保存即重建，强制 `127.0.0.1` |
| 只会用浏览器表单 | `engine studio` | 改**内容库**，表单 + 审核流，`export` 才回文件 |

两套链路**互不相通**（admin 不写库、studio 不写文件）。选了 studio 就不要让客户再去改文件，否则两边同时改同一个产品，谁覆盖谁说不清。

### 10.4 上线

| 路线 | 命令 | 适用 |
| --- | --- | --- |
| VPS | `rsync -a --delete .build/public/ user@host:/srv/www/site/` + nginx + systemd | 要跑询盘 API |
| Cloudflare Pages | `wrangler pages deploy .build/public` | 纯静态（API 需另行部署） |
| GitHub Pages | `examples/deploy-pages.yml` | 展示站、无询盘 |

`engine deploy --to <dir>` 只支持本机路径（做 sha256 比对 + 孤儿清理，第二次 0 copied）。nginx 关键两行：哈希媒体 `immutable` 长缓存、`/api/` 反代 `engine serve --trust-proxy`。

### 10.5 文档同步（三处，缺一处下一个人就会踩）

1. 本单的交付记录 → 引擎/client README（如有）与 `B5` 的交付索引；
2. 引擎若有改动 → `B5` 缺陷档案 + `engine/CHANGELOG.md` + **`cmd/engine/main.go` 的 `version` 常量**（B5 缺陷 13：漏了第四处，发布脚本打出错版本包）；
3. `.workbuddy/memory/YYYY-MM-DD.md` 当日日志。

### 10.6 引擎改动纪律

交付中遇到问题，先分类：

| 现象 | 归类 | 动作 |
| --- | --- | --- |
| 某个产品不显示 / 文案缺一截 / 图片不加载 | 内容错 | 按 8.1 查；不改代码 |
| 所有站都这样，且不报错 | **引擎缺陷** | 修，且必须通用（历史 17 条缺陷全部以「边界白名单 / fail-fast」收口） |
| 只有这家客户缺数据 | 客户资料缺 | 写进待确认清单，不停工 |

改引擎的验收线：`go vet ./...` 干净 + **全量 100 个测试绿** + 三个客户站重建 0 错 0 警（页数不变）、示例站 zentea 仍 0 错 3 警（原有内容提醒，不得新增）。改完在 B5 登记（症状 / 根因 / 修复版本 / 沉淀约束），不登记等于没修——下一个人会重踩。

---

## 十一、已知坑位速查（按 B5 编号，不复制细节）

| B5 # | 坑 | 一句反制 |
| --- | --- | --- |
| 1 | flag 静默失效 | 参数改完实测输出，不靠「应该生效了」 |
| 2 | 模板静默截断 | `validate` + 页面抽查 |
| 3 | theme 解析失败被吞 | v0.8 起报错；仍要查字体引号 |
| 4 | **流式 i18n 截断** | 块式映射 + `I18N_UNKNOWN_KEY` |
| 5 | **`0w` srcset** | 引用 PNG 原图，WebP 同名并存 |
| 15 | 主题键拼错 | 键是 `primary-fill`；v0.8.2 起拼错构建失败 |
| 15b | 图标名拼错 | `icon:` 与 `scenarios[].icon` 走白名单，0.9.0 起拼错构建失败（`ICON_UNKNOWN`） |
| 15c | 皮肤名写错 | 0.10.0 起构建失败（`SKIN_UNKNOWN`，报错附候选名）；`engine skins` 查可用清单 |
| 16 | 死字段 `density` | 已删；别再往 theme 里加无效键 |
| 17 | 骨架非法 YAML | 字体值整体加引号；`engine new` 后先 build |

全集（17 条，含版本号漂移、日闸缺失等）见 `B5` 第五节。规律只有一句：**「合法但不正确」的输入 + 零报错 = 最贵的缺陷**；能靠白名单堵的，绝不靠「多加小心」。

**交付期专属三条**（不在 B5 里，但每次都有人踩）：

- lazy 图截图不解码 → 用 Playwright 滚动触发，别信 `--virtual-time-budget`；
- curl 走代理返回空 → 一律 `--noproxy '*'`；
- 端口被上一单占着 → 新站从 8093 起，别复用旧端口导致 Lighthouse 测到上一版。

---

## 十二、验收清单（交付前逐项打勾）

**门禁**

- [ ] Veto 9 条全过（V1–V9）
- [ ] B2 总分 ≥75，七维无一项低于 60%，评分报告已归档（v2.0 口径；旧 72.5/81 不可沿用）
- [ ] 引擎欠账影响本单的项已在 README 披露

**技术**

- [ ] `engine validate` 0 错 0 警
- [ ] `engine build` 成功，页面数 = 集合规划的双语总数
- [ ] sitemap 条数正确；hreflang 互链 0 断
- [ ] Lighthouse 四项 ≥95（目标 100），移动模拟，JSON 存档
- [ ] LCP < 2.5s（参考 1.1–1.9s）
- [ ] 桌面 1440 与移动 390 整页截图各 ≥3 张，肉眼确认
- [ ] 询盘：提交 → 落库 → 邮件到达 → `digest` 可发
- [ ] `report.json` 的 `budget.over = 0`（无单页超 1 MB）
- [ ] 四个机器可读产物都在：`/llms.txt`、`/data/sku-list.csv`、`/data/product-images.zip`、每产品 `specs.csv`（数量 = 已发布产品 × 语言数）
- [ ] 目标市场含 EU/EEA/UK → `settings.compliance.eu_responsible` 已填，且产品页与页脚都能看到责任方；含 DE 另有 `epr.de_packaging`

**内容**

- [ ] 每个 SKU：MOQ（含单位）+ 现货/定制交期 + 包装 + 样品政策 + 6 位 HS
- [ ] 十问 ≥8 问有带数字的答案
- [ ] contact 页实体信息块双语齐；回复承诺写明工作日
- [ ] trade-terms 与 privacy 页存在（EU/UK 必查），并从 contact 页链入
- [ ] 证书 PDF 为实件、无 90 天内到期告警
- [ ] 每页有 cover（分享卡）；产物无字面 `**`
- [ ] `factory` 区块：设备有型号/台数、审核报告**有编号与日期**（缺则构建就失败，能过说明有）
- [ ] `testimonials` 区块：每条都有署名 + 公司 + 国家，且挂到了对应产品
- [ ] `legal-notice` 页有注册号/法院/VAT/法人；`business-terms` 页四个问题（OEM/独家/IP/报价有效期）都有答案

**交付物**

- [ ] 客户 README 八项齐（含待确认清单与缺口披露）
- [ ] `.data/ip_salt` + `inquiries.db` 备份说明已交付
- [ ] 后台方案二选一已向客户明示边界
- [ ] 上线路径确定，`--trust-proxy` 与 nginx 两行已交代
- [ ] 文档同步三处完成；引擎如有改动：测试 100 绿 + B5 已登记
- [ ] 当日记忆已写

---

## 十三、附录

### A 辣研社关键文件（节选）

**engine.yaml**（只管版本锁与产物路径，**不放品牌与布局**——它不进内容库）

```yaml
engine_version: 1
output: .build/public
```

**settings.yaml**（品牌、布局在这里；旧 `13` 把 brand 放进 engine.yaml 是错的）

```yaml
brand:
  name: Layanshi Foods
  tagline:
    en: "Hot sauce, engineered."
    zh: 辣酱，是研发出来的。
theme: layanshi
```

**theme/theme.yaml**（视觉 token，不含结构）

```yaml
layout: editorial
colors:
  primary: "#8C2F1B"
  accent: "#9A5B21"
  ink: "#2B211B"
  paper: "#FAF5EC"
fonts:
  heading: "Iowan Old Style, Georgia, serif"
  body: "Helvetica Neue, Arial, sans-serif"
```

**content/products/chili-crisp-210g.yaml**（示范 6.3 的硬内容写法）

```yaml
slug: chili-crisp-210g
status: published
order: 1
model_no: LS-210
category: retail-jars
scenarios: [retail-private-label, ecommerce]
certificates: [iso-22000, halal, fda]
markets: [US, UK, AE]
hs_code: "210390"
moq: 3000
moq_unit: pcs
lead_time:
  en: 15 days ex-stock, 45 days OEM
  zh: 现货 15 天，OEM 45 天
packaging:
  en: 24 jars per carton, 1,200 cartons per 20ft
  zh: 每箱 24 瓶，20 尺柜 1200 箱
sample_policy:
  en: 3 jars free, freight collect
  zh: 3 瓶免费，运费到付
title:
  en: Classic Chili Crisp 210 g
  zh: 经典油泼辣子 210 g
summary:
  en: 18% chili solids, batch SHU printed on every label.
  zh: 辣椒固形物 18%，每批实测 SHU 印在标签上。
highlights:
  - en: 18% chili solids, acid value ≤ 2 mg KOH/g
    zh: 辣椒固形物 18%，酸价 ≤ 2 mg KOH/g
  - en: Batch SHU tested, drift within ±8%
    zh: 每批实测辣度，批间漂移 ±8% 以内
cover: media/products/chili-crisp.png
```

**content/pages/privacy.yaml**（引擎无骨架，最小可用版）

```yaml
slug: privacy
status: published
title:
  en: Privacy & Cookies
  zh: 隐私与 Cookie
summary:
  en: What we collect when you send an enquiry, why, and how long we keep it.
  zh: 你发询盘时我们收集什么、为什么、保留多久。
cover: media/brand/privacy.png    # 有就填；没有就删这行——引用不存在的图会 MEDIA_NOT_FOUND
blocks:
  - type: richtext
    title:
      en: What we collect
      zh: 我们收集什么
    body:
      en: |
        Name, company, country, email and the content of your message.
        Server logs keep a hashed IP for rate limiting; the raw address is not stored.

        Retention: enquiry records 24 months. Write to sales@layanshi.example to have them removed.
      zh: |
        姓名、公司、国家、邮箱与询盘正文。
        服务端日志只保留限流用的哈希 IP，不存明文地址。

        保留期：询盘记录 24 个月。需要删除请写邮件至 sales@layanshi.example。
  - type: cta
    title:
      en: Questions about your data?
      zh: 对数据有疑问？
    cta_text:
      en: Email us
      zh: 发邮件
    cta_href: "mailto:sales@layanshi.example"
```

**content/certificates/iso-22000.yaml**

```yaml
- slug: iso-22000
  status: published
  order: 1
  name:
    en: ISO 22000:2018
    zh: ISO 22000:2018
  issuer: SGS-CSTC
  number: SG25-008417
  issued: "2026-02-20"
  expires: "2029-02-19"
  markets: [US, UK, AE, SG]
  file: media/certs/iso-22000.pdf
```

### B 一页速记

> **顺序**：资料（S0）→ 定位（S1）→ 骨架（S2）→ 内容（S3）→ 媒体（S4）→ QA（S5）→ 门禁（S5.5）→ 移交（S6）。
> **钱花在**：S3 的内容（40 分）与 S5 的门禁（25 分），不是配色。
> **必须人补的五件**：实体信息块、回复承诺、条款页、隐私页、每页 cover。
> **交付线**：Veto 9/9 + 总分 ≥75 + 七维每维 ≥60%。对外承诺「优秀」，不承诺「一等」——v2.0 之后一等还要 ≥3 条 Distinction，属报价范围外的逐单定制。

---

*本篇随每单迭代：新坑位进第十一节（并同步 B5 编号），新硬约束进 6.1 与 6.2。连续两单未再触发的坑位，降级为脚注。*
