FROM golang:1.23-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/vpn-service ./cmd/vpn-service

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates dbus systemd \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/vpn-service /usr/local/bin/vpn-service
COPY configs/ /app/configs/
COPY migrations/ /app/migrations/

ENV CONFIG_PATH=/app/configs/config.yaml

CMD ["/usr/local/bin/vpn-service", "-config", "/app/configs/config.yaml"]
