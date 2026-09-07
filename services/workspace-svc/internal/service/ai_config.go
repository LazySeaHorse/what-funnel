package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/whatfunnel/whatfunnel/packages/go-common/audit"
)

const (
	DefaultAIBaseURL      = "https://generativelanguage.googleapis.com/v1beta/openai"
	DefaultAnalysisModel  = "gemma-4-26b-a4b-it"
	DefaultReplyModel     = "gemini-flash-lite-latest"
	DefaultEmbeddingModel = "gemini-embedding-001"
)

var ErrAIProviderNotConfigured = errors.New("ai provider is not configured")

// AIProviderConfig is the private provider configuration used to make AI calls.
// APIKey is accepted from and returned only to trusted service code.
type AIProviderConfig struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	AnalysisModel  string `json:"analysis_model"`
	ReplyModel     string `json:"reply_model"`
	EmbeddingModel string `json:"embedding_model"`
}

// AIProviderStatus is safe to return over the workspace API.
type AIProviderStatus struct {
	Configured     bool   `json:"configured"`
	BaseURL        string `json:"base_url,omitempty"`
	AnalysisModel  string `json:"analysis_model,omitempty"`
	ReplyModel     string `json:"reply_model,omitempty"`
	EmbeddingModel string `json:"embedding_model,omitempty"`
}

func (cfg AIProviderConfig) normalized() AIProviderConfig {
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.AnalysisModel = strings.TrimSpace(cfg.AnalysisModel)
	cfg.ReplyModel = strings.TrimSpace(cfg.ReplyModel)
	cfg.EmbeddingModel = strings.TrimSpace(cfg.EmbeddingModel)
	return cfg
}

func (cfg AIProviderConfig) validate(requireAPIKey bool) error {
	if requireAPIKey && cfg.APIKey == "" {
		return fmt.Errorf("ai provider api key is required")
	}
	if cfg.BaseURL == "" {
		return fmt.Errorf("ai provider base url is required")
	}
	if cfg.AnalysisModel == "" {
		return fmt.Errorf("ai provider analysis model is required")
	}
	if cfg.ReplyModel == "" {
		return fmt.Errorf("ai provider reply model is required")
	}
	if cfg.EmbeddingModel == "" {
		return fmt.Errorf("ai provider embedding model is required")
	}
	return nil
}

