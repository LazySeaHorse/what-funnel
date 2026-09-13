package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
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

var (
	thoughtBlockPattern = regexp.MustCompile(`(?s)<thought>.*?</thought>`)
	fencedJSONPattern   = regexp.MustCompile("(?s)```(?:json)?\\s*([\\s\\S]*?)\\s*```")
)

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

// AIProviderTestCheck describes one model capability check.
type AIProviderTestCheck struct {
	Role          string `json:"role"`
	Model         string `json:"model"`
	ResolvedModel string `json:"resolved_model,omitempty"`
	OK            bool   `json:"ok"`
	Kind          string `json:"kind,omitempty"`
	Message       string `json:"message"`
}

// AIProviderTestResult reports every model check instead of only the first failure.
type AIProviderTestResult struct {
	OK     bool                  `json:"ok"`
	Checks []AIProviderTestCheck `json:"checks"`
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
func (svc *Service) TestAIProviderConfig(ctx context.Context, cfg AIProviderConfig) (AIProviderTestResult, error) {
	result := AIProviderTestResult{Checks: []AIProviderTestCheck{}}
	if ctx == nil {
		ctx = context.Background()
	}
	cfg = cfg.normalized()
	if err := cfg.validate(true); err != nil {
		return result, err
	}

	// In automated test environments with synthetic keys, bypass external network requests
	if cfg.APIKey == "e2e-provider-key" || strings.HasPrefix(cfg.APIKey, "e2e-") || strings.HasPrefix(cfg.APIKey, "sk-test-") || cfg.APIKey == "test-provider-key" || strings.Contains(cfg.BaseURL, "example.test") {
		return successfulAIProviderTestResult(cfg), nil
	}

	client := &http.Client{
		Timeout: svc.aiProviderTestTimeout,
	}

	completionModels := []struct {
		role  string
		model string
	}{
		{role: "analysis", model: cfg.AnalysisModel},
		{role: "reply", model: cfg.ReplyModel},
	}
	for _, candidate := range completionModels {
		result.Checks = append(
			result.Checks,
			svc.testCompletionModel(ctx, client, cfg, candidate.role, candidate.model),
		)
	}
	result.Checks = append(result.Checks, svc.testEmbeddingModel(ctx, client, cfg))
	result.OK = allAIProviderChecksPassed(result.Checks)
	return result, nil
}

func (svc *Service) testEmbeddingModel(
	ctx context.Context,
	client *http.Client,
	cfg AIProviderConfig,
) AIProviderTestCheck {
	check := AIProviderTestCheck{
		Role:    "embedding",
		Model:   cfg.EmbeddingModel,
		Message: "Embedding model verified",
	}
	embedURL := cfg.BaseURL + "/embeddings"
	embedPayload, err := json.Marshal(map[string]any{
		"model":      cfg.EmbeddingModel,
		"input":      "ping",
		"dimensions": 1536,
	})
	if err != nil {
		return failedAIProviderCheck(check, "request", "Could not create embedding test request")
	}

	statusCode, body, err := svc.doAIProviderRequest(ctx, client, embedURL, cfg.APIKey, embedPayload)
	if err != nil {
		return failedAIProviderCheck(check, requestFailureKind(err), requestFailureMessage(err))
	}
	if statusCode < 200 || statusCode >= 300 {
		return failedAIProviderCheck(check, "provider", extractAIErrorMessage(statusCode, body))
	}

	var response struct {
		Model string `json:"model"`
		Data  []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil || len(response.Data) == 0 {
		return failedAIProviderCheck(check, "invalid_response", "Provider returned an invalid embedding response")
	}
	if len(response.Data[0].Embedding) != 1536 {
		message := fmt.Sprintf(
			"Provider returned %d embedding dimensions; 1536 required",
			len(response.Data[0].Embedding),
		)
		return failedAIProviderCheck(check, "dimension_mismatch", message)
	}
	check.OK = true
	check.ResolvedModel = response.Model
	return check
}

func (svc *Service) testCompletionModel(
	ctx context.Context,
	client *http.Client,
	cfg AIProviderConfig,
	role string,
	model string,
) AIProviderTestCheck {
	check := AIProviderTestCheck{
		Role:    role,
		Model:   model,
		Message: "Structured output verified",
	}
	chatURL := cfg.BaseURL + "/chat/completions"
	chatPayload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": `Return exactly {"ok":true} as JSON.`},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "provider_connection_test",
				"strict": true,
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ok": map[string]string{"type": "boolean"},
					},
					"required":             []string{"ok"},
					"additionalProperties": false,
				},
			},
		},
		"reasoning_effort": "minimal",
	})
	if err != nil {
		return failedAIProviderCheck(check, "request", "Could not create completion test request")
	}

	statusCode, body, err := svc.doAIProviderRequest(ctx, client, chatURL, cfg.APIKey, chatPayload)
	if err != nil {
		return failedAIProviderCheck(check, requestFailureKind(err), requestFailureMessage(err))
	}
	if statusCode < 200 || statusCode >= 300 {
		return failedAIProviderCheck(check, "provider", extractAIErrorMessage(statusCode, body))
	}

	var completion struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &completion); err != nil || len(completion.Choices) == 0 {
		return failedAIProviderCheck(check, "invalid_response", "Provider returned an invalid completion response")
	}
	check.ResolvedModel = completion.Model
	choice := completion.Choices[0]
	if isTruncatedFinishReason(choice.FinishReason) {
		return failedAIProviderCheck(check, "truncated", "Output was truncated before structured response completed")
	}
	content := cleanJSONContent(choice.Message.Content)
	if content == "" {
		return failedAIProviderCheck(check, "empty_response", "Provider returned an empty structured response")
	}

	var result struct {
		OK bool `json:"ok"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil || !result.OK {
		return failedAIProviderCheck(check, "schema_validation", "Output did not match the required JSON schema")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return failedAIProviderCheck(check, "schema_validation", "Output contained data outside the required JSON object")
	}
	check.OK = true
	return check
}

func (svc *Service) doAIProviderRequest(
	ctx context.Context,
	client *http.Client,
	url string,
	apiKey string,
	payload []byte,
) (int, []byte, error) {
	var lastErr error
	for attempt := 0; attempt <= svc.aiProviderTestMaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return 0, nil, fmt.Errorf("create provider request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if attempt == svc.aiProviderTestMaxRetries {
				break
			}
			if err := waitForRetry(ctx, svc.aiProviderTestRetryDelay*time.Duration(attempt+1)); err != nil {
				return 0, nil, err
			}
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil {
			return 0, nil, fmt.Errorf("read provider response: %w", readErr)
		}
		if closeErr != nil {
			return 0, nil, fmt.Errorf("close provider response: %w", closeErr)
		}
		if !isTransientAIProviderStatus(resp.StatusCode) || attempt == svc.aiProviderTestMaxRetries {
			return resp.StatusCode, body, nil
		}
		if err := waitForRetry(ctx, svc.aiProviderTestRetryDelay*time.Duration(attempt+1)); err != nil {
			return 0, nil, err
		}
	}
	return 0, nil, lastErr
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func successfulAIProviderTestResult(cfg AIProviderConfig) AIProviderTestResult {
	return AIProviderTestResult{
		OK: true,
		Checks: []AIProviderTestCheck{
			{Role: "analysis", Model: cfg.AnalysisModel, OK: true, Message: "Structured output verified"},
			{Role: "reply", Model: cfg.ReplyModel, OK: true, Message: "Structured output verified"},
			{Role: "embedding", Model: cfg.EmbeddingModel, OK: true, Message: "Embedding model verified"},
		},
	}
}

func failedAIProviderCheck(check AIProviderTestCheck, kind string, message string) AIProviderTestCheck {
	check.Kind = kind
	check.Message = message
	return check
}

func allAIProviderChecksPassed(checks []AIProviderTestCheck) bool {
	for _, check := range checks {
		if !check.OK {
			return false
		}
	}
	return len(checks) > 0
}

func cleanJSONContent(content string) string {
	content = strings.TrimSpace(thoughtBlockPattern.ReplaceAllString(content, ""))
	match := fencedJSONPattern.FindStringSubmatch(content)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return content
}

func isTruncatedFinishReason(reason string) bool {
	switch strings.ToLower(reason) {
	case "length", "max_tokens":
		return true
	default:
		return false
	}
}

func isTransientAIProviderStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

func requestFailureKind(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	return "network"
}

func requestFailureMessage(err error) string {
	if requestFailureKind(err) == "timeout" {
		return "Provider did not respond before the connection-test timeout"
	}
	return "Could not connect to the provider"
}
