"""生成示例站点的占位媒体文件（PNG / PDF）。

M0 阶段不做图片管线（转码/裁切/srcset 属于 M1 的 Assets 阶段），
这里只需要「文件真实存在且尺寸合理」，让规则 14（媒体引用校验）能跑通。
"""

import struct
import zlib
import os
import sys

ROOT = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "examples", "zentea")


def make_png(path, w, h, rgb, accent=None):
    raw = bytearray()
    for y in range(h):
        raw.append(0)  # filter type 0
        for x in range(w):
            c = rgb
            if accent and (x // 40 + y // 40) % 2 == 0 and w > 100:
                c = accent
            raw += bytes(c)
    def chunk(tag, data):
        return (
            struct.pack(">I", len(data))
            + tag
            + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)
        )
    ihdr = struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(bytes(raw), 9)) + chunk(b"IEND", b"")
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(png)
    print("png ", os.path.relpath(path, ROOT), f"{w}x{h}", len(png), "bytes")


def make_pdf(path, title):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    text = f"BT /F1 18 Tf 72 720 Td ({title}) Tj ET"
    objs = [
        "<< /Type /Catalog /Pages 2 0 R >>",
        "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
        "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>",
        f"<< /Length {len(text)} >>\nstream\n{text}\nendstream",
        "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
    ]
    out = b"%PDF-1.4\n"
    offsets = []
    for i, o in enumerate(objs, start=1):
        offsets.append(len(out))
        out += f"{i} 0 obj\n{o}\nendobj\n".encode()
    xref_pos = len(out)
    out += f"xref\n0 {len(objs)+1}\n0000000000 65535 f \n".encode()
    for off in offsets:
        out += f"{off:010d} 00000 n \n".encode()
    out += f"trailer\n<< /Size {len(objs)+1} /Root 1 0 R >>\nstartxref\n{xref_pos}\n%%EOF\n".encode()
    with open(path, "wb") as f:
        f.write(out)
    print("pdf ", os.path.relpath(path, ROOT), len(out), "bytes")


def make_svg(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    svg = (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 60" width="240" height="60">'
        '<rect width="240" height="60" fill="none"/>'
        f'<text x="8" y="40" font-family="Georgia,serif" font-size="26" fill="#1A1A1A">{text}</text>'
        "</svg>"
    )
    with open(path, "w", encoding="utf-8") as f:
        f.write(svg)
    print("svg ", os.path.relpath(path, ROOT))


if __name__ == "__main__":
    make_png(os.path.join(ROOT, "media", "products", "sencha-cover.png"), 1600, 1200, (0x3B, 0x5E, 0x43), (0x4C, 0x72, 0x52))
    make_png(os.path.join(ROOT, "media", "products", "matcha-cover.png"), 1600, 1200, (0x5B, 0x7F, 0x5B), (0x6E, 0x93, 0x6C))
    make_png(os.path.join(ROOT, "media", "cases", "eu-distributor.png"), 1600, 1200, (0x8A, 0x7B, 0x5E), (0x9C, 0x8D, 0x6E))
    make_svg(os.path.join(ROOT, "media", "brand", "logo.svg"), "Misty Peak")
    make_pdf(os.path.join(ROOT, "media", "certs", "usda-organic.pdf"), "USDA Organic Certificate - Sample")
    make_pdf(os.path.join(ROOT, "media", "certs", "eu-organic.pdf"), "EU Organic Certificate - Sample")
    make_pdf(os.path.join(ROOT, "media", "certs", "haccp.pdf"), "HACCP Certificate - Sample")
