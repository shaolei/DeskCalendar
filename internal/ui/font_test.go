package ui

import (
	"os"
	"testing"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// resetCJKFontCache 仅供测试：清空已缓存的字体 source，使下一次 resolveCJKFont
// 重新解析字体文件（用于隔离各测试的计数）。
func resetCJKFontCache() {
	cjkFontMu.Lock()
	cjkFontSrc = nil
	cjkFontMu.Unlock()
}

// findAnySystemFont 跨平台找一个真实存在的字体文件（用于在不依赖特定平台的
// 条件下驱动 CJK 字体加载路径）。找不到则返回空串（测试 Skip）。
func findAnySystemFont() string {
	paths := []string{
		`C:/Windows/Fonts/msyh.ttc`,
		`C:/Windows/Fonts/msyh.ttf`,
		`C:/Windows/Fonts/simsun.ttc`,
		`/System/Library/Fonts/PingFang.ttc`,
		`/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf`,
		`/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc`,
		`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc`,
		`/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf`,
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TestApplyFont_ReadsFontFileOnce 是 S4 的核心回归：单次 Render 内 Draw 会调用
// applyFont 约 86 次（表头 + 星期 + 42 格日 + 42 格农历，不同字号）。旧实现每次
// LoadFontFace 都从磁盘解析 ~20MB 的 .ttc；修复后只解析一次。本测试用计数 seam
// 断言无论调用多少次 applyFont，字体文件仅被解析一次。
func TestApplyFont_ReadsFontFileOnce(t *testing.T) {
	p := findAnySystemFont()
	if p == "" {
		t.Skip("no system font available to exercise CJK font path")
	}
	resetCJKFontCache()
	origCandidates := cjkFontCandidates
	cjkFontCandidates = []string{p}
	origLoad := loadFontSource
	var reads int
	loadFontSource = func(path string, opts ...text.SourceOption) (*text.FontSource, error) {
		reads++
		return origLoad(path, opts...)
	}
	defer func() {
		cjkFontCandidates = origCandidates
		loadFontSource = origLoad
		resetCJKFontCache()
	}()

	dc := gg.NewContext(100, 100)
	// 模拟一次 Draw 的全部 applyFont 调用：表头 22、星期 13、42 格日 16、42 格农历 11。
	sizes := []float64{22, 13}
	for i := 0; i < 42; i++ {
		sizes = append(sizes, 16, 11)
	}
	for _, s := range sizes {
		applyFont(dc, s)
	}
	if reads != 1 {
		t.Fatalf("expected font file parsed exactly once across %d applyFont calls (S4), got %d reads", len(sizes), reads)
	}
}

// TestApplyFont_NoPanicWhenNoFont 验证无候选字体时 applyFont 不 panic、且回退到
// gg 默认字体（Latin）；resolveCJKFont 返回 nil。
func TestApplyFont_NoPanicWhenNoFont(t *testing.T) {
	resetCJKFontCache()
	origCandidates := cjkFontCandidates
	cjkFontCandidates = []string{`/nonexistent-font-xyz.ttf`}
	defer func() {
		cjkFontCandidates = origCandidates
		resetCJKFontCache()
	}()

	dc := gg.NewContext(50, 50)
	applyFont(dc, 12) // 不应 panic
	if resolveCJKFont() != nil {
		t.Errorf("resolveCJKFont should return nil when no candidate font exists")
	}
}
