package windowstyle_test

import (
	"testing"

	windowstyle "github.com/shaolei/DeskCalendar/internal/platform/windowstyle"
)

func TestDefaultWindowStyle(t *testing.T) {
	ws := windowstyle.DefaultWindowStyle()
	if !ws.Frameless {
		t.Error("Frameless should be true")
	}
	if !ws.Layered {
		t.Error("Layered should be true")
	}
	if !ws.PerPixelAlpha {
		t.Error("PerPixelAlpha should be true")
	}
	if ws.CornerRadius != 16 {
		t.Errorf("CornerRadius = %d, want 16", ws.CornerRadius)
	}
	if !ws.Shadow {
		t.Error("Shadow should be true")
	}
	if ws.RenderMode != windowstyle.RenderModeAuto {
		t.Errorf("RenderMode = %v, want RenderModeAuto", ws.RenderMode)
	}
}