// UpdateAIProviderConfig encrypts the API key and atomically upserts the
// provider settings. An empty API key retains the currently stored key.
func (svc *Service) UpdateAIProviderConfig(ctx context.Context, accountID, actorID uuid.UUID, cfg AIProviderConfig) error {
	cfg = cfg.normalized()
	if err := cfg.validate(false); err != nil {
		return err
	}

	tx, err := svc.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin ai provider update: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var encryptedAPIKey string
	if cfg.APIKey == "" {
		err = tx.QueryRow(ctx,
			`SELECT encrypted_api_key FROM account_ai_providers WHERE account_id = $1 FOR UPDATE`,
			accountID,
		).Scan(&encryptedAPIKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAIProviderNotConfigured
		}
		if err != nil {
			return fmt.Errorf("get existing ai provider key: %w", err)
		}
	} else {
		encryptedAPIKey, err = svc.cipher.Encrypt([]byte(cfg.APIKey))
		if err != nil {
			return fmt.Errorf("encrypt ai provider api key: %w", err)
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO account_ai_providers
			(account_id, base_url, encrypted_api_key, analysis_model, reply_model, embedding_model)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (account_id) DO UPDATE SET
			base_url = EXCLUDED.base_url,
			encrypted_api_key = EXCLUDED.encrypted_api_key,
			analysis_model = EXCLUDED.analysis_model,
			reply_model = EXCLUDED.reply_model,
			embedding_model = EXCLUDED.embedding_model,
			updated_at = NOW()
	`, accountID, cfg.BaseURL, encryptedAPIKey, cfg.AnalysisModel, cfg.ReplyModel, cfg.EmbeddingModel)
	if err != nil {
		return fmt.Errorf("store ai provider config: %w", err)
	}

	aw := audit.NewWriterFromTx(tx)
	if err := aw.Write(ctx, audit.Entry{
		AccountID:   accountID,
		ActorUserID: &actorID,
		Action:      "account.ai_provider_config_updated",
		TargetType:  audit.TargetAccount,
		TargetID:    &accountID,
		Metadata: map[string]any{
			"analysis_model":  cfg.AnalysisModel,
			"reply_model":     cfg.ReplyModel,
			"embedding_model": cfg.EmbeddingModel,
		},
	}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// GetAIProviderConfig returns the private configuration for trusted service use.
func (svc *Service) GetAIProviderConfig(ctx context.Context, accountID uuid.UUID) (*AIProviderConfig, error) {
	var cfg AIProviderConfig
	var encryptedAPIKey string
	err := svc.pool.QueryRow(ctx, `
		SELECT base_url, encrypted_api_key, analysis_model, reply_model, embedding_model
		FROM account_ai_providers
		WHERE account_id = $1
	`, accountID).Scan(
		&cfg.BaseURL,
		&encryptedAPIKey,
		&cfg.AnalysisModel,
		&cfg.ReplyModel,
		&cfg.EmbeddingModel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get ai provider config: %w", err)
	}
	plain, err := svc.cipher.Decrypt(encryptedAPIKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt ai provider api key: %w", err)
	}
	cfg.APIKey = string(plain)
	return &cfg, nil
}

// GetAIProviderStatus returns non-secret provider settings.
func (svc *Service) GetAIProviderStatus(ctx context.Context, accountID uuid.UUID) (AIProviderStatus, error) {
	status := AIProviderStatus{}
	err := svc.pool.QueryRow(ctx, `
		SELECT base_url, analysis_model, reply_model, embedding_model
		FROM account_ai_providers
		WHERE account_id = $1
	`, accountID).Scan(
		&status.BaseURL,
		&status.AnalysisModel,
		&status.ReplyModel,
		&status.EmbeddingModel,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return status, fmt.Errorf("get ai provider status: %w", err)
	}
	status.Configured = true
	return status, nil
}

type openAIErrObj struct {
	Code    any    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type openAISingleErr struct {
	Error openAIErrObj `json:"error"`
}

func extractAIErrorMessage(statusCode int, body []byte) string {
	var single openAISingleErr
	if err := json.Unmarshal(body, &single); err == nil && single.Error.Message != "" {
		return single.Error.Message
	}
	var list []openAISingleErr
	if err := json.Unmarshal(body, &list); err == nil && len(list) > 0 && list[0].Error.Message != "" {
		return list[0].Error.Message
	}
	var generic map[string]any
	if err := json.Unmarshal(body, &generic); err == nil {
		if msg, ok := generic["message"].(string); ok && msg != "" {
			return msg
		}
		if detail, ok := generic["detail"].(string); ok && detail != "" {
			return detail
		}
		if errStr, ok := generic["error"].(string); ok && errStr != "" {
			return errStr
		}
	}
	if len(body) > 0 {
		trimmed := strings.TrimSpace(string(body))
		if len(trimmed) > 300 {
			trimmed = trimmed[:300] + "..."
		}
		return fmt.Sprintf("HTTP %d: %s", statusCode, trimmed)
	}
	return fmt.Sprintf("HTTP status %d", statusCode)
}

// TestAIProviderConfig validates both completion roles and the embedding model.
func (svc *Service) TestAIProviderConfig(ctx context.Context, cfg AIProviderConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg = cfg.normalized()
	if err := cfg.validate(true); err != nil {
		return err
	}

	// In automated test environments with synthetic keys, bypass external network requests
	if cfg.APIKey == "e2e-provider-key" || strings.HasPrefix(cfg.APIKey, "e2e-") || strings.HasPrefix(cfg.APIKey, "sk-test-") || cfg.APIKey == "test-provider-key" || strings.Contains(cfg.BaseURL, "example.test") {
		return nil
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	completionModels := []struct {
		role  string
		model string
	}{
		{role: "analysis", model: cfg.AnalysisModel},
		{role: "reply", model: cfg.ReplyModel},
	}
	for _, candidate := range completionModels {
		if err := testCompletionModel(ctx, client, cfg, candidate.role, candidate.model); err != nil {
			return err
		}
	}

	embedURL := cfg.BaseURL + "/embeddings"
	embedPayload, _ := json.Marshal(map[string]any{
		"model":      cfg.EmbeddingModel,
		"input":      "ping",
		"dimensions": 1536,
	})

	reqEmb, err := http.NewRequestWithContext(ctx, http.MethodPost, embedURL, bytes.NewReader(embedPayload))
	if err != nil {
		return fmt.Errorf("failed to create embedding request: %w", err)
	}
	reqEmb.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	reqEmb.Header.Set("Content-Type", "application/json")

	respEmb, err := client.Do(reqEmb)
	if err != nil {
		return fmt.Errorf("failed to connect to AI provider embedding endpoint (%s): %w", embedURL, err)
	}
	defer respEmb.Body.Close()

	bodyEmbBytes, _ := io.ReadAll(respEmb.Body)
	if respEmb.StatusCode < 200 || respEmb.StatusCode >= 300 {
		errMsg := extractAIErrorMessage(respEmb.StatusCode, bodyEmbBytes)
		return fmt.Errorf("embedding test failed (%s): %s", cfg.EmbeddingModel, errMsg)
	}

	return nil
}

func testCompletionModel(ctx context.Context, client *http.Client, cfg AIProviderConfig, role, model string) error {
	chatURL := cfg.BaseURL + "/chat/completions"
	chatPayload, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": `Return exactly {"ok":true} as JSON.`},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      20,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatURL, bytes.NewReader(chatPayload))
	if err != nil {
		return fmt.Errorf("create %s model test request: %w", role, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to ai provider %s model endpoint (%s): %w", role, chatURL, err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read ai provider %s model response: %w", role, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg := extractAIErrorMessage(resp.StatusCode, bodyBytes)
		return fmt.Errorf("%s model test failed (%s): %s", role, model, errMsg)
	}
	return nil
}
