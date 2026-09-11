module github.com/whatfunnel/whatfunnel/adapters/telegram-botapi

go 1.25.0

require (
	github.com/mattn/go-sqlite3 v1.14.45
	github.com/whatfunnel/whatfunnel/packages/go-common v0.0.0
	golang.org/x/sync v0.21.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/redis/go-redis/v9 v9.21.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace github.com/whatfunnel/whatfunnel/packages/go-common => ../../packages/go-common
