# go-study-examples

配套 **Go 语言学习网站** 的实战示例项目合集。每个子目录都是一个**独立的 Go module**，可以单独构建、测试、运行，代码按工程化规范组织（`cmd/` + `internal/` 分层、配置、日志 `log/slog`、错误处理、优雅关闭、单元测试与集成测试）。

| 项目 | 简介 | 技术栈 | 重点知识 |
|---|---|---|---|
| [todo-cli](./todo-cli) | 命令行 Todo 工具 | cobra、go-runewidth、encoding/json | 子命令、接口与依赖注入、原子写文件、彩色表格 |
| [rest-api](./rest-api) | 图书管理 REST API | Gin、GORM、SQLite（纯 Go）、JWT、bcrypt | 分层架构、参数校验、统一响应、中间件、httptest、Docker |
| [crawler](./crawler) | 并发网页爬虫 | x/net/html、x/time/rate | worker pool、channel 调度、去重、限速、robots.txt、context 取消 |
| [chatroom](./chatroom) | WebSocket 聊天室 | gorilla/websocket、embed | Hub 广播、房间、心跳、慢客户端处理、优雅关闭长连接 |
| [shortener](./shortener) | 短链接服务 | net/http（Go 1.22 路由）、modernc.org/sqlite | base62、泛型 LRU、令牌桶限流、异步批量统计 |

推荐学习顺序：todo-cli → rest-api → crawler → chatroom → shortener。

## 环境要求

- Go **1.24** 及以上（`go version` 查看）
- 无需安装 gcc：所有 SQLite 驱动都是纯 Go 实现

## 获取代码

```bash
git clone https://github.com/dmxj/go-study-examples.git
cd go-study-examples
```

> 国内访问 GitHub 较慢时，可以：
> - 使用 Gitee 等国内代码托管平台的"从 GitHub 导入仓库"功能创建镜像后再 clone；
> - 为 git 配置本地代理：`git config --global http.https://github.com.proxy http://127.0.0.1:7890`（端口按你的代理软件修改）；
> - 只需要某一个项目时用浅克隆减少下载量：`git clone --depth 1 https://github.com/dmxj/go-study-examples.git`。

## 国内网络下载依赖

```bash
go env -w GOPROXY=https://goproxy.cn,direct     # 或 https://mirrors.aliyun.com/goproxy/,direct
go env -w GOSUMDB=sum.golang.google.cn
```

## 运行任一项目

```bash
cd rest-api          # 换成任意子目录
go mod download
go build ./...
go vet ./...
go test ./...        # 推荐加 -race 做数据竞争检测
```

各项目的运行方式与 API/命令示例见其目录下的 README.md。

## 目录约定

```text
<project>/
├── cmd/<app>/main.go   # 程序入口：只负责组装依赖、处理信号与退出码
├── internal/           # 私有代码，外部 module 无法 import
│   ├── config/         # 配置解析
│   └── ...             # 按职责划分的业务包，每个包都有 _test.go
├── go.mod
└── README.md
```

## 许可证

[MIT](./LICENSE) © dmxj
