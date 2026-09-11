FROM golang:1.27-bookworm AS builder
WORKDIR /src
COPY packages/go-common/go.mod packages/go-common/go.sum packages/go-common/
COPY adapters/telegram-botapi/go.mod adapters/telegram-botapi/go.sum adapters/telegram-botapi/
RUN cd adapters/telegram-botapi && GOWORK=off go mod download
COPY packages/go-common/ packages/go-common/
COPY adapters/telegram-botapi/ adapters/telegram-botapi/
RUN cd adapters/telegram-botapi && CGO_ENABLED=1 GOWORK=off go build -trimpath -o /bin/telegram-adapter ./cmd/telegram-adapter

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --create-home adapter
COPY --from=builder /bin/telegram-adapter /bin/telegram-adapter
RUN mkdir -p /data && chown adapter:adapter /data
USER adapter
EXPOSE 8087
ENTRYPOINT ["/bin/telegram-adapter"]
