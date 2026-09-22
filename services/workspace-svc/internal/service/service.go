// Package service implements workspace-svc business logic:
// user management (list, invite, role change), account settings
// (with an encrypted AI provider API key), and pipeline management.
// All state-changing operations write audit_logs rows.
package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/whatfunnel/whatfunnel/packages/go-common/crypto"
)

// Option configures a workspace Service.
type Option func(*Service)

// WithIdentityProvisioner configures a custom IdentityProvisioner.
func WithIdentityProvisioner(p IdentityProvisioner) Option {
	return func(s *Service) {
		s.identity = p
	}
}

// Service handles workspace operations.
type Service struct {
	pool                     *pgxpool.Pool
	cipher                   *crypto.Cipher
	aiProviderTestTimeout    time.Duration
	aiProviderTestMaxRetries int
	aiProviderTestRetryDelay time.Duration
	identity                 IdentityProvisioner
}

// New creates a workspace Service.
// encryptionKey is a 32-byte raw string or 64-char hex key for AES-256-GCM.
func New(pool *pgxpool.Pool, encryptionKey string, opts ...Option) (*Service, error) {
	cipher, err := crypto.NewCipherFromHex(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("workspace service: %w", err)
	}
	svc := &Service{
		pool:                     pool,
		cipher:                   cipher,
		aiProviderTestTimeout:    durationFromEnv("AI_PROVIDER_TEST_TIMEOUT_SECONDS", 60*time.Second),
		aiProviderTestMaxRetries: 2,
		aiProviderTestRetryDelay: 250 * time.Millisecond,
		identity:                 NewIdentityProvisioner(pool, os.Getenv("IDENTITY_SVC_URL")),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc, nil
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds * float64(time.Second))
}

// Pool returns the underlying database connection pool.
func (svc *Service) Pool() *pgxpool.Pool {
	return svc.pool
}

// generateToken creates a cryptographically random URL-safe token.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// pgxExecer adapts pgxpool.Pool to the audit.Writer's Exec interface.
type pgxExecer struct {
	pool *pgxpool.Pool
}

func (e *pgxExecer) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return e.pool.Exec(ctx, sql, args...)
}

// parseSettings safely unmarshals an accounts.settings JSONB blob into a
// map[string]any. If the bytes are empty or unparseable it returns an empty
// map so callers never need to nil-check or branch on length.
func parseSettings(raw []byte) map[string]any {
	out := make(map[string]any)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out) // silently ignore corrupt JSON
	}
	return out
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// boolSetting reads a boolean key from a parsed settings map. If the key is
// absent or not a bool, defaultVal is returned.
func boolSetting(settings map[string]any, key string, defaultVal bool) bool {
	v, ok := settings[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// marshalAny round-trips an any value through JSON to produce []byte suitable
// for parseSettings. Returns nil (which parseSettings handles gracefully) on error.
func marshalAny(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
