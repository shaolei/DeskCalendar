#!/usr/bin/env bash
# DeskCalendar 发布打包脚本（v1.0，见 docs/100-Release/Package.md）。
# 前置：dist/deskcalendar-<arch>.exe 已由 Build 产出（通常先 `make build-<arch>`）。
# 步骤：① 调用 cmd/packager 生成便携版 zip（+ 安装器，若 makensis 可用）
#       ② 重新生成 dist/sha256.txt（覆盖已存在的全部产物）
# 在 Linux CI runner 或本地（Windows Git Bash）运行。无 makensis 时自动跳过 NSIS。
set -euo pipefail

ARCH="${1:-amd64}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

EXE="dist/deskcalendar-$ARCH.exe"
if [ ! -f "$EXE" ]; then
  echo "缺少 $EXE，请先执行 make build-$ARCH" >&2
  exit 1
fi

VER="${GITHUB_REF_NAME:-$(git describe --tags --always 2>/dev/null || echo dev)}"
ICON="build/assets/icon/app.ico"
[ -f "$ICON" ] || ICON=""

NSIS=1
[ "${DESKCALENDAR_SKIP_NSIS:-}" = "1" ] && NSIS=0
command -v makensis >/dev/null 2>&1 || NSIS=0

echo "==> 打包 $ARCH (version=$VER, nsis=$NSIS)"
go run ./cmd/packager -exe "$EXE" -arch "$ARCH" -version "$VER" -outdir dist -icon "$ICON" -nsis=$NSIS

echo "==> 重新生成 sha256.txt"
cd dist
: > sha256.txt
for f in deskcalendar-*.exe DeskCalendar-Setup-*.exe DeskCalendar-Portable-*.zip; do
  [ -f "$f" ] && sha256sum "$f" >> sha256.txt
done
echo "done."
