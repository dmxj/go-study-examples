package todo

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// 业务错误。调用方用 errors.Is 判断，而不是比较错误字符串。
var (
	ErrNotFound   = errors.New("任务不存在")
	ErrEmptyTitle = errors.New("标题不能为空")
)

// Repository 抽象任务的持久化。接口定义在"使用方"这一侧（Go 的惯用法），
// storage 包只需实现这两个方法即可，无需 import 本包以外的东西。
type Repository interface {
	Load() ([]Task, error)
	Save(tasks []Task) error
}

// Clock 返回当前时间。生产代码用 time.Now，测试注入固定时间。
type Clock func() time.Time

// Service 实现 Todo 的增删改查。每个方法都是"加载 → 修改 → 保存"的完整事务，
// 对 CLI 这种一次执行一条命令的场景足够简单可靠。
type Service struct {
	repo Repository
	now  Clock
}

// NewService 创建服务。clock 为 nil 时使用 time.Now。
func NewService(repo Repository, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, now: clock}
}

// Now 暴露服务使用的时钟，供展示层判断逾期。
func (s *Service) Now() time.Time { return s.now() }

// AddInput 是新增任务的参数。
type AddInput struct {
	Title    string
	Priority Priority
	Due      time.Time
	Tags     []string
}

// Add 新增任务并返回它（含分配的 ID）。
func (s *Service) Add(in AddInput) (Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Task{}, ErrEmptyTitle
	}
	tasks, err := s.repo.Load()
	if err != nil {
		return Task{}, fmt.Errorf("加载任务: %w", err)
	}
	if in.Priority == PriorityNone {
		in.Priority = PriorityMedium
	}
	t := Task{
		ID:        nextID(tasks),
		Title:     title,
		Priority:  in.Priority,
		Due:       in.Due,
		Tags:      normalizeTags(in.Tags),
		CreatedAt: s.now(),
	}
	tasks = append(tasks, t)
	if err := s.repo.Save(tasks); err != nil {
		return Task{}, fmt.Errorf("保存任务: %w", err)
	}
	return t, nil
}

// List 返回满足过滤条件的任务，按 sortBy 排序。
func (s *Service) List(f Filter, sortBy SortBy) ([]Task, error) {
	tasks, err := s.repo.Load()
	if err != nil {
		return nil, fmt.Errorf("加载任务: %w", err)
	}
	now := s.now()
	out := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if f.Match(t, now) {
			out = append(out, t)
		}
	}
	SortTasks(out, sortBy)
	return out, nil
}

// UpdateInput 是修改任务的参数。指针字段为 nil 表示"不修改"，
// 这是 Go 里表达"可选更新"的常见手法（类似 Python 的 None 默认参数）。
type UpdateInput struct {
	Title    *string
	Priority *Priority
	Due      *time.Time // 指向零值 time.Time 表示清除截止日期
	Tags     *[]string
}

// Update 修改指定任务。
func (s *Service) Update(id int, in UpdateInput) (Task, error) {
	return s.mutate(id, func(t *Task) error {
		if in.Title != nil {
			title := strings.TrimSpace(*in.Title)
			if title == "" {
				return ErrEmptyTitle
			}
			t.Title = title
		}
		if in.Priority != nil {
			t.Priority = *in.Priority
		}
		if in.Due != nil {
			t.Due = *in.Due
		}
		if in.Tags != nil {
			t.Tags = normalizeTags(*in.Tags)
		}
		return nil
	})
}

// Complete 标记任务完成；done=false 时重新打开任务。
func (s *Service) Complete(id int, done bool) (Task, error) {
	return s.mutate(id, func(t *Task) error {
		t.Done = done
		if done {
			t.CompletedAt = s.now()
		} else {
			t.CompletedAt = time.Time{}
		}
		return nil
	})
}

// Remove 删除指定任务。
func (s *Service) Remove(id int) (Task, error) {
	tasks, err := s.repo.Load()
	if err != nil {
		return Task{}, fmt.Errorf("加载任务: %w", err)
	}
	i := slices.IndexFunc(tasks, func(t Task) bool { return t.ID == id })
	if i < 0 {
		return Task{}, fmt.Errorf("%w: #%d", ErrNotFound, id)
	}
	removed := tasks[i]
	tasks = slices.Delete(tasks, i, i+1)
	if err := s.repo.Save(tasks); err != nil {
		return Task{}, fmt.Errorf("保存任务: %w", err)
	}
	return removed, nil
}

// ClearDone 删除所有已完成任务，返回删除数量。
func (s *Service) ClearDone() (int, error) {
	tasks, err := s.repo.Load()
	if err != nil {
		return 0, fmt.Errorf("加载任务: %w", err)
	}
	before := len(tasks)
	tasks = slices.DeleteFunc(tasks, func(t Task) bool { return t.Done })
	if err := s.repo.Save(tasks); err != nil {
		return 0, fmt.Errorf("保存任务: %w", err)
	}
	return before - len(tasks), nil
}

// mutate 封装"按 ID 查找 → 修改 → 保存"的公共流程。
// fn 拿到的是切片元素的指针，修改会直接反映到 tasks 中。
func (s *Service) mutate(id int, fn func(*Task) error) (Task, error) {
	tasks, err := s.repo.Load()
	if err != nil {
		return Task{}, fmt.Errorf("加载任务: %w", err)
	}
	i := slices.IndexFunc(tasks, func(t Task) bool { return t.ID == id })
	if i < 0 {
		return Task{}, fmt.Errorf("%w: #%d", ErrNotFound, id)
	}
	if err := fn(&tasks[i]); err != nil {
		return Task{}, err
	}
	if err := s.repo.Save(tasks); err != nil {
		return Task{}, fmt.Errorf("保存任务: %w", err)
	}
	return tasks[i], nil
}

// nextID 返回最大 ID + 1。删除中间的任务不会产生重复 ID；
// 但删除最大 ID 的任务后，这个 ID 会被下一个新任务复用（扩展练习：持久化 next_id 计数器避免复用）。
func nextID(tasks []Task) int {
	maxID := 0
	for _, t := range tasks {
		maxID = max(maxID, t.ID)
	}
	return maxID + 1
}

// normalizeTags 去空白、去重（大小写不敏感），保留首次出现的顺序。
func normalizeTags(tags []string) []string {
	var out []string
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		key := strings.ToLower(tag)
		if tag == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tag)
	}
	return out
}
