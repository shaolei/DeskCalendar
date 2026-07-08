//go:build !gg

// This file is the OFFLINE-safe stand-in for the gg renderer. It produces the
// exact same byte layout gg.ImageSurface/Pixmap would: a PREMULTIPLIED RGBA
// buffer (4 bytes/pixel, row-major), which is what a Win32 layered window
// (UpdateLayeredWindow + AC_SRC_ALPHA) consumes. See draw_gg.go (build tag
// `gg`) for the real gg implementation that drops in unchanged.
package main

import "math"

// RenderPanel draws a rounded, semi-transparent panel with a soft drop shadow
// and returns a PREMULTIPLIED RGBA byte buffer. See package doc for layout.
func RenderPanel() (premul []byte, w int, h int) {
	const (
		W, H   = 320, 240
		pad    = 16
		radius = 14
	)
	w, h = W, H

	// straight-alpha accumulation, all channels in 0..255 (float for precision)
	type px struct{ r, g, b, a float64 }
	buf := make([]px, W*H)

	// composite one rounded-rect layer using straight-alpha "over"
	drawRRect := func(x, y, bw, bh, rad int, r, g, b, a uint16) {
		for py := y; py < y+bh; py++ {
			if py < 0 || py >= H {
				continue
			}
			for pxX := x; pxX < x+bw; pxX++ {
				if pxX < 0 || pxX >= W {
					continue
				}
				d := sdfRRect(pxX, py, x, y, bw, bh, rad)
				var cov float64
				switch {
				case d <= -1:
					cov = float64(a) // fully inside -> full straight-alpha coverage
				case d >= 1:
					cov = 0
				default:
					cov = float64(a) * (0.5 - 0.5*d) // 2px AA edge
				}
				if cov <= 0 {
					continue
				}
				idx := py*W + pxX
				sa := cov / 255.0
				da := buf[idx].a / 255.0
				oa := sa + da*(1-sa)
				if oa <= 0 {
					continue
				}
				buf[idx].r = (float64(r)*sa + buf[idx].r*da*(1-sa)) / oa
				buf[idx].g = (float64(g)*sa + buf[idx].g*da*(1-sa)) / oa
				buf[idx].b = (float64(b)*sa + buf[idx].b*da*(1-sa)) / oa
				buf[idx].a = oa * 255
			}
		}
	}

	// shadow (offset, dark, low alpha, 2 passes for a touch of softness)
	drawRRect(pad+5, pad+9, W-2*pad, H-2*pad, radius+2, 0, 0, 0, 55)
	drawRRect(pad+2, pad+4, W-2*pad, H-2*pad, radius+1, 0, 0, 0, 45)
	// panel (light, mostly opaque)
	drawRRect(pad, pad, W-2*pad, H-2*pad, radius, 245, 247, 250, 235)

	// straight -> premultiplied (what UpdateLayeredWindow eats)
	premul = make([]byte, W*H*4)
	for i := 0; i < W*H; i++ {
		s := buf[i]
		af := s.a / 255.0
		premul[i*4+0] = uint8(float64(s.r) * af)
		premul[i*4+1] = uint8(float64(s.g) * af)
		premul[i*4+2] = uint8(float64(s.b) * af)
		premul[i*4+3] = uint8(s.a)
	}
	return premul, w, h
}

// sdfRRect: signed distance (px) from point to a rounded rect. <0 inside.
func sdfRRect(px, py, x, y, bw, bh, rad int) float64 {
	cx := float64(x) + float64(bw)/2
	cy := float64(y) + float64(bh)/2
	qx := math.Abs(float64(px)-cx) - (float64(bw)/2 - float64(rad))
	qy := math.Abs(float64(py)-cy) - (float64(bh)/2 - float64(rad))
	ax := math.Max(qx, 0)
	ay := math.Max(qy, 0)
	outside := math.Sqrt(ax*ax + ay*ay)
	inside := math.Min(math.Max(qx, qy), 0)
	return outside + inside - float64(rad)
}
