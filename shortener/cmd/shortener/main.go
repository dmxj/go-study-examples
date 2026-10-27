// Command shortener 启动短链接服务。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dmxj/go-study-examples/shortener/internal/cache"
	"github.com/dmxj/go-study-examples/shortener/internal/config"
	"github.com/dmxj/go-study-examples/shortener/internal/httpapi"
	"github.com/dmxj/go-study-examples/shortener/internal/ratelimit"
	"github.com/dmxj/go-study-examples/shortener/internal/service"
	"github.com/dmxj/go-study-examples/shortener/internal/stats"
	"github.com/dmxj/go-study-examples/shortener/internal/store"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "配置错误:", err)
		os.Exit(2)
	}
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	if err := run(cfg, log); err != nil {
		log.Error("服务异常退出", "err", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config, log *slog.Logger) error {
	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	// defer 按后进先出执行：先注册的 db.Close 最后执行，保证 recorder 最后一次 Flush 时数据库仍可用。
	defer db.Close()

	rec := stats.NewRecorder(db, cfg.FlushInterval, log)
	svc := service.New(db, cache.New[string, store.Link](cfg.CacheSize), rec, cfg.BaseURL)
	api := &httpapi.API{
		Svc:        svc,
		BaseURL:    cfg.BaseURL,
		Limiter:    ratelimit.New(cfg.RatePerSec, cfg.Burst),
		TrustProxy: cfg.TrustProxy,
		Log:        log,
		Health:     func(r *http.Request) error { return db.Ping(r.Context()) },
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("短链接服务启动", "addr", cfg.Addr, "base_url", cfg.BaseURL, "db", cfg.DBPath)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		stop() // 恢复默认信号行为：再按一次 Ctrl+C 可以强制退出
	}

	log.Info("开始优雅关闭")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	// 顺序很重要：1) 停止 HTTP，确保不会再有新的访问计数；2) 把内存中的计数刷入数据库；3) 关闭数据库（defer）
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("HTTP 关闭超时", "err", err)
	}
	if err := rec.Close(shutdownCtx); err != nil {
		log.Error("最后一次写入访问统计失败", "err", err)
	}
	log.Info("服务已停止")
	return nil
}
