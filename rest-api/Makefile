# 常用命令。运行 make help 查看说明。
APP      := book-api
BIN      := bin/$(APP)
PKG      := ./cmd/server
GOPROXY  ?= https://goproxy.cn,direct

export GOPROXY
export GOSUMDB ?= sum.golang.google.cn

.PHONY: help deps run build test race cover vet fmt docker-build docker-run clean

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

deps: ## 下载依赖（默认使用 goproxy.cn）
	go mod download

run: ## 本地运行（debug 模式）
	go run $(PKG)

build: ## 编译二进制到 bin/
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) $(PKG)

test: ## 运行全部测试
	go test ./...

race: ## 开启数据竞争检测运行测试
	go test -race ./...

cover: ## 生成覆盖率报告
	go test -coverpkg=./... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

vet: ## 静态检查
	go vet ./...

fmt: ## 格式化代码
	gofmt -w .

docker-build: ## 构建镜像
	docker build --build-arg GOPROXY=$(GOPROXY) -t $(APP):latest .

docker-run: ## 运行镜像（数据保存在 ./data）
	docker run --rm -p 8080:8080 -e APP_JWT_SECRET=please-change-this-secret -v $(PWD)/data:/data $(APP):latest

clean: ## 清理构建产物
	rm -rf bin coverage.out
