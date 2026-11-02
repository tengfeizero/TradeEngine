# Kestrel Aero — 交付案例记录

第三个行业站的完整交付记录（无人机 / UAV），验证引擎对暗色科技风
（`tech` 布局）与强合规行业（FCC / CE RED / UN38.3）的支撑能力。

## 需求来源

`D:/Users/ASK/Desktop/ztf4/d04/design/aeronova-website.html` —— 客户提供的参考站
（AeroNova 单页 HTML，深色科技风：#0A0E1A 底 + 青色霓虹 + 网格背景）。
要求：沿用其信息架构与视觉语言，行业内容换成无人机整机出口。

## 参考站 → 引擎实体的映射

| 参考站 section | 引擎产出 |
|---|---|
| Hero + 4 stats | 首页 `hero` + `stats`（6 平台 · 15 年 · 48 国 · 99.2% 一次实飞通过） |
| Product Lines（6 系列） | `categories.yaml` 6 品类 + 首页 `category_grid` |
| Use Cases（6 场景） | `scenarios.yaml` 6 条 + `/scenarios/` 自动页 |
| Core Technology（6 卡） | `/technology/`（`image_text` × 3 + 工程师 FAQ） |
| 50,000m² Factory | `/factory/`（`richtext` + `gallery` + 6 步 `process` + `stats`） |
| Certifications（6 项） | `certificates.yaml` 6 条（FCC ID / CE RED / RoHS / UN38.3 / ISO 9001 / IEC 60529） |
| Global Cases（4 家） | `cases/` 5 个案例（DE 果园 / BR 电网 / AE 安防 / AU 矿山 / JP FPV） |
| Inquiry 表单 | `inquiry` 区块（落库 + 邮件 + 反垃圾 + 无 JS 降级） |

产品线：X9 Cine（影视）、M4 Mini（249 g）、AG-40 Hexa（40 L 植保）、AG-16（小地块）、
IN-25（双光巡检）、SR-30 Sentinel（系留安防）、LG-10 Carrier（10 kg 载货）、FX-5 Racer（FPV）。
HS 编码按 HS 2022 的 8806 无人机子目逐一对应（880610 / 880621 / 880622 / 880623）。

## 交付物

- **74 页双语站**（en/zh），sitemap 70 条，hreflang 72 组互链 0 断链
- 内容：6 品类 · 8 产品（含 HS 编码 / MOQ / 三组参数表 / 装箱数据 / SEO）· 6 场景 · 6 认证 · 5 案例 · 7 页面
- 媒体：`tools/gen_media.py` 生成深色分色渐变 + 技术网格（PNG 源 → WebP 交付，每张约 4KB），客户实拍图同名替换即可
- 主题：`theme/theme.yaml`（**`layout: tech` 暗色布局** + 青色 #35D6F0 + 按钮填充深青 #0E7490 + 琥珀 #FFC069）

## 质量实测

| 项 | 结果 |
|---|---|
| 构建 | 0 错误 0 警告 |
| Lighthouse（移动端，本地 Edge 无头） | **P 100 / A 100 / BP 100 / SEO 100**，LCP 1.1s · CLS 0 · TBT 0ms（报告 `lighthouse-en-home.json`） |
| 校验 | 36 条规则全过 |

## 交付过程中发现并修复的引擎缺陷（第 4–6 号「静默失败」）

1. **I18N_UNKNOWN_KEY（本次最重要的修复）**：YAML 流式映射 `{ en: A, 20 MP, zh: B }`
   在规范里合法——逗号后的裸值成为「无值键」，en 被静默截断到第一个逗号，页面无任何报错。
   校验器新增语言键白名单规则后一次性抓出 kestrel 104 处、**ridgeway 存量 41 处**
   （r100-city 的「6061-T6 aluminium, triple butted」一直只显示前半句）。
   两个站点共 290 处内容已转换为块式映射并复核。
2. **tech 布局漏覆盖 `.image-text ul`**：图文区块的项目符号仍是浅色主题的
   `rgba(0,0,0,.75)`，在深色底上对比度 1.06:1，整段文字几乎不可见。
   全局改为 `var(--muted)`，所有主题自适应。
3. **`process` / `team` 区块用 h4 造成标题跳级**：h2 → h4 违反标题层级审计。
   模板与样式同步改为 h3。

配套改进：

- `--primary-fill` 主题变量：暗色主题里「深底文字色」与「按钮填充色」不能取同一个值
  （文字要亮、按钮要深），拆分后按钮白字对比恒定达标，未设置时回落 `--primary`，
  浅色主题行为不变。
