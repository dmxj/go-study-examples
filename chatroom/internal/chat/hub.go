package chat

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// room 是一个聊天房间。只在 Hub goroutine 中访问。
type room struct {
	name    string
	clients map[*Client]struct{}
	history [][]byte // 最近的聊天记录（已编码的 JSON），新成员加入时回放
}

type joinReq struct {
	c     *Client
	reply chan error // 请求-响应模式：调用方通过 reply 拿到处理结果
}

type inboundMsg struct {
	c   *Client
	msg Inbound
}

// Hub 是聊天室的中枢。所有状态（房间、成员、历史）都只由 Run 所在的 goroutine 修改，
// 外部通过 channel 发送"请求"——这就是 Go 的 actor/CSP 风格，天然没有数据竞争。
type Hub struct {
	join    chan joinReq
	leave   chan *Client
	inbound chan inboundMsg
	rooms   chan chan []RoomInfo
	done    chan struct{}

	state       map[string]*room
	historySize int
	log         *slog.Logger
	now         func() time.Time
}

// NewHub 创建 Hub。historySize 为每个房间保留的历史消息条数。
func NewHub(historySize int, log *slog.Logger) *Hub {
	return &Hub{
		join:  make(chan joinReq),
		leave: make(chan *Client),
		// 全部使用无缓冲 channel：Receive 返回即表示 Hub 已取走消息，且 Hub 会在处理完它之后
		// 才进入下一轮 select。这保证了"同一调用方先后发出的请求按顺序生效"。
		// 若改成带缓冲，select 在多个就绪分支间随机选择，后发的 Rooms() 可能先于缓冲中的消息被处理。
		inbound:     make(chan inboundMsg),
		rooms:       make(chan chan []RoomInfo),
		done:        make(chan struct{}),
		state:       make(map[string]*room),
		historySize: historySize,
		log:         log,
		now:         time.Now,
	}
}

// Run 运行事件循环，直到 ctx 取消。退出时关闭所有客户端的 send，通知连接层断开。
func (h *Hub) Run(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case req := <-h.join:
			req.reply <- h.handleJoin(req.c)
		case c := <-h.leave:
			h.handleLeave(c)
		case in := <-h.inbound:
			h.handleInbound(in.c, in.msg)
		case reply := <-h.rooms:
			reply <- h.roomInfos()
		case <-ctx.Done():
			for _, r := range h.state {
				for c := range r.clients {
					close(c.send)
				}
			}
			h.state = nil
			h.log.Info("hub 已停止")
			return
		}
	}
}

// Join 把客户端加入其房间。昵称重复时返回 ErrNickTaken。
func (h *Hub) Join(c *Client) error {
	req := joinReq{c: c, reply: make(chan error, 1)}
	select {
	case h.join <- req:
		return <-req.reply
	case <-h.done:
		return ErrHubClosed
	}
}

// Leave 让客户端离开。重复调用是安全的。
func (h *Hub) Leave(c *Client) {
	select {
	case h.leave <- c:
	case <-h.done: // Hub 已退出时不要阻塞，否则调用方 goroutine 会泄漏
	}
}

// Receive 投递客户端发来的消息。
func (h *Hub) Receive(c *Client, msg Inbound) {
	select {
	case h.inbound <- inboundMsg{c: c, msg: msg}:
	case <-h.done:
	}
}

// Rooms 返回当前所有房间及人数。
func (h *Hub) Rooms() []RoomInfo {
	reply := make(chan []RoomInfo, 1)
	select {
	case h.rooms <- reply:
		return <-reply
	case <-h.done:
		return nil
	}
}

// Done 在 Hub 退出后关闭。
func (h *Hub) Done() <-chan struct{} { return h.done }

// ---------- 以下方法只在 Run 的 goroutine 中调用 ----------

func (h *Hub) handleJoin(c *Client) error {
	r := h.state[c.room]
	if r == nil {
		r = &room{name: c.room, clients: make(map[*Client]struct{})}
		h.state[c.room] = r
	}
	if h.nickTaken(r, c.nick) {
		if len(r.clients) == 0 {
			delete(h.state, c.room)
		}
		return ErrNickTaken
	}
	r.clients[c] = struct{}{}
	// 先给新成员回放历史，再广播"xx 加入"，顺序对用户体验很重要
	for _, m := range r.history {
		h.deliver(r, c, m)
	}
	h.broadcast(r, h.system(r.name, c.nick+" 进入了房间"), false)
	h.broadcastUsers(r)
	h.log.Info("用户加入", "room", r.name, "nick", c.nick, "online", len(r.clients))
	return nil
}

