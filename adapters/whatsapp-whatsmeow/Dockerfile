FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev
WORKDIR /src

COPY packages/go-common/go.mod packages/go-common/go.sum packages/go-common/
COPY adapters/whatsapp-whatsmeow/go.mod adapters/whatsapp-whatsmeow/go.sum adapters/whatsapp-whatsmeow/
RUN cd adapters/whatsapp-whatsmeow && GOWORK=off go mod download

COPY packages/go-common/ packages/go-common/
COPY adapters/whatsapp-whatsmeow/ adapters/whatsapp-whatsmeow/
RUN cd adapters/whatsapp-whatsmeow && CGO_ENABLED=1 GOWORK=off go build -trimpath -o /bin/whatsapp-adapter ./cmd/whatsapp-adapter

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S whatfunnel \
    && adduser -S -G whatfunnel -h /app whatfunnel \
    && install -d -m 0700 -o whatfunnel -g whatfunnel /data

COPY --from=builder /bin/whatsapp-adapter /bin/whatsapp-adapter
USER whatfunnel
VOLUME ["/data"]
EXPOSE 8086
ENTRYPOINT ["/bin/whatsapp-adapter"]
