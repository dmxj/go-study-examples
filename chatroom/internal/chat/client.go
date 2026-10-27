package chat

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Client 代表一个已连接的用户。
//
// 并发约定（非常重要）：
//   - send 只由 Hub 关闭，且只由 Hub 写入；连接层只从中读取。
//   - nick / room 只在 Hub 的 goroutine 中读写（Join 之前由创建者设置）。
//
// 遵守"一个数据只有一个 owner goroutine"的约定，就不需要任何互斥锁。
type Client struct {
	nick string
	room string
	send chan []byte
}

// NewClient 创建客户端。buf 是发送队列容量：消费跟不上时 Hub 会断开该客户端，
// 而不是让一个慢客户端拖慢整个房间。
func NewClient(nick, room string, buf int) *Client {
	return &Client{nick: nick, room: room, send: make(chan []byte, buf)}
}

// Send 返回只读的发送队列，连接层的 writePump 从这里取消息写到 WebSocket。
// channel 被关闭表示服务端要求断开连接。
func (c *Client) Send() <-chan []byte { return c.send }

// 校验错误。
var (
	ErrNickTaken   = errors.New("昵称已被占用")
	ErrInvalidNick = errors.New("昵称需为 1~20 个字符，不能包含空白和 / 符号")
	ErrInvalidRoom = errors.New("房间名只能包含字母、数字、下划线和短横线，长度 1~32")
	ErrHubClosed   = errors.New("聊天服务已关闭")
)

var roomRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

// ValidateNick 校验昵称。
func ValidateNick(nick string) error {
	n := utf8.RuneCountInString(nick)
	if n == 0 || n > 20 || strings.ContainsAny(nick, " \t\r\n/") {
		return ErrInvalidNick
	}
	return nil
}

// ValidateRoom 校验房间名。
func ValidateRoom(room string) error {
	if !roomRe.MatchString(room) {
		return ErrInvalidRoom
	}
	return nil
}

// MaxTextLen 是单条消息的最大字符数。
const MaxTextLen = 1000

func validateText(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("消息不能为空")
	}
	if utf8.RuneCountInString(text) > MaxTextLen {
		return "", fmt.Errorf("消息不能超过 %d 个字符", MaxTextLen)
	}
	return text, nil
}
