package stats

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/dmxj/go-study-examples/shortener/internal/store"
)

type fakeFlusher struct {
	mu      sync.Mutex
	totals  map[string]int64
	fail    bool
	flushes int
}

func (f *fakeFlusher) AddClicks(_ context.Context, d map[string]store.ClickDelta) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushes++
	if f.fail {
		return errors.New("db down")
	}
	for k, v := range d {
		f.totals[k] += v.Count
	}
	return nil
}

func (f *fakeFlusher) total(code string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.totals[code]
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRecorderFlushOnClose(t *testing.T) {
	f := &fakeFlusher{totals: map[string]int64{}}
	r := NewRecorder(f, time.Hour, quiet) // 间隔很长：只有 Close 会触发写入
	now := time.Now()
	for i := 0; i < 10; i++ {
		r.Record("abc", now)
	}
	r.Record("xyz", now)
	if p := r.Pending("abc"); p.Count != 10 {
		t.Errorf("Pending = %d，期望 10", p.Count)
	}
	if err := r.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.total("abc") != 10 || f.total("xyz") != 1 {
		t.Errorf("Close 后应全部落库，实际 %v", f.totals)
	}
	if f.flushes != 1 {
		t.Errorf("10 次访问应合并为 1 次写入，实际 %d 次", f.flushes)
	}
}

func TestRecorderRetainsOnFailure(t *testing.T) {
	f := &fakeFlusher{totals: map[string]int64{}, fail: true}
	r := NewRecorder(f, time.Hour, quiet)
	defer r.Close(context.Background())
	r.Record("abc", time.Now())
	if err := r.Flush(context.Background()); err == nil {
		t.Fatal("期望写入失败")
	}
	if r.Pending("abc").Count != 1 {
		t.Error("写入失败时计数应被保留，等待下次重试")
	}
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	r.Flush(context.Background())
	if f.total("abc") != 1 || r.Pending("abc").Count != 0 {
		t.Error("重试成功后应落库并清空")
	}
}

func TestRecorderPeriodicFlush(t *testing.T) {
	f := &fakeFlusher{totals: map[string]int64{}}
	r := NewRecorder(f, 20*time.Millisecond, quiet)
	defer r.Close(context.Background())
	r.Record("abc", time.Now())
	deadline := time.Now().Add(time.Second)
	for f.total("abc") == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if f.total("abc") != 1 {
		t.Error("后台 goroutine 应定期写入")
	}
}
