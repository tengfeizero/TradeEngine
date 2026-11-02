"""为交付站生成媒体素材：实拍图裁切 + 调色板感知的品牌图案占位图。

交付期最常见的两种状态是「客户照片还没给」和「只有一堆原图」。
这个工具把两种状态都收口到同一套约定上：

  1. **有原图**：写 media/sources.yaml 声明「哪张原图 → 哪个槽位、裁哪里」，
     工具负责裁成 4:3、降到 1600px 宽、压成 JPEG（q86）。
  2. **没原图**：按主题调色板生成抽象构图——同一站点同一视觉语言，
     每个槽位又各不相同（种子取自路径），换掉客户真图时路径不变。

为什么不再用纯色/网格占位：全站二十多张图如果长得一样，页面看起来像
「图没加载完」，比留白更伤专业感。占位图也要当成设计资产来做。

输出统一为 JPEG：
  - 卡片是 object-fit: cover 的 4:3 裁切，1600×1200 足够 2 倍屏；
  - PNG 存渐变会落到 1MB 以上，撞穿 settings.yaml 的 max_kb: 400 告警线。

依赖：pillow（`pip install pillow`）。生成的图会进版本库，构建本身不依赖本工具。

用法：
    python tools/gen_media.py --site clients/kestrel            # 缺什么补什么
    python tools/gen_media.py --site clients/leafworks --force  # 全部重生成
    python tools/gen_media.py --site clients/kestrel --list     # 只列出槽位
"""

from __future__ import annotations

import argparse
import colorsys
import hashlib
import os
import re
import sys

try:
    from PIL import Image, ImageDraw, ImageFilter
except ImportError:  # pragma: no cover - 工具链缺失时的明确提示
    sys.exit("需要 pillow：pip install pillow")

RASTER = (".png", ".jpg", ".jpeg", ".webp")
W, H = 1600, 1200  # 4:3，与卡片的 object-fit 比例一致


# ---------------------------------------------------------------- 主题解析

def read_theme(site: str) -> tuple[dict[str, str], str]:
    """读取 theme/theme.yaml 的色板与版式。

    只做行级解析，不引入 yaml 依赖：工具要能在只有 pillow 的机器上跑。
    能识别的形态就是 theme.yaml 里真实存在的形态（两空格缩进的 colors 块）。
    """
    path = os.path.join(site, "theme", "theme.yaml")
    colors: dict[str, str] = {}
    layout = "classic"
    in_colors = False
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.rstrip("\n")
            if re.match(r"^layout:\s*", line):
                layout = line.split(":", 1)[1].strip().strip("'\"") or "classic"
            if re.match(r"^colors:\s*$", line):
                in_colors = True
                continue
            if in_colors:
                m = re.match(r"^\s{2}([a-z-]+):\s*(.+?)\s*$", line)
                if not m:
                    if line.strip() and not line.startswith((" ", "\t", "#")):
                        in_colors = False
                    continue
                key, val = m.group(1), m.group(2)
                val = re.sub(r"\s+#.*$", "", val).strip().strip("'\"")
                colors[key] = val
    return colors, layout


def parse_color(value: str, fallback: tuple[int, int, int]) -> tuple[int, int, int]:
    """把 #RRGGBB / rgba(...) 解析成 RGB；暗色叠加用的 alpha 在这里被丢弃。"""
    if not value:
        return fallback
    v = value.strip()
    if v.startswith("#"):
        h = v[1:]
        if len(h) == 3:
            h = "".join(c * 2 for c in h)
        if len(h) >= 6:
            return tuple(int(h[i : i + 2], 16) for i in (0, 2, 4))  # type: ignore[return-value]
    m = re.match(r"rgba?\(([^)]+)\)", v)
    if m:
        parts = [p.strip() for p in m.group(1).split(",")[:3]]
        if len(parts) == 3:
            return tuple(int(float(p)) for p in parts)  # type: ignore[return-value]
    return fallback


def luma(c: tuple[int, int, int]) -> float:
    return (0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]) / 255.0


