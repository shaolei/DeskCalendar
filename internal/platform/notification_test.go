package platform

import (
	"context"
	"testing"
)

// recordingSender 是测试用 NotificationSender，记录收到的通知。
type recordingSender struct {
	got []Notification
}

func (s *recordingSender) Send(ctx context.Context, n Notification) error {
	s.got = append(s.got, n)
	return nil
}

// TestNotificationSender_Contract 编译期校验 recordingSender 满足接口。
func TestNotificationSender_Contract(t *testing.T) {
	var _ NotificationSender = (*recordingSender)(nil)
	var _ NotificationSender = NewNotificationSender("DeskCalendar")
}

// TestRecordingSender_ReceivesNotification 验证 fake 正确捕获 Send 内容。
func TestRecordingSender_ReceivesNotification(t *testing.T) {
	s := &recordingSender{}
	n := Notification{Title: "待办提醒", Body: "15:00 团队周会", Silent: false}
	if err := s.Send(context.Background(), n); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(s.got) != 1 {
		t.Fatalf("expected 1 recorded notification, got %d", len(s.got))
	}
	if s.got[0].Title != "待办提醒" || s.got[0].Body != "15:00 团队周会" {
		t.Errorf("recorded notification mismatch: %+v", s.got[0])
	}
}

// TestNoopSender_SendReturnsNil MVP 默认实现不阻塞、不报错（真实 Toast 留 v1.1）。
func TestNoopSender_SendReturnsNil(t *testing.T) {
	s := NewNotificationSender("DeskCalendar")
	n := Notification{Title: "x", Body: "y", Silent: true}
	if err := s.Send(context.Background(), n); err != nil {
		t.Errorf("noop sender should not error, got %v", err)
	}
}

// TestNotification_Fields 基本字段可构造与读取。
func TestNotification_Fields(t *testing.T) {
	n := Notification{Title: "t", Body: "b", Silent: true}
	if n.Title != "t" || n.Body != "b" || !n.Silent {
		t.Errorf("Notification fields wrong: %+v", n)
	}
}
