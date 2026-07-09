package ui

import (
	"os"
	"sync"

	"github.com/gogpu/gg"
)

// cjkFontCandidates 按平台列举可能的中文字体路径。首个可加载者被缓存复用，
// 缺失时回退 gg 默认字体（仅 Latin，中文会显示为缺字框但不致崩溃）。
//
// 说明：gg 的 LoadFontFace 对 .ttc/.otc 集合默认取索引 0；Windows 的
// msyh.ttc 索引 0 即「Microsoft YaHei」，满足中文渲染需求（已实测支持）。
var cjkFontCandidates = []string{
	`C:/Windows/Fonts/msyh.ttc`,
	`C:/Windows/Fonts/msyh.ttf`,
	`C:/Windows/Fonts/simsun.ttc`,
	`C:/Windows/Fonts/simhei.ttf`,
	`/System/Library/Fonts/PingFang.ttc`,
	`/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc`,
	`/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc`,
}

var (
	cjkFontOnce sync.Once
	cjkFontPath string
)

// resolveCJKFont 懒加载并缓存首个存在的中文字体路径（无则空串）。
func resolveCJKFont() string {
	cjkFontOnce.Do(func() {
		for _, p := range cjkFontCandidates {
			if _, err := os.Stat(p); err == nil {
				cjkFontPath = p
				return
			}
		}
	})
	return cjkFontPath
}

// applyFont 将 CJK 字体（若可用）以指定字号加载到上下文；失败则保留 gg 默认
// 字体（Latin）。每帧调用成本极低（仅一次 os.Stat 命中缓存路径后做文件读取，
// 渲染仅发生在显隐/主题变更/跨午夜，非逐帧热路径）。
func applyFont(dc *gg.Context, points float64) {
	p := resolveCJKFont()
	if p == "" {
		return
	}
	_ = dc.LoadFontFace(p, points)
}
