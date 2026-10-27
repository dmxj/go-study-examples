# shortener：短链接服务

只用标准库 `net/http`（Go 1.22+ 增强路由）+ 纯 Go SQLite 驱动 [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) 实现的短链接服务，涵盖 base62 编码、LRU 缓存、异步访问统计、令牌桶限流与优雅关闭。

## 功能

- `POST /api/links` 创建短链：自增 ID → base62 短码；支持自定义短码与有效期（TTL）
- `GET /{code}` 302 跳转；过期返回 410 Gone
- `GET /api/links/{code}` 查看访问统计（点击数、最后访问时间）
- **LRU 缓存**：泛型 + `container/list` 手写，热点短码不查库
- **异步统计**：内存聚合点击数，定时批量写库；优雅关闭时最后一次落库，不丢计数
- **限流**：手写按 IP 的令牌桶（惰性补充 + 闲置清理），超限返回 429 与 `Retry-After`
- **安全**：URL 协议校验、拒绝自引用、请求体大小限制、`DisallowUnknownFields`、可信代理才读取 `X-Forwarded-For`
- slog JSON 日志、panic 恢复、`/healthz`、`http.Server` 超时、`SIGINT/SIGTERM` 优雅关闭

## 目录结构

```text
shortener/
├── cmd/shortener/main.go     # 入口：组装依赖、关闭顺序（HTTP → 统计落库 → 关库）
└── internal/
    ├── config/               # flag + SHORT_* 环境变量
    ├── base62/               # 编码/解码（含模糊测试与基准测试）
    ├── cache/                # 并发安全的泛型 LRU
    ├── ratelimit/            # 按 key 的令牌桶
    ├── store/                # SQLite 持久化（database/sql）
    ├── stats/                # 访问统计聚合与批量写入
    ├── service/              # 业务：校验、创建、解析（缓存）、统计
    └── httpapi/              # 路由、处理函数、中间件（日志/恢复/限流）
```

## 运行

```bash
# 国内网络：配置模块代理
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn

cd shortener
go test -race ./...
go run ./cmd/shortener -addr :8080 -base-url http://localhost:8080
```

> 依赖下载说明：`modernc.org/sqlite` 是把 SQLite C 源码自动翻译成 Go 的纯 Go 实现，依赖较多（modernc.org/libc 等，约 30MB），首次下载请确保 GOPROXY 指向 goproxy.cn 或 mirrors.aliyun.com/goproxy。它不需要 cgo 和 gcc。

### 配置

| 参数 | 环境变量 | 默认 | 说明 |
|---|---|---|---|
| `-addr` | `SHORT_ADDR` | `:8080` | 监听地址 |
| `-db` | `SHORT_DB` | `shortener.db` | SQLite 文件 |
| `-base-url` | `SHORT_BASE_URL` | `http://localhost:8080` | 对外短链前缀 |
| `-cache-size` | `SHORT_CACHE_SIZE` | 10000 | LRU 条目数 |
| `-rate` / `-burst` | `SHORT_RATE` / `SHORT_BURST` | 1 / 5 | 每 IP 创建限流 |
| `-flush-interval` | - | 5s | 统计写库间隔 |
| `-trust-proxy` | `SHORT_TRUST_PROXY=true` | false | 信任 X-Forwarded-For |

## API 示例

```bash
$ curl -s -XPOST localhost:8080/api/links -d '{"url":"https://go.dev/doc/effective_go"}'
{"code":"1","short_url":"http://localhost:8080/1","url":"https://go.dev/doc/effective_go","clicks":0,"created_at":"2026-10-27T15:32:44.870811+08:00"}

# 自定义短码 + 30 天有效期
$ curl -s -XPOST localhost:8080/api/links -d '{"url":"https://golang.google.cn","code":"go-cn","ttl":"720h"}'
{"code":"go-cn","short_url":"http://localhost:8080/go-cn",...,"expires_at":"2026-11-26T15:32:44.890162+08:00"}

$ curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" localhost:8080/1
302 https://go.dev/doc/effective_go

$ curl -s localhost:8080/api/links/1
{"code":"1",...,"clicks":1,"last_access_at":"2026-10-27T15:32:44.907995+08:00"}

# 连续创建超过突发上限
$ for i in 1 2 3 4 5 6 7; do curl -s -o /dev/null -w "%{http_code} " -XPOST localhost:8080/api/links -d '{"url":"https://x.test"}'; done
201 201 201 201 201 429 429
```

| 状态码 | 含义 |
|---|---|
| 201 | 创建成功（`Location` 头为短链） |
| 302 | 跳转（不用 301，避免浏览器永久缓存导致统计失真） |
| 400 | 参数错误 |
| 404 / 410 | 短码不存在 / 已过期 |
| 409 | 自定义短码已被占用 |
| 429 | 触发限流 |

## 扩展练习

1. 短码不可预测：自增 ID 编码后是连续的，可以用 Feistel 置换或乘法逆元打乱。
2. 增加管理接口（删除、禁用），并加上 API Key 鉴权。
3. 把 LRU 换成带 TTL 的缓存，或接入 Redis 做多实例共享缓存。
4. 用 `singleflight` 合并缓存未命中时对同一短码的并发查库。
5. 编写 Dockerfile（参考 rest-api），`CGO_ENABLED=0` 构建 scratch 镜像。
