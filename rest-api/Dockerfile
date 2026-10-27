# syntax=docker/dockerfile:1
#
# 多阶段构建：第一阶段用完整的 Go 镜像编译，第二阶段只拷贝二进制到极小的运行镜像。
# 最终镜像约 20MB，不含 Go 工具链和源码。
#
# 国内构建：
#   docker build --build-arg GOPROXY=https://goproxy.cn,direct -t book-api .
# 若 Docker Hub 拉取慢，可在 /etc/docker/daemon.json 配置 registry-mirrors，
# 或把下面的镜像名替换为你所在云厂商的镜像仓库地址。

ARG GO_VERSION=1.24

# ---------- 构建阶段 ----------
FROM golang:${GO_VERSION}-alpine AS builder

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=sum.golang.google.cn \
    CGO_ENABLED=0 \
    GOTOOLCHAIN=local

WORKDIR /src

# 先只拷贝 go.mod/go.sum 下载依赖：只要依赖不变，这一层就会命中缓存，
# 改业务代码时不必重新下载所有依赖。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# -trimpath 去掉本机路径；-ldflags "-s -w" 去掉符号表与调试信息，减小体积。
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- 运行阶段 ----------
FROM alpine:3.20

# 国内可把 apk 源换成阿里云镜像加速
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 app

ENV TZ=Asia/Shanghai \
    APP_ADDR=:8080 \
    APP_DB=/data/books.db \
    APP_MODE=release

WORKDIR /app
COPY --from=builder /out/server /app/server

# 用非 root 用户运行，降低容器被攻破后的影响面
RUN mkdir -p /data && chown app:app /data
USER app
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/app/server"]
