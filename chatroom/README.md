# chatroom：WebSocket 聊天室

基于 [gorilla/websocket](https://github.com/gorilla/websocket) 的多房间聊天室，前端页面通过 `embed` 编译进二进制，一个可执行文件即可部署。

## 功能

- **Hub 广播**：单 goroutine 事件循环管理所有房间与成员，无锁、无数据竞争
- **多房间**：`/ws?room=go&nick=alice`，空房间自动销毁
- **昵称**：同房间内唯一（大小写不敏感），`/nick 新昵称` 改名，`/users` 查看在线
- **历史消息**：新成员进入时回放最近 N 条
- **心跳**：服务端定时 Ping，超过 `PongWait` 未响应即断开"假死"连接
- **慢客户端保护**：发送队列满时断开该客户端，不拖慢整个房间
- **安全**：Origin 校验防跨站 WebSocket 劫持、消息大小限制、前端只用 `textContent` 防 XSS
- **优雅关闭**：停止接收新连接 → 停止 Hub → 向所有客户端发送 Close 帧 → 等待连接 goroutine 退出
- 前端页面 `embed` 内嵌；`/api/rooms` 查看房间列表；`/healthz` 健康检查

## 目录结构

```text
chatroom/
├── cmd/chatroom/main.go        # 入口：配置、启动顺序、优雅关闭顺序
└── internal/
    ├── chat/                   # 核心逻辑（不依赖网络，可纯单元测试）
    │   ├── message.go          # 消息协议
    │   ├── client.go           # 客户端抽象（send 队列）与校验
    │   └── hub.go              # Hub 事件循环：加入/离开/广播/命令
    ├── server/                 # 传输层
    │   ├── server.go           # 路由、Upgrade、Origin 校验
    │   └── conn.go             # readPump / writePump / 心跳
    ├── config/                 # flag + 环境变量
    └── web/                    # embed 静态页面
        └── static/index.html
```

## 运行

```bash
# 国内网络：配置模块代理
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn

cd chatroom
go test -race ./...
go run ./cmd/chatroom            # 打开 http://localhost:8080 ，多开几个浏览器标签页体验
```

> 依赖下载说明：唯一的第三方依赖是 `github.com/gorilla/websocket`，通过 goproxy.cn 下载即可。

### 配置

| 参数 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| `-addr` | `CHAT_ADDR` | `:8080` | 监听地址 |
| `-origins` | `CHAT_ALLOWED_ORIGINS` | 空 | 额外允许的 Origin，逗号分隔；默认只允许同源 |
| `-history` | `CHAT_HISTORY` | 50 | 每房间历史条数 |
| `-send-buffer` | `CHAT_SEND_BUFFER` | 64 | 每客户端发送队列长度 |
| `-shutdown-timeout` | - | 5s | 优雅关闭超时 |
| `-debug` | - | false | 调试日志 |

## 协议示例

客户端 → 服务端（文本帧，JSON 或纯文本均可）：

```json
{"text": "大家好"}
{"text": "/nick 小明"}
{"text": "/users"}
```

服务端 → 客户端：

```json
{"type":"system","room":"go","text":"alice 进入了房间","time":"2026-10-27T15:25:26+08:00"}
{"type":"users","room":"go","users":["alice","bob"],"time":"..."}
{"type":"chat","room":"go","nick":"bob","text":"hi alice","time":"..."}
{"type":"error","text":"昵称已被占用","time":"..."}
```

命令行调试（需安装 [websocat](https://github.com/vi/websocat)）：

```bash
websocat "ws://localhost:8080/ws?room=go&nick=cli"
curl -s localhost:8080/api/rooms     # [{"name":"go","users":2}]
```

## 部署提示

- 放在 Nginx 后面时需要转发升级头：

```nginx
location /ws {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 120s;   # 大于心跳间隔
}
```

- 不要给 `http.Server` 设置 `ReadTimeout/WriteTimeout`，它们会把长连接强行断开；WebSocket 超时由心跳控制。
- 单机 Hub 无法横向扩展；多实例部署需要用 Redis Pub/Sub 或 NATS 在实例间转发消息。

## 扩展练习

1. 私聊：`/msg bob 你好`。
2. 每个客户端限流（如每秒 5 条），防刷屏。
3. 把历史消息持久化到 SQLite，重启不丢。
4. 多实例：用 Redis Pub/Sub 广播，让不同实例上的用户互通。
5. 换成 `github.com/coder/websocket`（原 nhooyr.io/websocket），对比 API 设计（context 友好、并发写安全）。
