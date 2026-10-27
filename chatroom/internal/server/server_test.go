package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dmxj/go-study-examples/chatroom/internal/chat"
)

type env struct {
	srv     *httptest.Server
	stopHub context.CancelFunc
	hub     *chat.Hub
	s       *Server
}

func newEnv(t *testing.T, hb Heartbeat) *env {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := chat.NewHub(10, log)
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	// fstest.MapFS 是内存文件系统，测试时代替 embed.FS
	static := fstest.MapFS{"index.html": {Data: []byte("<h1>chat</h1>")}}
	s := New(hub, static, log, Options{Heartbeat: hb})
	srv := httptest.NewServer(s)
	t.Cleanup(func() {
		cancel()
		<-hub.Done()
		srv.Close()
	})
	return &env{srv: srv, stopHub: cancel, hub: hub, s: s}
}

func (e *env) wsURL(query string) string {
	return "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws?" + query
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("连接 %s 失败: %v (resp=%v)", url, err, resp)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// readUntil 读取消息直到出现指定类型（与可选文本），超时失败。
func readUntil(t *testing.T, conn *websocket.Conn, typ, text string) chat.Message {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		var m chat.Message
		if err := conn.ReadJSON(&m); err != nil {
			t.Fatalf("等待 %s/%q 时读取失败: %v", typ, text, err)
		}
		if m.Type == typ && (text == "" || m.Text == text) {
			return m
		}
	}
}

func TestChatOverWebSocket(t *testing.T) {
	e := newEnv(t, DefaultHeartbeat)
	alice := dial(t, e.wsURL("room=go&nick=alice"))
	readUntil(t, alice, chat.TypeSystem, "alice 进入了房间")
	bob := dial(t, e.wsURL("room=go&nick=bob"))
	readUntil(t, alice, chat.TypeSystem, "bob 进入了房间")

	if err := alice.WriteJSON(chat.Inbound{Text: "hello"}); err != nil {
		t.Fatal(err)
	}
	if m := readUntil(t, bob, chat.TypeChat, ""); m.Nick != "alice" || m.Text != "hello" {
		t.Errorf("bob 收到 %+v", m)
	}
	// 纯文本帧也兼容
	bob.WriteMessage(websocket.TextMessage, []byte("plain text"))
	readUntil(t, alice, chat.TypeChat, "plain text")

	resp, err := http.Get(e.srv.URL + "/api/rooms")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var rooms []chat.RoomInfo
	json.NewDecoder(resp.Body).Decode(&rooms)
	if len(rooms) != 1 || rooms[0].Users != 2 {
		t.Errorf("/api/rooms = %+v", rooms)
	}

	bob.Close()
	readUntil(t, alice, chat.TypeSystem, "bob 离开了房间")
}

func TestRejects(t *testing.T) {
	e := newEnv(t, DefaultHeartbeat)

	// 参数非法：握手前直接返回 400
	_, resp, err := websocket.DefaultDialer.Dial(e.wsURL("room=go&nick="), nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
		t.Errorf("空昵称应返回 400，实际 err=%v resp=%v", err, resp)
	}

	// 跨域：Origin 与 Host 不一致且不在白名单
	h := http.Header{"Origin": []string{"https://evil.example"}}
	_, resp, err = websocket.DefaultDialer.Dial(e.wsURL("room=go&nick=x"), h)
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("跨域连接应被拒绝(403)，实际 err=%v resp=%v", err, resp)
	}

	// 昵称重复：握手成功，随后收到错误消息与 1008 关闭帧
	dial(t, e.wsURL("room=go&nick=alice"))
	dup := dial(t, e.wsURL("room=go&nick=alice"))
	readUntil(t, dup, chat.TypeError, chat.ErrNickTaken.Error())
	_, _, err = dup.ReadMessage()
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Errorf("期望 1008 关闭，实际 %v", err)
	}
}

func TestHeartbeatPing(t *testing.T) {
	hb := Heartbeat{WriteWait: time.Second, PongWait: 300 * time.Millisecond, PingPeriod: 50 * time.Millisecond}
	e := newEnv(t, hb)
	conn := dial(t, e.wsURL("room=go&nick=alice"))

	pings := make(chan struct{}, 10)
	conn.SetPingHandler(func(data string) error {
		pings <- struct{}{}
		// 默认的 PingHandler 会自动回复 Pong；自定义后需要手动回复
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(time.Second))
	})
	// 控制帧在 Read 过程中被处理，所以必须有 goroutine 持续读
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	for i := 0; i < 3; i++ {
		select {
		case <-pings:
		case <-time.After(time.Second):
			t.Fatal("没有收到服务端 Ping")
		}
	}
	// 持续回复 Pong 的连接，在超过 PongWait 之后仍然在线
	time.Sleep(400 * time.Millisecond)
	if rooms := e.hub.Rooms(); len(rooms) != 1 || rooms[0].Users != 1 {
		t.Errorf("回复 Pong 的连接不应被断开，rooms=%+v", rooms)
	}
}

func TestDeadClientTimesOut(t *testing.T) {
	hb := Heartbeat{WriteWait: time.Second, PongWait: 200 * time.Millisecond, PingPeriod: 50 * time.Millisecond}
	e := newEnv(t, hb)
	// 建立连接后完全不读：客户端库不会处理 Ping，也就不会回 Pong，模拟"假死"客户端
	dial(t, e.wsURL("room=go&nick=zombie"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(e.hub.Rooms()) == 0 {
			return // 已被心跳超时清理
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("不回 Pong 的连接应在 PongWait 后被断开")
}

func TestGracefulShutdown(t *testing.T) {
	e := newEnv(t, DefaultHeartbeat)
	conn := dial(t, e.wsURL("room=go&nick=alice"))
	readUntil(t, conn, chat.TypeUsers, "")

	e.stopHub()
	<-e.hub.Done()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
				t.Errorf("期望收到 1001 Going Away，实际 %v", err)
			}
			break
		}
	}
	if !e.s.WaitTimeout(2 * time.Second) {
		t.Error("关闭后连接 goroutine 应全部退出")
	}
}

func TestStaticAndHealth(t *testing.T) {
	e := newEnv(t, DefaultHeartbeat)
	for path, want := range map[string]string{"/": "<h1>chat</h1>", "/healthz": "ok"} {
		resp, err := http.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), want) {
			t.Errorf("GET %s = %q，期望包含 %q", path, b, want)
		}
	}
}
