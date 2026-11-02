# Ridgeway Cycles — 交付案例记录

第二个真实行业站的完整交付记录，用于验证《B3 外贸站交付引擎设计实现》第十二节 M2 验收标准
（「同一引擎交付两个不同行业站」）与复用率指标。

## 需求来源

`D:/Users/ASK/Desktop/ztf4/d04/design/zentea-website.html` —— 客户提供的参考站
（单页 HTML，Misty Peak Tea 风格）。要求：沿用其信息架构与视觉语言，行业内容换成自行车 OEM/ODM。

## 参考站 → 引擎实体的映射

| 参考站 section | 引擎产出 |
|---|---|
| Hero + 4 stats | 首页 `hero` + `stats` 区块（120k 台/年 · 15 年 · 45 国 · 98.6% 合格率） |
| Tea Collection（6 类） | `categories.yaml` 6 类 + 首页 `product_grid` |
| Applications（4 场景） | `scenarios.yaml` 4 条 + `/scenarios/` 自动页 |
| Tea Garden | `/about/`（`image_text` + `stats` + `richtext`） |
| Craftsmanship（6 步） | `/factory/` 的 `process` 区块（下料→焊接→校形→涂装→装配→出厂检验） |
| Factory 双栏 | `/factory/` 两个 `image_text` |
| Certifications（6 项） | `certificates.yaml` 6 条 + `/certifications/`（含证书编号公开与到期自动下架） |
| Clients（4 家） | `cases/` 4 个案例 + `/cases/` |
| Inquiry 表单 | `inquiry` 区块（落库 + 邮件 + 反垃圾 + 无 JS 降级） |

## 交付物

- **66 页双语站**（en/zh），sitemap 62 条，hreflang 64 组互链 0 断链
- 内容：6 分类 · 8 产品（42 字段级，含 HS 编码 / MOQ / 参数表 / 装箱数据 / SEO）· 4 场景 · 6 认证 · 4 案例 · 6 页面
- 媒体：`tools/gen_media.py` 生成占位渐变图（与参考站分色卡一致），客户实拍图同名替换即可
- 主题：`theme/theme.yaml`（钢青 #35505C + 深琥珀 #7A5218 + 暖纸 #F4F1EA，editorial 布局）

## 质量实测

| 项 | 结果 |
|---|---|
| 构建 | 0 错误 0 警告 |
| Lighthouse（移动端，本地 Edge 无头） | **Performance 100 / Accessibility 100 / Best Practices 100 / SEO 100**，LCP 1.9s · CLS 0 · TBT 0ms（报告 `lighthouse-en-home.json`） |
| 校验 | 36 条规则全过（含 MOQ 单位、HS 编码、案例结果数字、证书到期） |

## 工时（AI 辅助全流程）

| 阶段 | 用时 |
|---|---|
| `engine new` + 读参考站 | 1 min |
| 内容撰写（settings / theme / 6 分类 / 8 产品 / 4 案例 / 6 页面） | 8 min |
| 媒体生成 + 构建修复 | 3 min |
| Lighthouse + 视觉核对 | 1 min |
| **合计** | **13 min**（10:14–10:27） |

诚实备注：这是 AI 全程代写的交付，不代表人工工时。它说明的是——
引擎把交付瓶颈压缩到了「内容生产」这一件事上；人工使用引擎时，
撰写 8 个产品 × 42 字段的双语内容仍需数小时，但无需任何代码或模板改动。

## 复用率（本案例的核心结论）

- **引擎代码改动：0 行**（交付全程未改任何 Go 代码；唯一的改动是修复一个引擎缺陷，见下）
- **站点定制：100% 落在 `content/` + `theme/` + `tools/`** —— 三个目录与引擎完全解耦
- 模板、区块、校验规则、询盘链路、sitemap、SEO 输出全部原样复用
- M2 验收「同一引擎交付两个不同行业站」：**达成**（茶叶 zentea / 自行车 ridgeway）

## 过程中发现并修复的引擎缺陷（1 个）

`theme.yaml` 存在但解析失败时被静默吞掉（`_ = readYAML`），整站悄悄回退默认配色——
页面照常产出，肉眼极难发现。已改为报错（`internal/loader/file.go`）。
根因是 `fonts.body` 的值以 `"` 开头（`"Helvetica Neue", Helvetica, ...`），
YAML 将其解析为引号标量后遇语法垃圾。教训沉淀为一条内容规则：
**主题里的字体值要么整体加引号，要么不带内部引号。**

## 0.13.0 标准追平（2026-10-23）

本站是七项新能力在交付站的**首个完整落点**（kestrel / leafworks 随后按同一模式补齐）：

- **R2**：`settings.compliance`（Ridgeway Europe B.V. 鹿特丹 + KvK/VAT/法人 + LUCID），三处出口；新增 `/legal-notice/`
- **C9**：新增 `/business-terms/`（OEM/白标/独家/IP/报价有效期）
- **D6**：`/factory/` 插入 `factory` 区块（设备 / SGS·TÜV·BSCI 审核编号 / 留样 24 个月）
- **D7**：`/about/` 插入 `testimonials`（SE / ES / DE 三家，挂到对应产品）
- **C10/E7/F2**：specs.csv ×16、`sku-list.csv`、图包 531KB、`llms.txt`、字节预算最差 56KB

新增页面 ×2（business-terms / legal-notice）+ 隐私页补发布，**70 → 74 页，0 错 0 警**。

## 0.13.1 内容追平：C4 定价策略（2026-10-23）

- `/trade-terms/` 补「Why there is no price list」段：几何×材质×配置定价格的解释 + 公开锚点（样品 2× FOB 可抵扣、12h FOB 估价、报价 30 天 + 铝/电芯 5% 波动条款、现货平台季度价目表）
- 页数不变（74），0 错 0 警；复评分（B1 v2.0，E6 N/A）：**92 → 94**（C4 +2）
