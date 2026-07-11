# Makefile —— DeskCalendar 零 CGO 交叉构建 (v1.0)
# 参考 docs/100-Release/Build.md §9。产物落 dist/，CI 亦用 scripts/ci-build.sh。
VER    ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     = github.com/shaolei/DeskCalendar/build
LDFLAGS = -s -w -X $(PKG).Version=$(VER) -X $(PKG).Commit=$(COMMIT) -X $(PKG).BuildTime=$(DATE)
DIST    = dist

.PHONY: build build-amd64 build-arm64 sha256 clean

build: build-amd64 build-arm64 sha256

build-amd64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS) -X $(PKG).TargetArch=amd64" -o $(DIST)/deskcalendar-amd64.exe ./cmd/deskcalendar

build-arm64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS) -X $(PKG).TargetArch=arm64" -o $(DIST)/deskcalendar-arm64.exe ./cmd/deskcalendar

sha256:
	@cd $(DIST) && sha256sum deskcalendar-amd64.exe deskcalendar-arm64.exe > sha256.txt
	@echo "sha256 -> $(DIST)/sha256.txt"

clean:
	@rm -rf $(DIST)