def mix(a: tuple[int, int, int], b: tuple[int, int, int], t: float) -> tuple[int, int, int]:
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))  # type: ignore[return-value]


def shift(c: tuple[int, int, int], dh: float, ds: float = 0.0, dl: float = 0.0) -> tuple[int, int, int]:
    """在 HLS 空间轻微偏移色相/饱和/亮度，用来让同色板下的槽位互相区分。"""
    r, g, b = (v / 255.0 for v in c)
    h, l, s = colorsys.rgb_to_hls(r, g, b)
    h = (h + dh) % 1.0
    s = max(0.0, min(1.0, s + ds))
    l = max(0.02, min(0.98, l + dl))
    r, g, b = colorsys.hls_to_rgb(h, l, s)
    return (int(r * 255), int(g * 255), int(b * 255))


def seed_of(text: str) -> int:
    return int(hashlib.sha256(text.encode("utf-8")).hexdigest()[:8], 16)


# ---------------------------------------------------------------- 程序化构图

def radial_mask(size: tuple[int, int], cx: int, cy: int, radius: int) -> Image.Image:
    """径向渐变蒙版（中心实、边缘虚），用来叠柔光斑。"""
    mask = Image.new("L", size, 0)
    d = ImageDraw.Draw(mask)
    steps = 24
    for i in range(steps, 0, -1):
        t = i / steps
        r = int(radius * t)
        val = int(255 * (1 - t) ** 2)
        d.ellipse((cx - r, cy - r, cx + r, cy + r), fill=val)
    return mask.filter(ImageFilter.GaussianBlur(radius * 0.18))


def gradient(size: tuple[int, int], c0: tuple[int, int, int], c1: tuple[int, int, int], angle: str) -> Image.Image:
    """两色渐变底。angles: diag / v / h。"""
    w, h = size
    if angle == "h":
        small = Image.new("RGB", (256, 1))
        px = small.load()
        for x in range(256):
            px[x, 0] = mix(c0, c1, x / 255)
        return small.resize(size, Image.BICUBIC)
    small = Image.new("RGB", (256, 256))
    px = small.load()
    span = 510
    for y in range(256):
        for x in range(256):
            t = (x + (y if angle == "diag" else 0)) / span
            px[x, y] = mix(c0, c1, min(1.0, t))
    return small.resize(size, Image.BICUBIC)


