package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

func (h *HTTPIdentityProvisioner) CreateUser(ctx context.Context, accountID, actorID uuid.UUID, req CreateUserRequest) (*CreateUserResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal create user request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/auth/users", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	h.setAuthHeaders(httpReq, accountID, actorID)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("identity-svc create user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return nil, fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}

	var res CreateUserResult
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("decode create user response: %w", err)
	}
	return &res, nil
}

func (h *HTTPIdentityProvisioner) ResetUserPassword(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newPassword string) error {
	body, err := json.Marshal(map[string]string{"password": newPassword})
	if err != nil {
		return fmt.Errorf("marshal reset password request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/auth/users/%s/password", h.baseURL, targetUserID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	h.setAuthHeaders(httpReq, accountID, actorID)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("identity-svc reset password: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}
	return nil
}

func (h *HTTPIdentityProvisioner) DeleteUser(ctx context.Context, accountID, actorID, targetUserID uuid.UUID) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/auth/users/%s", h.baseURL, targetUserID), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	h.setAuthHeaders(httpReq, accountID, actorID)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("identity-svc delete user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}
	return nil
}

func (h *HTTPIdentityProvisioner) ChangeUserRole(ctx context.Context, accountID, actorID, targetUserID uuid.UUID, newRole string) error {
	body, err := json.Marshal(map[string]string{"role": newRole})
	if err != nil {
		return fmt.Errorf("marshal change role request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/auth/users/%s/role", h.baseURL, targetUserID), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	h.setAuthHeaders(httpReq, accountID, actorID)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("identity-svc change role: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("identity-svc: %s", errResp.Error)
		}
		return fmt.Errorf("identity-svc returned status %d", resp.StatusCode)
	}
	return nil
}
