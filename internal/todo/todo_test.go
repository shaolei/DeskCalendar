package todo

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func ptrTime(t time.Time) *time.Time { return &t }

// ── Todo 领域规则 ───────────────────────────────────────────────────

func TestIsOverdue_Branches(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)
	cases := []struct {
		name string
		todo *Todo
		want bool
	}{
		{"active+due past", &Todo{Status: StatusActive, Due: ptrTime(now.Add(-time.Hour))}, true},
		{"active+due future", &Todo{Status: StatusActive, Due: ptrTime(now.Add(time.Hour))}, false},
		{"done+due past", &Todo{Status: StatusDone, Due: ptrTime(now.Add(-time.Hour))}, false},
		{"active+no due", &Todo{Status: StatusActive}, false},
		{"done+no due", &Todo{Status: StatusDone}, false},
		{"active+due exactly now", &Todo{Status: StatusActive, Due: ptrTime(now)}, false}, // now.After(due)=false
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.todo.IsOverdue(now); got != c.want {
				t.Errorf("IsOverdue = %v, want %v", got, c.want)
			}
		})
	}
}

func TestIsDueForReminder_Branches(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.Local)
	cases := []struct {
		name string
		todo *Todo
		want bool
	}{
		{"active+reminder past", &Todo{Status: StatusActive, ReminderAt: ptrTime(now.Add(-time.Hour))}, true},
		{"active+reminder now", &Todo{Status: StatusActive, ReminderAt: ptrTime(now)}, true}, // !now.Before = true
		{"active+reminder future", &Todo{Status: StatusActive, ReminderAt: ptrTime(now.Add(time.Hour))}, false},
		{"done+reminder past", &Todo{Status: StatusDone, ReminderAt: ptrTime(now.Add(-time.Hour))}, false},
		{"active+no reminder", &Todo{Status: StatusActive}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.todo.IsDueForReminder(now); got != c.want {
				t.Errorf("IsDueForReminder = %v, want %v", got, c.want)
			}
		})
	}
}

// ── Repository：List / QueryByDate ─────────────────────────────────

func sampleTodos() []*Todo {
	base := time.Date(2026, 7, 10, 9, 0, 0, 0, time.Local)
	return []*Todo{
		{ID: "a", Title: "买菜", Status: StatusActive, Due: ptrTime(time.Date(2026, 7, 12, 18, 0, 0, 0, time.Local)), Tags: []string{"生活"}, CreatedAt: base.Add(-2 * time.Hour)},
		{ID: "b", Title: "周报", Status: StatusDone, Due: ptrTime(time.Date(2026, 7, 12, 17, 0, 0, 0, time.Local)), CreatedAt: base.Add(-1 * time.Hour)},
		{ID: "c", Title: "读书", Status: StatusActive, Tags: []string{"学习", "生活"}, CreatedAt: base},
		{ID: "d", Title: "缴费", Status: StatusActive, Due: ptrTime(time.Date(2026, 7, 13, 10, 0, 0, 0, time.Local)), CreatedAt: base.Add(time.Hour)},
	}
}

func TestRepository_ListAndQuery(t *testing.T) {
	repos := map[string]func() TodoRepository{
		"memory": func() TodoRepository { return NewMemoryRepository() },
		"json": func() TodoRepository {
			r, err := NewJSONFileRepository(filepath.Join(t.TempDir(), "todos.json"))
			if err != nil {
				t.Fatalf("json repo: %v", err)
			}
			return r
		},
	}
	for name, mk := range repos {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := mk()
			for _, td := range sampleTodos() {
				cp := *td
				if err := repo.Add(ctx, &cp); err != nil {
					t.Fatalf("add: %v", err)
				}
			}

			// 全量：应排序（Due 升序、无 Due 置底、再 CreatedAt 升序）。
			all, err := repo.List(ctx, ListFilter{})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(all) != 4 {
				t.Fatalf("list len = %d, want 4", len(all))
			}
			// b(7-12 17:00),a(7-12 18:00),d(7-13),c(nil due last)
			wantOrder := []string{"b", "a", "d", "c"}
			for i, id := range wantOrder {
				if all[i].ID != id {
					t.Errorf("list order[%d] = %s, want %s", i, all[i].ID, id)
				}
			}

			// Status 过滤。
			done, _ := repo.List(ctx, ListFilter{Status: ptr(StatusDone)})
			if len(done) != 1 || done[0].ID != "b" {
				t.Errorf("status=done filter = %v, want [b]", ids(done))
			}

			// Tag 过滤（精确、多标签其一命中）。
			life, _ := repo.List(ctx, ListFilter{Tag: "生活"})
			if len(life) != 2 {
				t.Errorf("tag=生活 filter = %v, want 2 items", ids(life))
			}

			// OnlyDue。
			onlyDue, _ := repo.List(ctx, ListFilter{OnlyDue: true})
			if len(onlyDue) != 3 {
				t.Errorf("onlyDue filter = %v, want 3 items", ids(onlyDue))
			}

			// From/To 区间（含边界）。
			from := time.Date(2026, 7, 12, 0, 0, 0, 0, time.Local)
			to := time.Date(2026, 7, 12, 23, 59, 59, 0, time.Local)
			rng, _ := repo.List(ctx, ListFilter{From: ptrTime(from), To: ptrTime(to)})
			if len(rng) != 2 {
				t.Errorf("range filter = %v, want 2 items (a,b)", ids(rng))
			}

			// QueryByDate（7-12 当天，含已完成 b）。
			day := time.Date(2026, 7, 12, 0, 0, 0, 0, time.Local)
			q, _ := repo.QueryByDate(ctx, day)
			if len(q) != 2 {
				t.Errorf("queryByDate(7-12) = %v, want 2 items (a,b)", ids(q))
			}
			// QueryByDate（7-13 当天，仅 d）。
			q13, _ := repo.QueryByDate(ctx, time.Date(2026, 7, 13, 0, 0, 0, 0, time.Local))
			if len(q13) != 1 || q13[0].ID != "d" {
				t.Errorf("queryByDate(7-13) = %v, want [d]", ids(q13))
			}
		})
	}
}

