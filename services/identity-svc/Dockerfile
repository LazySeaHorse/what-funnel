FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go workspace files
COPY go.work go.work.sum* ./

# Copy module files
COPY packages/go-common/go.mod packages/go-common/go.sum* packages/go-common/
COPY services/identity-svc/go.mod services/identity-svc/go.sum* services/identity-svc/

# Download dependencies
RUN cd services/identity-svc && go mod download

# Copy source
COPY packages/go-common/ packages/go-common/
COPY services/identity-svc/ services/identity-svc/

# Build
RUN cd services/identity-svc && go build -o /bin/identity-svc ./cmd/identity-svc/

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /bin/identity-svc /bin/identity-svc
EXPOSE 8081
ENTRYPOINT ["/bin/identity-svc"]
