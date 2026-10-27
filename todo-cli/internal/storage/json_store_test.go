package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dmxj/go-study-examples/todo-cli/internal/todo"
)

func TestJSONStoreRoundTrip(t *testing.T) {
	// t.TempDir 在测试结束后自动删除，不会污染工作目录。
	path := filepath.Join(t.TempDir(), "sub", "todo.json")
	s := NewJSONStore(path)

	tasks, err := s.Load()
	if err != nil || len(tasks) != 0 {
		t.Fatalf("文件不存在时 Load() = %v, %v，期望空列表且无错误", tasks, err)
	}

	due := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	want := []todo.Task{
		{ID: 1, Title: "a", Priority: todo.PriorityHigh, Due: due, Tags: []string{"x"}},
		{ID: 2, Title: "b", Priority: todo.PriorityLow, Done: true},
	}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "a" || !got[0].Due.Equal(due) || !got[1].Done {
		t.Errorf("Load() = %+v，与保存的数据不一致", got)
	}

	// 临时文件应已被 rename，目录中只剩数据文件
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("目录中应只有 1 个文件，实际 %d 个", len(entries))
	}
}

func TestJSONStoreSaveEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.json")
	if err := NewJSONStore(path).Save(nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"tasks": []`) {
		t.Errorf("空列表应保存为 []，实际 %s", data)
	}
}

func TestJSONStoreCorrupted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.json")
	os.WriteFile(path, []byte("{not json"), 0o644)
	if _, err := NewJSONStore(path).Load(); err == nil {
		t.Error("损坏的 JSON 应返回错误")
	}
}

func TestJSONStoreFutureVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "todo.json")
	os.WriteFile(path, []byte(`{"version": 99, "tasks": []}`), 0o644)
	if _, err := NewJSONStore(path).Load(); err == nil {
		t.Error("更高版本的数据文件应返回错误")
	}
}
