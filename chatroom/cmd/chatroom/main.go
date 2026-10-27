// Command chatroom 启动 WebSocket 聊天室服务。
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dmxj/go-study-examples/chatroom/internal/chat"
	"github.com/dmxj/go-study-examples/chatroom/internal/config"
	"github.com/dmxj/go-study-examples/chatroom/internal/server"
	"github.com/dmxj/go-study-examples/chatroom/internal/web"
)

func main() {
	cfg, err := config.Load(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		os.Exit(2)
	}
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	if err := run(cfg, log); err != nil {
		log.Error("服务异常退出", "err", err)
		os.Exit(1)
	}
}

func run(cfg *config.Config, log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Hub 使用独立的 context：收到信号后，先停止接收新连接，再停止 Hub。
	hubCtx, stopHub := context.WithCancel(context.Background())
	defer stopHub()
	hub := chat.NewHub(cfg.HistorySize, log)
	go hub.Run(hubCtx)

	srv := server.New(hub, web.Static(), log, server.Options{
		AllowedOrigins: cfg.AllowedOrigins,
		SendBuffer:     cfg.SendBuffer,
	})
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		// 注意：不要设置 ReadTimeout/WriteTimeout！它们作用于整个连接，
		// 会把长连接的 WebSocket 在固定时间后强行断开。WebSocket 的超时由心跳单独控制。
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("聊天室已启动", "addr", cfg.Addr, "url", "http://localhost"+cfg.Addr)
		if err := httpSrv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	log.Info("开始优雅关闭")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	// 1. 停止接收新的 HTTP 请求 / WebSocket 握手
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Warn("HTTP 关闭超时", "err", err)
	}
	// 2. 停止 Hub：它会关闭所有客户端的 send，writePump 随之发送 Close 帧并断开
	stopHub()
	<-hub.Done()
	// 3. 等待所有连接 goroutine 退出（Shutdown 不管被 Hijack 的 WebSocket 连接）
	if !srv.WaitTimeout(cfg.ShutdownTimeout) {
		log.Warn("部分连接未能在超时前关闭")
	}
	log.Info("服务已停止")
	return nil
}
