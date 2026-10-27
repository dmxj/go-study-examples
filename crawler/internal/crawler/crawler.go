// Package crawler 实现并发爬虫的调度核心。
//
// 架构（一个调度者 + N 个 worker）：
//
//	          tasks (无缓冲)
//	调度者 ───────────────▶ worker × N ──▶ 限速 → robots 检查 → 下载 → 解析
//	  ▲                                         │
//	  └──────────── results ◀───────────────────┘
//
// 调度者是唯一读写 seen（去重集合）和 queue（待抓队列）的 goroutine，
// 因此这两个数据结构不需要任何锁——"不要通过共享内存来通信，而要通过通信来共享内存"。
package crawler

import (
	"bytes"
	"context"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/dmxj/go-study-examples/crawler/internal/fetcher"
	"github.com/dmxj/go-study-examples/crawler/internal/parser"
	"github.com/dmxj/go-study-examples/crawler/internal/robots"
)

// Options 是爬虫的行为参数。
type Options struct {
	Workers       int           // 并发 worker 数
	MaxDepth      int           // 最大深度，种子为 0
	MaxPages      int           // 最多抓取页数，0 表示不限
	Rate          float64       // 全局每秒请求数，0 表示不限速
	Burst         int           // 令牌桶容量
	SameHost      bool          // 只抓取与种子同主机的页面
	RespectRobots bool          // 遵守 robots.txt
	UserAgent     string        // 请求头 UA
	Timeout       time.Duration // 单个请求超时
	MaxBodyBytes  int64         // 单页最大读取字节
}

