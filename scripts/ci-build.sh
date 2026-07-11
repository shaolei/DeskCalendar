#!/usr/bin/env bash
# DeskCalendar CI 构建脚本（v1.0）。
# 步骤：① 刷新节假日数据（holiday-cn，fail-soft，满足 ADR-05c 离线优先）
#       ② 双架构交叉编译（CGO_ENABLED=0, windows/amd64+arm64）+ ldflags 注入版本
#       ③ 生成 sha256 校验文件
# 在 CI runner（Linux）上运行，产出 Windows exe。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# 可选参数：amd64 | arm64；缺省=双架构（CLI.md §9 约定按 --arch 循环调用）。
ARCH="${1:-}"
case "$ARCH" in
  ""|amd64|arm64) ;;
  *) echo "usage: ci-build.sh [amd64|arm64]" >&2; exit 2 ;;
esac

YEAR="$(date +%Y)"
HOLIDAY_FILE="internal/calendar/embed/holidays/${YEAR}.json"

echo "==> [1/3] 刷新节假日数据 (holiday-cn, fail-soft)"
# 上游格式为 {days:[{name,date,isOffDay}]}，与本地嵌入格式 {holidays,workdays}(MM-DD→name) 不同；
# 必须转换后再写入，禁止盲覆盖（盲覆盖会写坏嵌入文件导致构建/解析失败）。
if command -v gh >/dev/null 2>&1; then
  if gh api "repos/NateScarlet/holiday-cn/contents/${YEAR}.json" --jq '.content' \
       | base64 -d > "${HOLIDAY_FILE}.tmp" 2>/dev/null; then
    HOLIDAY_FILE="$HOLIDAY_FILE" python3 - <<'PY'
import json, os, sys
holiday_file = os.environ['HOLIDAY_FILE']
try:
    up = json.load(open(holiday_file + ".tmp", encoding='utf-8'))
    holidays, workdays = {}, {}
    for d in up.get('days', []):
        mmdd = d['date'][5:]
        if d.get('isOffDay'):
            holidays[mmdd] = d['name']
        else:
            workdays[mmdd] = d['name'] + '补班'
    out = {
        "_comment": "真实 %s 中国法定节假日/调休，来源 holiday-cn (MIT) + 国务院通知；构建期刷新（ADR-05c）。键 MM-DD，加载时按文件名年份补全为 YYYY-MM-DD。" % up.get('year', ''),
        "holidays": holidays,
        "workdays": workdays,
    }
    json.dump(out, open(holiday_file, 'w', encoding='utf-8'), ensure_ascii=False, indent=2)
    print("    已更新 %s（%d 节假日 / %d 补班）" % (holiday_file, len(holidays), len(workdays)))
except Exception as e:
    print("    转换失败，保留静态兜底：", e)
    sys.exit(1)
PY
    if [ $? -eq 0 ]; then
      rm -f "${HOLIDAY_FILE}.tmp"
    else
      echo "    转换失败，保留静态兜底 $HOLIDAY_FILE"
      rm -f "${HOLIDAY_FILE}.tmp"
    fi
  else
    echo "    拉取失败，保留静态兜底 $HOLIDAY_FILE"
    rm -f "${HOLIDAY_FILE}.tmp"
  fi
else
  echo "    无 gh CLI，保留静态兜底 $HOLIDAY_FILE"
fi

echo "==> [2/3] 交叉编译 (CGO_ENABLED=0, windows${ARCH:+/$ARCH})"
export CGO_ENABLED=0
VER="${GITHUB_REF_NAME:-$(git describe --tags --always 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
PKG="github.com/shaolei/DeskCalendar/build"
LDFLAGS="-s -w -X ${PKG}.Version=${VER} -X ${PKG}.Commit=${COMMIT} -X ${PKG}.BuildTime=${DATE}"

mkdir -p dist
build_one() {
  local a="$1"
  echo "    GOOS=windows GOARCH=$a"
  GOOS=windows GOARCH="$a" go build -trimpath -ldflags "${LDFLAGS} -X ${PKG}.TargetArch=$a" -o "dist/deskcalendar-$a.exe" ./cmd/deskcalendar
}
if [ -z "$ARCH" ] || [ "$ARCH" = amd64 ]; then build_one amd64; fi
if [ -z "$ARCH" ] || [ "$ARCH" = arm64 ]; then build_one arm64; fi

echo "==> [3/3] 生成 sha256 校验"
( cd dist && sha256sum deskcalendar-*.exe > sha256.txt )
echo "done."
