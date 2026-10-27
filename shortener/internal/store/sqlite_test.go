package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func openTest(t *testing.T) *SQLite {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var now = time.UnixMilli(1_760_000_000_000)

func TestCreateGeneratedAndCustom(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	a, err := s.Create(ctx, "https://go.dev", "", now, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if a.Code != "1" { // 第一条记录 ID=1，base62(1)="1"
		t.Errorf("生成的短码 = %q，期望 1", a.Code)
	}

	if _, err := s.Create(ctx, "https://a.test", "golang", now, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, "https://b.test", "golang", now, time.Time{}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("重复自定义短码应返回 ErrCodeTaken，实际 %v", err)
	}

	got, err := s.Get(ctx, "golang")
	if err != nil || got.URL != "https://a.test" || !got.CreatedAt.Equal(now) {
		t.Errorf("Get = %+v, %v", got, err)
	}
	if _, err := s.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("不存在的短码应返回 ErrNotFound，实际 %v", err)
	}
}

func TestGeneratedCodeAvoidsCustom(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	// 有人先把 "2" 注册成了自定义短码（绕过 service 层的长度限制，模拟历史数据）
	if _, err := s.Create(ctx, "https://x.test", "2", now, time.Time{}); err != nil {
		t.Fatal(err)
	}
	// 这条记录 ID=2，base62(2)="2" 已被占用，应自动追加后缀
	l, err := s.Create(ctx, "https://y.test", "", now, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if l.Code != "2-1" {
		t.Errorf("冲突时生成的短码 = %q，期望 2-1", l.Code)
	}
}

func TestAddClicksAndExpiry(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	exp := now.Add(time.Hour)
	l, _ := s.Create(ctx, "https://go.dev", "", now, exp)

	err := s.AddClicks(ctx, map[string]ClickDelta{
		l.Code:    {Count: 3, LastAccess: now.Add(time.Minute)},
		"missing": {Count: 1, LastAccess: now}, // 不存在的短码不影响其他更新
	})
	if err != nil {
		t.Fatal(err)
	}
	s.AddClicks(ctx, map[string]ClickDelta{l.Code: {Count: 2, LastAccess: now}}) // 更早的访问时间不应覆盖

	got, _ := s.Get(ctx, l.Code)
	if got.Clicks != 5 || !got.LastAccessAt.Equal(now.Add(time.Minute)) {
		t.Errorf("clicks=%d last=%v，期望 5 与 +1m", got.Clicks, got.LastAccessAt)
	}
	if got.Expired(now) || !got.Expired(exp) {
		t.Error("Expired 判断错误")
	}
}