// Result 是一个页面的抓取结果，会被序列化为 JSON 输出。
type Result struct {
	URL       string `json:"url"`
	Depth     int    `json:"depth"`
	Status    int    `json:"status,omitempty"`
	Title     string `json:"title,omitempty"`
	Links     int    `json:"links"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Error     string `json:"error,omitempty"`
	Skipped   string `json:"skipped,omitempty"` // 跳过原因，如 "robots"
}

// Stats 是一次运行的汇总。
type Stats struct {
	Pages   int           `json:"pages"`
	Errors  int           `json:"errors"`
	Skipped int           `json:"skipped"`
	Elapsed time.Duration `json:"-"`
}

// Crawler 是并发爬虫。
type Crawler struct {
	opts    Options
	fetch   *fetcher.Fetcher
	robots  *robots.Cache
	limiter *rate.Limiter
	log     *slog.Logger
}

// New 创建 Crawler。
func New(opts Options, log *slog.Logger) *Crawler {
	if opts.Workers <= 0 {
		opts.Workers = 4
	}
	if opts.Burst <= 0 {
		opts.Burst = 1
	}
	f := fetcher.New(opts.UserAgent, opts.Timeout, opts.MaxBodyBytes)
	limit := rate.Inf // 不限速
	if opts.Rate > 0 {
		limit = rate.Limit(opts.Rate)
	}
	return &Crawler{
		opts:    opts,
		fetch:   f,
		robots:  robots.NewCache(f.Client, opts.UserAgent, log),
		limiter: rate.NewLimiter(limit, opts.Burst), // 令牌桶：每秒补充 Rate 个令牌，最多存 Burst 个
		log:     log,
	}
}

type task struct {
	url   *url.URL
	depth int
}

type outcome struct {
	task  task
	res   Result
	links []*url.URL
}

// Run 从种子 URL 开始抓取，每完成一个页面调用一次 emit（只在调度者 goroutine 中调用，
// 所以 emit 无需并发安全）。ctx 取消后尽快停止并返回已有统计和 ctx.Err()。
func (c *Crawler) Run(parent context.Context, seeds []*url.URL, emit func(Result)) (Stats, error) {
	start := time.Now()
	// 派生一个可取消的子 ctx：Run 返回前 cancel()，确保所有 worker 和在途请求都能退出。
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	tasks := make(chan task)
	results := make(chan outcome)

	// 启动 worker 池
	var wg sync.WaitGroup
	for i := range c.opts.Workers { // Go 1.22+：range 整数
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			c.worker(ctx, id, tasks, results)
		}(i)
	}

	// 调度者状态：只在当前 goroutine 中访问，无需加锁
	var (
		seen       = make(map[string]bool)
		allowHost  = make(map[string]bool)
		queue      []task
		pending    int // 已派发、尚未收到结果的任务数
		dispatched int
		stats      Stats
	)
	push := func(u *url.URL, depth int) {
		key := u.String()
		if seen[key] {
			return
		}
		if c.opts.SameHost && !allowHost[u.Host] {
			return
		}
		seen[key] = true
		queue = append(queue, task{url: u, depth: depth})
	}
	for _, s := range seeds {
		s = parser.Normalize(s)
		allowHost[s.Host] = true
		push(s, 0)
	}

loop:
	for {
		// nil channel 技巧：向 nil channel 发送会永远阻塞，所以 select 会自动"跳过"这个分支。
		// 队列为空或已达页数上限时把 send 设为 nil，就只等待结果。
		var send chan<- task
		var next task
		canDispatch := c.opts.MaxPages == 0 || dispatched < c.opts.MaxPages
		if len(queue) > 0 && canDispatch {
			send, next = tasks, queue[0]
		}
		if send == nil && pending == 0 {
			break // 没有可派发的任务，也没有在途任务：爬取结束
		}
		select {
		case send <- next:
			queue = queue[1:]
			pending++
			dispatched++
		case o := <-results:
			pending--
			switch {
			case o.res.Skipped != "":
				stats.Skipped++
			case o.res.Error != "":
				stats.Errors++
			default:
				stats.Pages++
			}
			emit(o.res)
			if o.task.depth < c.opts.MaxDepth {
				for _, l := range o.links {
					push(l, o.task.depth+1)
				}
			}
		case <-ctx.Done():
			break loop
		}
	}

	close(tasks) // 通知 worker 没有新任务了，worker 的 for range 随之结束
	cancel()     // 若因 ctx 取消退出，让仍在请求中的 worker 尽快返回
	wg.Wait()
	stats.Elapsed = time.Since(start)
	// 注意返回的是 parent 的错误：子 ctx 总会被我们自己 cancel，它的 Err() 恒为 Canceled，
	// 只有 parent.Err() 才能区分"正常结束(nil)"、"用户中断"与"超时"。
	return stats, parent.Err()
}

func (c *Crawler) worker(ctx context.Context, id int, tasks <-chan task, results chan<- outcome) {
	for t := range tasks {
		o := c.process(ctx, t)
		// 发送结果时也要监听 ctx：调度者退出后没人接收，不加这个分支会导致 goroutine 泄漏。
		select {
		case results <- o:
		case <-ctx.Done():
			return
		}
	}
	c.log.Debug("worker 退出", "id", id)
}

// process 抓取并解析单个页面。
func (c *Crawler) process(ctx context.Context, t task) outcome {
	res := Result{URL: t.url.String(), Depth: t.depth}
	o := outcome{task: t}

	if c.opts.RespectRobots {
		rules := c.robots.Get(ctx, t.url)
		if !rules.Allowed(t.url.RequestURI()) {
			res.Skipped = "robots"
			o.res = res
			return o
		}
	}
	// 限速：拿不到令牌就等待；ctx 取消时立即返回错误
	if err := c.limiter.Wait(ctx); err != nil {
		res.Error = err.Error()
		o.res = res
		return o
	}

	start := time.Now()
	resp, err := c.fetch.Fetch(ctx, t.url)
	res.ElapsedMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = err.Error()
		o.res = res
		return o
	}
	res.Status = resp.StatusCode
	if resp.IsHTML && len(resp.Body) > 0 {
		page, err := parser.Parse(bytes.NewReader(resp.Body), resp.URL)
		if err != nil {
			c.log.Debug("HTML 解析出错，使用已解析部分", "url", res.URL, "err", err)
		}
		res.Title = page.Title
		res.Links = len(page.Links)
		o.links = page.Links
	}
	c.log.Debug("已抓取", "url", res.URL, "status", res.Status, "depth", t.depth)
	o.res = res
	return o
}
