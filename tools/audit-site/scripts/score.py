#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""audit-site score —— B1 v2.0 计分校验器（skill: audit-site）

职责边界：
  判定（哪个条目 pass/fail/na）必须由人/LLM 依据 B1 条目 + 证据作出；
  本脚本只负责「算分、验结构、堵漏洞」——铁律：分数由脚本产出，不许先写分再凑证据。

用法：
  python score.py --init out/verdict.json     # 生成空判定表（50 条目 + Veto 9 + Distinction 8）
  python score.py --verdict out/verdict.json  # 校验并计分 → out/score.json + scorecard.md

计分口径（B1 v2.0 §三）：
  50 条（A7/B8/C10/D7/E7/F5/R6）× 2 分 = 100；分数 = 通过条数 × 2；
  N/A 条目不适用需给理由，满分上限相应下降。
  一等 = 总分 ≥90 + Veto 9 条全过 + 每维通过率 ≥60% + Distinction 通过 ≥3。
"""
import argparse
import json
import sys
from datetime import date

# B1 v2.0 评分模型（B1 §三原文取证，2026-11-02）：
# 50 条 × 2 分 = 100；七维条数 A7/B8/C10/D7/E7/F5/R6（权重列 14/16/20/14/14/10/12 是条数×2）
DIM_SIZES = {"A": 7, "B": 8, "C": 10, "D": 7, "E": 7, "F": 5, "R": 6}
VETO_N = 9
DISTINCTION_N = 8
VALID = {"pass", "fail", "na"}


def item_ids():
    out = []
    for d, n in DIM_SIZES.items():
        out += [f"{d}{i}" for i in range(1, n + 1)]
    return out


def init_template(path, site=""):
    tpl = {
        "_说明": [
            "每个条目填 verdict(pass|fail|na) + evidence(指向证据文件/引用/实测值)。",
            "na 必须给 reason；pass 没有证据一律被 score.py 拒收。",
            "条目标题从 B1 正文/附录 E 取，禁止凭记忆编造条目含义。",
        ],
        "site": site,
        "date": date.today().isoformat(),
        "items": {i: {"verdict": "", "evidence": "", "reason": ""} for i in item_ids()},
        "veto": {f"V{i}": {"verdict": "", "evidence": ""}
                 for i in range(1, VETO_N + 1)},
        "distinction": {f"X{i}": {"verdict": "", "evidence": ""}
                        for i in range(1, DISTINCTION_N + 1)},
    }
    with open(path, "w", encoding="utf-8") as f:
        json.dump(tpl, f, ensure_ascii=False, indent=2)
    print(f"[score] verdict template -> {path}")
    print(f"[score] 共 {len(item_ids())} 条评分项 + {VETO_N} Veto + {DISTINCTION_N} Distinction")


def validate(v):
    errs, warns = [], []
    items = v.get("items", {})
    for i in item_ids():
        it = items.get(i)
        if it is None:
            errs.append(f"{i}: 缺条目")
            continue
        if it.get("verdict") not in VALID:
            errs.append(f"{i}: verdict 非法（{it.get('verdict')!r}，须 pass/fail/na）")
            continue
        if it["verdict"] in ("pass", "na") and not (it.get("evidence") or "").strip():
            errs.append(f"{i}: pass/na 必须附 evidence")
        if it["verdict"] == "na" and not (it.get("reason") or "").strip():
            errs.append(f"{i}: na 必须附 reason")
    veto = v.get("veto", {})
    for i in range(1, VETO_N + 1):
        k = f"V{i}"
        if veto.get(k, {}).get("verdict") not in ("pass", "fail"):
            errs.append(f"{k}: Veto 只允许 pass/fail，缺则按未评处理（不许 na）")
        elif veto[k]["verdict"] == "pass" and not veto[k].get("evidence"):
            errs.append(f"{k}: Veto pass 必须附 evidence")
    dist = v.get("distinction", {})
    for i in range(1, DISTINCTION_N + 1):
        k = f"X{i}"
        if dist.get(k, {}).get("verdict") not in ("pass", "fail", ""):
            errs.append(f"{k}: Distinction verdict 非法")
        elif dist[k].get("verdict") == "pass" and not dist[k].get("evidence"):
            errs.append(f"{k}: Distinction pass 必须附 evidence")
    return errs, warns


def compute(v):
    per_dim, na_items, passed_total, applicable_total = {}, [], 0, 0
    for d, n in DIM_SIZES.items():
        p = a = 0
        for i in range(1, n + 1):
            k = f"{d}{i}"
            vd = v["items"][k]["verdict"]
            if vd == "pass":
                p += 1
            elif vd == "na":
                na_items.append(k)
            else:
                pass
            if vd != "na":
                a += 1
        per_dim[d] = {"passed": p, "applicable": a, "total": n,
                      "rate": round(p / a, 3) if a else None,
                      "score": p * 2, "full": a * 2}
        passed_total += p
        applicable_total += a
    total = passed_total * 2
    full = applicable_total * 2
    x_pass = sum(1 for k in v.get("distinction", {}).values()
                 if k.get("verdict") == "pass")
    veto_all_pass = all(x.get("verdict") == "pass"
                        for x in v.get("veto", {}).values()) and len(v.get("veto", {})) == VETO_N
    dims_ok = all(d["rate"] is not None and d["rate"] >= 0.6 for d in per_dim.values())
    first_tier = (total >= 90 and veto_all_pass and dims_ok and x_pass >= 3
                  and full == 100)
    return {
        "site": v.get("site"), "date": v.get("date"),
        "score": total, "full": full,
        "per_dim": per_dim, "na_items": na_items,
        "veto_all_pass": veto_all_pass,
        "distinction_passed": x_pass,
        "first_tier": first_tier,
        "first_tier_reasons": {
            "score_ge_90": total >= 90,
            "veto_all_pass": veto_all_pass,
            "every_dim_ge_60pct": dims_ok,
            "distinction_ge_3": x_pass >= 3,
            "no_na_cap_below_100": full == 100,
        },
    }


def scorecard_md(r):
    lines = ["| 维 | 通过/适用/满额 | 得分 | 通过率 |", "| --- | --- | --- | --- |"]
    for d, x in r["per_dim"].items():
        lines.append(f"| {d} | {x['passed']}/{x['applicable']}/{x['total']} "
                     f"| {x['score']}/{x['full']} | {x['rate']} |")
    lines.append(f"| **合计** | | **{r['score']}/{r['full']}** | |")
    if r["na_items"]:
        lines.append(f"\nN/A：{', '.join(r['na_items'])}（满分上限 {r['full']}）")
    vt = "✅ 全过" if r["veto_all_pass"] else "❌ 有不过"
    lines.append(f"\nVeto 9 条：{vt}｜Distinction：{r['distinction_passed']} 条通过")
    tier = "🥇 一等" if r["first_tier"] else "— 未达一等"
    lines.append(f"\n评级：{r['score']}/{r['full']}，{tier}")
    return "\n".join(lines)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--init")
    ap.add_argument("--verdict")
    ap.add_argument("--site", default="")
    args = ap.parse_args()

    if args.init:
        init_template(args.init, args.site)
        return
    if not args.verdict:
        ap.error("需要 --init 或 --verdict 之一")

    with open(args.verdict, encoding="utf-8") as f:
        v = json.load(f)
    errs, _ = validate(v)
    if errs:
        print(f"[score] 判定表未通过校验，拒算（共 {len(errs)} 处）：", file=sys.stderr)
        for e in errs[:12]:
            print("  -", e, file=sys.stderr)
        if len(errs) > 12:
            print(f"  … 其余 {len(errs) - 12} 处从略", file=sys.stderr)
        sys.exit(1)

    r = compute(v)
    outdir = args.verdict.rsplit("/", 1)[0] if "/" in args.verdict else "."
    with open(outdir + "/score.json", "w", encoding="utf-8") as f:
        json.dump(r, f, ensure_ascii=False, indent=2)
    md = scorecard_md(r)
    with open(outdir + "/scorecard.md", "w", encoding="utf-8") as f:
        f.write(md + "\n")
    print(md)
    print(f"\n[score] -> {outdir}/score.json, {outdir}/scorecard.md")


if __name__ == "__main__":
    main()
