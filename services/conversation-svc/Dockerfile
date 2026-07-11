FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go workspace files
COPY go.work go.work.sum* ./

# Copy module files
COPY packages/go-common/go.mod packages/go-common/go.sum* packages/go-common/
COPY adapters/fake/go.mod adapters/fake/
COPY adapters/matrix-mautrix/go.mod adapters/matrix-mautrix/
COPY services/conversation-svc/go.mod services/conversation-svc/

# Download dependencies
RUN cd services/conversation-svc && go mod download

# Copy source
COPY packages/go-common/ packages/go-common/
COPY adapters/fake/ adapters/fake/
COPY adapters/matrix-mautrix/ adapters/matrix-mautrix/
COPY services/conversation-svc/ services/conversation-svc/

# Build
RUN cd services/conversation-svc && go build -o /bin/conversation-svc ./cmd/conversation-svc/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/conversation-svc /bin/conversation-svc
EXPOSE 8083
ENTRYPOINT ["/bin/conversation-svc"]
