package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/middleware"
	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// IdentityProvisioner decouples workspace-svc from direct user credential hashing and identity storage.
// It delegates user provisioning, password resets, role changes, and identity deletion to the Identity domain.
type IdentityProvisioner interface {
	CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error)
	ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error
	DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error
	ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error
}

// ErrIdentityNotConfigured is returned by user lifecycle operations when no
// identity provisioner is available (IDENTITY_SVC_URL unset).
var ErrIdentityNotConfigured = errors.New("identity service is not configured (IDENTITY_SVC_URL)")

// NewIdentityProvisioner returns an HTTPIdentityProvisioner for identityURL.
// Production code always delegates user lifecycle to identity-svc over HTTP;
// there is no direct-database fallback.
func NewIdentityProvisioner(identityURL string, secret ...string) (*HTTPIdentityProvisioner, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(identityURL), "/")
	if trimmed == "" {
		return nil, ErrIdentityNotConfigured
	}
	return NewHTTPIdentityProvisioner(trimmed, secret...), nil
}

// unconfiguredIdentityProvisioner fails every operation with ErrIdentityNotConfigured.
type unconfiguredIdentityProvisioner struct{}

func (unconfiguredIdentityProvisioner) CreateUser(context.Context, uuid.UUID, uuid.UUID, CreateUserRequest) (*CreateUserResult, error) {
	return nil, ErrIdentityNotConfigured
}
func (unconfiguredIdentityProvisioner) ResetUserPassword(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) error {
	return ErrIdentityNotConfigured
}
func (unconfiguredIdentityProvisioner) DeleteUser(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return ErrIdentityNotConfigured
}
func (unconfiguredIdentityProvisioner) ChangeUserRole(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) error {
	return ErrIdentityNotConfigured
}

// HTTPIdentityProvisioner calls identity-svc over HTTP to manage user credentials.
type HTTPIdentityProvisioner struct {
	baseURL string
	client  *http.Client
	secret  string
}

// NewHTTPIdentityProvisioner creates an HTTPIdentityProvisioner for the given identity-svc base URL.
// Optional secret can be provided; if omitted or empty, it falls back to middleware.InternalServiceSecret().
func NewHTTPIdentityProvisioner(baseURL string, secret ...string) *HTTPIdentityProvisioner {
	sec := ""
	if len(secret) > 0 {
		sec = strings.TrimSpace(secret[0])
	}
	return &HTTPIdentityProvisioner{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 10 * time.Second},
		secret:  sec,
	}
}

func (h *HTTPIdentityProvisioner) getSecret() string {
	if h.secret != "" {
		return h.secret
	}
	return middleware.InternalServiceSecret()
}

func (h *HTTPIdentityProvisioner) setAuthHeaders(httpReq *http.Request, accountID, actorID uuid.UUID) {
	if sec := h.getSecret(); sec != "" {
		httpReq.Header.Set("X-Internal-Token", sec)
	}
	httpReq.Header.Set("X-Account-ID", accountID.String())
	httpReq.Header.Set("X-User-ID", actorID.String())
	httpReq.Header.Set("X-Actor-ID", actorID.String())
	httpReq.Header.Set("X-User-Role", types.RoleManager)
}

// do sends one authenticated request to identity-svc. A non-nil body is sent as
// JSON and a non-nil out receives the decoded JSON response. Statuses in
// okStatus count as success (default: 200); any other status is turned into an
// error carrying identity-svc's {"error": ...} message when it has one. op
// names the operation in error messages.
func (h *HTTPIdentityProvisioner) do(ctx context.Context, op, method, path string, accountID, actorID uuid.UUID, body, out any, okStatus ...int) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal %s request: %w", op, err)
		}
		reader = bytes.NewReader(raw)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	h.setAuthHeaders(httpReq, accountID, actorID)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("identity-svc %s: %w", op, err)
	}
	defer resp.Body.Close()

	if len(okStatus) == 0 {
		okStatus = []int{http.StatusOK}
	}
	if !slices.Contains(okStatus, resp.StatusCode) {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode %s response: %w", op, err)
		}
	}
	return nil
}

func (h *HTTPIdentityProvisioner) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error) {
	var res CreateUserResult
	if err := h.do(ctx, "create user", http.MethodPost, "/auth/users", accountID, actorID, req, &res, http.StatusCreated, http.StatusOK); err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *HTTPIdentityProvisioner) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	return h.do(ctx, "reset password", http.MethodPut, fmt.Sprintf("/auth/users/%s/password", targetUserID), accountID, actorID,
		map[string]string{"password": newPassword}, nil)
}

func (h *HTTPIdentityProvisioner) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	return h.do(ctx, "delete user", http.MethodDelete, fmt.Sprintf("/auth/users/%s", targetUserID), accountID, actorID, nil, nil)
}

func (h *HTTPIdentityProvisioner) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	return h.do(ctx, "change role", http.MethodPut, fmt.Sprintf("/auth/users/%s/role", targetUserID), accountID, actorID,
		map[string]string{"role": newRole}, nil)
}
