# ── 编译阶段 ──────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o server .

# ── 运行阶段 ──────────────────────────────────────────────────
FROM alpine:3.20

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 43966
ENTRYPOINT ["./server"]
