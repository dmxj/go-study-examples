package todo

import (
	"errors"
	"testing"
	"time"
)

// memRepo 是测试用的内存仓库，同时记录 Save 次数，用于断言"出错时不保存"。
type memRepo struct {
	tasks   []Task
	saves   int
	loadErr error
}

func (m *memRepo) Load() ([]Task, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	// 返回副本，模拟真实存储"每次读出新数据"的语义
	return append([]Task(nil), m.tasks...), nil
}

func (m *memRepo) Save(tasks []Task) error {
	m.saves++
	m.tasks = append([]Task(nil), tasks...)
	return nil
}

func newTestService() (*Service, *memRepo) {
	repo := &memRepo{}
	return NewService(repo, func() time.Time { return fixedNow }), repo
}

func TestServiceAddAndList(t *testing.T) {
	svc, repo := newTestService()
	a, err := svc.Add(AddInput{Title: "  学习 Go  ", Tags: []string{"study", "Study", " "}})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 1 || a.Title != "学习 Go" || a.Priority != PriorityMedium {
		t.Errorf("Add 结果 %+v 不符合预期（ID=1、标题去空白、默认 medium）", a)
	}
	if len(a.Tags) != 1 {
		t.Errorf("标签应去重去空，实际 %v", a.Tags)
	}
	if _, err := svc.Add(AddInput{Title: "买菜", Priority: PriorityHigh}); err != nil {
		t.Fatal(err)
	}
	if repo.saves != 2 {
		t.Errorf("Save 次数 = %d，期望 2", repo.saves)
	}
	got, err := svc.List(Filter{}, SortByPriority)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "买菜" {
		t.Errorf("List 按优先级排序后第一个应为 买菜，实际 %+v", got)
	}
}

func TestServiceAddEmptyTitle(t *testing.T) {
	svc, repo := newTestService()
	_, err := svc.Add(AddInput{Title: "   "})
	if !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("期望 ErrEmptyTitle，实际 %v", err)
	}
	if repo.saves != 0 {
		t.Error("校验失败时不应写入存储")
	}
}

func TestServiceNextID(t *testing.T) {
	svc, _ := newTestService()
	svc.Add(AddInput{Title: "a"})
	svc.Add(AddInput{Title: "b"})
	if _, err := svc.Remove(2); err != nil {
		t.Fatal(err)
	}
	c, _ := svc.Add(AddInput{Title: "c"})
	// 删除最大 ID 后，新 ID = 当前最大 ID + 1 = 2。这里验证的是"按最大值递增"的规则。
	if c.ID != 2 {
		t.Errorf("新任务 ID = %d，期望 2", c.ID)
	}
}

func TestServiceCompleteAndClear(t *testing.T) {
	svc, _ := newTestService()
	svc.Add(AddInput{Title: "a"})
	svc.Add(AddInput{Title: "b"})
	done, err := svc.Complete(1, true)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Done || !done.CompletedAt.Equal(fixedNow) {
		t.Errorf("Complete 后应 Done=true 且记录完成时间，实际 %+v", done)
	}
	if _, err := svc.Complete(99, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的 ID 应返回 ErrNotFound，实际 %v", err)
	}
	n, err := svc.ClearDone()
	if err != nil || n != 1 {
		t.Fatalf("ClearDone() = %d, %v，期望 1, nil", n, err)
	}
	all, _ := svc.List(Filter{Status: StatusAll}, SortByID)
	if len(all) != 1 || all[0].Title != "b" {
		t.Errorf("清理后剩余 %+v，期望只剩 b", all)
	}
}

func TestServiceUpdate(t *testing.T) {
	svc, _ := newTestService()
	svc.Add(AddInput{Title: "old"})
	title, prio := "new", PriorityHigh
	got, err := svc.Update(1, UpdateInput{Title: &title, Priority: &prio})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new" || got.Priority != PriorityHigh {
		t.Errorf("Update 结果 %+v 不符合预期", got)
	}
	empty := ""
	if _, err := svc.Update(1, UpdateInput{Title: &empty}); !errors.Is(err, ErrEmptyTitle) {
		t.Errorf("空标题应返回 ErrEmptyTitle，实际 %v", err)
	}
}

func TestServiceLoadError(t *testing.T) {
	boom := errors.New("磁盘坏了")
	svc := NewService(&memRepo{loadErr: boom}, nil)
	if _, err := svc.List(Filter{}, SortByID); !errors.Is(err, boom) {
		t.Errorf("应使用 %%w 包装底层错误，实际 %v", err)
	}
}
