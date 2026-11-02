#!/usr/bin/env bash
# 发布脚本：三平台交叉编译 + 打包 zip 到 release/
#
# 用法：
#   ./scripts/release.sh [版本号]     # 默认取 cmd/engine/main.go 里的 version
#
# 产物：
#   release/tradeengine_<ver>_linux_amd64.tar.gz
#   release/tradeengine_<ver>_linux_arm64.tar.gz
#   release/tradeengine_<ver>_windows_amd64.zip
#   release/tradeengine_<ver>_darwin_arm64.tar.gz
#
# 每个包内含：engine 可执行文件 + README.md + LICENSE
set -euo pipefail
cd "$(dirname "$0")/.."

VER="${1:-$(grep -o 'version = "[^"]*"' cmd/engine/main.go | cut -d'"' -f2)}"
NAME="tradeengine_${VER}"
OUT="release"
mkdir -p "$OUT"

if ! command -v go >/dev/null 2>&1; then
  echo "✗ 找不到 go（需要 Go 1.27+）。请先把 go 加入 PATH，或用 GO=... 前缀指定。" >&2
  exit 1
fi

echo "→ 版本 $VER"

build_one() { # goos goarch ext archive_ext
  local goos=$1 goarch=$2 arext=$4
  local ext=$3
  local bin="engine${ext}"
  local pkg="${NAME}_${goos}_${goarch}"
  echo "  · ${goos}/${goarch}"
  GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w" -o "$OUT/$NAME/$bin" ./cmd/engine
  cp README.md LICENSE "$OUT/$NAME/"
  if [ "$arext" = zip ]; then
    (cd "$OUT" && rm -f "$pkg.zip" \
      && if command -v zip >/dev/null 2>&1; then zip -qr "$pkg.zip" "$NAME"
         else powershell.exe -NoProfile -Command "Compress-Archive -Path '$NAME' -DestinationPath '$pkg.zip' -Force" >/dev/null
         fi)
  else
    tar -czf "$OUT/$pkg.tar.gz" -C "$OUT" "$NAME"
  fi
  rm -rf "$OUT/$NAME"
}

build_one linux amd64 ""  tar
build_one linux arm64 ""  tar
build_one darwin arm64 "" tar
build_one windows amd64 .exe zip

echo "✓ 产物："
ls -lh "$OUT" | grep "$VER"
