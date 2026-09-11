module github.com/whatfunnel/whatfunnel/adapters/whatsapp-whatsmeow

go 1.25.0

require (
	github.com/mattn/go-sqlite3 v1.14.45
	github.com/whatfunnel/whatfunnel/packages/go-common v0.0.0
	go.mau.fi/whatsmeow v0.0.0-20260630180629-b572e5bcb92b
	golang.org/x/sync v0.21.0
	google.golang.org/protobuf v1.36.11
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/beeper/argo-go v1.1.2 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/elliotchance/orderedmap/v3 v3.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/petermattis/goid v0.0.0-20260330135022-df67b199bc81 // indirect
	github.com/redis/go-redis/v9 v9.21.0 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/vektah/gqlparser/v2 v2.5.27 // indirect
	go.mau.fi/libsignal v0.2.2 // indirect
	go.mau.fi/util v0.9.10 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/exp v0.0.0-20260611194520-c48552f49976 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)

replace (
	github.com/whatfunnel/whatfunnel/packages/go-common => ../../packages/go-common
	// The sandbox image contains v0.36.0; v0.37.0 is only a transitive
	// go.mod requirement and is not imported by the adapter build.
	golang.org/x/mod => golang.org/x/mod v0.36.0
	golang.org/x/telemetry => golang.org/x/telemetry v0.0.0-20260409153401-be6f6cb8b1fa
	golang.org/x/term => golang.org/x/term v0.43.0
	golang.org/x/tools => golang.org/x/tools v0.45.0
)
