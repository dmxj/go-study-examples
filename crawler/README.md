# crawler：并发网页爬虫

一个"小而全"的并发爬虫：worker pool、全局限速、URL 去重、深度控制、遵守 robots.txt、context 取消、流式输出 JSON。除 `golang.org/x/net/html`（HTML 解析）和 `golang.org/x/time/rate`（令牌桶限速）外只用标准库。

## 功能

- **worker pool**：1 个调度者 goroutine + N 个 worker，通过 channel 通信
- **去重**：URL 规范化（去 fragment、小写 host、去默认端口）后放入 `seen` 集合，每个页面只抓一次
- **深度控制**：`-depth`；**页数上限**：`-max-pages`
- **限速**：`golang.org/x/time/rate` 令牌桶，`-rate` 每秒请求数、`-burst` 突发容量
- **robots.txt**：按主机缓存、每主机只下载一次（`sync.Once`），支持 `Allow`/`Disallow`/`*`/`$`/最长匹配
- **范围控制**：默认只抓与种子同主机的链接（`-same-host=false` 关闭）
- **取消**：`Ctrl+C` 或 `-deadline` 超时后，在途请求立即中止，已抓到的结果完整输出
- **输出**：JSON 数组（`-format json`）或 JSON Lines（`-format jsonl`），流式写出，不占用大量内存
- **安全**：请求超时、响应体大小限制、最多 5 次重定向、只解析 HTML

## 目录结构

```text
crawler/
├── cmd/crawler/main.go       # 入口：参数、信号、输出文件、退出码
└── internal/
    ├── config/               # flag 解析（独立 FlagSet，可测试）
    ├── crawler/              # 调度核心：worker pool、去重、深度、限速
    ├── fetcher/              # HTTP 下载（超时、大小限制、Content-Type 判断）
    ├── parser/               # x/net/html 提取标题与链接、URL 规范化
    ├── robots/               # robots.txt 解析与按主机缓存
    └── output/               # JSON / JSON Lines 输出
```

## 运行

```bash
# 国内网络：配置模块代理
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn

cd crawler
go test -race ./...             # 测试使用 httptest 本地站点，不访问外网
go build -o bin/crawler ./cmd/crawler
```

> 依赖下载说明：`golang.org/x/...` 系列模块在国内无法直连 `go.googlesource.com`，务必通过 GOPROXY（goproxy.cn / mirrors.aliyun.com/goproxy）下载。

## 使用示例

```bash
# 抓取 golang.google.cn（国内可访问的 Go 官网镜像），深度 1，最多 5 页，每秒 3 个请求
./bin/crawler -depth 1 -max-pages 5 -rate 3 https://golang.google.cn/
```

输出（stdout）：

```json
[
  {
    "url": "https://golang.google.cn/",
    "depth": 0,
    "status": 200,
    "title": "The Go Programming Language",
    "links": 148,
    "elapsed_ms": 317
  },
  {
    "url": "https://golang.google.cn/learn/",
    "depth": 1,
    "status": 200,
    "title": "Get Started - The Go Programming Language",
    "links": 123,
    "elapsed_ms": 293
  }
]
```

日志（stderr）：

```text
level=INFO msg=抓取结束 pages=5 errors=0 skipped=0 elapsed=1.62s
```

更多用法：

```bash
./bin/crawler -o result.json -workers 8 -rate 5 -burst 3 https://example.com/
./bin/crawler -format jsonl https://example.com/ | jq -r 'select(.status != 200) | .url'
./bin/crawler -deadline 30s -max-pages 0 https://example.com/   # 不限页数，最多跑 30 秒
./bin/crawler -v https://example.com/                            # 调试日志
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `-workers` | 4 | 并发 worker 数 |
| `-depth` | 2 | 最大深度（种子为 0） |
| `-max-pages` | 100 | 最多抓取页数，0 不限 |
| `-rate` / `-burst` | 2 / 1 | 每秒请求数 / 令牌桶容量，rate=0 不限速 |
| `-same-host` | true | 只抓同主机 |
| `-robots` | true | 遵守 robots.txt |
| `-timeout` | 10s | 单请求超时 |
| `-max-body` | 2MiB | 单页最大读取字节 |
| `-deadline` | 0 | 整体运行时长上限 |
| `-o` / `-format` | `-` / json | 输出位置 / 格式 |

> 请合理设置限速并遵守目标站点的 robots.txt 与服务条款。需要走代理时设置 `HTTPS_PROXY` 环境变量即可（Transport 使用了 `http.ProxyFromEnvironment`）。

## 扩展练习

1. 支持 `Crawl-delay`：为每个主机建立独立的 `rate.Limiter`。
2. 断点续爬：把 `seen` 与 `queue` 定期持久化到文件。
3. 提取更多内容：meta description、正文文本、图片链接。
4. 换成分布式：调度者改为 Redis 队列 + 布隆过滤器去重，worker 部署多实例。
