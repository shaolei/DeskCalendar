module github.com/shaolei/DeskCalendar

go 1.25

// Phase 0 基础设施依赖：
// - coregx/signals：响应式 Signal 原语（gogpu/ui/state.Signal 的类型别名来源，
//   同一版本 v0.1.0 保证与未来 UI 层类型统一；纯 Go、零 CGO、可离线构建）。
// gogpu 全栈（wgpu）推迟到 Phase 3（shell 装配）再引入，保持 Phase 0 精简。
require github.com/coregx/signals v0.1.0
