module github.com/whatfunnel/whatfunnel/services/conversation-svc

go 1.25.0

require (
	github.com/google/uuid v1.6.0
	github.com/gorilla/mux v1.8.1
	github.com/jackc/pgx/v5 v5.10.0
	github.com/whatfunnel/whatfunnel/packages/go-common v0.0.0
	github.com/whatfunnel/whatfunnel/adapters/fake v0.0.0
)

replace (
	github.com/whatfunnel/whatfunnel/packages/go-common => ../../packages/go-common
	github.com/whatfunnel/whatfunnel/adapters/fake => ../../adapters/fake
)
