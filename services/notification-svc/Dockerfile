FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go workspace files
COPY go.work go.work.sum* ./

# Copy module files
COPY packages/go-common/go.mod packages/go-common/go.sum* packages/go-common/
COPY services/api-gateway/go.mod services/api-gateway/go.sum* services/api-gateway/
COPY services/identity-svc/go.mod services/identity-svc/go.sum* services/identity-svc/
COPY services/workspace-svc/go.mod services/workspace-svc/go.sum* services/workspace-svc/
COPY services/conversation-svc/go.mod services/conversation-svc/go.sum* services/conversation-svc/
COPY services/notification-svc/go.mod services/notification-svc/go.sum* services/notification-svc/
COPY adapters/fake/go.mod adapters/fake/
COPY adapters/matrix-mautrix/go.mod adapters/matrix-mautrix/
COPY tests/integration/go.mod tests/integration/go.sum* tests/integration/

# Download dependencies
RUN cd services/notification-svc && go mod download

# Copy source
COPY packages/go-common/ packages/go-common/
COPY services/notification-svc/ services/notification-svc/

# Build
RUN cd services/notification-svc && go build -o /bin/notification-svc ./cmd/notification-svc/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/notification-svc /bin/notification-svc
EXPOSE 8084
ENTRYPOINT ["/bin/notification-svc"]
