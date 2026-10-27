// Command server 启动图书管理 REST API 服务。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dmxj/go-study-examples/rest-api/internal/auth"
	"github.com/dmxj/go-study-examples/rest-api/internal/config"
	"github.com/dmxj/go-study-examples/rest-api/internal/database"
	"github.com/dmxj/go-study-examples/rest-api/internal/router"
)

func main() {
	if err := run(); err != nil {
		slog.Error("服务异常退出", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	// 结构化 JSON 日志，便于 ELK/Loki 等日志系统采集。
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	db, err := database.Open(cfg.DSN, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(db); err != nil {
			log.Error("关闭数据库失败", "err", err)
		}
	}()

	engine := router.New(router.Deps{
		DB:      db,
		Tokens:  auth.NewTokenManager(cfg.JWTSecret, cfg.JWTTTL),
		Log:     log,
		GinMode: cfg.Mode,
	})

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: engine,
		// 一定要设置超时：默认值 0 表示永不超时，慢客户端可以耗尽服务器连接（Slowloris 攻击）。
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ListenAndServe 会阻塞，放到 goroutine 里，主 goroutine 等待信号。
	errCh := make(chan error, 1)
	go func() {
		log.Info("服务启动", "addr", cfg.Addr, "mode", cfg.Mode, "db", cfg.DSN)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh: // 启动失败（如端口被占用）
		return err
	case <-ctx.Done():
		log.Info("收到退出信号，开始优雅关闭", "timeout", cfg.ShutdownTimeout.String())
	}

	// Shutdown：停止接收新连接，等待进行中的请求处理完，超时后强制返回。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("服务已停止")
	return nil
}
