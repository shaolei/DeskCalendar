package main

// POC: gg + native layered window for ADR-03 (full形态).
//
// Usage:
//   poc.exe            -> headless proof: render the premultiplied panel,
//                         write panel.png, assert transparent corners + opaque
//                         center. Runs anywhere (proves the bitmap half).
//   poc.exe window     -> create a WS_EX_LAYERED window and show the panel
//                         (run on a real Windows desktop to see the popup).
//
// The gg renderer (draw_gg.go, build tag `gg`) produces the same premultiplied
// RGBA buffer as draw_stdlib.go; the window half (layered_windows.go) is
// identical either way.
import (
	"image"
	"image/png"
	"os"
)

func main() {
	mode := "png"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	if mode == "window" {
		runLayeredWindow() // defined in layered_windows.go (//go:build windows)
		return
	}
	runPNGProof()
}

func runPNGProof() {
	premul, w, h := RenderPanel()

	// assertions on the PREMULTIPLIED buffer (what UpdateLayeredWindow eats)
	cornerA := premul[0*4+3]             // top-left corner: outside rounded rect -> must be 0
	ci := (h/2*w + w/2) * 4             // center pixel
	centerA := premul[ci+3]             // center: inside panel -> opaque-ish
	centerR := premul[ci+0]             // premultiplied red of panel fill
	ok := cornerA == 0 && centerA > 200 && centerR > 180

	// write an eyeball-able (un-premultiplied) PNG
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		a := float64(premul[i*4+3]) / 255
		var r, g, b uint8
		if a > 0 {
			r = uint8(float64(premul[i*4+0]) / a)
			g = uint8(float64(premul[i*4+1]) / a)
			b = uint8(float64(premul[i*4+2]) / a)
		}
		img.Pix[i*4+0], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, premul[i*4+3]
	}
	f, err := os.Create("panel.png")
	if err != nil {
		println("create panel.png:", err.Error())
		os.Exit(1)
	}
	if err := png.Encode(f, img); err != nil {
		println("encode png:", err.Error())
		os.Exit(1)
	}
	f.Close()

	if ok {
		println("PNG proof PASS:",
			"cornerAlpha=", cornerA, "(transparent)",
			"centerAlpha=", centerA, "(opaque panel)",
			"centerPremulR=", centerR, "-> panel.png written")
	} else {
		println("PNG proof FAIL: cornerAlpha=", cornerA, " centerAlpha=", centerA, " centerPremulR=", centerR)
		os.Exit(1)
	}
}
