package service

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/db"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestIsProviderConnectionLabelConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "matching unique index",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "idx_channels_account_provider_label"},
			want: true,
		},
		{
			name: "different unique index",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "other_index"},
		},
		{
			name: "different database error",
			err:  &pgconn.PgError{Code: "23503", ConstraintName: "idx_channels_account_provider_label"},
		},
		{
			name: "wrapped matching error",
			err:  errors.Join(errors.New("insert channel"), &pgconn.PgError{Code: "23505", ConstraintName: "idx_channels_account_provider_label"}),
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := isProviderConnectionLabelConflict(test.err); got != test.want {
				t.Errorf("isProviderConnectionLabelConflict() = %t, want %t", got, test.want)
			}
		})
	}
}

type mockAdapterControl struct {
	mu           sync.Mutex
	createFunc   func(ctx context.Context, channelID, credential string) (AdapterSnapshot, error)
	retryFunc    func(ctx context.Context, channelID, credential string) (AdapterSnapshot, error)
	snapshotFunc func(ctx context.Context, channelID string) (AdapterSnapshot, error)
	logoutFunc   func(ctx context.Context, channelID string) error
	listFunc     func(ctx context.Context) ([]AdapterSnapshot, error)

	createCalls []string
	logoutCalls []string
	listCalls   int
}

var _ AdapterControl = (*mockAdapterControl)(nil)

func (m *mockAdapterControl) Create(ctx context.Context, channelID, credential string) (AdapterSnapshot, error) {
	m.mu.Lock()
	m.createCalls = append(m.createCalls, channelID)
	m.mu.Unlock()
	if m.createFunc != nil {
		return m.createFunc(ctx, channelID, credential)
	}
	return AdapterSnapshot{
		ChannelID: channelID,
		State:     messaging.ConnectionConnected,
	}, nil
}

func (m *mockAdapterControl) Retry(ctx context.Context, channelID, credential string) (AdapterSnapshot, error) {
	if m.retryFunc != nil {
		return m.retryFunc(ctx, channelID, credential)
	}
	return AdapterSnapshot{ChannelID: channelID}, nil
}

func (m *mockAdapterControl) Snapshot(ctx context.Context, channelID string) (AdapterSnapshot, error) {
	if m.snapshotFunc != nil {
		return m.snapshotFunc(ctx, channelID)
	}
	return AdapterSnapshot{ChannelID: channelID}, nil
}

func (m *mockAdapterControl) Logout(ctx context.Context, channelID string) error {
	m.mu.Lock()
	m.logoutCalls = append(m.logoutCalls, channelID)
	m.mu.Unlock()
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, channelID)
	}
	return nil
}

func (m *mockAdapterControl) List(ctx context.Context) ([]AdapterSnapshot, error) {
	m.mu.Lock()
	m.listCalls++
	m.mu.Unlock()
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []AdapterSnapshot{}, nil
}

// evictionTestService returns a connection service whose clock the test drives
// and whose "database has channel data" answer is fixed, so eviction policy
// can be tested without a database.
func evictionTestService(policy OrphanEvictionPolicy, hasData bool, mock *mockAdapterControl) (*ConnectionService, *time.Time) {
	svc := NewConnectionService(nil)
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return clock }
	svc.hasChannelData = func(context.Context, messaging.Provider) (bool, error) { return hasData, nil }
	svc.ConfigureOrphanEviction(policy)
	svc.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)
	return svc, &clock
}

