package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// defaultIcon 生成内置托盘图标：32×32 圆角日历字形（品牌蓝底 + 白色日历网格 +
// 当天高亮格），纯标准库绘制，零依赖、零 CGO，避免依赖外部二进制资源。
func defaultIcon() []byte {
	const s = 32
	img := image.NewRGBA(image.Rect(0, 0, s, s))

	accent     := color.NRGBA{0x4C, 0x8D, 0xFF, 0xFF} // 品牌蓝（正文区底色）
	header     := color.NRGBA{0x35, 0x6E, 0xD6, 0xFF} // 略深蓝（顶部标题栏）
	white      := color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	transparent := color.NRGBA{0x00, 0x00, 0x00, 0x00}

	const (
		x0, y0, x1, y1 = 2, 2, 30, 30 // 外框（留 2px 边距）
		rad           = 7             // 圆角半径
		headerBottom  = 11            // 标题栏下边界（含）
		sepY          = 12            // 标题栏与正文之间的白色分隔线
	)

	// inside 判定像素是否落在圆角矩形内。
	inside := func(x, y int) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		var cx, cy int
		switch {
		case x < x0+rad && y < y0+rad:
			cx, cy = x0+rad, y0+rad
		case x > x1-rad && y < y0+rad:
			cx, cy = x1-rad, y0+rad
		case x < x0+rad && y > y1-rad:
			cx, cy = x0+rad, y1-rad
		case x > x1-rad && y > y1-rad:
			cx, cy = x1-rad, y1-rad
		default:
			return true // 中间区域
		}
		dx, dy := float64(x)-float64(cx), float64(y)-float64(cy)
		return dx*dx+dy*dy <= float64(rad)*float64(rad)
	}

	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			if !inside(x, y) {
				img.Set(x, y, transparent)
				continue
			}
			// 白色元素优先：当天高亮格 / 分隔线 / 网格线
			if y > headerBottom && y <= 18 && x >= x0 && x < 11 { // 左上格 = 今天
				img.Set(x, y, white)
				continue
			}
			if y == sepY { // 标题栏分隔白线
				img.Set(x, y, white)
				continue
			}
			if y == 19 || y == 25 { // 正文两道横线（3 行）
				img.Set(x, y, white)
				continue
			}
			if (x == 11 || x == 21) && y > sepY { // 两道竖线（3 列）
				img.Set(x, y, white)
				continue
			}
			if y <= headerBottom { // 标题栏
				img.Set(x, y, header)
				continue
			}
			img.Set(x, y, accent) // 正文区底色
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
