FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go workspace files
COPY go.work go.work.sum* ./

# Copy module files
COPY packages/go-common/go.mod packages/go-common/go.sum* packages/go-common/
COPY services/api-gateway/go.mod services/api-gateway/go.sum* services/api-gateway/
COPY services/identity-svc/go.mod services/identity-svc/go.sum* services/identity-svc/
COPY services/workspace-svc/go.mod services/workspace-svc/go.sum* services/workspace-svc/
COPY tests/integration/go.mod tests/integration/go.sum* tests/integration/

# Download dependencies
RUN cd services/api-gateway && go mod download

# Copy source
COPY packages/go-common/ packages/go-common/
COPY services/api-gateway/ services/api-gateway/

# Build
RUN cd services/api-gateway && go build -o /bin/api-gateway ./cmd/api-gateway/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/api-gateway /bin/api-gateway
EXPOSE 8080
ENTRYPOINT ["/bin/api-gateway"]
