// Package cache 提供并发安全的泛型 LRU 缓存。
package cache

import (
	"container/list"
	"sync"
)

// LRU 是"最近最少使用"淘汰策略的定长缓存。
//
// 经典实现 = 哈希表 + 双向链表：
//   - map 负责 O(1) 定位节点；
//   - 链表维护访问顺序：每次访问把节点移到表头，容量满时淘汰表尾。
//
// 注意 Get 也会修改链表顺序（MoveToFront），所以读操作也必须加写锁，
// 不能用 RWMutex 的 RLock——这是实现 LRU 时最常见的并发 bug。
type LRU[K comparable, V any] struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[K]*list.Element

	hits, misses uint64
}

type entry[K comparable, V any] struct {
	key   K
	value V
}

// New 创建容量为 capacity 的 LRU。capacity <= 0 时缓存不保存任何数据。
func New[K comparable, V any](capacity int) *LRU[K, V] {
	return &LRU[K, V]{cap: capacity, ll: list.New(), items: make(map[K]*list.Element)}
}

// Get 查询 key，命中时把它标记为最近使用。
func (c *LRU[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		c.hits++
		return el.Value.(*entry[K, V]).value, true
	}
	c.misses++
	var zero V
	return zero, false
}

// Add 写入或更新 key。超过容量时淘汰最久未使用的条目。
func (c *LRU[K, V]) Add(key K, value V) {
	if c.cap <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*entry[K, V]).value = value
		return
	}
	c.items[key] = c.ll.PushFront(&entry[K, V]{key: key, value: value})
	if c.ll.Len() > c.cap {
		oldest := c.ll.Back()
		c.ll.Remove(oldest)
		delete(c.items, oldest.Value.(*entry[K, V]).key)
	}
}

// Remove 删除 key。
func (c *LRU[K, V]) Remove(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.Remove(el)
		delete(c.items, key)
	}
}

// Len 返回当前条目数。
func (c *LRU[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// Stats 返回命中与未命中次数。
func (c *LRU[K, V]) Stats() (hits, misses uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}
