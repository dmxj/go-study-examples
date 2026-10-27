# rest-api：Gin + GORM 图书管理 REST API

一个分层清晰、可直接作为业务项目脚手架的 REST API 示例：Gin 做 Web 框架，GORM + 纯 Go SQLite 驱动（[glebarez/sqlite](https://github.com/glebarez/sqlite)，无需 cgo）做持久化，JWT 做鉴权。

## 功能

- 图书 CRUD：列表（分页、关键字搜索、作者过滤、白名单排序）、详情、新增、部分更新（PATCH）、删除
- 用户注册 / 登录（bcrypt 哈希密码），JWT（HS256）鉴权中间件，写接口需要登录
- 参数校验（`binding` 标签）并转换为中文错误提示
- 统一响应格式 `{"code":0,"message":"ok","data":...}` 与业务错误码
- 中间件：Request ID、slog 结构化访问日志、panic 恢复、JWT 鉴权
- 配置全部来自环境变量；`http.Server` 超时设置；`SIGINT/SIGTERM` 优雅关闭
- `httptest` 集成测试（每个测试独立内存数据库）+ service 层单元测试（内存 fake 仓库）
- 多阶段 Dockerfile、Makefile

## 目录结构

```text
rest-api/
├── cmd/server/main.go        # 入口：加载配置、组装依赖、启动与优雅关闭
├── internal/
│   ├── config/               # 环境变量配置
│   ├── database/             # 打开 SQLite、连接池、AutoMigrate
│   ├── model/                # GORM 模型 + 请求/响应 DTO
│   ├── repository/           # 数据访问层（接口 + GORM 实现）
│   ├── service/              # 业务逻辑层（不依赖 HTTP 与 GORM）
│   ├── handler/              # HTTP 层：绑定参数 → 调用 service → 写响应
│   ├── middleware/           # RequestID / Logger / Recovery / JWTAuth
│   ├── response/             # 统一响应与错误映射
│   ├── auth/                 # JWT 签发与校验
│   ├── apperr/               # 跨层共享的业务错误
│   └── router/               # 路由注册与依赖注入；router_test.go 为集成测试
├── Dockerfile
└── Makefile
```

依赖方向：`handler → service → repository → model`，下层不 import 上层。

## 运行

```bash
# 国内网络：配置模块代理
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn

cd rest-api
make deps     # go mod download
make test     # 运行全部测试
make run      # 启动服务，默认监听 :8080，数据库 data/books.db
```

> 依赖下载说明：Gin、GORM、validator、jwt、x/crypto 等均可通过 goproxy.cn 或 `https://mirrors.aliyun.com/goproxy/` 下载。SQLite 驱动是纯 Go 实现（modernc.org/sqlite 的分支），**不需要安装 gcc**，`CGO_ENABLED=0` 也能编译，Windows 用户同样省心。

### 配置项

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `APP_ADDR` | `:8080` | 监听地址 |
| `APP_DB` | `data/books.db` | SQLite 文件路径 |
| `APP_JWT_SECRET` | 开发用默认值 | JWT 密钥，**release 模式必须设置**，至少 16 字符 |
| `APP_JWT_TTL` | `24h` | Token 有效期 |
| `APP_MODE` | `debug` | `debug` / `release` / `test` |
| `APP_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `APP_SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭最长等待时间 |

## API 示例

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/healthz` | 否 | 健康检查 |
| POST | `/api/v1/auth/register` | 否 | 注册 |
| POST | `/api/v1/auth/login` | 否 | 登录，返回 token |
| GET | `/api/v1/users/me` | 是 | 当前用户 |
| GET | `/api/v1/books` | 否 | 列表：`page`、`page_size`(≤100)、`q`、`author`、`sort` |
| GET | `/api/v1/books/:id` | 否 | 详情 |
| POST | `/api/v1/books` | 是 | 新增 |
| PATCH | `/api/v1/books/:id` | 是 | 部分更新 |
| DELETE | `/api/v1/books/:id` | 是 | 删除 |

```bash
# 注册与登录
curl -s -XPOST localhost:8080/api/v1/auth/register -d '{"username":"alice","password":"password123"}'
TOKEN=$(curl -s -XPOST localhost:8080/api/v1/auth/login \
  -d '{"username":"alice","password":"password123"}' | sed -E 's/.*"token":"([^"]+)".*/\1/')

# 新增图书（price 单位：分）
curl -s -XPOST localhost:8080/api/v1/books -H "Authorization: Bearer $TOKEN" \
  -d '{"title":"Go 程序设计语言","author":"Alan Donovan","isbn":"978-7-111-55842-2","price":7900,"year":2016,"stock":10}'
# {"code":0,"message":"created","data":{"id":1,"title":"Go 程序设计语言",...,"isbn":"9787111558422",...}}

# 校验失败
curl -s -XPOST localhost:8080/api/v1/books -H "Authorization: Bearer $TOKEN" -d '{"author":"x","isbn":"1"}'
# {"code":40000,"message":"title 为必填字段; isbn 不是合法的 ISBN"}

# 未登录
curl -s -XPOST localhost:8080/api/v1/books -d '{}'
# {"code":40100,"message":"未登录或登录已过期"}

# 分页 + 搜索 + 排序
curl -s 'localhost:8080/api/v1/books?page=1&page_size=5&q=Go&sort=-price'

# 部分更新：只改库存
curl -s -XPATCH localhost:8080/api/v1/books/1 -H "Authorization: Bearer $TOKEN" -d '{"stock":8}'
```

错误码：`0` 成功、`40000` 参数错误、`40100` 未认证、`40300` 无权限、`40400` 不存在、`40900` 冲突、`50000` 内部错误。

## Docker

```bash
make docker-build      # 等价于 docker build --build-arg GOPROXY=https://goproxy.cn,direct -t book-api .
make docker-run
```

> 国内 Docker 说明：
> 1. 构建阶段通过 `--build-arg GOPROXY=https://goproxy.cn,direct` 加速 Go 依赖下载；
> 2. 运行阶段 Dockerfile 已把 Alpine 源替换为 `mirrors.aliyun.com`；
> 3. 拉取 `golang`、`alpine` 基础镜像慢时，在 Docker Desktop / `/etc/docker/daemon.json` 中配置 `registry-mirrors`（使用你所在云厂商提供的镜像加速地址），或把 `FROM` 改成私有镜像仓库地址。

## Swagger（可选）

handler 中已按 [swaggo/swag](https://github.com/swaggo/swag) 格式写了 `List` 的注释示例。如需生成文档：

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g cmd/server/main.go -o docs
# 再引入 github.com/swaggo/gin-swagger 注册 /swagger/*any 路由
```

## 扩展练习

1. 增加"借阅"资源：借书扣减库存，用 GORM 事务 + `UPDATE ... WHERE stock > 0` 防止超卖。
2. 只允许图书创建者修改/删除（返回 403），在 service 层实现权限判断。
3. 把 SQLite 换成 MySQL/PostgreSQL：只改 `database.Open`，其余代码不动。
4. 增加刷新 Token、登出（Token 黑名单存 Redis）。
5. 接入 Prometheus 指标中间件与 OpenTelemetry 链路追踪。
