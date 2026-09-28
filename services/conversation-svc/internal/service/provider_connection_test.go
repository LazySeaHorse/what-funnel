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

func TestEvictOrphanedChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		provider   messaging.Provider
		setup      func(*ConnectionService, *mockAdapterControl)
		channelID  string
		wantErr    bool
		errContain string
		wantLogout []string
	}{
		{
			name:       "unconfigured provider",
			provider:   messaging.ProviderWhatsApp,
			setup:      func(cs *ConnectionService, mock *mockAdapterControl) {},
			channelID:  "ch-1",
			wantErr:    true,
			errContain: "is not configured",
		},
		{
			name:     "logout returns error",
			provider: messaging.ProviderWhatsApp,
			setup: func(cs *ConnectionService, mock *mockAdapterControl) {
				mock.logoutFunc = func(ctx context.Context, channelID string) error {
					return errors.New("logout failed")
				}
				cs.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)
			},
			channelID:  "ch-err",
			wantErr:    true,
			errContain: "logout failed",
			wantLogout: []string{"ch-err"},
		},
		{
			name:     "logout succeeds",
			provider: messaging.ProviderWhatsApp,
			setup: func(cs *ConnectionService, mock *mockAdapterControl) {
				mock.logoutFunc = func(ctx context.Context, channelID string) error {
					return nil
				}
				cs.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)
			},
			channelID:  "ch-success",
			wantErr:    false,
			wantLogout: []string{"ch-success"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mock := &mockAdapterControl{}
			connSvc := NewConnectionService(nil)
			tt.setup(connSvc, mock)

			err := connSvc.EvictOrphanedChannel(context.Background(), tt.provider, tt.channelID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("EvictOrphanedChannel() err = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Errorf("EvictOrphanedChannel() err = %v, want substring %q", err, tt.errContain)
				}
			} else {
				if err != nil {
					t.Fatalf("EvictOrphanedChannel() unexpected err: %v", err)
				}
			}

			if tt.wantLogout != nil {
				if !reflect.DeepEqual(mock.logoutCalls, tt.wantLogout) {
					t.Errorf("logoutCalls = %v, want %v", mock.logoutCalls, tt.wantLogout)
				}
			}
		})
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

func TestCreateChannelConnection_RetrySelfHealing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	pool := testDBPool(t)
	ctx := context.Background()

	var accountID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO accounts (name) VALUES ('retry-test') RETURNING id`).Scan(&accountID)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), `DELETE FROM provider_connections WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(context.Background(), `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	t.Run("succeeds on second create attempt after reconciliation", func(t *testing.T) {
		createAttempts := 0
		mock := &mockAdapterControl{
			createFunc: func(ctx context.Context, channelID, credential string) (AdapterSnapshot, error) {
				createAttempts++
				if createAttempts == 1 {
					return AdapterSnapshot{}, errors.New("adapter conflict / stale state")
				}
				return AdapterSnapshot{
					ChannelID: channelID,
					State:     messaging.ConnectionConnected,
					Detail:    "connected successfully",
				}, nil
			},
			listFunc: func(ctx context.Context) ([]AdapterSnapshot, error) {
				return []AdapterSnapshot{}, nil
			},
		}

		connSvc := NewConnectionService(pool)
		connSvc.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)

		conn, err := connSvc.CreateChannelConnection(ctx, accountID, messaging.ProviderWhatsApp, "healing-channel", "")
		if err != nil {
			t.Fatalf("CreateChannelConnection() unexpected error = %v", err)
		}
		if conn.State != messaging.ConnectionConnected {
			t.Errorf("conn.State = %v, want %v", conn.State, messaging.ConnectionConnected)
		}
		if len(mock.createCalls) != 2 {
			t.Errorf("createCalls = %d, want 2", len(mock.createCalls))
		}
		if mock.listCalls < 1 {
			t.Errorf("listCalls = %d, want at least 1 (reconciliation should run)", mock.listCalls)
		}
	})

	t.Run("fails when retry also fails", func(t *testing.T) {
		mock := &mockAdapterControl{
			createFunc: func(ctx context.Context, channelID, credential string) (AdapterSnapshot, error) {
				return AdapterSnapshot{}, errors.New("adapter persistent failure")
			},
			listFunc: func(ctx context.Context) ([]AdapterSnapshot, error) {
				return []AdapterSnapshot{}, nil
			},
		}

		connSvc := NewConnectionService(pool)
		connSvc.RegisterAdapterControl(messaging.ProviderWhatsApp, mock)

		conn, err := connSvc.CreateChannelConnection(ctx, accountID, messaging.ProviderWhatsApp, "failing-channel", "")
		if err == nil {
			t.Fatal("CreateChannelConnection() error = nil, want error")
		}
		if conn == nil || conn.State != messaging.ConnectionError {
			t.Errorf("conn.State = %v, want %v", conn.State, messaging.ConnectionError)
		}
		if len(mock.createCalls) != 2 {
			t.Errorf("createCalls = %d, want 2 (initial + 1 retry)", len(mock.createCalls))
		}
	})
}
