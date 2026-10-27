// Package ratelimit 实现按 key（通常是客户端 IP）限流的令牌桶。
//
// 生产中可以直接用 golang.org/x/time/rate；这里手写一个，目的是讲清楚令牌桶算法：
// 桶里最多存 burst 个令牌，每秒补充 rate 个；每个请求消耗 1 个，没有令牌就拒绝。
// 它既限制了平均速率，又允许短时间的突发。
package ratelimit

import (
	"math"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time // 上次补充令牌的时间
}

// Limiter 是按 key 隔离的令牌桶集合。
type Limiter struct {
	mu      sync.Mutex
	rate    float64 // 每秒补充的令牌数
	burst   float64 // 桶容量
	buckets map[string]*bucket
	now     func() time.Time
	idleTTL time.Duration // 超过该时长未访问的桶会被清理
	lastGC  time.Time
}

// New 创建限流器：每个 key 每秒 rate 个请求，最多突发 burst 个。
func New(rate float64, burst int) *Limiter {
	return &Limiter{
		rate:    rate,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
		now:     time.Now,
		idleTTL: 10 * time.Minute,
	}
}

// Allow 报告 key 的这次请求是否放行；不放行时返回建议的重试等待时间。
//
// 采用"惰性补充"：不需要后台 goroutine 定时加令牌，而是在每次请求时，
// 按"距离上次的时间 × rate"一次性补上，计算量 O(1)。
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.gc(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now} // 新客户端从满桶开始
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens = math.Min(l.burst, b.tokens+elapsed*l.rate)
	b.last = now

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	// 还差 (1 - tokens) 个令牌，按速率换算成等待时间
	wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, wait
}

// gc 定期清理长时间不活跃的桶，防止 map 随 IP 数量无限增长（内存泄漏）。
// 在持有锁的情况下调用。
func (l *Limiter) gc(now time.Time) {
	if now.Sub(l.lastGC) < time.Minute {
		return
	}
	l.lastGC = now
	for k, b := range l.buckets {
		if now.Sub(b.last) > l.idleTTL {
			delete(l.buckets, k)
		}
	}
}

// Len 返回当前跟踪的 key 数量（用于测试与监控）。
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}
