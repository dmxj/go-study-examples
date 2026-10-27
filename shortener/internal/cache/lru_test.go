package cache

import (
	"fmt"
	"sync"
	"testing"
)

func TestLRUEviction(t *testing.T) {
	c := New[string, int](2)
	c.Add("a", 1)
	c.Add("b", 2)
	c.Get("a")    // a 变为最近使用，b 成为最久未使用
	c.Add("c", 3) // 容量满，淘汰 b

	if _, ok := c.Get("b"); ok {
		t.Error("b 应已被淘汰")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Errorf("Get(a) = %d, %v", v, ok)
	}
	if c.Len() != 2 {
		t.Errorf("Len = %d，期望 2", c.Len())
	}

	c.Add("a", 10) // 更新已有 key 不应改变容量
	if v, _ := c.Get("a"); v != 10 || c.Len() != 2 {
		t.Errorf("更新后 a=%d len=%d", v, c.Len())
	}
	c.Remove("a")
	if _, ok := c.Get("a"); ok {
		t.Error("Remove 后不应命中")
	}
	hits, misses := c.Stats()
	if hits != 3 || misses != 2 {
		t.Errorf("hits=%d misses=%d，期望 3/2", hits, misses)
	}
}

func TestLRUZeroCapacity(t *testing.T) {
	c := New[int, int](0)
	c.Add(1, 1)
	if _, ok := c.Get(1); ok || c.Len() != 0 {
		t.Error("容量为 0 时不应缓存任何数据")
	}
}

// TestLRUConcurrent 用 go test -race 运行可以发现并发 bug（比如 Get 只加了读锁）。
func TestLRUConcurrent(t *testing.T) {
	c := New[string, int](100)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				k := fmt.Sprint(i % 150)
				c.Add(k, i)
				c.Get(k)
			}
		}(g)
	}
	wg.Wait()
	if c.Len() > 100 {
		t.Errorf("Len = %d 超过容量", c.Len())
	}
}
