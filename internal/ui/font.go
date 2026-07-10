package ui

import (
	"os"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

// cjkFontCandidates 按平台列举可能的中文字体路径。首个可加载者被缓存复用，
// 缺失时回退 gg 默认字体（仅 Latin，中文会显示为缺字框但不致崩溃）。
//
// 说明：gg 的 NewFontSourceFromFile 对 .ttc/.otc 集合默认取索引 0；Windows 的
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
	cjkFontMu  sync.Mutex
	cjkFontSrc *text.FontSource // 已解析的字体 source；只创建一次，跨渲染复用
)

// loadFontSource 是可替换 seam：生产用 text.NewFontSourceFromFile（读盘 + 解析
// 字体文件一次）；测试可包一层计数器，断言「整次渲染只解析一次字体文件」
// （S4 回归——避免 Draw 内 ~86 次 applyFont 每次都重读 ~20MB 的 .ttc）。
var loadFontSource = text.NewFontSourceFromFile

// resolveCJKFont 懒加载并缓存首个存在的中文字体 source（无则 nil）。
// 仅解析一次字体文件；后续 applyFont 复用该 source 生成不同字号 face，不再读盘。
func resolveCJKFont() *text.FontSource {
	cjkFontMu.Lock()
	defer cjkFontMu.Unlock()
	if cjkFontSrc != nil {
		return cjkFontSrc
	}
	for _, p := range cjkFontCandidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		src, err := loadFontSource(p)
		if err != nil {
			continue
		}
		cjkFontSrc = src
		return cjkFontSrc
	}
	return nil
}

// applyFont 将 CJK 字体（若可用）以指定字号应用到上下文；失败/缺失则保留 gg 默认
// 字体（Latin）。复用已解析的 FontSource，仅生成对应字号 face（SetFont），不重读文件。
//
// 与旧实现（每次 dc.LoadFontFace 都从磁盘解析 .ttc）相比，单次 Render 的 ~86 次
// 调用现在只触发一次字体文件读取（S4 修复）。
func applyFont(dc *gg.Context, points float64) {
	src := resolveCJKFont()
	if src == nil {
		return
	}
	dc.SetFont(src.Face(points))
}
