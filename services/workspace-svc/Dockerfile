FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go workspace files
COPY go.work go.work.sum* ./

# Copy module files
COPY packages/go-common/go.mod packages/go-common/go.sum* packages/go-common/
COPY services/workspace-svc/go.mod services/workspace-svc/go.sum* services/workspace-svc/

# Download dependencies
RUN cd services/workspace-svc && go mod download

# Copy source
COPY packages/go-common/ packages/go-common/
COPY services/workspace-svc/ services/workspace-svc/

# Build
RUN cd services/workspace-svc && go build -o /bin/workspace-svc ./cmd/workspace-svc/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/workspace-svc /bin/workspace-svc
EXPOSE 8082
ENTRYPOINT ["/bin/workspace-svc"]
