FROM golang:1.27.1-bookworm@sha256:8d48e12ec56735e9358640898b9d9b9fcca110612ed8a5567438c0a1baa24e66 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY migrations/ migrations/
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/syranet ./cmd/vpn-service

FROM ghcr.io/sagernet/sing-box:v1.14.3@sha256:80c639ba467885b28511fb7c980565c2d30a8752f778e7bc8abdbc91fbfd95ec AS singbox

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && mkdir -m 700 /state && chown 10001:10001 /state
COPY --from=builder /out/syranet /usr/local/bin/syranet
USER 10001:10001
WORKDIR /app
ENTRYPOINT ["/usr/local/bin/syranet"]
CMD ["-config", "/app/config.yaml"]

FROM runtime AS node
ENV SYRANET_ROLE=node
COPY licenses/SINGBOX-LICENSE /usr/share/doc/sing-box/LICENSE
COPY --from=singbox /usr/local/bin/sing-box /usr/local/bin/sing-box
CMD ["node"]

FROM builder AS tests
COPY --from=singbox /usr/local/bin/sing-box /usr/local/bin/sing-box
COPY tests/e2e/ tests/e2e/
RUN apt-get update && apt-get install -y --no-install-recommends openssl && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["go"]
CMD ["test", "./...", "-count=1"]
