package platform

import (
	"context"
	"time"
)

// Notification 一条系统通知内容。
type Notification struct {
	Title     string    // 标题（如 "待办提醒"）
	Body      string    // 正文（如 "15:00 团队周会"）
	Silent    bool      // 是否静音（不发声）
	ExpiresAt time.Time // 过期时间（可选，零值表示系统默认）
}

// NotificationSender 系统通知发送者（接口隔离，便于 mock/替换）。
// 实现方封装零 CGO 的 Windows Toast（Win10+）或降级 balloon（见 §9 设计）。
//
// MVP（v1.0）阶段仅定义接口与占位实现（noopSender），真实 Toast 实现延后至 v1.1
// 与 60-Todo 联动；接口现在就定义以保证上层可直接依赖、决策可逆。
type NotificationSender interface {
	// Send 异步发送一条通知，不阻塞调用方。
	Send(ctx context.Context, n Notification) error
}

// noopSender MVP 占位实现：不实际弹出通知，仅满足接口契约。
// 真实实现（toastSender）将在 v1.1 提供，经零 CGO 的 COM/XML 或托盘 balloon 降级。
type noopSender struct {
	appID string
}

// NewNotificationSender 构造默认实现（MVP 为 noop 占位）。
func NewNotificationSender(appID string) NotificationSender {
	return &noopSender{appID: appID}
}

// Send MVP 占位：不实际弹出，直接返回 nil（不阻塞）。
// v1.1 真实实现将封装零 CGO 的 Windows Toast / balloon 降级。
func (s *noopSender) Send(ctx context.Context, n Notification) error {
	return nil
}
