// Package chat 实现聊天室的核心逻辑：Hub、房间、昵称、广播。
//
// 本包完全不依赖 WebSocket：客户端被抽象为一个"发送队列"（send channel），
// 网络读写由 server 包负责。这样 Hub 可以脱离网络做纯单元测试。
package chat

import "time"

// 消息类型。
const (
	TypeChat   = "chat"   // 用户聊天消息
	TypeSystem = "system" // 系统通知：进入/离开/改名
	TypeUsers  = "users"  // 当前房间在线用户列表
	TypeError  = "error"  // 发给单个客户端的错误提示
)

// Message 是服务端发给客户端的消息（序列化为 JSON）。
type Message struct {
	Type  string    `json:"type"`
	Room  string    `json:"room,omitempty"`
	Nick  string    `json:"nick,omitempty"`
	Text  string    `json:"text,omitempty"`
	Users []string  `json:"users,omitempty"`
	Time  time.Time `json:"time"`
}

// Inbound 是客户端发来的消息。目前只有一种：{"text": "..."}。
// 以 "/" 开头的文本被视为命令：/nick 新昵称、/users。
type Inbound struct {
	Text string `json:"text"`
}

// RoomInfo 是房间概况，供 /api/rooms 使用。
type RoomInfo struct {
	Name  string `json:"name"`
	Users int    `json:"users"`
}
