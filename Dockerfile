# =============================================================================
# openPic-mcp Dockerfile
# =============================================================================
# 多阶段构建：
#   - 第一阶段（builder）：编译 Go 应用
#   - 第二阶段（runtime）：运行时环境
# =============================================================================

# -----------------------------------------------------------------------------
# 第一阶段：构建
# -----------------------------------------------------------------------------
FROM golang:1.23-alpine AS builder

# 设置工作目录
WORKDIR /build

# 使用阿里云 Go 模块代理加速依赖下载
ENV GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64

# 安装必要的构建工具
RUN apk add --no-cache git ca-certificates tzdata

# 复制 go.mod 和 go.sum（利用 Docker 缓存层）
COPY go.mod go.sum* ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 编译应用
# -ldflags="-s -w" 用于减小二进制文件大小
RUN go build -ldflags="-s -w" -o openPic-mcp ./cmd/vision-mcp

# -----------------------------------------------------------------------------
# 第二阶段：运行时
# -----------------------------------------------------------------------------
FROM alpine:latest

# 设置工作目录
WORKDIR /app

# 安装运行时依赖
# ca-certificates: HTTPS 请求需要
# procps: pgrep 命令用于健康检查
RUN apk add --no-cache ca-certificates procps

# 从构建阶段复制二进制文件
COPY --from=builder /build/openPic-mcp /app/openPic-mcp

# 从构建阶段复制时区数据
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# 设置时区（可选，默认 UTC）
ENV TZ=UTC

# 创建非 root 用户运行应用（安全最佳实践）
RUN adduser -D -u 1000 appuser
USER appuser

# 设置入口点
ENTRYPOINT ["/app/openPic-mcp"]

# =============================================================================
# 构建命令：
#   docker build -t openpic-mcp:latest .
#
# 运行命令：
#   docker run -it --rm \
#     -e VISION_API_BASE_URL=https://api.openai.com/v1 \
#     -e VISION_API_KEY=sk-xxx \
#     -e VISION_MODEL=gpt-4o \
#     openpic-mcp:latest
# =============================================================================

