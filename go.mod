module github.com/shaolei/DeskCalendar

go 1.25.0

require (
	// Phase 0 基础设施依赖：
	// - coregx/signals：响应式 Signal 原语（gogpu/ui/state.Signal 的类型别名来源，
	//   同一版本 v0.1.0 保证与未来 UI 层类型统一；纯 Go、零 CGO、可离线构建）。
	// gogpu 全栈（wgpu）推迟到 Phase 3（shell 装配）再引入，保持 Phase 0 精简。
	github.com/coregx/signals v0.1.1
	golang.org/x/sys v0.47.0
)

require (
	github.com/6tail/lunar-go v1.4.6
	github.com/gogpu/gg v0.50.6
	github.com/gogpu/systray v0.1.2
)

require (
	github.com/go-webgpu/goffi v0.6.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gogpu/gpucontext v0.21.1 // indirect
	github.com/gogpu/gputypes v0.5.1 // indirect
	golang.org/x/image v0.44.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)