func TestRepository_UpdateRemoveErrors(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	if err := repo.Update(ctx, &Todo{ID: "x"}); err != ErrNotFound {
		t.Errorf("update missing = %v, want ErrNotFound", err)
	}
	if err := repo.Remove(ctx, "x"); err != ErrNotFound {
		t.Errorf("remove missing = %v, want ErrNotFound", err)
	}
	if err := repo.Add(ctx, &Todo{ID: ""}); err != ErrInvalidID {
		t.Errorf("add empty id = %v, want ErrInvalidID", err)
	}
	// 正常 update/remove。
	_ = repo.Add(ctx, &Todo{ID: "y", Title: "t"})
	if err := repo.Update(ctx, &Todo{ID: "y", Title: "t2", Status: StatusDone}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := repo.List(ctx, ListFilter{})
	if got[0].Title != "t2" {
		t.Errorf("updated title = %q, want t2", got[0].Title)
	}
	if err := repo.Remove(ctx, "y"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got, _ := repo.List(ctx, ListFilter{}); len(got) != 0 {
		t.Errorf("after remove len = %d, want 0", len(got))
	}
}

func TestJSONFileRepository_PersistAndReload(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "todos.json")
	repo, err := NewJSONFileRepository(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	now := time.Date(2026, 7, 12, 9, 0, 0, 0, time.Local)
	if err := repo.Add(ctx, &Todo{ID: "k", Title: "持久化", Due: ptrTime(now), Status: StatusActive, CreatedAt: now}); err != nil {
		t.Fatalf("add: %v", err)
	}
	// 重新打开（模拟进程重启）。
	repo2, err := NewJSONFileRepository(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, _ := repo2.List(ctx, ListFilter{})
	if len(got) != 1 || got[0].ID != "k" || !got[0].Due.Equal(now) {
		t.Fatalf("reload mismatch: %+v", got)
	}
}

func ids(list []*Todo) []string {
	out := make([]string, 0, len(list))
	for _, t := range list {
		out = append(out, t.ID)
	}
	return out
}

// ── Service 状态机 ──────────────────────────────────────────────────

func TestService_AddAndTransitions(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryRepository())

	// 空标题被拒。
	if _, err := svc.Add(ctx, "   ", AddOpts{}); err != ErrEmptyTitle {
		t.Errorf("empty title = %v, want ErrEmptyTitle", err)
	}
	// 正常新增：生成 ID + CreatedAt，状态 active。
	todo, err := svc.Add(ctx, " 写测试 ", AddOpts{Tags: []string{"go", "go", ""}})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if todo.ID == "" {
		t.Error("generated ID is empty")
	}
	if todo.Status != StatusActive {
		t.Errorf("status = %q, want active", todo.Status)
	}
	if len(todo.Tags) != 1 || todo.Tags[0] != "go" {
		t.Errorf("tags deduped = %v, want [go]", todo.Tags)
	}
	if todo.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}

	// Complete -> done（幂等）。
	done, err := svc.Complete(ctx, todo.ID)
	if err != nil || done.Status != StatusDone {
		t.Fatalf("complete: %v status=%v", err, done.Status)
	}
	done2, _ := svc.Complete(ctx, todo.ID)
	if done2.Status != StatusDone {
		t.Error("complete should be idempotent")
	}

	// 查不到返回 ErrNotFound。
	if _, err := svc.Complete(ctx, "nope"); err != ErrNotFound {
		t.Errorf("complete missing = %v, want ErrNotFound", err)
	}

	// Reopen -> active（幂等）。
	re, err := svc.Reopen(ctx, todo.ID)
	if err != nil || re.Status != StatusActive {
		t.Fatalf("reopen: %v status=%v", err, re.Status)
	}
	if _, err := svc.Reopen(ctx, todo.ID); err != nil {
		t.Errorf("reopen idempotent err = %v", err)
	}

	// Snooze：仅当有 ReminderAt。
	if _, err := svc.Snooze(ctx, todo.ID, time.Now().Add(time.Hour)); err != ErrInvalidStatus {
		t.Errorf("snooze without reminder = %v, want ErrInvalidStatus", err)
	}
	todo.ReminderAt = ptrTime(time.Now().Add(-time.Hour))
	_ = svc.repo.Update(ctx, todo) // 重新设置 reminder（service 不直接暴露，借 repo）
	later := time.Now().Add(2 * time.Hour)
	if _, err := svc.Snooze(ctx, todo.ID, later); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	all, _ := svc.List(ctx, ListFilter{})
	if all[0].ReminderAt == nil || !all[0].ReminderAt.Equal(later) {
		t.Errorf("snooze did not update reminder_at: %v", all[0].ReminderAt)
	}

	// Remove。
	if err := svc.Remove(ctx, todo.ID); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := svc.Remove(ctx, todo.ID); err != ErrNotFound {
		t.Errorf("remove missing = %v, want ErrNotFound", err)
	}
}

// ── Reminder 调度 ───────────────────────────────────────────────────

type fakeNotifier struct {
	mu      sync.Mutex
	titles  []string
	bodies  []string
	failAll bool
}

func (n *fakeNotifier) Notify(_ context.Context, title, body string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.failAll {
		return fmt.Errorf("boom")
	}
	n.titles = append(n.titles, title)
	n.bodies = append(n.bodies, body)
	return nil
}

type fakeStore struct{ fired int }

func (s *fakeStore) EmitReminderFired(*Todo) { s.fired++ }

func TestReminderService_TickAndDedup(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	// 相对当前时刻构造，避免依赖 tick 内部 time.Now 与固定时钟不一致。
	now := time.Now()
	// 两个待提醒（active + reminder<=now），一个已完成，一个未到。
	repo.Add(ctx, &Todo{ID: "due1", Title: "A", Status: StatusActive, ReminderAt: ptrTime(now.Add(-time.Minute)), CreatedAt: now})
	repo.Add(ctx, &Todo{ID: "due2", Title: "B", Status: StatusActive, ReminderAt: ptrTime(now), CreatedAt: now})
	repo.Add(ctx, &Todo{ID: "done", Title: "C", Status: StatusDone, ReminderAt: ptrTime(now.Add(-time.Minute)), CreatedAt: now})
	repo.Add(ctx, &Todo{ID: "future", Title: "D", Status: StatusActive, ReminderAt: ptrTime(now.Add(time.Hour)), CreatedAt: now})

	notif := &fakeNotifier{}
	store := &fakeStore{}
	r := &ReminderService{repo: repo, notifier: notif, store: store, cfg: SchedulerConfig{ImmediateRun: false}}
	r.fired = make(map[string]struct{})
	if err := r.tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	// 仅 due1/due2 命中；done 不触发；future 未到。
	if len(notif.titles) != 2 {
		t.Errorf("notified = %v, want 2 (due1,due2)", notif.titles)
	}
	if store.fired != 2 {
		t.Errorf("store fired = %d, want 2", store.fired)
	}
	// 进程内去重：再次 tick 不应重复弹。
	_ = r.tick(ctx)
	if len(notif.titles) != 2 {
		t.Errorf("after dedup tick notified = %v, want still 2", notif.titles)
	}
}

func TestReminderService_NotifyFailureNoInterrupt(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryRepository()
	now := time.Now()
	repo.Add(ctx, &Todo{ID: "x", Title: "X", Status: StatusActive, ReminderAt: ptrTime(now), CreatedAt: now})
	notif := &fakeNotifier{failAll: true}
	r := &ReminderService{repo: repo, notifier: notif, store: nil, cfg: SchedulerConfig{ImmediateRun: false}}
	r.fired = make(map[string]struct{})
	if err := r.tick(ctx); err != nil {
		t.Fatalf("tick with failing notifier should not error: %v", err)
	}
	// 通知失败也应标记 fired（避免反复弹），store 为 nil 不 panic。
	if _, ok := r.fired["x"]; !ok {
		t.Error("fired should be marked even when notify fails")
	}
}

func TestReminderService_StartStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := NewMemoryRepository()
	notif := &fakeNotifier{}
	r := NewReminderService(repo, notif, nil, SchedulerConfig{Interval: 20 * time.Millisecond, ImmediateRun: true})
	if err := r.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	// 重复 Start 报错。
	if err := r.Start(ctx); err == nil {
		t.Error("second Start should error")
	}
	time.Sleep(60 * time.Millisecond) // 应至少 tick 2~3 次（空仓储，无通知）
	if err := r.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// Stop 后 cancel 不应 panic。
	cancel()
	time.Sleep(30 * time.Millisecond)
}
