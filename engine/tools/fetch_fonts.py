#!/usr/bin/env python3
"""把 Google Fonts 的 latin 子集抓成本地 woff2，供主题自托管。

为什么自托管而不是直接 <link href="fonts.googleapis.com">：
外贸站的买家大量来自中东、南美、东南亚，Google Fonts 在这些地区不稳，
而字体是渲染阻塞资源——链接一慢，FCP 直接崩；国内访问更是常年不通。
自托管 + font-display:swap 是唯一能同时保住「设计稿字形」和「Lighthouse 分数」的做法。

用法：
    python tools/fetch_fonts.py --out clients/kestrel/media/fonts \
        "Space+Grotesk:wght@500;600;700" "Inter:wght@400;500;600" "JetBrains+Mono:wght@400;500"

只抓 latin / latin-ext 两段：客户站点是英文为主，中文走系统字体（思源/苹方），
把 CJK 字体打进站里会让产物从 3MB 涨到 30MB。
"""
import argparse
import os
import re
import sys
import urllib.request

UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/120.0 Safari/537.36")
CSS2 = "https://fonts.googleapis.com/css2?family={}&display=swap"
# 只保留这两个子集；其余（cyrillic / vietnamese / greek）对英文站是死重量
KEEP = ("latin", "latin-ext")


def fetch(url: str) -> bytes:
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=20) as r:
        return r.read()


def parse_css(css: str):
    """按 /* subset */ 切块，返回 [(subset, family, weight, url)]。"""
    out = []
    for m in re.finditer(r"/\*\s*([a-z-]+)\s*\*/\s*@font-face\s*\{(.*?)\}", css, re.S):
        subset, body = m.group(1), m.group(2)
        if subset not in KEEP:
            continue
        fam = re.search(r"font-family:\s*'([^']+)'", body)
        wt = re.search(r"font-weight:\s*(\d+)", body)
        url = re.search(r"src:\s*url\((https://[^)]+)\)", body)
        if fam and wt and url:
            out.append((subset, fam.group(1), wt.group(1), url.group(1)))
    return out


def slug(s: str) -> str:
    return re.sub(r"[^a-z0-9]+", "-", s.lower()).strip("-")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True, help="输出目录，通常是 <site>/media/fonts")
    ap.add_argument("families", nargs="+", help='形如 "Inter:wght@400;600"')
    args = ap.parse_args()

    os.makedirs(args.out, exist_ok=True)
    total = 0
    for fam in args.families:
        css = fetch(CSS2.format(fam)).decode("utf-8")
        faces = parse_css(css)
        if not faces:
            print("  ! 没解析出 @font-face：", fam, file=sys.stderr)
            continue
        for subset, family, weight, url in faces:
            # latin-ext 只在有该子集时才生成，文件名带后缀
            name = f"{slug(family)}-{weight}"
            if subset == "latin-ext":
                name += "-ext"
            dest = os.path.join(args.out, name + ".woff2")
            data = fetch(url)
            with open(dest, "wb") as f:
                f.write(data)
            total += len(data)
            print(f"  {os.path.basename(dest):44s} {len(data)/1024:6.1f} KB")
    print(f"合计 {total/1024:.1f} KB -> {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
