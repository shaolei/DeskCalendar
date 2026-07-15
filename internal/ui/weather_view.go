package ui

import (
	"fmt"
	"image"

	"github.com/gogpu/gg"
	"github.com/shaolei/DeskCalendar/internal/theme"
)

// WeatherView 面板顶部天气卡片视图（#149，v1.2 EPIC）。
//
// 直接从持有的 Model.Weather 读取当前/预报/状态绘制，不依赖 internal/weather
// （app 在重渲时把 weather.Service.Snapshot 映射为 ui.WeatherCard），保持 ui 不反向
// 依赖天气域包（依赖方向约束 ADR-07a）。图标用 CJK 单字（晴/云/雨…），避免 emoji
// 缺字形（CJK 字体可用，emoji 可能不渲染）。
type WeatherView struct{}

// OnShow MVP 无操作（天气数据由 app 主循环每次重渲时重建）。
func (WeatherView) OnShow() {}

// OnHide MVP 无操作。
func (WeatherView) OnHide() {}

// Draw 在 rect（天气带，宽=面板宽、高=WeatherBandH）内绘制天气卡片。
// rect 已由 Render 填好不透明背景，本视图先以 Surface 覆写整带以与日历区分。
func (WeatherView) Draw(dc *gg.Context, rect image.Rectangle, m Model, th *theme.Theme) {
	if m.Weather == nil {
		return
	}
	w := float64(rect.Dx())
	x0, y0 := float64(rect.Min.X), float64(rect.Min.Y)
	bandH := float64(rect.Dy())
	card := m.Weather

	// 天气带底色（略亮于背景，与日历区分）。
	dc.SetColor(opaque(th.Palette.Surface))
	dc.DrawRectangle(x0, y0, w, bandH)
	_ = dc.Fill()

	// 底部分隔线（与日历头部风格一致）。
	dc.SetColor(opaque(th.Palette.Border))
	dc.SetLineWidth(1)
	dc.DrawLine(x0, y0+bandH-0.5, x0+w, y0+bandH-0.5)
	_ = dc.Stroke()

	// 降级态：无网/无缓存/加载失败 → 仅占位，绝不阻塞日历。
	if card.Status == WeatherError || card.Current == nil {
		applyFont(dc, 14)
		dc.SetColor(opaque(th.Palette.Muted))
		dc.DrawStringAnchored("天气暂不可用", x0+16, y0+bandH/2, 0, 0.5)
		return
	}

	cur := card.Current
	// 第一行：图标 + 温度 + 文字（+ 旧数据角标）。
	applyFont(dc, 22)
	dc.SetColor(opaque(th.Palette.Foreground))
	dc.DrawStringAnchored(cur.Icon, x0+20, y0+26, 0.5, 0.5)
	dc.DrawStringAnchored(fmt.Sprintf("%.0f°", cur.TempC), x0+56, y0+26, 0, 0.5)
	applyFont(dc, 14)
	dc.SetColor(opaque(th.Palette.Foreground))
	dc.DrawStringAnchored(cur.ConditionText, x0+110, y0+22, 0, 0.5)
	if card.Stale {
		applyFont(dc, 11)
		dc.SetColor(opaque(th.Palette.Muted))
		dc.DrawStringAnchored("·旧数据", x0+110, y0+38, 0, 0.5)
	}

	// 第二行：短期预报（今/明/后，固定步长避免依赖字体度量 API）。
	labels := []string{"今", "明", "后"}
	applyFont(dc, 12)
	dc.SetColor(opaque(th.Palette.Muted))
	fx := x0 + 16.0
	fy := y0 + bandH - 12
	for i, f := range card.Forecast {
		if i >= len(labels) {
			break
		}
		txt := fmt.Sprintf("%s %.0f°/%.0f°", labels[i], f.TempC, f.LowC)
		dc.DrawStringAnchored(txt, fx, fy, 0, 0.5)
		fx += 116.0
	}
}
