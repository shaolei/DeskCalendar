package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// defaultIcon 生成内置托盘图标（32×32 蓝色圆形 PNG），避免依赖外部二进制资源。
// 视觉细节（圆角面板、日历字形）留待 90-UI 阶段打磨；MVP 仅需一个可识别图标。
func defaultIcon() []byte {
	const s = 32
	img := image.NewRGBA(image.Rect(0, 0, s, s))
	cx, cy, r := float64(s)/2, float64(s)/2, float64(s)/2-3
	accent := color.NRGBA{0x4C, 0x8D, 0xFF, 0xFF}
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, accent)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