func TestEvictOrphanedChannel(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("unconfigured provider", func(t *testing.T) {
		t.Parallel()
		svc := NewConnectionService(nil)
		_, err := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-1")
		if err == nil || !strings.Contains(err.Error(), "is not configured") {
			t.Fatalf("err = %v, want 'is not configured'", err)
		}
	})

	t.Run("disabled by default never logs out", func(t *testing.T) {
		t.Parallel()
		mock := &mockAdapterControl{}
		svc := NewConnectionService(nil)
		svc.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)
		evicted, err := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-1")
		if evicted || err != nil || len(mock.logoutCalls) != 0 {
			t.Fatalf("evicted = %v, err = %v, logouts = %v; want no eviction", evicted, err, mock.logoutCalls)
		}
	})

	t.Run("only after the grace period", func(t *testing.T) {
		t.Parallel()
		mock := &mockAdapterControl{}
		svc, clock := evictionTestService(OrphanEvictionPolicy{Enabled: true, Grace: time.Hour}, true, mock)

		if evicted, err := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-grace"); evicted || err != nil {
			t.Fatalf("first sighting: evicted = %v, err = %v; want neither", evicted, err)
		}
		*clock = clock.Add(59 * time.Minute)
		if evicted, _ := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-grace"); evicted {
			t.Fatal("evicted before the grace period elapsed")
		}
		*clock = clock.Add(2 * time.Minute)
		evicted, err := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-grace")
		if !evicted || err != nil {
			t.Fatalf("after grace: evicted = %v, err = %v; want eviction", evicted, err)
		}
		if !reflect.DeepEqual(mock.logoutCalls, []string{"ch-grace"}) {
			t.Errorf("logoutCalls = %v, want [ch-grace]", mock.logoutCalls)
		}
	})

	t.Run("never evicts when the database has no channel data", func(t *testing.T) {
		t.Parallel()
		mock := &mockAdapterControl{}
		svc, clock := evictionTestService(OrphanEvictionPolicy{Enabled: true}, false, mock)
		for i := 0; i < 3; i++ {
			*clock = clock.Add(24 * time.Hour)
			if evicted, err := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-empty-db"); evicted || err != nil {
				t.Fatalf("evicted = %v, err = %v against an empty database", evicted, err)
			}
		}
		if len(mock.logoutCalls) != 0 {
			t.Fatalf("logoutCalls = %v, want none", mock.logoutCalls)
		}
	})

	t.Run("logout failure is an error and not an eviction", func(t *testing.T) {
		t.Parallel()
		mock := &mockAdapterControl{logoutFunc: func(context.Context, string) error { return errors.New("logout failed") }}
		svc, _ := evictionTestService(OrphanEvictionPolicy{Enabled: true}, true, mock)
		evicted, err := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-err")
		if evicted || err == nil || !strings.Contains(err.Error(), "logout failed") {
			t.Fatalf("evicted = %v, err = %v; want a logout error and no eviction", evicted, err)
		}
	})

	t.Run("session already gone is not counted", func(t *testing.T) {
		t.Parallel()
		mock := &mockAdapterControl{logoutFunc: func(context.Context, string) error { return ErrAdapterConnectionNotFound }}
		svc, _ := evictionTestService(OrphanEvictionPolicy{Enabled: true}, true, mock)
		if evicted, err := svc.EvictOrphanedChannel(ctx, messaging.ProviderWhatsApp, "ch-gone"); evicted || err != nil {
			t.Fatalf("evicted = %v, err = %v; want neither", evicted, err)
		}
	})
}

func TestReconcileAdapterConnections_CountsOnlySuccessfulEvictions(t *testing.T) {
	t.Parallel()
	mock := &mockAdapterControl{
		listFunc: func(context.Context) ([]AdapterSnapshot, error) {
			return []AdapterSnapshot{{ChannelID: "orphan-ok"}, {ChannelID: "orphan-fails"}}, nil
		},
		logoutFunc: func(_ context.Context, channelID string) error {
			if channelID == "orphan-fails" {
				return errors.New("adapter unreachable")
			}
			return nil
		},
	}
	svc, _ := evictionTestService(OrphanEvictionPolicy{Enabled: true}, true, mock)
	evicted, err := svc.ReconcileAdapterConnections(context.Background(), messaging.ProviderWhatsApp)
	if evicted != 1 {
		t.Errorf("evicted = %d, want 1 (the failed logout must not count)", evicted)
	}
	if err == nil || !strings.Contains(err.Error(), "adapter unreachable") {
		t.Errorf("err = %v, want the logout failure to be reported", err)
	}
}

