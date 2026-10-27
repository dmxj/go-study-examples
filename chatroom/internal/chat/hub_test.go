package chat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"
)

func newTestHub(t *testing.T, history int) *Hub {
	t.Helper()
	h := NewHub(history, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	t.Cleanup(func() { cancel(); <-h.Done() })
	return h
}

// next 从客户端发送队列读取下一条消息，超时则测试失败。
func next(t *testing.T, c *Client) Message {
	t.Helper()
	select {
	case b, ok := <-c.Send():
		if !ok {
			t.Fatal("send 已被关闭")
		}
		var m Message
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	case <-time.After(time.Second):
		t.Fatal("等待消息超时")
	}
	return Message{}
}

// nextOf 跳过其他类型，读取下一条指定类型的消息。
func nextOf(t *testing.T, c *Client, typ string) Message {
	t.Helper()
	for {
		if m := next(t, c); m.Type == typ {
			return m
		}
	}
}

// waitFor 持续读取消息直到 match 返回 true（中间的其他消息被忽略）。
func waitFor(t *testing.T, c *Client, match func(Message) bool) {
	t.Helper()
	for {
		if match(next(t, c)) {
			return
		}
	}
}

func mustJoin(t *testing.T, h *Hub, nick, room string) *Client {
	t.Helper()
	c := NewClient(nick, room, 32)
	if err := h.Join(c); err != nil {
		t.Fatalf("Join(%s) 失败: %v", nick, err)
	}
	return c
}

func TestBroadcastAndRooms(t *testing.T) {
	h := newTestHub(t, 10)
	alice := mustJoin(t, h, "alice", "go")
	bob := mustJoin(t, h, "bob", "go")
	carol := mustJoin(t, h, "carol", "python") // 不同房间

	if m := nextOf(t, alice, TypeSystem); m.Text != "alice 进入了房间" {
		t.Errorf("alice 收到 %q", m.Text)
	}
	if m := nextOf(t, alice, TypeUsers); !slices.Equal(m.Users, []string{"alice"}) {
		t.Errorf("在线列表 %v", m.Users)
	}
	if m := nextOf(t, alice, TypeUsers); !slices.Equal(m.Users, []string{"alice", "bob"}) {
		t.Errorf("bob 加入后 alice 看到的在线列表 %v，期望 [alice bob]", m.Users)
	}

	h.Receive(bob, Inbound{Text: "  你好 Go  "})
	for _, c := range []*Client{alice, bob} {
		m := nextOf(t, c, TypeChat)
		if m.Nick != "bob" || m.Text != "你好 Go" || m.Room != "go" {
			t.Errorf("聊天消息 %+v 不正确", m)
		}
	}
	// carol 在另一个房间，只会收到自己房间的系统消息与用户列表
	nextOf(t, carol, TypeUsers)
	select {
	case b := <-carol.Send():
		t.Errorf("其他房间不应收到消息: %s", b)
	case <-time.After(50 * time.Millisecond):
	}

	rooms := h.Rooms()
	if len(rooms) != 2 || rooms[0] != (RoomInfo{Name: "go", Users: 2}) {
		t.Errorf("Rooms() = %+v", rooms)
	}
}

func TestNickRules(t *testing.T) {
	h := newTestHub(t, 0)
	alice := mustJoin(t, h, "alice", "r")
	if err := h.Join(NewClient("ALICE", "r", 8)); !errors.Is(err, ErrNickTaken) {
		t.Errorf("同房间重复昵称（大小写不敏感）应返回 ErrNickTaken，实际 %v", err)
	}
	if err := h.Join(NewClient("alice", "other", 8)); err != nil {
		t.Errorf("不同房间可以使用相同昵称，实际 %v", err)
	}
	bob := mustJoin(t, h, "bob", "r")

	h.Receive(bob, Inbound{Text: "/nick alice"})
	if m := nextOf(t, bob, TypeError); m.Text != ErrNickTaken.Error() {
		t.Errorf("改名为已存在昵称应报错，实际 %q", m.Text)
	}
	h.Receive(bob, Inbound{Text: "/nick 鲍勃"})
	waitFor(t, alice, func(m Message) bool { return m.Type == TypeSystem && m.Text == "bob 改名为 鲍勃" })

	h.Receive(alice, Inbound{Text: "/users"})
	waitFor(t, alice, func(m Message) bool {
		return m.Type == TypeUsers && slices.Equal(m.Users, []string{"alice", "鲍勃"})
	})
}

func TestHistoryAndLeave(t *testing.T) {
	h := newTestHub(t, 2)
	a := mustJoin(t, h, "a", "r")
	for _, s := range []string{"1", "2", "3"} {
		h.Receive(a, Inbound{Text: s})
	}
	// 用 Rooms() 做一次同步：它返回时，之前投递给 Hub 的消息都已处理完
	h.Rooms()

	b := mustJoin(t, h, "b", "r")
	// 新成员先收到最近 2 条历史
	if m := next(t, b); m.Text != "2" {
		t.Errorf("第一条历史 %q，期望 2", m.Text)
	}
	if m := next(t, b); m.Text != "3" {
		t.Errorf("第二条历史 %q，期望 3", m.Text)
	}

	h.Leave(a)
	if m := nextOf(t, b, TypeSystem); m.Text != "b 进入了房间" {
		t.Errorf("系统消息 %q", m.Text)
	}
	if m := nextOf(t, b, TypeSystem); m.Text != "a 离开了房间" {
		t.Errorf("离开通知 %q", m.Text)
	}
	h.Leave(a) // 重复 Leave 不应 panic（send 不会被重复关闭）
}

func TestSlowClientIsDropped(t *testing.T) {
	h := newTestHub(t, 0)
	slow := NewClient("slow", "r", 1) // 队列只有 1 个位置，且从不读取
	if err := h.Join(slow); err != nil {
		t.Fatal(err)
	}
	fast := mustJoin(t, h, "fast", "r")
	for i := 0; i < 5; i++ {
		h.Receive(fast, Inbound{Text: "spam"})
	}
	h.Rooms()
	// slow 的 send 应已被关闭：读完缓冲后得到 ok=false
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-slow.Send():
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("慢客户端应被断开")
		}
	}
}

func TestShutdownClosesClients(t *testing.T) {
	h := NewHub(0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	c := mustJoin(t, h, "a", "r")
	cancel()
	<-h.Done()
	for range c.Send() { // 排空缓冲；range 能结束说明 send 已关闭
	}
	if err := h.Join(NewClient("b", "r", 1)); !errors.Is(err, ErrHubClosed) {
		t.Errorf("Hub 停止后 Join 应返回 ErrHubClosed，实际 %v", err)
	}
	h.Leave(c) // 不应阻塞
}

func TestValidate(t *testing.T) {
	for _, n := range []string{"", "a b", "a/b", "这是一个超过二十个字符的非常非常长的昵称啊啊"} {
		if ValidateNick(n) == nil {
			t.Errorf("昵称 %q 应校验失败", n)
		}
	}
	for _, r := range []string{"", "has space", "中文", "a.b"} {
		if ValidateRoom(r) == nil {
			t.Errorf("房间名 %q 应校验失败", r)
		}
	}
}
