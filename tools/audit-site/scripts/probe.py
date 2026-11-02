#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""audit-site probe —— B2 P0/P1/P2 确定性取证脚本（skill: audit-site）

职责边界（与 SKILL.md 一致）：
  本脚本只做「机器可测」的项，输出 evidence/evidence.json + 关键页快照。
  所有判定性条目（文案质量/商业模式/证言真实性等）由 LLM 按 B1/B2 口径另行判定，
  禁止用本脚本的输出代替人工核查（P3）。

用法：
  python probe.py --url https://example.com --out ./reports/example.com-20261102
  python probe.py --url http://127.0.0.1:8931 --out ./reports/local-test --fast

设计约束：
  - 纯 Python 标准库，零第三方依赖（目标机可能没装 dnspython/brotli）
  - 显式绕过系统代理（评测环境常挂代理，会把 --noproxy 的规矩吃掉）
  - 一切网络请求有超时、有字节上限；失败项记 error 而不是中断
  - 输出 JSON 稳定可 diff：同一站点两次运行，除时间戳外字段应一致
"""
import argparse
import concurrent.futures
import gzip
import json
import re
import socket
import ssl
import subprocess
import sys
import time
import zlib
from datetime import datetime, timezone
from urllib.error import HTTPError, URLError
from urllib.parse import urljoin, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener

UA = "Mozilla/5.0 (compatible; AuditProbe/1.0; external-site-review)"
TIMEOUT = 15
MAX_BODY = 3 * 1024 * 1024  # 单响应最多读 3MB
CRAWL_MAX_PAGES = 80
CRAWL_MAX_DEPTH = 2
HEAD_CHECK_MAX = 40

# 绕过系统代理：评测必须直连目标站
import urllib.request
_urlopener = build_opener(urllib.request.ProxyHandler({}))
_urlopener.addheaders = [("User-Agent", UA), ("Accept-Encoding", "gzip, deflate"),
                         ("Accept-Language", "en")]

SUSPICIOUS_PAT = re.compile(
    r"(casino|viagra|pharma|porn|xxx|loan|payday|escort|replica[-_]?(watch|bag)|"
    r"fucking|sluts|backlink|seo[-_]?service)",
    re.I)


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


opener_noredir = build_opener(urllib.request.ProxyHandler({}), NoRedirect())
opener_noredir.addheaders = [("User-Agent", UA)]


def fetch(url, timeout=TIMEOUT, method="GET", allow_redirect=True, max_bytes=MAX_BODY):
    """返回 dict；任何异常转成 {'error': ...}，绝不向上抛。"""
    t0 = time.time()
    try:
        op = _urlopener if allow_redirect else opener_noredir
        req = Request(url, method=method)
        with op.open(req, timeout=timeout) as resp:
            raw = resp.read(max_bytes + 1)
            truncated = len(raw) > max_bytes
            raw = raw[:max_bytes]
            enc = (resp.headers.get("Content-Encoding") or "").lower()
            body = None
            if enc == "gzip" and raw[:2] == b"\x1f\x8b":
                try:
                    body = gzip.decompress(raw)
                except OSError:
                    body, raw = raw, raw
            elif enc == "deflate":
                try:
                    body = zlib.decompress(raw)
                except zlib.error:
                    body = zlib.decompress(raw, -zlib.MAX_WBITS)
            else:
                body = raw
            return {
                "status": resp.status,
                "final_url": resp.geturl(),
                "headers": {k.lower(): v for k, v in resp.headers.items()},
                "body": body if method == "GET" else b"",
                "raw_len": len(raw),
                "elapsed_ms": int((time.time() - t0) * 1000),
                "truncated": truncated,
            }
    except HTTPError as e:
        return {"status": e.code, "final_url": url, "headers": dict(e.headers or {}),
                "body": b"", "raw_len": 0, "elapsed_ms": int((time.time() - t0) * 1000),
                "error": f"HTTP {e.code}"}
    except Exception as e:  # noqa: BLE001
        return {"status": None, "final_url": url, "headers": {}, "body": b"",
                "raw_len": 0, "elapsed_ms": int((time.time() - t0) * 1000),
                "error": f"{type(e).__name__}: {e}"}


def txt_re(b):
    return b.decode("utf-8", "replace") if b else ""


# ---------------------------------------------------------------- DNS（邮件三记录 V9/R4）

def _run_decode(cmd):
    """capture bytes 自行解码：中文 Windows nslookup 输出 GBK，不能用 text=True 的默认 utf-8。"""
    r = subprocess.run(cmd, capture_output=True, timeout=10)
    for enc in ("utf-8", "gbk"):
        try:
            return r.stdout.decode(enc)
        except UnicodeDecodeError:
            continue
    return r.stdout.decode("utf-8", "replace")


def _dns_query(kind, name):
    """dig 优先，Windows 回落 nslookup。返回 (record_list, resolver)。"""
    for cmd in (
        ["dig", "+short", kind, name],
        ["nslookup", "-type=" + kind, name],
    ):
        try:
            out = _run_decode(cmd)
            recs = []
            if cmd[0] == "dig":
                recs = [l.strip().strip('"') for l in out.splitlines()
                        if l.strip() and not l.strip().startswith(";")]
            else:  # nslookup：TXT 记录取所有引号段；MX 取 mail exchanger =
                if kind == "MX":
                    recs = re.findall(r"mail exchanger\s*=\s*(\S+)", out, re.I)
                else:
                    recs = [m.strip() for m in re.findall(r'"([^"]*)"', out) if m.strip()]
            if recs:
                return recs, cmd[0]
            if r_ok(out):  # 解析器有响应但无记录 → 确认查询完成，不必再试下一个工具
                return [], cmd[0]
        except (FileNotFoundError, subprocess.TimeoutExpired):
            continue
    return [], "none"


def r_ok(out):
    """nslookup 输出里出现权威应答迹象（含服务器地址段）即认为查询完成。"""
    return "Address" in out or "地址" in out or "server" in out.lower()


def mail_records(domain):
    mx, r1 = _dns_query("MX", domain)
    spf, r2 = _dns_query("TXT", domain)
    spf_recs = [r for r in spf if r.lower().startswith("v=spf1")]
    dmarc, r3 = _dns_query("TXT", f"_dmarc.{domain}")
    dmarc_recs = [r for r in dmarc if r.lower().startswith("v=dmarc1")]
    # DKIM 常见 selector 探测（全查不到≠缺失，标注 unknown 而不是 fail——判定交给人）
    dkim_found = []
    for sel in ("default", "google", "selector1", "selector2", "k1", "s1", "dkim",
                "zoho", "protonmail", "smtp"):
        recs, _ = _dns_query("TXT", f"{sel}._domainkey.{domain}")
        if any("p=" in r for r in recs):
            dkim_found.append(sel)
    return {
        "domain": domain,
        "resolver": [r1, r2, r3],
        "mx": mx[:5],
        "mx_present": bool(mx),
        "spf": spf_recs,
        "spf_present": bool(spf_recs),
        "dmarc": dmarc_recs,
        "dmarc_present": bool(dmarc_recs),
        "dmarc_policy": (re.search(r"p=(\w+)", dmarc_recs[0]).group(1)
                         if dmarc_recs and re.search(r"p=(\w+)", dmarc_recs[0]) else None),
        "dkim_selectors_found": dkim_found,
        "dkim": "found" if dkim_found else "unknown",  # 未知 selector ≠ 缺失
    }


# ---------------------------------------------------------------- HTML 静态检查

RE_TITLE = re.compile(r"<title[^>]*>(.*?)</title>", re.I | re.S)
RE_META_DESC = re.compile(r'<meta[^>]+name=["\']description["\'][^>]+content=["\'](.*?)["\']', re.I | re.S)
RE_OG = re.compile(r'<meta[^>]+property=["\']og:(\w+)["\'][^>]+content=["\'](.*?)["\']', re.I | re.S)
RE_CANONICAL = re.compile(r'<link[^>]+rel=["\']canonical["\'][^>]+href=["\'](.*?)["\']', re.I)
RE_HREFLANG = re.compile(r'<link[^>]+hreflang=["\'](.*?)["\'][^>]*>', re.I)
RE_LANG = re.compile(r'<html[^>]+lang=["\'](.*?)["\']', re.I)
RE_H1 = re.compile(r"<h1[\s>]", re.I)
RE_IMG = re.compile(r"<img\b[^>]*>", re.I)
RE_ALT = re.compile(r'\balt=["\'][^"\']+["\']')
RE_A_HREF = re.compile(r'<a\b[^>]+href=["\'](.*?)["\']', re.I | re.S)
RE_CSS_LINK = re.compile(r'<link[^>]+rel=["\']stylesheet["\'][^>]+href=["\'](.*?)["\']', re.I)
RE_SCRIPT_EXT = re.compile(r'<script[^>]+src=["\'](.*?)["\']', re.I)
RE_FORM = re.compile(r"<form\b", re.I)
RE_MAILTO = re.compile(r'href=["\']mailto:', re.I)
RE_TEL = re.compile(r'href=["\']tel:', re.I)
RE_WHATSAPP = re.compile(r"(wa\.me|api\.whatsapp\.com)", re.I)


def html_check(url, body_bytes):
    html = txt_re(body_bytes)
    imgs = RE_IMG.findall(html)
    imgs_with_alt = sum(1 for t in imgs if RE_ALT.search(t))
    hrefs = RE_A_HREF.findall(html)
    split = urlsplit(url)
    host = split.netloc.lower()
    internal, external = [], []
    for h in hrefs:
        h = h.strip()
        if h.startswith(("mailto:", "tel:", "javascript:", "#")) or not h:
            continue
        absu = urljoin(url, h)
        u = urlsplit(absu)
        if u.netloc in ("", host) or u.netloc.lower().endswith("." + host):
            internal.append(absu)
        else:
            external.append(absu)
    return {
        "bytes_html": len(body_bytes),
        "title": RE_TITLE.search(html).group(1).strip()[:200] if RE_TITLE.search(html) else None,
        "meta_description": (RE_META_DESC.search(html).group(1).strip()[:300]
                             if RE_META_DESC.search(html) else None),
        "og": dict(RE_OG.findall(html)),
        "canonical": (urljoin(url, RE_CANONICAL.search(html).group(1))
                      if RE_CANONICAL.search(html) else None),
        "html_lang": RE_LANG.search(html).group(1) if RE_LANG.search(html) else None,
        "hreflang": [h for h in RE_HREFLANG.findall(html)],
        "h1_count": len(RE_H1.findall(html)),
        "img_total": len(imgs),
        "img_no_alt": len(imgs) - imgs_with_alt,
        "form_count": len(RE_FORM.findall(html)),
        "contact_signals": {
            "mailto": bool(RE_MAILTO.search(html)),
            "tel": bool(RE_TEL.search(html)),
            "whatsapp": bool(RE_WHATSAPP.search(html)),
        },
        "css_links": [urljoin(url, h) for h in RE_CSS_LINK.findall(html)][:5],
        "external_scripts": [urljoin(url, h) for h in RE_SCRIPT_EXT.findall(html)][:20],
        "internal_hrefs": sorted(set(internal)),
        "external_hrefs": sorted(set(external)),
    }


# ---------------------------------------------------------------- 爬取

def crawl(start_url, max_pages=CRAWL_MAX_PAGES, max_depth=CRAWL_MAX_DEPTH):
    seen, queue, pages = {start_url}, [(start_url, 0)], []
    titles = {}
    dead_samples = []
    while queue and len(pages) < max_pages:
        url, depth = queue.pop(0)
        r = fetch(url, timeout=12)
        entry = {"url": url, "status": r["status"], "depth": depth}
        if r.get("error"):
            entry["error"] = r["error"]
            if r["status"] is None:
                dead_samples.append(url)
        if r["status"] == 200 and r.get("body"):
            c = html_check(url, r["body"])
            entry["bytes"] = c["bytes_html"]
            entry["title"] = c["title"]
            if c["title"]:
                titles.setdefault(c["title"].lower(), []).append(url)
            if depth < max_depth:
                for u in c["internal_hrefs"]:
                    u = u.split("#", 1)[0].rstrip("/") or u
                    if u not in seen and u.startswith(("http://", "https://")):
                        if urlsplit(u).path.lower().endswith(
                                (".jpg", ".jpeg", ".png", ".webp", ".gif", ".pdf",
                                 ".zip", ".svg", ".woff2", ".woff", ".css", ".js", ".ico")):
                            continue
                        seen.add(u)
                        queue.append((u, depth + 1))
        pages.append(entry)
    dup_titles = {t: u for t, u in titles.items() if len(u) > 1}
    return {"pages": pages, "crawled": len(pages),
            "dup_titles": dup_titles, "dead_samples": dead_samples}


# ---------------------------------------------------------------- TLS 证书

def tls_info(host):
    """先按正常校验连接；失败则降级读取真实证书留证（证书是谁、覆盖哪些域）。"""
    from datetime import datetime as dt

    def _parse(cert):
        na = cert.get("notAfter")
        days = None
        if na:
            try:
                days = (dt.strptime(na, "%b %d %H:%M:%S %Y %Z") - dt.utcnow()).days
            except ValueError:
                pass
        return {"subject": dict(x[0] for x in cert.get("subject", ())),
                "issuer": dict(x[0] for x in cert.get("issuer", ())),
                "not_after": na, "days_left": days,
                "sans": [v for k, v in cert.get("subjectAltName", ()) if k == "DNS"][:12]}

    try:
        ctx = ssl.create_default_context()
        with socket.create_connection((host, 443), timeout=10) as sock:
            with ctx.wrap_socket(sock, server_hostname=host) as s:
                out = _parse(s.getpeercert())
                out["verification"] = "ok"
                return out
    except ssl.SSLCertVerificationError as e:
        info = {"verification": "failed",
                "error": getattr(e, "verify_message", None) or str(e),
                "verify_code": getattr(e, "verify_code", None)}
        try:
            ctx = ssl.create_default_context()
            ctx.check_hostname = False
            ctx.verify_mode = ssl.CERT_NONE
            with socket.create_connection((host, 443), timeout=10) as sock:
                with ctx.wrap_socket(sock, server_hostname=host) as s:
                    der = s.getpeercert(binary_form=True)
            import tempfile
            pem = ssl.DER_cert_to_PEM_cert(der)
            with tempfile.NamedTemporaryFile("w", suffix=".pem", delete=False) as tf:
                tf.write(pem)
                path = tf.name
            try:
                info.update(_parse(ssl._ssl._test_decode_cert(path)))
            finally:
                import os
                os.unlink(path)
            info["decoded"] = True
        except Exception as e2:  # noqa: BLE001
            info["decode_error"] = f"{type(e2).__name__}: {e2}"
        return info
    except Exception as e:  # noqa: BLE001
        return {"verification": "error", "error": f"{type(e).__name__}: {e}"}


# ---------------------------------------------------------------- 主流程

def main():
    ap = argparse.ArgumentParser(description="audit-site 确定性取证")
    ap.add_argument("--url", required=True, help="被审站点起始 URL")
    ap.add_argument("--out", required=True, help="证据输出目录")
    ap.add_argument("--fast", action="store_true", help="跳过爬取（快速冒烟）")
    args = ap.parse_args()

    start = args.url if "://" in args.url else "https://" + args.url
    host = urlsplit(start).netloc.lower()
    # 去掉端口后再算注册域（邮箱三记录用）
    dom = re.sub(r":\d+$", "", host)
    if re.match(r"^(\d{1,3}\.){3}\d{1,3}$|^localhost$", dom):
        registrable = dom  # IP/localhost 无注册域概念
    else:
        labels = dom.split(".")
        two = {"com.cn", "net.cn", "org.cn", "gov.cn", "edu.cn", "ac.cn",
               "co.uk", "org.uk", "ac.uk", "gov.uk", "co.jp", "or.jp",
               "ne.jp", "co.kr", "or.kr", "com.hk", "org.hk", "edu.hk",
               "com.tw", "org.tw", "com.au", "net.au", "org.au",
               "co.nz", "com.sg", "com.br", "com.mx", "com.tr", "com.ar"}
        # 两段式公共后缀（.com.cn/.co.uk 等）：注册域取「后缀 + 1 段」
        if ".".join(labels[-2:]) in two and len(labels) >= 3:
            registrable = ".".join(labels[-3:])
        else:
            registrable = ".".join(labels[-2:])

    ev = {"tool": "audit-site probe", "started_at": datetime.now(timezone.utc).isoformat(),
          "start_url": start, "host": host, "registrable_domain": registrable,
          "mode": "fast" if args.fast else "full"}

    print(f"[probe] target: {start}  (registrable: {registrable})", file=sys.stderr)

    # ---- P0 可达性与跳转链
    print("[probe] P0 reachability ...", file=sys.stderr)
    http_r = fetch(start.replace("https://", "http://", 1).replace("http://", "https://", 1)
                   if start.startswith("https") else start, allow_redirect=False)
    home = fetch(start)
    effective_home = start
    ev["reachability"] = {
        "homepage_status": home["status"],
        "final_url": home["final_url"],
        "redirected": home["final_url"] != start,
        "elapsed_ms": home["elapsed_ms"],
        "error": home.get("error"),
    }
    # 根路径 404 回退链：sitemap 第一个同深度 loc → 常见语言前缀。
    # 回退本身就是一个评测发现（裸域不可直达），如实记录 root_404_fallback。
    if home["status"] == 404 and urlsplit(start).path in ("", "/"):
        alt = None
        s = fetch(urljoin(start, "sitemap.xml"))
        if s["status"] == 200 and s["body"]:
            for loc in re.findall(r"<loc>\s*(.*?)\s*</loc>", txt_re(s["body"])):
                u = urlsplit(loc)
                if (u.netloc == urlsplit(start).netloc and u.path not in ("", "/")
                        and u.path.count("/") <= 1):
                    alt = loc
                    break
        if not alt:
            for p in ("en/", "zh/", "index.html"):
                r2 = fetch(urljoin(start, p), allow_redirect=False)
                if r2["status"] == 200:
                    alt = urljoin(start, p)
                    break
        if alt:
            ev["reachability"]["root_404_fallback"] = alt
            effective_home = alt
            home = fetch(alt)
            ev["reachability"]["effective_home_status"] = home["status"]
        else:
            ev["reachability"]["root_404_fallback"] = None
    if start.startswith("https://"):
        plain = fetch(start.replace("https://", "http://", 1), allow_redirect=False)
        ev["reachability"]["http_to_https"] = (
            plain["status"] in (301, 302, 308, 307) or
            (plain["status"] == 200 and False))
        ev["reachability"]["http_probe_status"] = plain["status"]

    # ---- TLS 证书（公网域）；证书无效时降级读取真实证书留证，这本身就是评测发现。
    # 放在首页检查之前：HTTPS 挂掉时切到 HTTP 重取，下游全部内容取证才有着落。
    if start.startswith("https") and not re.match(r"^(127\.|10\.|192\.168\.|localhost)", host):
        print("[probe] TLS ...", file=sys.stderr)
        ev["tls"] = tls_info(host)
        if ev["tls"].get("verification") == "failed":
            ev["tls_failure"] = {
                "severity": "critical",
                "finding": "HTTPS certificate verification failed for " + host,
                "impact": "browsers show security warning; site effectively HTTPS-down",
            }
            http_status = fetch(start.replace("https://", "http://", 1),
                                allow_redirect=False)["status"]
            ev["tls_failure"]["http_fallback_status"] = http_status
            if http_status in (200, 301, 302, 307, 308):
                start = start.replace("https://", "http://", 1)
                effective_home = start
                home = fetch(start)
                ev["reachability"]["homepage_status"] = home["status"]
                ev["reachability"]["note"] = "audited over HTTP after TLS failure"
                ev["reachability"]["error"] = None

    # ---- P2 响应头（缓存/安全/压缩）
    h = home["headers"]
    sec_headers = ["strict-transport-security", "x-frame-options", "x-content-type-options",
                   "content-security-policy", "referrer-policy", "permissions-policy"]
    ev["headers"] = {
        "server": h.get("server"), "content_encoding": h.get("content-encoding"),
        "cache_control": h.get("cache-control"),
        "security_headers_present": {k: k in h for k in sec_headers},
        "strict_transport_security": h.get("strict-transport-security"),
    }

    # ---- 关键文件三件套 + favicon
    print("[probe] robots/sitemap/llms.txt ...", file=sys.stderr)
    trio = {}
    for f in ("robots.txt", "sitemap.xml", "llms.txt", "favicon.ico", "404.html",
              "no-such-probe-page-for-404-test"):
        r = fetch(urljoin(start, "/" + f))
        trio[f] = {"status": r["status"], "bytes": r["raw_len"],
                   "error": r.get("error")}
    ev["key_files"] = trio
    ev["key_files"]["http404_status"] = trio.pop("no-such-probe-page-for-404-test")["status"]

    # ---- 首页 HTML 静态检查
    hc = html_check(effective_home, home["body"]) if home["status"] == 200 and home["body"] else {}
    if hc:
        hc["url"] = effective_home
    ev["homepage"] = hc

    # ---- 首屏字节预算：HTML + 第一个 CSS
    budget = {"html_bytes": len(home["body"])}
    if hc.get("css_links"):
        css = fetch(hc["css_links"][0])
        budget["first_css_bytes"] = css["raw_len"]
        budget["first_css_url"] = hc["css_links"][0]
        budget["html_plus_first_css"] = len(home["body"]) + css["raw_len"]
    ev["page_budget"] = budget

    # ---- 静态资源哈希化抽查（缓存正确性）
    hashed, unhashed = [], []
    for u in hc.get("css_links", []) + hc.get("external_scripts", []):
        (hashed if re.search(r"\.[0-9a-f]{8}\.", urlsplit(u).path) else unhashed).append(u)
    ev["asset_hashing"] = {"hashed": len(hashed), "unhashed": unhashed[:5]}

    # ---- 暗链排查（V8）
    ext = hc.get("external_hrefs", []) if hc else []
    dark = [u for u in ext if SUSPICIOUS_PAT.search(u)]
    ev["dark_link_scan"] = {
        "external_link_count": len(ext),
        "suspicious": dark,
        "external_sample": ext[:15],
    }

    # ---- 邮件三记录（V9/R4）——仅对公网域做
    if not re.match(r"^(127\.|10\.|192\.168\.|localhost)", host):
        print("[probe] mail records (SPF/DKIM/DMARC) ...", file=sys.stderr)
        ev["mail_records"] = mail_records(registrable)
    else:
        ev["mail_records"] = {"skipped": "local target"}

    # ---- 爬取 + 死链
    if not args.fast and home["status"] == 200:
        print("[probe] crawl ...", file=sys.stderr)
        c = crawl(effective_home)
        ev["crawl"] = {"crawled": c["crawled"], "dup_titles": c["dup_titles"],
                       "error_pages": [p for p in c["pages"] if p["status"] != 200]}
        # 内链抽样 HEAD 验死链
        cands = [p["url"] for p in c["pages"] if p["status"] == 200][:0]
        internal = list(dict.fromkeys(hc.get("internal_hrefs", [])))
        head_cands = [u for u in internal if u not in
                      {p["url"] for p in c["pages"]}][:HEAD_CHECK_MAX]
        if head_cands:
            print(f"[probe] HEAD check {len(head_cands)} links ...", file=sys.stderr)
            with concurrent.futures.ThreadPoolExecutor(8) as ex:
                res = list(ex.map(lambda u: (u, fetch(u, method="HEAD", timeout=8)["status"]),
                                  head_cands))
            ev["dead_links"] = {"checked": len(res),
                                "non_200": [{"url": u, "status": s} for u, s in res
                                            if s is None or s >= 400]}

    # ---- 快照（留证）
    import os
    os.makedirs(args.out, exist_ok=True)
    snapdir = os.path.join(args.out, "evidence")
    os.makedirs(snapdir, exist_ok=True)
    if home["body"]:
        with open(os.path.join(snapdir, "home.html"), "wb") as f:
            f.write(home["body"])
    for f in ("robots.txt", "sitemap.xml", "llms.txt"):
        if trio.get(f, {}).get("status") == 200:
            r = fetch(urljoin(start, "/" + f))
            with open(os.path.join(snapdir, f), "wb") as fh:
                fh.write(r["body"] or b"")

    ev["finished_at"] = datetime.now(timezone.utc).isoformat()
    out = os.path.join(snapdir, "evidence.json")
    with open(out, "w", encoding="utf-8") as f:
        json.dump(ev, f, ensure_ascii=False, indent=2)
    print(f"[probe] evidence -> {out}", file=sys.stderr)

    # ---- 快筛摘要（机器可判定项的红绿灯，供 P1 Veto 快筛引用）
    summary = {}
    if hc:
        summary["homepage_200"] = home["status"] == 200
        summary["has_title"] = bool(hc.get("title"))
        summary["has_meta_desc"] = bool(hc.get("meta_description"))
        summary["has_og_image"] = bool((hc.get("og") or {}).get("image"))
        summary["has_canonical"] = bool(hc.get("canonical"))
        summary["has_form"] = hc.get("form_count", 0) > 0
        summary["contact_signals"] = hc.get("contact_signals")
    if trio.get("robots.txt"): summary["robots"] = trio["robots.txt"]["status"] == 200
    if trio.get("sitemap.xml"): summary["sitemap"] = trio["sitemap.xml"]["status"] == 200
    if trio.get("llms.txt"): summary["llms_txt"] = trio["llms.txt"]["status"] == 200
    mr = ev.get("mail_records", {})
    if "spf_present" in mr:
        summary["mail_spf"] = mr["spf_present"]
        summary["mail_dmarc"] = mr["dmarc_present"]
        summary["mail_dmarc_policy"] = mr.get("dmarc_policy")
    dl = ev.get("dark_link_scan", {})
    summary["dark_links_suspicious"] = len(dl.get("suspicious", []))
    ev["summary"] = summary
    with open(out, "w", encoding="utf-8") as f:
        json.dump(ev, f, ensure_ascii=False, indent=2)

    print(json.dumps(summary, ensure_ascii=False, indent=2))
    print(f"[probe] DONE -> {out}", file=sys.stderr)


if __name__ == "__main__":
    main()
