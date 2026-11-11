# 后端镜像（多阶段构建）。
#
# 构建上下文是 apps/server：
#   docker build -t app-server ./apps/server
# 基础镜像版本跟随 go.mod 的 go 指令，避免构建时临时下载新版工具链。
FROM golang:1.26-alpine AS builder

WORKDIR /src

# 先拷依赖清单，利用层缓存：改代码不会重新下载模块。
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 版本号在构建期注入，与 task server:build 注的是同一批变量。
# 构建上下文是本目录（拿不到仓库根目录的 VERSION.txt），所以缺省落到 dev；
# 发布流水线传 tag 名，本地要打准版本就 --build-arg VERSION=...
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
      -o /out/server ./cmd/server

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/server /app/server
COPY configs /app/configs

EXPOSE 8080

ENTRYPOINT ["/app/server", "-config", "configs/config.prod.yaml"]
