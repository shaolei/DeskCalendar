module github.com/deskcalendar/poc/systray-spike

go 1.25.0

require (
	github.com/gogpu/gg v0.50.2
	github.com/gogpu/gogpu v0.43.4
	github.com/gogpu/systray v0.0.0-00010101000000-000000000000
	github.com/gogpu/ui v0.1.42
)

require (
	github.com/coregx/signals v0.1.0 // indirect
	github.com/go-webgpu/goffi v0.5.6 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/gogpu/gpucontext v0.21.0 // indirect
	github.com/gogpu/gputypes v0.5.1 // indirect
	github.com/gogpu/naga v0.17.15 // indirect
	github.com/gogpu/wgpu v0.30.9 // indirect
	golang.org/x/image v0.43.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)

// 指向本地打过 patch 的 gogpu / gpucontext / ui / systray 克隆副本。
// 这些 replace 仅用于本 spike；正式工程请改用具体版本号（见评估文档 ADR-06）。
replace (
	github.com/gogpu/gogpu => D:\workspace\github\gogpu
	github.com/gogpu/gpucontext => D:\workspace\github\gpucontext
	github.com/gogpu/ui => D:\workspace\github\ui
	github.com/gogpu/systray => D:\workspace\github\systray
)
