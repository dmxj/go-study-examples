// Package storage 提供基于 JSON 文件的任务存储实现。
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dmxj/go-study-examples/todo-cli/internal/todo"
)

// fileFormat 是磁盘上的 JSON 结构。外面包一层带 version 的对象，
// 而不是直接存数组，方便以后做数据格式迁移。
type fileFormat struct {
	Version int         `json:"version"`
	Tasks   []todo.Task `json:"tasks"`
}

const currentVersion = 1

// JSONStore 把任务保存到一个 JSON 文件中。
type JSONStore struct {
	path string
}

// NewJSONStore 创建存储。文件不存在时不会报错，首次 Save 时自动创建。
func NewJSONStore(path string) *JSONStore {
	return &JSONStore{path: path}
}

// Path 返回数据文件路径。
func (s *JSONStore) Path() string { return s.path }

// Load 读取全部任务。文件不存在时返回空列表。
func (s *JSONStore) Load() ([]todo.Task, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 %s: %w", s.path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var f fileFormat
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("解析 %s: %w", s.path, err)
	}
	if f.Version > currentVersion {
		return nil, fmt.Errorf("数据文件版本 %d 高于程序支持的版本 %d，请升级 todo", f.Version, currentVersion)
	}
	return f.Tasks, nil
}

// Save 原子地写入全部任务：先写临时文件，再 rename 覆盖。
// 这样即使写到一半进程被杀，原文件也不会损坏。
func (s *JSONStore) Save(tasks []todo.Task) error {
	if tasks == nil {
		tasks = []todo.Task{} // 让 JSON 输出 [] 而不是 null
	}
	data, err := json.MarshalIndent(fileFormat{Version: currentVersion, Tasks: tasks}, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化任务: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".todo-*.json.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tmpName := tmp.Name()
	// 出错时清理临时文件；成功 rename 后 Remove 会返回 ErrNotExist，忽略即可。
	defer os.Remove(tmpName)

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("刷盘: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("替换数据文件: %w", err)
	}
	return nil
}

// 编译期断言：*JSONStore 实现了 todo.Repository。
var _ todo.Repository = (*JSONStore)(nil)