func TestReconcileAdapterConnections_Unit(t *testing.T) {
	t.Parallel()

	t.Run("unconfigured provider", func(t *testing.T) {
		t.Parallel()
		connSvc := NewConnectionService(nil)
		_, err := connSvc.ReconcileAdapterConnections(context.Background(), messaging.ProviderWhatsApp)
		if err == nil || !strings.Contains(err.Error(), "is not configured") {
			t.Fatalf("err = %v, want 'is not configured'", err)
		}
	})

	t.Run("list error", func(t *testing.T) {
		t.Parallel()
		connSvc := NewConnectionService(nil)
		connSvc.ConfigureOrphanEviction(OrphanEvictionPolicy{Enabled: true})
		mock := &mockAdapterControl{
			listFunc: func(ctx context.Context) ([]AdapterSnapshot, error) {
				return nil, errors.New("list failed")
			},
		}
		connSvc.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)
		_, err := connSvc.ReconcileAdapterConnections(context.Background(), messaging.ProviderWhatsApp)
		if err == nil || !strings.Contains(err.Error(), "list adapter connections") {
			t.Fatalf("err = %v, want 'list adapter connections' error", err)
		}
	})
}

func testDBPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://whatfunnel:whatfunnel@localhost:5432/whatfunnel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("integration test database is required in CI: %v", err)
		}
		t.Skipf("skipping integration test: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestReconcileAdapterConnections_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testDBPool(t)
	ctx := context.Background()

	var accountID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('reconcile-test') RETURNING id`).Scan(&accountID)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	existingChannelID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO channels (id, account_id, type, provider, label, status, capabilities, updated_at)
		VALUES ($1, $2, 'whatsapp', 'whatsapp', 'reconcile-ch', 'connected', '{}', NOW())
	`, existingChannelID, accountID)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	missingChannelID := uuid.New().String()
	invalidChannelID := "not-a-valid-uuid"

	mock := &mockAdapterControl{
		listFunc: func(ctx context.Context) ([]AdapterSnapshot, error) {
			return []AdapterSnapshot{
				{ChannelID: existingChannelID.String()},
				{ChannelID: missingChannelID},
				{ChannelID: invalidChannelID},
			}, nil
		},
	}

	connSvc := NewConnectionService(pool)
	connSvc.ConfigureOrphanEviction(OrphanEvictionPolicy{Enabled: true})
	connSvc.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)

	evicted, err := connSvc.ReconcileAdapterConnections(ctx, messaging.ProviderWhatsApp)
	if err != nil {
		t.Fatalf("ReconcileAdapterConnections error = %v", err)
	}
	if evicted != 2 {
		t.Errorf("evicted = %d, want 2", evicted)
	}
	if len(mock.logoutCalls) != 2 {
		t.Fatalf("logoutCalls len = %d, want 2; calls: %v", len(mock.logoutCalls), mock.logoutCalls)
	}
	hasMissing := false
	hasInvalid := false
	for _, call := range mock.logoutCalls {
		if call == missingChannelID {
			hasMissing = true
		}
		if call == invalidChannelID {
			hasInvalid = true
		}
		if call == existingChannelID.String() {
			t.Errorf("existing channel %s was evicted!", call)
		}
	}
	if !hasMissing || !hasInvalid {
		t.Errorf("expected %s and %s in logoutCalls, got %v", missingChannelID, invalidChannelID, mock.logoutCalls)
	}
}

func TestStartProviderConnection_DoesNotRetryOrReconcileOnCreateFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testDBPool(t)
	ctx := context.Background()

	var accountID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('start-failure-test') RETURNING id`).Scan(&accountID)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM provider_connections WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	mock := &mockAdapterControl{
		createFunc: func(ctx context.Context, channelID, credential string) (AdapterSnapshot, error) {
			return AdapterSnapshot{}, errors.New("bad token")
		},
	}
	connSvc := NewConnectionService(pool)
	connSvc.ConfigureOrphanEviction(OrphanEvictionPolicy{Enabled: true})
	connSvc.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)

	conn, err := connSvc.StartProviderConnection(ctx, accountID, messaging.ProviderWhatsApp, "failing-channel", "")
	if err == nil {
		t.Fatal("StartProviderConnection() error = nil, want error")
	}
	if conn == nil || conn.State != messaging.ConnectionError {
		t.Fatalf("connection = %+v, want the durable error-state record", conn)
	}
	// A bad credential must reach the provider exactly once, and a failed
	// create must not trigger an adapter-wide reconcile.
	if len(mock.createCalls) != 1 {
		t.Errorf("createCalls = %d, want 1", len(mock.createCalls))
	}
	if mock.listCalls != 0 || len(mock.logoutCalls) != 0 {
		t.Errorf("listCalls = %d, logoutCalls = %v; want no reconcile", mock.listCalls, mock.logoutCalls)
	}
}
