//go:build gg

// Real gg renderer for the ADR-03 POC (路径 D). Drop-in replacement for
// draw_stdlib.go (build tag !gg). Produces the SAME premultiplied RGBA buffer
// that layered_windows.go consumes.
//
// Why this works: gg's Pixmap stores PREMULTIPLIED RGBA, 4 bytes/pixel
// (pixmap.go docstring + Data() accessor). That is exactly the byte layout
// UpdateLayeredWindow wants under AC_SRC_ALPHA + AC_SRC_OVER. So the flow is:
//   gg rasterises the rounded, semi-transparent panel  ->  pm.Data()
//   native layered window pushes pm.Data() to the screen  ->  real per-pixel
//   alpha rounded corners, no dependency patching.
//
// Build (requires network + gg's GPU stack — go-webgpu/webgpu, wgpu):
//
//	cd poc/layered-window
//	# add gg to go.mod first (go get github.com/gogpu/gg), then:
//	go run -tags gg .            # headless proof (panel.png)
//	go run -tags gg . window     # real layered window on Windows
package main

import (
	"image/color"

	"github.com/gogpu/gg"
)

// RenderPanel draws the rounded, semi-transparent panel with a soft drop
// shadow using gg and returns its PREMULTIPLIED RGBA byte buffer.
func RenderPanel() (premul []byte, w int, h int) {
	const (
		W, H   = 320, 240
		pad    = 16
		radius = 14
	)
	w, h = W, H

	// Own the Pixmap so we can read its premultiplied bytes directly after fill.
	pm := gg.NewPixmap(W, H)
	dc := gg.NewContextForPixmap(pm)

	// shadow: two offset passes, low alpha, slightly larger radius
	dc.SetColor(color.RGBA{0, 0, 0, 55})
	dc.DrawRoundedRectangle(float64(pad+5), float64(pad+9), float64(W-2*pad), float64(H-2*pad), float64(radius+2))
	_ = dc.Fill()
	dc.SetColor(color.RGBA{0, 0, 0, 45})
	dc.DrawRoundedRectangle(float64(pad+2), float64(pad+4), float64(W-2*pad), float64(H-2*pad), float64(radius+1))
	_ = dc.Fill()

	// panel: light, mostly opaque
	dc.SetColor(color.RGBA{245, 247, 250, 235})
	dc.DrawRoundedRectangle(float64(pad), float64(pad), float64(W-2*pad), float64(H-2*pad), float64(radius))
	_ = dc.Fill()

	// Pixmap.Data() is premultiplied RGBA — hand it straight to the window.
	premul = append([]byte(nil), pm.Data()...)
	return premul, w, h
}
