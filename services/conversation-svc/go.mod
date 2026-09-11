module github.com/whatfunnel/whatfunnel/services/conversation-svc

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/jackc/pgx/v5 v5.10.0
	github.com/whatfunnel/whatfunnel/packages/go-common v0.0.0
	golang.org/x/sync v0.17.0
)

require rsc.io/qr v0.2.0 // indirect

replace (
	github.com/whatfunnel/whatfunnel/packages/go-common => ../../packages/go-common
)
