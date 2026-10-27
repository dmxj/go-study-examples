package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dmxj/go-study-examples/chatroom/internal/chat"
)

// Heartbeat 是心跳参数。
//
// 心跳的意义：TCP 连接"静默断开"（拔网线、手机切网络、NAT 超时）时双方都收不到 FIN，
// 不主动探测的话服务端会一直保留这个死连接。做法是服务端定期发 Ping，
// 客户端（浏览器自动）回 Pong；超过 PongWait 没收到任何数据就判定连接已死。
type Heartbeat struct {
	WriteWait  time.Duration // 单次写操作的超时
	PongWait   time.Duration // 读超时：在此期间必须收到消息或 Pong
	PingPeriod time.Duration // 发送 Ping 的间隔，必须小于 PongWait
}

// DefaultHeartbeat 是默认心跳参数。
var DefaultHeartbeat = Heartbeat{
	WriteWait:  10 * time.Second,
	PongWait:   60 * time.Second,
	PingPeriod: 54 * time.Second, // PongWait 的 90%
}

// maxMessageSize 限制客户端单条消息大小，防止恶意的超大帧耗尽内存。
const maxMessageSize = 4 << 10

// readPump 从 WebSocket 读消息交给 Hub。每个连接一个 readPump goroutine。
//
// gorilla/websocket 的并发规则：同一时刻最多一个 goroutine 读、一个 goroutine 写。
// 所以读写拆成 readPump / writePump 两个 goroutine，各自独占一个方向。
func readPump(conn *websocket.Conn, c *chat.Client, hub *chat.Hub, hb Heartbeat, log *slog.Logger) {
	defer hub.Leave(c) // 读失败（对方关闭、超时）→ 离开房间 → Hub 关闭 send → writePump 退出并关闭连接

	conn.SetReadLimit(maxMessageSize)
	_ = conn.SetReadDeadline(time.Now().Add(hb.PongWait))
	conn.SetPongHandler(func(string) error {
		// 收到 Pong 说明连接还活着，延长读超时
		return conn.SetReadDeadline(time.Now().Add(hb.PongWait))
	})
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				log.Debug("连接异常断开", "err", err)
			}
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(hb.PongWait)) // 收到普通消息同样说明连接活着
		var in chat.Inbound
		if err := json.Unmarshal(data, &in); err != nil {
			// 兼容直接发送纯文本的客户端（如 websocat 调试）
			in.Text = string(data)
		}
		hub.Receive(c, in)
	}
}

// writePump 把 Hub 投递的消息写到 WebSocket，并定时发送 Ping。
func writePump(conn *websocket.Conn, c *chat.Client, hb Heartbeat) {
	ticker := time.NewTicker(hb.PingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close() // 关闭连接会让 readPump 的 ReadMessage 立即返回错误
	}()
	for {
		select {
		case msg, ok := <-c.Send():
			_ = conn.SetWriteDeadline(time.Now().Add(hb.WriteWait))
			if !ok {
				// Hub 关闭了 send：可能是服务器关闭或客户端被踢出。礼貌地发送 Close 帧。
				_ = conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "bye"))
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(hb.WriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// rejectConn 在握手成功后、加入房间失败时，发送错误消息并以 1008（策略违规）关闭连接。
// 浏览器的 WebSocket API 读不到握手阶段的 HTTP 状态码，所以错误要在连接建立后通过消息告知。
func rejectConn(conn *websocket.Conn, err error, wait time.Duration) {
	defer conn.Close()
	msg, _ := json.Marshal(chat.Message{Type: chat.TypeError, Text: err.Error(), Time: time.Now()})
	_ = conn.SetWriteDeadline(time.Now().Add(wait))
	_ = conn.WriteMessage(websocket.TextMessage, msg)
	code := websocket.ClosePolicyViolation
	if errors.Is(err, chat.ErrHubClosed) {
		code = websocket.CloseGoingAway
	}
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, ""))
}
