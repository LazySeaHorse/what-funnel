FROM golang:1.22-alpine AS builder
RUN go install github.com/pressly/goose/v3/cmd/goose@v3.20.0

FROM alpine:latest
COPY --from=builder /go/bin/goose /usr/local/bin/goose
ENTRYPOINT ["goose"]
