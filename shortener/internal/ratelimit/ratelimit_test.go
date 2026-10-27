package ratelimit

import (
	"testing"
	"time"
)

// fakeClock 让测试可以"拨动时间"，而不必真的 sleep。
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func TestTokenBucket(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := New(2, 3) // 每秒 2 个，突发 3 个
	l.now = clk.now

	// 满桶：前 3 个立即放行，第 4 个被拒
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("1.1.1.1"); !ok {
			t.Fatalf("第 %d 个请求应放行", i+1)
		}
	}
	ok, wait := l.Allow("1.1.1.1")
	if ok || wait != 500*time.Millisecond {
		t.Errorf("第 4 个请求 = %v, wait=%v，期望拒绝且等待 500ms", ok, wait)
	}

	// 不同 IP 互不影响
	if ok, _ := l.Allow("2.2.2.2"); !ok {
		t.Error("另一个 IP 应有独立的桶")
	}

	// 过 500ms 补充 1 个令牌
	clk.advance(500 * time.Millisecond)
	if ok, _ := l.Allow("1.1.1.1"); !ok {
		t.Error("补充令牌后应放行")
	}
	// 很久之后令牌最多补满到 burst，而不是无限累积
	clk.advance(time.Hour)
	allowed := 0
	for i := 0; i < 10; i++ {
		if ok, _ := l.Allow("1.1.1.1"); ok {
			allowed++
		}
	}
	if allowed != 3 {
		t.Errorf("令牌应封顶为 burst=3，实际放行 %d", allowed)
	}
}

func TestGC(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := New(1, 1)
	l.now = clk.now
	l.Allow("a")
	l.Allow("b")
	clk.advance(11 * time.Minute)
	l.Allow("c") // 触发清理：a、b 已闲置超过 10 分钟
	if l.Len() != 1 {
		t.Errorf("闲置的桶应被清理，剩余 %d", l.Len())
	}
}
