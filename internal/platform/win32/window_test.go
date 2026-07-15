package win32

import (
	"image"
	"testing"
)

// TestFakeWindow_ShowHideVisible 验证 fake 的显隐与 Visible 状态。
func TestFakeWindow_ShowHideVisible(t *testing.T) {
	w := &fakeWindow{}
	if w.Visible() {
		t.Fatal("new fake should start hidden")
	}
	w.Show()
	if !w.Visible() {
		t.Fatal("after Show, Visible must be true")
	}
	if w.showCalls != 1 {
		t.Errorf("showCalls = %d, want 1", w.showCalls)
	}
	w.Hide()
	if w.Visible() {
		t.Fatal("after Hide, Visible must be false")
	}
	if w.hideCalls != 1 {
		t.Errorf("hideCalls = %d, want 1", w.hideCalls)
	}
}

// TestFakeWindow_AnchorRecords 验证 AnchorAboveTray 记录传入矩形。
func TestFakeWindow_AnchorRecords(t *testing.T) {
	w := &fakeWindow{}
	r := image.Rect(100, 900, 124, 924) // 托盘图标屏幕坐标
	w.AnchorAboveTray(r)
	if w.anchorRect != r {
		t.Errorf("anchorRect = %v, want %v", w.anchorRect, r)
	}
}

// TestFakeWindow_PresentAppends 验证 Present 追加缓冲。
func TestFakeWindow_PresentAppends(t *testing.T) {
	w := &fakeWindow{}
	b1 := image.NewRGBA(image.Rect(0, 0, 4, 4))
	b2 := image.NewRGBA(image.Rect(0, 0, 4, 4))
	w.Present(b1)
	w.Present(b2)
	if len(w.presents) != 2 {
		t.Fatalf("presents len = %d, want 2", len(w.presents))
	}
	if w.presents[0] != b1 || w.presents[1] != b2 {
		t.Error("presents order mismatch")
	}
}

// TestBlitScaled_Identity 同尺寸拷贝须做 R/B 互换（BGRA 字节序）。
func TestBlitScaled_Identity(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	// 像素 (0,0): straight RGBA = (10,20,30,255)
	src.Pix[0], src.Pix[1], src.Pix[2], src.Pix[3] = 10, 20, 30, 255

	bits := make([]byte, 2*2*4)
	blitScaled(bits, 2, 2, src)

	if bits[0] != 30 { // B
		t.Errorf("bits[0] (B) = %d, want 30", bits[0])
	}
	if bits[1] != 20 { // G
		t.Errorf("bits[1] (G) = %d, want 20", bits[1])
	}
	if bits[2] != 10 { // R
		t.Errorf("bits[2] (R) = %d, want 10", bits[2])
	}
	if bits[3] != 255 { // A
		t.Errorf("bits[3] (A) = %d, want 255", bits[3])
	}
}

// TestBlitScaled_ScaleDown 缩小拷贝须填满目标且无越界 panic。
func TestBlitScaled_ScaleDown(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range src.Pix {
		src.Pix[i] = 200 // 统一填充，便于断言
	}
	dibW, dibH := 2, 2
	bits := make([]byte, dibW*dibH*4)
	blitScaled(bits, dibW, dibH, src)

	for i := 0; i < dibW*dibH; i++ {
		if bits[i*4+0] != 200 || bits[i*4+1] != 200 || bits[i*4+2] != 200 || bits[i*4+3] != 200 {
			t.Fatalf("scaled pixel %d = %v, want all 200", i, bits[i*4:i*4+4])
		}
	}
}

// TestBlitScaled_NilSafe nil 源或过小目标不得 panic。
func TestBlitScaled_NilSafe(t *testing.T) {
	blitScaled(make([]byte, 16), 2, 2, nil)        // nil 源
	blitScaled(make([]byte, 4), 2, 2, image.NewRGBA(image.Rect(0, 0, 4, 4))) // bits 过小
}

// TestBlitScaled_1to1Crisp 验证 #41：当源位图物理尺寸 == DIB 尺寸时，blitScaled 是
// 严格的逐像素 1:1 直拷（sy=y、sx=x），不做最近邻插值——这是高 DPI 下「gg 位图与
// DIB 同尺寸 → 清晰无缩放模糊」的关键不变量。用逐像素相异的源确认无跨像素串扰。
func TestBlitScaled_1to1Crisp(t *testing.T) {
	const w, h = 3, 2
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	// 逐像素相异：R/G/B 按 (x,y) 编码，便于断言每个目标像素精确来自对应源像素。
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := src.PixOffset(x, y)
			src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = byte(x*10+1), byte(y*10+2), byte(x*7+y*3+5), 255
		}
	}
	bits := make([]byte, w*h*4)
	blitScaled(bits, w, h, src) // 同尺寸 → 1:1

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			wantR, wantG, wantB := byte(x*10+1), byte(y*10+2), byte(x*7+y*3+5)
			// BGRA 字节序：bits[i]=B, [i+1]=G, [i+2]=R。
			if bits[i] != wantB || bits[i+1] != wantG || bits[i+2] != wantR || bits[i+3] != 255 {
				t.Errorf("pixel (%d,%d) = (%d,%d,%d,%d), want (%d,%d,%d,255) [B,G,R,A]",
					x, y, bits[i], bits[i+1], bits[i+2], bits[i+3], wantB, wantG, wantR)
			}
		}
	}
}
