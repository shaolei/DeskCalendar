#!/usr/bin/env bash
# DeskCalendar CI 构建脚本（v1.0）。
# 步骤：① 刷新节假日数据（holiday-cn，fail-soft，满足 ADR-05c 离线优先）
#       ② 双架构交叉编译（CGO_ENABLED=0, windows/amd64+arm64）+ ldflags 注入版本
#       ③ 生成 sha256 校验文件
# 在 CI runner（Linux）上运行，产出 Windows exe。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

YEAR="$(date +%Y)"
HOLIDAY_FILE="internal/calendar/embed/holidays/${YEAR}.json"

echo "==> [1/3] 刷新节假日数据 (holiday-cn, fail-soft)"
if command -v gh >/dev/null 2>&1; then
  if gh api "repos/NateScarlet/holiday-cn/contents/${YEAR}.json" --jq '.content' \
       | base64 -d > "${HOLIDAY_FILE}.tmp" 2>/dev/null \
       && python3 -c "import json,sys; json.load(open('${HOLIDAY_FILE}.tmp'))" 2>/dev/null; then
    mv "${HOLIDAY_FILE}.tmp" "$HOLIDAY_FILE"
    echo "    已更新 $HOLIDAY_FILE"
  else
    echo "    拉取/校验失败，保留静态兜底 $HOLIDAY_FILE"
    rm -f "${HOLIDAY_FILE}.tmp"
  fi
else
  echo "    无 gh CLI，保留静态兜底 $HOLIDAY_FILE"
fi

echo "==> [2/3] 交叉编译 (CGO_ENABLED=0, windows/amd64+arm64)"
export CGO_ENABLED=0
VER="${GITHUB_REF_NAME:-$(git describe --tags --always 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PKG="github.com/shaolei/DeskCalendar/build"
LDFLAGS="-s -w -X ${PKG}.Version=${VER} -X ${PKG}.Commit=${COMMIT} -X ${PKG}.BuildTime=${DATE}"

mkdir -p dist
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "${LDFLAGS} -X ${PKG}.TargetArch=amd64" -o dist/deskcalendar-amd64.exe ./cmd/deskcalendar
GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "${LDFLAGS} -X ${PKG}.TargetArch=arm64" -o dist/deskcalendar-arm64.exe ./cmd/deskcalendar

echo "==> [3/3] 生成 sha256 校验"
( cd dist && sha256sum deskcalendar-amd64.exe deskcalendar-arm64.exe > sha256.txt )
echo "done."
