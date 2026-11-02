---
name: audit-site
description: 站点评测：输入一个网站 URL，按 B1 v2.0（50 条 × 2 分）与 B2 评分 SOP 输出一份带证据的评分报告。触发词：站点评测、给 xx.com 评分、审一下这个网站、site audit、评分报告。
---

# audit-site — 外贸站点评测

## 权威出处（铁律：不复制，只引用）

- **评什么** → `docs/B1优秀的外贸站的标准.md`（50 条 × 2 分 = 100；七维条数 A7/B8/C10/D7/E7/F5/R6，权重 14/16/20/14/14/10/12 = 条数×2；Veto 9 条；Distinction 8 条；一等 = ≥90 + Veto 全过 + 每维 ≥60% + X≥3）
- **怎么评** → `docs/B2外贸站评分SOP.md`（六步 P0–P5；判定口径表 §五；评分陷阱 §七；报告模板 §八）
- 本 skill 只承载**流程编排与工具**，条目定义与判定口径一律回读上述两份文档，禁止凭记忆写条目含义。

## 输入 / 输出

- 输入：一个站点 URL（公网域名，或本地 `engine serve` 地址）
- 输出：`reports/<域名>-<YYYYMMDD>/` 目录，含：
  - `evidence/`（probe.py 产物：evidence.json + 关键页快照）
  - `verdict.json`（50 + 9 + 8 条逐条判定，由执行者填写）
  - `score.json` / `scorecard.md`（score.py 产出，**分数只能来自这里**）
  - `report.md`（最终报告，模板见 `templates/report.md`）

## 执行流程（对应 B2 六步）

### P0 立项取数（≈2 min，脚本化）

```bash
cd <skill 目录>/scripts
python probe.py --url https://<domain> --out <repo>/reports/<domain>-<date>
```

- 产物直接覆盖 B2 的 P0+P2（可达性/跳转链、响应头、robots/sitemap/llms.txt、404、首屏预算、暗链扫描、SPF/DKIM/DMARC、TLS 到期、爬虫死链/重复标题）。
- `--fast` 用于冒烟；正式评审禁用（爬取是死链与重复标题的唯一来源）。
- 本地站点（127.0.0.1）自动跳过 DNS/TLS 项，报告中标注「本地实测，部署侧另计」。

### P1 Veto 快筛（LLM 判 + 脚本证据）

九条 Veto（V1–V9）逐一给出 pass/fail，**只有脚本证据的用脚本，需要看的回读快照**：

- V8 被入侵 → `dark_link_scan.suspicious` + 快照 home.html 底部人工复核（暗链常藏 footer）
- V9 询盘通道 → `mail_records`（DMARC 缺失按不过处理）+ 表单可提交性（`has_form` 只证明表单存在，提交链路需人工验）
- V3 移动端 → 脚本不覆盖，需浏览器目验（header 高度/横向溢出/导航可展开）

### P2 自动取证（已并入 P0 脚本，省下 ~25 min）

Lighthouse 四项**不在 probe.py 范围内**，若被审站可访问，另行跑移动端 Lighthouse（首页 + 1 产品页 + 1 列表页），结果手动并入 evidence 目录（文件名 `lighthouse-<page>.json`）。跑不了就记「未测」，不许编数。

### P3 人工核查（LLM 主战场，55 min 预算）

按 B1 条目逐条判定，铁律三条：

1. **每条 pass/na 必须附 evidence**——指向 evidence.json 的具体字段、快照文件、或引述页面原文；引不出证据的判 fail，不许凭印象过。
2. **na 必须给 reason**——N/A 收紧口径：只允许「该站点类型结构上不适用」（如无实体工厂的贸易公司判 D6 时需引 B1 口径，不是「没找到」就 N/A）。
3. **先读 B1 再填 verdict.json**：`python score.py --init <out>/verdict.json --site <domain>` 生成空表，条目标题回读 B1 附录 E。

### P4 计分（≈1 min，脚本化）

```bash
python score.py --verdict <out>/verdict.json
```

- score.py 拒收：缺条目 / verdict 非法 / pass 无证据 / na 无理由 / Veto na。
- 产出 `score.json` + `scorecard.md`；**报告中任何分数（含分维小分）必须与此一致**。

### P5 报告（≈15 min）

按 `templates/report.md` 组装 `report.md`：评分卡（贴 scorecard.md）→ Veto 结论 → 缺口清单（P0 阻塞交付 / P1 / P2，每条缺口指向对应 B1 条目号与证据）→ 修复路线（按投入产出排序）→ 证据索引。结尾必须带**口径声明**：哪些项脚本实测、哪些项 LLM 判定、哪些项未测（Lighthouse/移动端目验）。

## 判定边界（哪些机器测、哪些人判）

| 类别 | 项 | 来源 |
| --- | --- | --- |
| 脚本实测，禁止 LLM 改判 | 可达性/跳转、安全头、缓存头、压缩、robots/sitemap/llms、404、首屏字节、SPF/DKIM/DMARC、TLS、死链、重复标题、暗链 | evidence.json |
| LLM 判，必须引证据 | C 系（商业模式/价格/条款）、D 系（工厂/证言/内容真实性）、E 系（NAP/AI 可见性）、文案质量 | 快照 + 页面原文 |
| 需浏览器目验，脚本不覆盖 | V3 移动端三量、Lighthouse 四项、真实表单提交（V9 提交链路） | 另行实测，测不了标注「未测」 |

## 失败与降级

- 站点不可达 → 停在 P0，报告只写「不可达 + 错误详情」，不打分。
- DNS 解析器缺失（dig/nslookup 都没有）→ 邮件三记录标「无法实测」，按未测处理，不许默认 pass。
- 目标站反爬（403/挑战页）→ 快照留证，能取多少取多少，报告声明覆盖度。
