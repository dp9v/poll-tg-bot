# Stage 1: build
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bot .

# Stage 2: minimal runtime image
FROM alpine:3.21

COPY --from=builder /app/bot /bot

# Directory for persistent storage (activities.db)
RUN mkdir -p /data

VOLUME ["/data"]

ENTRYPOINT ["/bot"]

