// Package stats 异步聚合短链接访问统计，定期批量写入数据库。
//
// 为什么不在每次跳转时直接 UPDATE？跳转是最热的路径：
// 每次访问都写库会让 SQLite 的单写者锁成为瓶颈，延迟也会变高。
// 先在内存里累加，每隔几秒一次性写入，能把 N 次写合并为 1 个事务。
// 代价是进程崩溃时会丢失最近几秒的计数——对访问统计来说通常可以接受。
package stats

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/dmxj/go-study-examples/shortener/internal/store"
)

// Flusher 是 Recorder 需要的持久化能力（*store.SQLite 实现了它）。
type Flusher interface {
	AddClicks(ctx context.Context, deltas map[string]store.ClickDelta) error
}

// Recorder 聚合访问次数。
type Recorder struct {
	mu      sync.Mutex
	pending map[string]store.ClickDelta

	flusher  Flusher
	interval time.Duration
	log      *slog.Logger

	stop chan struct{}
	done chan struct{}
}

// NewRecorder 创建并启动后台刷新 goroutine。务必在退出前调用 Close。
func NewRecorder(f Flusher, interval time.Duration, log *slog.Logger) *Recorder {
	r := &Recorder{
		pending:  make(map[string]store.ClickDelta),
		flusher:  f,
		interval: interval,
		log:      log,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go r.loop()
	return r
}

// Record 记录一次访问。只做内存操作，耗时在纳秒级。
func (r *Recorder) Record(code string, at time.Time) {
	r.mu.Lock()
	d := r.pending[code]
	d.Count++
	if at.After(d.LastAccess) {
		d.LastAccess = at
	}
	r.pending[code] = d
	r.mu.Unlock()
}

// Pending 返回 code 尚未落库的访问次数，用于让统计接口返回"实时"数据。
func (r *Recorder) Pending(code string) store.ClickDelta {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pending[code]
}

func (r *Recorder) loop() {
	defer close(r.done)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			r.Flush(context.Background())
		case <-r.stop:
			return
		}
	}
}

// Flush 把内存中的计数写入数据库。
//
// 技巧：持锁时只做"交换 map"这一个 O(1) 操作，然后释放锁再写库，
// 这样写库期间新的 Record 不会被阻塞。
func (r *Recorder) Flush(ctx context.Context) error {
	r.mu.Lock()
	batch := r.pending
	r.pending = make(map[string]store.ClickDelta, len(batch))
	r.mu.Unlock()

	if len(batch) == 0 {
		return nil
	}
	if err := r.flusher.AddClicks(ctx, batch); err != nil {
		r.log.Error("写入访问统计失败，稍后重试", "err", err, "links", len(batch))
		r.merge(batch) // 失败时把数据放回去，下次一起写
		return err
	}
	r.log.Debug("访问统计已落库", "links", len(batch))
	return nil
}

func (r *Recorder) merge(batch map[string]store.ClickDelta) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for code, d := range batch {
		cur := r.pending[code]
		cur.Count += d.Count
		if d.LastAccess.After(cur.LastAccess) {
			cur.LastAccess = d.LastAccess
		}
		r.pending[code] = cur
	}
}

// Close 停止后台 goroutine，并做最后一次 Flush，保证优雅关闭时不丢计数。
func (r *Recorder) Close(ctx context.Context) error {
	close(r.stop)
	<-r.done
	return r.Flush(ctx)
}