def draw_motif(img: Image.Image, layout: str, accent: tuple[int, int, int], s: int) -> None:
    """版式化的纹理母题：tech 走细网格+辉光，editorial 走拱形，classic 走斜带。"""
    w, h = img.size
    layer = Image.new("RGBA", img.size, (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    if layout == "tech":
        step = 90
        for x in range(0, w, step):
            d.line([(x, 0), (x, h)], fill=(*accent, 30), width=1)
        for y in range(0, h, step):
            d.line([(0, y), (w, y)], fill=(*accent, 30), width=1)
        for i in range(3):  # 同心环，暗示雷达/镜头
            r = 180 + i * 150 + (s % 40)
            d.ellipse((w * 0.66 - r, h * 0.34 - r, w * 0.66 + r, h * 0.34 + r), outline=(*accent, 64), width=2)
    elif layout == "editorial":
        for i in range(4):  # 拱门
            rw = int(w * (0.30 + i * 0.13))
            rh = int(rw * 1.5)
            bx = int(w * (0.22 + (i % 2) * 0.42))
            by = int(h * 0.86)
            d.arc((bx - rw, by - rh, bx + rw, by + rh), 180, 360, fill=(*accent, 76), width=3)
    else:
        for i in range(6):  # 斜带
            x0 = -h + i * 300 + (s % 120)
            d.line([(x0, h), (x0 + h, 0)], fill=(*accent, 36), width=int(w * 0.045))
    img.alpha_composite(layer)


def brand_placeholder(path: str, colors: dict[str, str], layout: str) -> None:
    """按调色板生成一张品牌图案占位图（确定性：同路径永远同一张）。"""
    s = seed_of(path)
    primary = parse_color(colors.get("primary", ""), (47, 107, 63))
    accent = parse_color(colors.get("accent", ""), primary)
    secondary = parse_color(colors.get("secondary", ""), primary)
    paper = parse_color(colors.get("paper", ""), (247, 244, 236))
    surface = parse_color(colors.get("surface", ""), (255, 255, 255))
    ink = parse_color(colors.get("ink", ""), (26, 26, 26))
    dark = luma(paper) < 0.4

    # 同色板下按路径做色相/明度偏移，让二十多张图各有身份而不跑调。
    # 偏移量按种子分三档，相邻卡片肉眼可辨，整体又还在一个色系里。
    step = s % 3
    dh = (-0.055, 0.0, 0.055)[step]
    dl = (-0.06, 0.0, 0.06)[(s >> 4) % 3]
    if dark:
        c0 = shift(mix(paper, primary, 0.30 + step * 0.05), dh, 0.03, dl)
        c1 = shift(mix(surface, ink, 0.35), dh * 0.5)
    else:
        c0 = shift(mix(paper, primary, 0.40 + step * 0.06), dh, 0.04, -0.05 + dl)
        c1 = shift(mix(surface, primary, 0.08 + step * 0.04), dh * 0.5, 0.02, 0.03 + dl)
    base = gradient((W, H), c0, c1, ["diag", "v", "h"][s % 3]).convert("RGBA")

    # 3–4 个柔光斑：主色/强调色/次色各一枚，位置由种子决定。
    # 浅色主题往亮里提（像被阳光照到的纸面），暗色主题往主色压。
    tints = [primary, accent, secondary, shift(accent, 0.08)]
    for i in range(3 + (s >> 3) % 2):
        tint = tints[i % len(tints)]
        if not dark:
            tint = mix(tint, (255, 255, 255), 0.55)
        cx = int(W * (0.12 + ((s >> (4 + i * 3)) % 76) / 100.0))
        cy = int(H * (0.08 + ((s >> (7 + i * 2)) % 78) / 100.0))
        r = int(W * (0.30 + ((s >> (i + 2)) % 28) / 100.0))
        mask = radial_mask((W, H), cx, cy, r)
        glow = Image.new("RGBA", (W, H), (*shift(tint, dh), 0))
        alpha = 96 if dark else 110
        glow.putalpha(mask.point(lambda v, a=alpha: v * a // 255))
        base.alpha_composite(glow)

    # 焦点几何：一枚实心小圆 + 两道细环，清晰的包豪斯海报语言。
    # 不做模糊——几何要「像故意画上去的」，模糊会变成脏斑。
    fx = int(W * (0.24 + (s % 50) / 100.0))
    fy = int(H * (0.30 + ((s >> 5) % 40) / 100.0))
    fr = int(min(W, H) * (0.30 + (s >> 9) % 12 / 100.0))
    geo = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    gd = ImageDraw.Draw(geo)
    disc_c = shift(mix(primary, ink if not dark else paper, 0.25), dh)
    gd.ellipse((fx - fr * 0.30, fy - fr * 0.30, fx + fr * 0.30, fy + fr * 0.30), fill=(*disc_c, 200))
    gd.ellipse((fx - fr, fy - fr, fx + fr, fy + fr), outline=(*shift(accent, dh), 210), width=max(3, fr // 90))
    fr2 = int(fr * 1.45)
    gd.ellipse((fx - fr2, fy - fr2, fx + fr2, fy + fr2), outline=(*shift(primary, dh + 0.04), 90), width=2)
    base.alpha_composite(geo)

    draw_motif(base, layout, accent, s)

    # 暗角：把视线收到画面中心，避免边缘发灰显脏
    vig = Image.new("L", (W, H), 0)
    vd = ImageDraw.Draw(vig)
    vd.ellipse((-W * 0.18, -H * 0.18, W * 1.18, H * 1.18), fill=255)
    vig = vig.filter(ImageFilter.GaussianBlur(160))
    shade = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    a = 66 if dark else 40
    shade.putalpha(Image.eval(vig, lambda v: int((255 - v) * a / 255)))
    base.alpha_composite(shade)

    out = base.convert("RGB")
    # 细颗粒：消除渐变的色带（JPEG 会把色带放大成一道道光晕）
    noise = Image.effect_noise((W, H), 5).convert("L")
    out = Image.composite(out.point(lambda v: min(255, v + 3)), out, noise.point(lambda v: 60 if v > 128 else 0))
    save_jpeg(out, path)


def save_jpeg(img: Image.Image, path: str) -> None:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    img.convert("RGB").save(path, "JPEG", quality=86, optimize=True, progressive=True)
    print(f"  {os.path.relpath(path)}  {os.path.getsize(path) // 1024} KB")


# ---------------------------------------------------------------- 分享卡（M5-3）

CARD_W, CARD_H = 1200, 630  # 1.91:1，Facebook / LinkedIn / X 的横版大图比例


def load_font(size: int):
    """尽量用系统字体，找不到就退回 Pillow 内置位图字体。

    品牌卡上的字要能看清品牌名，位图字体缩放会糊——所以先找系统 TTF。
    """
    from PIL import ImageFont

    for cand in (
        "C:/Windows/Fonts/segoeuib.ttf", "C:/Windows/Fonts/arialbd.ttf",
        "/System/Library/Fonts/Helvetica.ttc", "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    ):
        if os.path.exists(cand):
            try:
                return ImageFont.truetype(cand, size)
            except OSError:
                pass
    try:
        return ImageFont.load_default(size=size)
    except TypeError:  # Pillow < 10.1
        return ImageFont.load_default()


def read_settings_brand(site: str) -> dict[str, str]:
    """行级读 settings.yaml：brand.name / brand.tagline.en / base_url / catalog.file。

    不引入 yaml 依赖——这个工具要在只有 pillow 的机器上跑得起来。
    """
    path = os.path.join(site, "settings.yaml")
    out = {"name": "", "tagline": "", "base_url": "", "catalog": ""}
    section, in_tagline = "", False
    if not os.path.exists(path):
        return out
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.rstrip("\n")
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            indent = len(line) - len(line.lstrip())
            key, _, val = line.strip().partition(":")
            val = val.strip().strip("'\"")
            if indent == 0:
                section, in_tagline = key, False
                if key == "base_url":
                    out["base_url"] = val
                continue
            if section == "brand":
                if indent == 2:
                    in_tagline = key == "tagline"
                    if key == "name":
                        out["name"] = val
                elif indent >= 4 and in_tagline and key in ("en", "zh"):
                    # 英文优先；只有中文时用中文，总比没有强
                    if key == "en" or not out["tagline"]:
                        out["tagline"] = val
            elif section == "catalog" and indent >= 2 and key == "file":
                out["catalog"] = val
    return out


def brand_card(path: str, colors: dict[str, str], brand: str, tagline: str) -> None:
    """按主题色生成 1200×630 品牌分享卡。

    没有 cover 的页面用它当 og:image——分享到 WhatsApp/LinkedIn 时
    至少是一张有品牌色的门面图，而不是空白灰块。
    """
    primary = parse_color(colors.get("primary", ""), (47, 107, 63))
    accent = parse_color(colors.get("accent", ""), primary)
    paper = parse_color(colors.get("paper", ""), (247, 244, 236))
    dark = luma(paper) < 0.42

    # 底色：把品牌主色压深做底（浅色站也得到一张有分量的深色卡），
    # 暗色站则反过来用主色提亮——两种站都能读清白字。
    if dark:
        c0 = mix(primary, paper, 0.55)
        c1 = mix(primary, paper, 0.30)
    else:
        c0 = mix(primary, (6, 14, 9), 0.74)
        c1 = mix(primary, (6, 14, 9), 0.52)
    base = gradient((CARD_W, CARD_H), c0, c1, "diag").convert("RGBA")

    # 两枚柔光斑：主色一枚、强调色一枚，位置固定在右上与左下。
    for cx, cy, r, tint in (
        (int(CARD_W * 0.82), int(CARD_H * -0.05), 460, mix(primary, (255, 255, 255), 0.25)),
        (int(CARD_W * 0.06), int(CARD_H * 0.98), 380, accent),
    ):
        mask = radial_mask((CARD_W, CARD_H), cx, cy, r).filter(ImageFilter.GaussianBlur(90))
        glow = Image.new("RGBA", (CARD_W, CARD_H), (*tint, 0))
        glow.putalpha(Image.eval(mask, lambda v: int(v * 0.30)))
        base.alpha_composite(glow)

    out = base.convert("RGB")
    d = ImageDraw.Draw(out)
    ink = (255, 255, 255)
    soft = (255, 255, 255, 210)

    name = brand or "Your Brand"
    f_name = load_font(78 if len(name) <= 18 else 62)
    f_tag = load_font(30)
    f_small = load_font(22)

    x, y = 88, 232
    d.text((x, y), name, font=f_name, fill=ink)
    # 品牌色小横线：给标题一个落点，也给卡片一点结构
    d.rectangle([x, y - 42, x + 64, y - 38], fill=mix(primary, (255, 255, 255), 0.35))
    if tagline:
        # 手工折行：Pillow 没有现成的段落排版，按字符长度切两行足够稳
        words, lines, cur = tagline.split(), [], ""
        for w in words:
            if len(cur) + len(w) + 1 > 44 and cur:
                lines.append(cur)
                cur = w
            else:
                cur = (cur + " " + w).strip()
        if cur:
            lines.append(cur)
        ty = y + 116
        for line in lines[:2]:
            d.text((x, ty), line, font=f_tag, fill=soft)
            ty += 46

    out.save(path, "JPEG", quality=88, optimize=True, progressive=True)
    print(f"  {os.path.relpath(path)}  {os.path.getsize(path) // 1024} KB")


# ---------------------------------------------------------------- 目录 / 规格书（M5-9）

def _pdf_text(x: int, y: int, size: int, text: str, font: str = "F1") -> str:
    esc = text.replace("\\", r"\\").replace("(", r"\(").replace(")", r"\)")
    return f"BT /{font} {size} Tf 1 0 0 1 {x} {y} Tm ({esc}) Tj ET\n"


def write_pdf(path: str, title: str, subtitle: str, lines: list[str], color: tuple[int, int, int]) -> None:
    """写一页 A4 的 PDF——只在没有真目录时当占位，让下载链路能跑通。

    不引第三方库：PDF 1.4 的最小可用结构就够，字体用标准的 Helvetica。
    """
    r, g, b = (round(v / 255, 3) for v in color)
    body = f"0.06 0.12 0.08 rg 0 742 595 100 re f\n"
    body += f"{r} {g} {b} rg\n"
    body += _pdf_text(56, 780, 22, title, "F2")
    y = 690
    if subtitle:
        body += _pdf_text(56, y, 13, subtitle)
        y -= 40
    body += _pdf_text(56, y, 11, "This is a placeholder generated by the engine. Replace with the real PDF.", "F1")
    y -= 34
    for line in lines[:16]:
        body += _pdf_text(56, y, 12, line)
        y -= 24
    body += "0 0 0 RG 56 96 m 539 96 l S\n"
    body += _pdf_text(56, 76, 10, "Generated by TradeEngine")

    objs = [
        "<< /Type /Catalog /Pages 2 0 R >>",
        "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources "
        "<< /Font << /F1 5 0 R /F2 6 0 R >> >> /Contents 4 0 R >>",
        f"<< /Length {len(body.encode('latin-1', 'replace'))} >>\nstream\n{body}endstream",
        "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
        "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>",
    ]
    out = bytearray(b"%PDF-1.4\n")
    offsets = []
    for i, obj in enumerate(objs, start=1):
        offsets.append(len(out))
        out += f"{i} 0 obj\n{obj}\nendobj\n".encode("latin-1", "replace")
    xref = len(out)
    out += f"xref\n0 {len(objs) + 1}\n0000000000 65535 f \n".encode()
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += f"trailer\n<< /Size {len(objs) + 1} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode()
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(out)
    print(f"  {os.path.relpath(path)}  {os.path.getsize(path) // 1024} KB")



# ---------------------------------------------------------------- 实拍图处理

def fit_photo(src: str, dst: str, focus: str = "0.5,0.5", zoom: float = 1.0, cut: float = 0.13) -> None:
    """把原图裁成 4:3 并压到 1600px 宽。

    focus 是保留区域的中心（0–1 归一化坐标）：同一张原图取不同焦点，
    就能得到两张「看起来是两个场景」的裁切，比重复使用一张图专业得多。
    cut 先切掉底部若干比例——生成图的水印几乎都盖在右下角，交稿前必须去掉。
    """
    im = Image.open(src).convert("RGB")
    w, h = im.size
    if cut > 0:
        im = im.crop((0, 0, w, int(h * (1 - cut))))
        w, h = im.size
    target = 4 / 3
    cw, ch = (w, int(w / target)) if w / h > target else (int(h * target), h)
    cw, ch = int(cw / zoom), int(ch / zoom)
    fx, fy = (float(v) for v in focus.split(","))
    left = max(0, min(w - cw, int(w * fx - cw / 2)))
    top = max(0, min(h - ch, int(h * fy - ch / 2)))
    im = im.crop((left, top, left + cw, top + ch))
    if im.width > W:
        im = im.resize((W, H), Image.LANCZOS)
    save_jpeg(im, dst)


def load_sources(site: str) -> list[dict]:
    """读 media/sources.yaml（行级解析：photos 列表，out/src/focus/zoom）。"""
    path = os.path.join(site, "media", "sources.yaml")
    if not os.path.exists(path):
        return []
    items: list[dict] = []
    cur: dict | None = None
    with open(path, encoding="utf-8") as f:
        for raw in f:
            line = raw.rstrip("\n")
            m = re.match(r"^\s*-\s*out:\s*(.+?)\s*$", line)
            if m:
                cur = {"out": m.group(1)}
                items.append(cur)
                continue
            m = re.match(r"^\s+(src|focus|zoom):\s*(.+?)\s*$", line)
            if m and cur is not None:
                cur[m.group(1)] = m.group(2).strip().strip("'\"")
    return items


# ---------------------------------------------------------------- 槽位收集

REF = re.compile(r"media/[A-Za-z0-9_./-]+\.(?:png|jpg|jpeg|webp)")


def collect_slots(site: str) -> list[str]:
    """扫描内容与设置，收集所有被引用到的栅格图槽位（去重、排序）。"""
    slots: set[str] = set()
    for root, _dirs, files in os.walk(site):
        if ".build" in root or ".data" in root or os.sep + "media" in root:
            continue
        for name in files:
            if not name.endswith((".yaml", ".yml")):
                continue
            with open(os.path.join(root, name), encoding="utf-8") as f:
                for ref in REF.findall(f.read()):
                    if ref.lower().endswith(RASTER):
                        slots.add(ref)
    return sorted(slots)


def jpeg_path(site: str, slot: str) -> str:
    return os.path.join(site, os.path.splitext(slot)[0] + ".jpg")


def scan_datasheets(site: str) -> list[tuple[str, str, list[str]]]:
    """扫产品内容里的 datasheet 字段，返回 (站内路径, 标题, 摘要行)。

    只做行级解析：拿得到 title / model_no / moq / lead_time.en 就写进 PDF，
    拿不到的字段直接少一行——占位文件的目的是让下载链路可验证，不是排版比赛。
    """
    out: list[tuple[str, str, list[str]]] = []
    root = os.path.join(site, "content", "products")
    if not os.path.isdir(root):
        return out
    for fn in sorted(os.listdir(root)):
        if not fn.endswith((".yaml", ".yml")):
            continue
        rel, section, vals, in_i18n = "", "", {}, False
        with open(os.path.join(root, fn), encoding="utf-8") as f:
            for raw in f:
                line = raw.rstrip("\n")
                if not line.strip() or line.lstrip().startswith("#"):
                    continue
                indent = len(line) - len(line.lstrip())
                key, _, val = line.strip().partition(":")
                val = val.strip().strip("'\"")
                if indent == 0:
                    section, in_i18n = key, False
                    if key == "datasheet":
                        rel = val
                    elif val:
                        vals[key] = val
                elif indent == 2:
                    in_i18n = key in ("title", "lead_time", "packaging", "subtitle")
                    if key == "en":
                        in_i18n = False
                elif indent >= 4 and in_i18n and key == "en":
                    vals.setdefault(section, val)
        if not rel.lower().endswith(".pdf"):
            continue
        lines = []
        for label, k in (("Model", "model_no"), ("MOQ", "moq"), ("Lead time", "lead_time")):
            if vals.get(k):
                lines.append(f"{label}: {vals[k]}")
        lines.append("Full specifications, packaging and certifications: see the product page.")
        out.append((rel, vals.get("title") or fn, lines))
    return out


def main() -> None:
    ap = argparse.ArgumentParser(description="生成站点媒体素材")
    ap.add_argument("--site", required=True, help="站点目录，如 clients/kestrel")
    ap.add_argument("--force", action="store_true", help="已存在的图也重新生成")
    ap.add_argument("--list", action="store_true", help="只列出槽位，不生成")
    args = ap.parse_args()

    site = args.site.rstrip("/\\")
    colors, layout = read_theme(site)
    sources = load_sources(site)
    slots = collect_slots(site)

    if args.list:
        print(f"{len(slots)} 个栅格槽位（layout={layout}）")
        for s in slots:
            print(" ", s, "→", os.path.basename(jpeg_path(site, s)))
        return

    print(f"{site}  layout={layout}  colors={len(colors)}  photos={len(sources)}  slots={len(slots)}")

    # 1) 实拍图：先按 manifest 裁切
    done: set[str] = set()
    for item in sources:
        out = os.path.join(site, item["out"].lstrip("/")) + ".jpg"
        done.add(os.path.splitext(item["out"].lstrip("/"))[0] + ".jpg")
        if os.path.exists(out) and not args.force:
            print(f"  skip {os.path.relpath(out)}")
            continue
        src = item["src"]
        if not os.path.isabs(src):
            src = os.path.normpath(os.path.join(site, "media", src))
        if not os.path.exists(src):
            print(f"  !! 原图缺失 {item['src']}（跳过 {item['out']}）")
            done.discard(os.path.splitext(item["out"].lstrip("/"))[0] + ".jpg")
            continue
        print(f"  photo {item['out']}")
        fit_photo(src, out, item.get("focus", "0.5,0.5"), float(item.get("zoom", 1.0)),
                  float(item.get("cut", 0.13)))

    # 2) 其余槽位：品牌图案占位
    for slot in slots:
        rel = os.path.splitext(slot)[0] + ".jpg"
        if rel in done:
            continue
        out = os.path.join(site, rel)
        if os.path.exists(out) and not args.force:
            continue
        print(f"  gen  {rel}")
        brand_placeholder(out, colors, layout)

    # 3) 品牌分享卡（M5-3）：每站一张。没有 cover 的页面用它兜底 og:image，
    #    分享到 WhatsApp / LinkedIn 时不是一块空白。
    brand = read_settings_brand(site)
    card = os.path.join(site, "media", "brand-card.jpg")
    if args.force or not os.path.exists(card):
        brand_card(card, colors, brand["name"], brand["tagline"])

    # 4) 目录与规格书占位 PDF（M5-9）：配置指向的位置缺文件时才生成。
    accent = parse_color(colors.get("primary", ""), (47, 107, 63))
    if brand["catalog"]:
        p = os.path.join(site, brand["catalog"].lstrip("/"))
        if args.force or not os.path.exists(p):
            write_pdf(p, f"{brand['name']} Product Catalogue".strip(), brand["tagline"],
                      [brand["base_url"], "Full specifications, MOQ, packaging and certifications per model."],
                      accent)
    for rel, title, lines in scan_datasheets(site):
        p = os.path.join(site, rel.lstrip("/"))
        if args.force or not os.path.exists(p):
            write_pdf(p, title, brand["name"], lines, accent)


if __name__ == "__main__":
    main()