func (h *Hub) handleLeave(c *Client) {
	r := h.state[c.room]
	if r == nil {
		return
	}
	if _, ok := r.clients[c]; !ok {
		return // 已被移除（例如因发送队列堵塞被踢出），忽略
	}
	h.remove(r, c)
	h.broadcast(r, h.system(r.name, c.nick+" 离开了房间"), false)
	h.broadcastUsers(r)
	h.log.Info("用户离开", "room", r.name, "nick", c.nick, "online", len(r.clients))
}

func (h *Hub) handleInbound(c *Client, in Inbound) {
	r := h.state[c.room]
	if r == nil {
		return
	}
	if _, ok := r.clients[c]; !ok {
		return
	}
	text := strings.TrimSpace(in.Text)
	if cmd, arg, isCmd := parseCommand(text); isCmd {
		h.handleCommand(r, c, cmd, arg)
		return
	}
	text, err := validateText(text)
	if err != nil {
		h.deliver(r, c, h.encode(Message{Type: TypeError, Text: err.Error(), Time: h.now()}))
		return
	}
	h.broadcast(r, h.encode(Message{Type: TypeChat, Room: r.name, Nick: c.nick, Text: text, Time: h.now()}), true)
}

func (h *Hub) handleCommand(r *room, c *Client, cmd, arg string) {
	switch cmd {
	case "nick":
		if err := ValidateNick(arg); err != nil {
			h.deliver(r, c, h.errorMsg(err.Error()))
			return
		}
		if h.nickTaken(r, arg) {
			h.deliver(r, c, h.errorMsg(ErrNickTaken.Error()))
			return
		}
		old := c.nick
		c.nick = arg
		h.broadcast(r, h.system(r.name, old+" 改名为 "+arg), false)
		h.broadcastUsers(r)
	case "users":
		h.deliver(r, c, h.usersMsg(r))
	default:
		h.deliver(r, c, h.errorMsg("未知命令 /"+cmd+"（支持 /nick 新昵称、/users）"))
	}
}

// broadcast 向房间内所有人发送同一条消息。消息只编码一次，再分发给 N 个客户端。
func (h *Hub) broadcast(r *room, msg []byte, keepHistory bool) {
	if keepHistory && h.historySize > 0 {
		r.history = append(r.history, msg)
		if len(r.history) > h.historySize {
			r.history = slices.Delete(r.history, 0, len(r.history)-h.historySize)
		}
	}
	for c := range r.clients {
		h.deliver(r, c, msg)
	}
}

// deliver 非阻塞地把消息放入客户端的发送队列。
//
// 队列满说明该客户端消费太慢（网络差或已失联），直接断开它。
// 如果这里用阻塞发送，一个慢客户端就会卡住 Hub，进而卡住整个聊天室。
func (h *Hub) deliver(r *room, c *Client, msg []byte) {
	select {
	case c.send <- msg:
	default:
		h.log.Warn("客户端发送队列已满，断开连接", "room", r.name, "nick", c.nick)
		h.remove(r, c)
	}
}

func (h *Hub) remove(r *room, c *Client) {
	delete(r.clients, c)
	close(c.send)
	if len(r.clients) == 0 {
		delete(h.state, r.name) // 空房间随即销毁，历史也一并丢弃
	}
}

func (h *Hub) nickTaken(r *room, nick string) bool {
	for c := range r.clients {
		if strings.EqualFold(c.nick, nick) {
			return true
		}
	}
	return false
}

func (h *Hub) broadcastUsers(r *room) {
	if len(r.clients) == 0 {
		return
	}
	h.broadcast(r, h.usersMsg(r), false)
}

func (h *Hub) usersMsg(r *room) []byte {
	users := make([]string, 0, len(r.clients))
	for c := range r.clients {
		users = append(users, c.nick)
	}
	slices.Sort(users) // map 遍历顺序随机，排序后输出稳定
	return h.encode(Message{Type: TypeUsers, Room: r.name, Users: users, Time: h.now()})
}

func (h *Hub) roomInfos() []RoomInfo {
	out := make([]RoomInfo, 0, len(h.state))
	for name, r := range h.state {
		out = append(out, RoomInfo{Name: name, Users: len(r.clients)})
	}
	slices.SortFunc(out, func(a, b RoomInfo) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

func (h *Hub) system(room, text string) []byte {
	return h.encode(Message{Type: TypeSystem, Room: room, Text: text, Time: h.now()})
}

func (h *Hub) errorMsg(text string) []byte {
	return h.encode(Message{Type: TypeError, Text: text, Time: h.now()})
}

func (h *Hub) encode(m Message) []byte {
	b, err := json.Marshal(m)
	if err != nil { // Message 只含基础类型，理论上不会失败
		h.log.Error("编码消息失败", "err", err)
	}
	return b
}

// parseCommand 解析 "/nick bob" 形式的命令。
func parseCommand(text string) (cmd, arg string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	cmd, arg, _ = strings.Cut(text[1:], " ")
	return strings.ToLower(cmd), strings.TrimSpace(arg), true
}