- `tech` 布局的 hero 氛围光晕由硬编码绿色改为 `color-mix(var(--primary))`，跟随主题色。
- `.skip:focus` 补文字色（白底上原来是浅色文字，键盘用户不可见）。

## 工时

内容与构建约 35 分钟（含 290 处 i18n 内容修复与三处引擎修复 + Lighthouse 两轮）。
诚实备注：AI 全程代写，不代表人工工时；它说明的是引擎把「第二类行业」的
新增工作量压缩到了内容层，无一处为行业特性改代码（三处均为通用缺陷修复）。

## 本地预览

```bash
cd clients/kestrel
engine build          # 产物 .build/public
engine serve -addr :8091
# EN http://localhost:8091/en/   ZH http://localhost:8091/zh/
```

示例数据说明：公司、证书编号、案例与联系信息均为虚构（`.example.com` 域名），
用于演示引擎交付流程，不构成真实商业凭证。

## 0.13.0 标准追平（2026-10-23）

引擎 0.13.0（B1 v2.0 七项欠账落地）后，本站按新能力补齐了七项中的五项：

| 项 | 落地 |
|---|---|
| R2 市场准入 | `settings.compliance`：Kestrel Europe B.V.（鹿特丹）为 GPSR 责任方，KvK/VAT/法人齐全，LUCID + WEEE 编号；三处出口 = 产品页 GPSR 段 + 页脚责任方行 + `/legal-notice/` |
| C9 商业模式政策 | 新增 `/business-terms/`：OEM 起订量（巡检 50 / 载具 30）、白标的证书挂靠条件、按区域独家（1,200 台/年）、**固件不开放源码**的明说、报价 30 天 |
| D6 工厂证据 | `/factory/` 插入 `factory` 区块：6 项设备（含 EMC 预测试暗室，注明"只做预测试"）、5 份第三方报告编号（SGS / TÜV RED / UN38.3 / ISO 9001 / BSCI）、留样 18 个月（电池老化窗口）、2 张带日期照片 |
| D7 客户证言 | `/about/` 插入 `testimonials`：BR 植保（ag-40）/ DE 巡检（in-25）/ US 影视租赁（x9-cine），各挂对应产品 |
| E7 / C10 / F2 | `llms.txt`、specs.csv ×16、`sku-list.csv` 9 行、图包 689KB、字节预算最差 58KB（上限 1024）——构建期自动 |

新增页面 ×3（privacy-policy / business-terms / legal-notice），**74 → 80 页，0 错 0 警**。

## 0.13.1 内容追平：P0 内容缺口闭环（2026-10-23）

按《三站B1v2.0评分报告》P0 清单补齐内容侧缺口（引擎零改动）：

- **C6 贸易条款**：新增 `/trade-terms/`（en/zh）——T/T 30% 定金 / 70% 见提单副本、L/C（年合同 ≥USD 150k 即期）、FOB 深圳（盐田）+ CIF/EXW/DAP、随货单证（CE-RED/FCC 副本、UN38.3 摘要、MSDS、产地证）、索赔与质保（到港 30 天；我方责任换件+延保 6 个月，与 `/quality/` 口径一致）
- **C4 价格策略**：trade-terms 页「Why there is no price list」段——按配置一机一价的解释 + 公开锚点（配置表含 FOB、演示机 1.3–1.6×、报价 30 天）
- **B6 十三问**：Q4（付款比例）/ Q5（FOB 港口）由 trade-terms 补齐；`/factory/` 补 Q13（视频验厂 + 区域参考客户引荐）→ **13/13 问**
- 页数 **80 → 82**（+trade-terms 双语），0 错 0 警
- 复评分（B1 v2.0，E6 N/A）：**90 → 96**（C4/C6/B6 各 +2）

## 0.13.4 换皮：aeronova 航宇暗色（2026-10-23）

- 皮肤 `tech → aeronova`（引擎 0.13.4 新增第九套预置，按 design/aeronova-website.html 逐段转译）：全页暗底 `#0A0E1A` + 固定 60px 青色网格与三枚辉光球；核心技术/认证/CTA/询盘/页脚坐 `#0D1220` bg2 满宽暗带（询盘带顶部青色辉光）；Space Grotesk 600 全系 + JetBrains Mono 编号；渐变按钮（青→靛，深字压底）+ 渐变 logo 底座 + indigo 药丸眉题 + 卡片悬停渐变线滑入
- theme.yaml 色板/字体/font_faces 本就按该设计稿配齐，零改动
- 页数不变（82），0 错 0 警；Lighthouse 四项 **100 / 97 / 100 / 100**
