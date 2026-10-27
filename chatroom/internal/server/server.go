// Package server 提供 HTTP 路由：静态页面、WebSocket 接入、房间列表、健康检查。
package server

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dmxj/go-study-examples/chatroom/internal/chat"
)

// Options 是服务器参数。
type Options struct {
	AllowedOrigins []string  // 允许的跨域来源（如 https://chat.example.com），为空时只允许同源
	SendBuffer     int       // 每个客户端发送队列长度
	Heartbeat      Heartbeat // 心跳参数
}

// Server 实现 http.Handler。
type Server struct {
	hub      *chat.Hub
	log      *slog.Logger
	opts     Options
	upgrader websocket.Upgrader
	mux      *http.ServeMux
	conns    sync.WaitGroup // 跟踪所有 WebSocket 连接 goroutine，用于优雅关闭
}

// New 创建 Server。static 是前端静态文件（由 embed 提供）。
func New(hub *chat.Hub, static fs.FS, log *slog.Logger, opts Options) *Server {
	if opts.SendBuffer <= 0 {
		opts.SendBuffer = 64
	}
	if opts.Heartbeat == (Heartbeat{}) {
		opts.Heartbeat = DefaultHeartbeat
	}
	s := &Server{hub: hub, log: log, opts: opts, mux: http.NewServeMux()}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     s.checkOrigin,
	}
	// Go 1.22+ 的增强路由：支持 "方法 路径" 写法
	s.mux.Handle("GET /", http.FileServerFS(static))
	s.mux.HandleFunc("GET /ws", s.handleWS)
	s.mux.HandleFunc("GET /api/rooms", s.handleRooms)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Wait 等待所有 WebSocket 连接 goroutine 退出（在 Hub 停止后调用）。
//
// http.Server.Shutdown 不会关闭、也不会等待被 Hijack 的连接（WebSocket 正是如此），
// 所以必须自己通知（停止 Hub → 关闭 send → writePump 发 Close 帧）并等待它们结束。
func (s *Server) Wait() { s.conns.Wait() }

// handleWS 处理 /ws?room=xxx&nick=yyy。
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	nick, room := strings.TrimSpace(q.Get("nick")), strings.TrimSpace(q.Get("room"))
	if room == "" {
		room = "lobby"
	}
	// 在升级之前做参数校验：此时还能返回普通 HTTP 错误（curl/调试工具可见）
	if err := chat.ValidateNick(nick); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := chat.ValidateRoom(room); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Upgrade 完成 HTTP → WebSocket 握手；失败时它已经写好了 HTTP 错误响应。
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Debug("websocket 握手失败", "err", err)
		return
	}
	c := chat.NewClient(nick, room, s.opts.SendBuffer)
	if err := s.hub.Join(c); err != nil {
		rejectConn(conn, err, s.opts.Heartbeat.WriteWait)
		return
	}

	s.conns.Add(2)
	go func() {
		defer s.conns.Done()
		writePump(conn, c, s.opts.Heartbeat)
	}()
	go func() {
		defer s.conns.Done()
		readPump(conn, c, s.hub, s.opts.Heartbeat, s.log)
	}()
}

func (s *Server) handleRooms(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	rooms := s.hub.Rooms()
	if rooms == nil {
		rooms = []chat.RoomInfo{}
	}
	json.NewEncoder(w).Encode(rooms)
}

// checkOrigin 防御跨站 WebSocket 劫持（CSWSH）。
//
// 浏览器发起 WebSocket 不受同源策略限制，且会自动带上 Cookie。如果不校验 Origin，
// 恶意网站就能以受害者身份连上你的服务。gorilla 的默认实现只允许同源；
// 这里额外支持配置白名单。千万不要图省事写成 `return true`。
func (s *Server) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // 非浏览器客户端（命令行工具、移动端）通常不带 Origin
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, o := range s.opts.AllowedOrigins {
		if strings.EqualFold(strings.TrimRight(o, "/"), origin) {
			return true
		}
	}
	s.log.Warn("拒绝跨域 WebSocket 连接", "origin", origin)
	return false
}

// WaitTimeout 带超时地等待连接退出，返回是否在超时前全部结束。
func (s *Server) WaitTimeout(d time.Duration) bool {
	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}
