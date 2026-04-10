# ---- Builder stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /build

# 安装构建依赖
RUN apk add --no-cache git make

# 依赖缓存层
COPY go.mod go.sum ./
RUN go mod download

# 构建参数（版本信息）
ARG VERSION=0.2.0
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
ARG BUILD_TIME=unknown
ARG GO_VERSION=unknown
ARG BUILD_PLATFORM=linux/amd64

# 构建
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w \
        -X 'rua.plus/lolly/internal/app.Version=${VERSION}' \
        -X 'rua.plus/lolly/internal/app.GitCommit=${GIT_COMMIT}' \
        -X 'rua.plus/lolly/internal/app.GitBranch=${GIT_BRANCH}' \
        -X 'rua.plus/lolly/internal/app.BuildTime=${BUILD_TIME}' \
        -X 'rua.plus/lolly/internal/app.GoVersion=${GO_VERSION}' \
        -X 'rua.plus/lolly/internal/app.BuildPlatform=${BUILD_PLATFORM}'" \
    -trimpath \
    -o /build/lolly \
    main.go

# ---- Tini stage ----
FROM alpine:3.19 AS tini-stage
RUN apk add --no-cache tini-static

# ---- Runtime stage ----
FROM scratch

# CA 证书（出站 HTTPS 代理需要）
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# 二进制文件
COPY --from=builder /build/lolly /lolly

# tini 静态版本（处理 PID 1 僵尸进程回收和信号转发）
COPY --from=tini-stage /sbin/tini-static /tini

# 优雅关闭：SIGQUIT 触发 30s graceful stop
STOPSIGNAL SIGQUIT

# HTTP/1.1, HTTP/2, HTTP/3 (QUIC)
EXPOSE 8080/tcp 443/tcp 443/udp

# 使用 tini 作为 init 进程（PID 1）
ENTRYPOINT ["/tini", "--"]
CMD ["/lolly", "-c", "/etc/lolly/lolly.yaml"]