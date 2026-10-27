# todo-cli：命令行 Todo 工具

一个用 Go 编写的命令行待办工具，基于 [cobra](https://github.com/spf13/cobra) 实现子命令，数据保存在本地 JSON 文件。适合作为学习 Go CLI 工程化的第一个项目。

## 功能

- 子命令：`add` / `list(ls)` / `done` / `undo` / `edit` / `rm` / `clear`
- 优先级：`high` / `medium` / `low`（也可写 `h`/`m`/`l` 或 `3`/`2`/`1`）
- 截止日期：`2026-11-01`、`today`、`tomorrow`、`+3d`、`+2w`；自动标记逾期
- 标签：`-t work,urgent`，大小写不敏感去重
- 过滤与排序：按状态、最低优先级、标签、标题关键字、是否逾期过滤；按 id/priority/due 排序
- 彩色表格输出：正确处理中文宽度；输出被重定向或设置 `NO_COLOR` 时自动关闭颜色
- 原子写入：先写临时文件再 `rename`，进程中途被杀也不会损坏数据
- 日志使用 `log/slog`，`-v` 输出调试信息；`Ctrl+C` 通过 `signal.NotifyContext` 优雅退出

## 目录结构

```text
todo-cli/
├── cmd/todo/main.go          # 入口：信号处理、退出码
└── internal/
    ├── cli/                  # cobra 命令定义（参数解析 → 调用 Service → 输出）
    │   ├── root.go
    │   ├── commands.go
    │   └── cli_test.go       # 端到端命令测试
    ├── config/               # 数据文件路径解析（flag > 环境变量 > 默认值）
    ├── storage/              # JSON 文件存储（原子写入、版本号）
    ├── todo/                 # 领域模型与业务逻辑（与 I/O 无关，可纯单元测试）
    │   ├── task.go           # Task / Priority
    │   ├── date.go           # 截止日期解析
    │   ├── filter.go         # 过滤与排序
    │   └── service.go        # 增删改查
    └── ui/                   # ANSI 颜色与表格渲染
```

## 快速开始

```bash
# 国内网络先配置模块代理（只需一次）
go env -w GOPROXY=https://goproxy.cn,direct
go env -w GOSUMDB=sum.golang.google.cn

cd todo-cli
go mod download        # 下载依赖
go test ./...          # 运行测试
go build -o bin/todo ./cmd/todo

# 或直接安装到 $GOPATH/bin
go install ./cmd/todo
```

> 依赖下载说明：本项目依赖 `github.com/spf13/cobra` 与 `github.com/mattn/go-runewidth`。国内直连 proxy.golang.org 通常超时，使用 `https://goproxy.cn` 或 `https://mirrors.aliyun.com/goproxy/` 即可；`GOSUMDB=sum.golang.google.cn` 用于校验和数据库的国内访问。

## 使用示例

```bash
$ todo add 写周报 -p high -d 2026-10-20 -t work,weekly
✔ 已新增任务 #1：写周报
$ todo add "Buy milk" -p low
✔ 已新增任务 #2：Buy milk
$ todo add 学习 Go 并发 -d +3d
✔ 已新增任务 #3：学习 Go 并发
$ todo done 2
✔ 已完成 #2：Buy milk
$ todo ls -a --sort priority
ID  状态  优先级  标题          截止日期           标签
──────────────────────────────────────────────────────────────
1   [ ]   high    写周报        2026-10-20 (逾期)  work,weekly
3   [ ]   medium  学习 Go 并发  2026-10-30
2   [x]   low     Buy milk      -
```

更多命令：

```bash
todo ls -p medium              # 只看 medium 及以上
todo ls --tag work --search 周报
todo ls --overdue              # 只看逾期
todo edit 3 --title 精读并发章节 -p high --due none   # none 清除截止日期
todo undo 2                    # 恢复为未完成
todo rm 2 3                    # 批量删除；部分失败时其余照常执行
todo clear                     # 清理已完成任务
todo --file ./my.json ls       # 指定数据文件
TODO_FILE=./my.json todo ls    # 或用环境变量
todo -v ls                     # 打印调试日志（写到 stderr）
```

## 配置

| 方式 | 说明 |
|---|---|
| `--file/-f` | 数据文件路径，优先级最高 |
| `TODO_FILE` | 环境变量，次优先 |
| 默认 | `~/.todo/todo.json` |
| `--no-color` / `NO_COLOR=1` | 关闭彩色输出 |

## 设计要点

- **分层**：`todo` 包只依赖 `Repository` 接口与 `Clock` 函数，测试时注入内存仓库与固定时间。
- **接口定义在使用方**：`Repository` 定义在 `todo` 包，`storage.JSONStore` 隐式实现它。
- **可选更新用指针**：`UpdateInput` 的指针字段为 `nil` 表示"不修改"，配合 `cmd.Flags().Changed` 区分"没传"与"传了空值"。
- **错误处理**：哨兵错误 `ErrNotFound` / `ErrEmptyTitle` + `%w` 包装；批量操作用 `errors.Join` 汇总。
- **JSON**：`Priority` 实现 `encoding.TextMarshaler`，以字符串存储；时间字段用 Go 1.24 的 `omitzero` 标签省略零值。

## 扩展练习

1. 增加 `todo stats` 子命令：统计各优先级数量、完成率。
2. 支持子任务或重复任务（每周一自动生成）。
3. 把存储换成 SQLite，只需新增一个实现 `todo.Repository` 的类型。
4. 用 `bubbletea` 做一个交互式 TUI 版本。
