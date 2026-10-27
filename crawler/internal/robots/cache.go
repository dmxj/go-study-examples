package robots

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
)

// Cache 按 scheme://host 缓存 robots 规则，每个主机只请求一次 robots.txt。
//
// 多个 worker 可能同时访问同一个新主机，这里用"每个主机一个 sync.Once"保证只下载一次，
// 其余 goroutine 会阻塞在 once.Do 上直到第一个完成——这就是简化版的 singleflight。
type Cache struct {
	client    *http.Client
	userAgent string
	log       *slog.Logger

	mu      sync.Mutex
	entries map[string]*entry
}

type entry struct {
	once  sync.Once
	rules *Rules
}

// NewCache 创建缓存。
func NewCache(client *http.Client, userAgent string, log *slog.Logger) *Cache {
	return &Cache{client: client, userAgent: userAgent, log: log, entries: map[string]*entry{}}
}

// Get 返回 u 所在主机的规则。获取失败时按"全部允许"处理（与主流搜索引擎一致：
// 4xx 视为没有限制；网络错误这里也宽松处理，生产中可改为暂缓抓取）。
func (c *Cache) Get(ctx context.Context, u *url.URL) *Rules {
	key := u.Scheme + "://" + u.Host
	c.mu.Lock()
	e, ok := c.entries[key]
	if !ok {
		e = &entry{}
		c.entries[key] = e
	}
	c.mu.Unlock() // 下载 robots.txt 时不持有全局锁，避免阻塞其他主机

	e.once.Do(func() {
		e.rules = c.fetch(ctx, key+"/robots.txt")
	})
	return e.rules
}

func (c *Cache) fetch(ctx context.Context, robotsURL string) *Rules {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return &Rules{}
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		c.log.Debug("获取 robots.txt 失败，按允许处理", "url", robotsURL, "err", err)
		return &Rules{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &Rules{}
	}
	// robots.txt 最多读 500KB（RFC 9309 要求至少支持 500KiB）
	rules := Parse(io.LimitReader(resp.Body, 500<<10), c.userAgent)
	c.log.Debug("已加载 robots.txt", "url", robotsURL, "rules", len(rules.rules))
	return rules
}
