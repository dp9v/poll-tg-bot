# Stage 1: build
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bot .

# Stage 2: runtime
FROM alpine:3.21

LABEL org.opencontainers.image.title="poll-tg-bot" \
      org.opencontainers.image.source="https://github.com/dp9v/go-notification-tg-bot"

RUN apk add --no-cache ca-certificates \
    && addgroup -S bot && adduser -S bot -G bot \
    && mkdir -p /data && chown bot:bot /data

COPY --from=builder /app/bot /bot

VOLUME ["/data"]

USER bot

ENTRYPOINT ["/bot"]
