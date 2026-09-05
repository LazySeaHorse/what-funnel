package matrix

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// Credentials holds session keys needed to connect to Synapse.
type Credentials struct {
	HomeserverURL    string `json:"homeserver_url"`
	UserID           string `json:"user_id"`
	AccessToken      string `json:"access_token"`
	ManagementRoomID string `json:"management_room_id,omitempty"`
}

// ProvisioningConfig is held only in service configuration. Never serialize it
// to a channel record or return it from an API.
type ProvisioningConfig struct {
	HomeserverURL            string
	ServerName               string
	RegistrationSharedSecret string
}

// BridgeMessage is a safe, normalized view of a bridge management event.
type BridgeMessage struct {
	EventID  string
	Sender   string
	Type     string
	Body     string
	MediaURL string
}

// Adapter implements types.ChannelAdapter for Matrix client-server API.
type Adapter struct {
	mu           sync.RWMutex
	creds        map[string]Credentials
	status       map[string]types.ChannelStatus
	sentEventIDs map[string]bool
	client       *http.Client
	ctx          context.Context
	onInbound    func(types.InboundEvent)
	onOutbound   func(types.ExternalOutboundEvent)
	workers      sync.WaitGroup
}

// New creates a new Matrix Adapter instance.
func New() *Adapter {
	return &Adapter{
		creds:        make(map[string]Credentials),
		status:       make(map[string]types.ChannelStatus),
		sentEventIDs: make(map[string]bool),
		client:       &http.Client{Timeout: 45 * time.Second},
	}
}

// Configure registers credentials and status for a channel.
func (a *Adapter) Configure(channelID string, creds Credentials) {
	a.mu.Lock()
	a.creds[channelID] = creds
	a.status[channelID] = types.ChannelStatus{
		Status: "disconnected",
		Detail: "Configured, not started",
	}
	ctx := a.ctx
	publishInbound := a.onInbound
	publishExternalOutbound := a.onOutbound
	startSync := ctx != nil && ctx.Err() == nil && publishInbound != nil && publishExternalOutbound != nil
	if startSync {
		a.workers.Add(1)
	}
	a.mu.Unlock()

	// If already started, dynamically spin up the sync loop for the new channel
	if startSync {
		go func() {
			defer a.workers.Done()
			a.syncLoop(ctx, channelID, publishInbound, publishExternalOutbound)
		}()
	}
}

// Remove forgets a channel's Matrix token and stops any future sync iteration
// after the current request completes. Call this after a bridge logout.
func (a *Adapter) Remove(channelID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.creds, channelID)
	delete(a.status, channelID)
}

// Status returns the current status of a channel.
func (a *Adapter) Status(channelID string) types.ChannelStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if stat, ok := a.status[channelID]; ok {
		return stat
	}
	return types.ChannelStatus{
		Status: "disconnected",
		Detail: "Channel not configured in matrix adapter",
	}
}

// SetStatus updates status for a channel.
func (a *Adapter) SetStatus(channelID string, status, detail string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status[channelID] = types.ChannelStatus{
		Status: status,
		Detail: detail,
	}
}

// Start spawns a background sync loop for each configured channel and blocks.
func (a *Adapter) Start(ctx context.Context, publishInbound func(types.InboundEvent), publishExternalOutbound func(types.ExternalOutboundEvent)) error {
	a.mu.Lock()
	if a.ctx != nil {
		a.mu.Unlock()
		return fmt.Errorf("matrix adapter already started")
	}
	a.ctx = ctx
	a.onInbound = publishInbound
	a.onOutbound = publishExternalOutbound

	channelIDs := make([]string, 0, len(a.creds))
	for cid := range a.creds {
		channelIDs = append(channelIDs, cid)
	}
	a.workers.Add(len(channelIDs))
	a.mu.Unlock()

	for _, cid := range channelIDs {
		go func(channelID string) {
			defer a.workers.Done()
			a.syncLoop(ctx, channelID, publishInbound, publishExternalOutbound)
		}(cid)
	}

	<-ctx.Done()
	a.mu.Lock()
	a.ctx = nil
	a.onInbound = nil
	a.onOutbound = nil
	a.mu.Unlock()
	a.workers.Wait()
	return nil
}

// SendMessage delivers a normalized message to a Matrix room.
func (a *Adapter) SendMessage(ctx context.Context, channelID, externalThreadID string, msg types.NormalizedMessage) (string, error) {
	a.mu.RLock()
	creds, exists := a.creds[channelID]
	a.mu.RUnlock()
	if !exists || creds.HomeserverURL == "mock" || creds.HomeserverURL == "" {
		return fmt.Sprintf("mock-event-id-%d", time.Now().UnixNano()), nil
	}

	txid := fmt.Sprintf("tx-%d", time.Now().UnixNano())
	sendURL := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		creds.HomeserverURL, url.PathEscape(externalThreadID), url.PathEscape(txid))

	var msgtype string
	switch msg.ContentType {
	case "text":
		msgtype = "m.text"
	case "image":
		msgtype = "m.image"
	case "video":
		msgtype = "m.video"
	case "audio":
		msgtype = "m.audio"
	case "document":
		msgtype = "m.file"
	case "location":
		msgtype = "m.location"
	default:
		msgtype = "m.text"
	}

	payload := map[string]any{
		"msgtype": msgtype,
		"body":    msg.Text,
	}
	if msg.MediaURL != "" {
		payload["url"] = msg.MediaURL
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sendURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to send message, Matrix response code %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		EventID string `json:"event_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode matrix send response: %w", err)
	}

	a.mu.Lock()
	a.sentEventIDs[result.EventID] = true
	a.mu.Unlock()

	return result.EventID, nil
}

// ProvisionUser creates a least-privilege Matrix user for one channel using
// Synapse's shared-secret registration API. Conversation Service encrypts the
// returned access token before it is written to Postgres.
func ProvisionUser(ctx context.Context, cfg ProvisioningConfig, username string) (Credentials, error) {
	if cfg.HomeserverURL == "" || cfg.ServerName == "" || cfg.RegistrationSharedSecret == "" {
		return Credentials{}, fmt.Errorf("Matrix bridge provisioning is not configured")
	}
	base := strings.TrimRight(cfg.HomeserverURL, "/")
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/_synapse/admin/v1/register", nil)
	if err != nil {
		return Credentials{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Credentials{}, fmt.Errorf("request Matrix registration nonce: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Credentials{}, fmt.Errorf("request Matrix registration nonce: %s", strings.TrimSpace(string(body)))
	}
	var nonceResponse struct {
		Nonce string `json:"nonce"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&nonceResponse); err != nil || nonceResponse.Nonce == "" {
		if err == nil {
			err = fmt.Errorf("empty nonce")
		}
		return Credentials{}, fmt.Errorf("decode Matrix registration nonce: %w", err)
	}
	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return Credentials{}, fmt.Errorf("generate Matrix user password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(passwordBytes)
	payload, _ := json.Marshal(map[string]any{"nonce": nonceResponse.Nonce, "username": username, "password": password, "admin": false, "mac": registrationMAC(cfg.RegistrationSharedSecret, nonceResponse.Nonce, username, password)})
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, base+"/_synapse/admin/v1/register", bytes.NewReader(payload))
	if err != nil {
		return Credentials{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		return Credentials{}, fmt.Errorf("register Matrix bridge user: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Credentials{}, fmt.Errorf("register Matrix bridge user: %s", strings.TrimSpace(string(body)))
	}
	var registered struct {
		UserID      string `json:"user_id"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&registered); err != nil {
		return Credentials{}, fmt.Errorf("decode Matrix bridge registration: %w", err)
	}
	if registered.UserID == "" || registered.AccessToken == "" {
		return Credentials{}, fmt.Errorf("Matrix registration did not return a user access token")
	}
	return Credentials{HomeserverURL: base, UserID: registered.UserID, AccessToken: registered.AccessToken}, nil
}

func registrationMAC(secret, nonce, username, password string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	for i, value := range []string{nonce, username, password, "notadmin"} {
		if i > 0 {
			_, _ = mac.Write([]byte{0})
		}
		_, _ = mac.Write([]byte(value))
	}
	return fmt.Sprintf("%x", mac.Sum(nil))
}

// CreateManagementRoom starts the private control room used to speak to one
// mautrix bridge bot. It is not considered a customer conversation.
func (a *Adapter) CreateManagementRoom(ctx context.Context, creds Credentials, bridgeIdentity string) (string, error) {
	payload, _ := json.Marshal(map[string]any{"invite": []string{bridgeIdentity}, "is_direct": true, "preset": "trusted_private_chat", "name": "WhatFunnel channel setup"})
	endpoint := strings.TrimRight(creds.HomeserverURL, "/") + "/_matrix/client/v3/createRoom"
	var result struct {
		RoomID string `json:"room_id"`
	}
	if err := a.matrixJSON(ctx, creds, http.MethodPost, endpoint, payload, &result); err != nil {
		return "", fmt.Errorf("create bridge management room: %w", err)
	}
	if result.RoomID == "" {
		return "", fmt.Errorf("Matrix did not return a management room")
	}
	return result.RoomID, nil
}

// WaitForManagementRoomReady waits until the invited bridge bot has joined.
// Commands sent while the bot is still invited are present in room history,
// but mautrix does not process them as live command events.
func (a *Adapter) WaitForManagementRoomReady(ctx context.Context, creds Credentials, roomID, bridgeIdentity string) error {
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/state/m.room.member/%s", strings.TrimRight(creds.HomeserverURL, "/"), url.PathEscape(roomID), url.PathEscape(bridgeIdentity))
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		var member struct {
			Membership string `json:"membership"`
		}
		if err := a.matrixJSON(waitCtx, creds, http.MethodGet, endpoint, nil, &member); err == nil && member.Membership == "join" {
			return nil
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("bridge bot did not join the management room: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

// SendManagementCommand posts a command to a mautrix management room.
func (a *Adapter) SendManagementCommand(ctx context.Context, creds Credentials, roomID, command string) (string, error) {
	txID := fmt.Sprintf("wf-setup-%d", time.Now().UnixNano())
	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/send/m.room.message/%s", strings.TrimRight(creds.HomeserverURL, "/"), url.PathEscape(roomID), url.PathEscape(txID))
	payload, _ := json.Marshal(map[string]string{"msgtype": "m.text", "body": command})
	var result struct {
		EventID string `json:"event_id"`
	}
	if err := a.matrixJSON(ctx, creds, http.MethodPut, endpoint, payload, &result); err != nil {
		return "", fmt.Errorf("send bridge command: %w", err)
	}
	return result.EventID, nil
}

// ReadManagementMessagesSince reads bridge replies that were sent after the
// given command event. Matrix returns room history newest-first, so the command
// itself is the stable boundary between the current login attempt and stale
// replies from earlier attempts in the same management room.
func (a *Adapter) ReadManagementMessagesSince(ctx context.Context, creds Credentials, roomID, commandEventID string) ([]BridgeMessage, error) {
	if commandEventID == "" {
		return nil, fmt.Errorf("management command event ID is required")
	}

	baseURL := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/messages", strings.TrimRight(creds.HomeserverURL, "/"), url.PathEscape(roomID))
	messages := make([]BridgeMessage, 0, 30)
	from := ""

	for page := 0; page < 100; page++ {
		query := url.Values{"dir": {"b"}, "limit": {"30"}}
		if from != "" {
			query.Set("from", from)
		}

		var result struct {
			Chunk []struct {
				EventID string         `json:"event_id"`
				Sender  string         `json:"sender"`
				Type    string         `json:"type"`
				Content map[string]any `json:"content"`
			} `json:"chunk"`
			End string `json:"end"`
		}
		endpoint := baseURL + "?" + query.Encode()
		if err := a.matrixJSON(ctx, creds, http.MethodGet, endpoint, nil, &result); err != nil {
			return nil, fmt.Errorf("read bridge management messages: %w", err)
		}

		for _, event := range result.Chunk {
			if event.EventID == commandEventID {
				return messages, nil
			}
			body, _ := event.Content["body"].(string)
			mediaURL, _ := event.Content["url"].(string)
			messages = append(messages, BridgeMessage{EventID: event.EventID, Sender: event.Sender, Type: event.Type, Body: body, MediaURL: mediaURL})
		}

		if result.End == "" || result.End == from || len(result.Chunk) == 0 {
			break
		}
		from = result.End
	}

	return nil, fmt.Errorf("management command event %s was not found in room history", commandEventID)
}

// ReadManagementMessages reads the most recent management-room events. New
// setup lifecycle code should prefer ReadManagementMessagesSince so replies
// from separate command attempts cannot be mixed.
func (a *Adapter) ReadManagementMessages(ctx context.Context, creds Credentials, roomID string) ([]BridgeMessage, error) {
	endpoint := fmt.Sprintf("%s/_matrix/client/v3/rooms/%s/messages?dir=b&limit=30", strings.TrimRight(creds.HomeserverURL, "/"), url.PathEscape(roomID))
	var result struct {
		Chunk []struct {
			EventID string         `json:"event_id"`
			Sender  string         `json:"sender"`
			Type    string         `json:"type"`
			Content map[string]any `json:"content"`
		} `json:"chunk"`
	}
	if err := a.matrixJSON(ctx, creds, http.MethodGet, endpoint, nil, &result); err != nil {
		return nil, fmt.Errorf("read bridge management messages: %w", err)
	}
	messages := make([]BridgeMessage, 0, len(result.Chunk))
	for _, event := range result.Chunk {
		body, _ := event.Content["body"].(string)
		mediaURL, _ := event.Content["url"].(string)
		messages = append(messages, BridgeMessage{EventID: event.EventID, Sender: event.Sender, Type: event.Type, Body: body, MediaURL: mediaURL})
	}
	return messages, nil
}

// DownloadMedia retrieves a bridge-issued QR image using the server-side
// Matrix token. The token itself is never sent to the browser.
func (a *Adapter) DownloadMedia(ctx context.Context, creds Credentials, mxcURL string) ([]byte, string, error) {
	parsed, err := url.Parse(mxcURL)
	if err != nil || parsed.Scheme != "mxc" || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return nil, "", fmt.Errorf("invalid Matrix media URL")
	}
	mediaID := strings.Trim(parsed.Path, "/")
	endpoint := fmt.Sprintf("%s/_matrix/client/v1/media/download/%s/%s", strings.TrimRight(creds.HomeserverURL, "/"), url.PathEscape(parsed.Host), url.PathEscape(mediaID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("download Matrix media: %s", strings.TrimSpace(string(body)))
	}
	limited := io.LimitReader(resp.Body, 2*1024*1024+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if len(body) > 2*1024*1024 {
		return nil, "", fmt.Errorf("bridge QR image exceeds 2 MiB")
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/png"
	}
	return body, contentType, nil
}

func (a *Adapter) matrixJSON(ctx context.Context, creds Credentials, method, endpoint string, body []byte, target any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Matrix response %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if target != nil {
		return json.NewDecoder(resp.Body).Decode(target)
	}
	return nil
}

func (a *Adapter) syncLoop(ctx context.Context, channelID string, publishInbound func(types.InboundEvent), publishExternalOutbound func(types.ExternalOutboundEvent)) {
	a.mu.RLock()
	creds, exists := a.creds[channelID]
	a.mu.RUnlock()
	if !exists {
		return
	}

	if creds.HomeserverURL == "mock" || creds.HomeserverURL == "" {
		a.SetStatus(channelID, "connected", "Mock active connection")
		<-ctx.Done()
		a.SetStatus(channelID, "disconnected", "Context cancelled")
		return
	}

	a.SetStatus(channelID, "connected", "Syncing events from homeserver")

	since := ""
	failCount := 0

	for {
		select {
		case <-ctx.Done():
			a.SetStatus(channelID, "disconnected", "Context cancelled")
			return
		default:
		}
		a.mu.RLock()
		_, configured := a.creds[channelID]
		a.mu.RUnlock()
		if !configured {
			return
		}

		syncURL := fmt.Sprintf("%s/_matrix/client/v3/sync?timeout=30000", creds.HomeserverURL)
		if since != "" {
			syncURL = fmt.Sprintf("%s&since=%s", syncURL, url.QueryEscape(since))
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, syncURL, nil)
		if err != nil {
			a.SetStatus(channelID, "error", fmt.Sprintf("failed to create request: %v", err))
			return
		}
		req.Header.Set("Authorization", "Bearer "+creds.AccessToken)

		resp, err := a.client.Do(req)
		if err != nil {
			failCount++
			if failCount > 5 {
				a.SetStatus(channelID, "error", fmt.Sprintf("homeserver connection failed: %v", err))
			}
			waitForRetry(ctx, 5*time.Second)
			continue
		}
		failCount = 0

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			waitForRetry(ctx, time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			a.SetStatus(channelID, "error", fmt.Sprintf("sync HTTP error %d: %s", resp.StatusCode, string(bodyBytes)))
			waitForRetry(ctx, 5*time.Second)
			continue
		}

		var syncResp struct {
			NextBatch string `json:"next_batch"`
			Rooms     struct {
				Join map[string]struct {
					Timeline struct {
						Events []struct {
							Type           string         `json:"type"`
							Sender         string         `json:"sender"`
							EventID        string         `json:"event_id"`
							OriginServerTs int64          `json:"origin_server_ts"`
							Content        map[string]any `json:"content"`
						} `json:"events"`
					} `json:"timeline"`
				} `json:"join"`
			} `json:"rooms"`
		}

		if err := json.Unmarshal(bodyBytes, &syncResp); err != nil {
			waitForRetry(ctx, 2*time.Second)
			continue
		}

		since = syncResp.NextBatch

		for roomID, roomData := range syncResp.Rooms.Join {
			if roomID == creds.ManagementRoomID {
				continue
			}
			for _, ev := range roomData.Timeline.Events {
				if ev.Sender == creds.UserID {
					a.mu.RLock()
					isEcho := a.sentEventIDs[ev.EventID]
					a.mu.RUnlock()
					if isEcho {
						a.mu.Lock()
						delete(a.sentEventIDs, ev.EventID)
						a.mu.Unlock()
						continue
					}

					if ev.Type == "m.room.message" {
						inboundEv := a.NormalizeEvent(channelID, roomID, ev.EventID, ev.Sender, ev.Type, ev.OriginServerTs, ev.Content)
						publishExternalOutbound(types.ExternalOutboundEvent{
							ChannelID:         inboundEv.ChannelID,
							ExternalThreadID:  inboundEv.ExternalThreadID,
							Message:           inboundEv.Message,
							ExternalMessageID: inboundEv.Message.ExternalMessageID,
							Timestamp:         inboundEv.Timestamp,
						})
					}
					continue
				}

				if ev.Type != "m.room.message" && ev.Type != "m.reaction" && ev.Type != "net.maunium.whatsapp.contact" {
					continue
				}

				inboundEv := a.NormalizeEvent(channelID, roomID, ev.EventID, ev.Sender, ev.Type, ev.OriginServerTs, ev.Content)
				publishInbound(inboundEv)
			}
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// NormalizeEvent maps Matrix event payloads into types.InboundEvent structures.
func (a *Adapter) NormalizeEvent(
	channelID string,
	roomID string,
	eventID string,
	sender string,
	eventType string,
	timestampMs int64,
	content map[string]any,
) types.InboundEvent {
	t := time.UnixMilli(timestampMs)

	cRef := types.ContactRef{
		ExternalIdentity: sender,
		DisplayName:      sender,
	}

	normMsg := types.NormalizedMessage{
		ExternalMessageID: eventID,
	}

	if eventType == "m.reaction" {
		normMsg.ContentType = "reaction"
		if relatesTo, ok := content["m.relates_to"].(map[string]any); ok {
			if key, kOk := relatesTo["key"].(string); kOk {
				normMsg.Text = key
			}
			if relEventID, rOk := relatesTo["event_id"].(string); rOk {
				normMsg.ReplyToExternalID = relEventID
			}
		}
		return types.InboundEvent{
			ChannelID:        channelID,
			ExternalThreadID: roomID,
			Contact:          cRef,
			Message:          normMsg,
			Timestamp:        t,
		}
	}

	if eventType == "net.maunium.whatsapp.contact" {
		normMsg.ContentType = "contact"
		if vcard, ok := content["vcard"].(string); ok {
			normMsg.Text = vcard
		}
		return types.InboundEvent{
			ChannelID:        channelID,
			ExternalThreadID: roomID,
			Contact:          cRef,
			Message:          normMsg,
			Timestamp:        t,
		}
	}

	msgtype, _ := content["msgtype"].(string)
	body, _ := content["body"].(string)
	url, _ := content["url"].(string)

	normMsg.Text = body
	normMsg.MediaURL = url

	if relatesTo, ok := content["m.relates_to"].(map[string]any); ok {
		if inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]any); ok {
			if parentID, ok := inReplyTo["event_id"].(string); ok {
				normMsg.ReplyToExternalID = parentID
			}
		}
	}

	switch msgtype {
	case "m.text", "m.notice", "m.emplaced":
		normMsg.ContentType = "text"
	case "m.image":
		normMsg.ContentType = "image"
	case "m.video":
		normMsg.ContentType = "video"
	case "m.audio":
		normMsg.ContentType = "audio"
	case "m.file":
		normMsg.ContentType = "document"
	case "m.location":
		normMsg.ContentType = "location"
	default:
		normMsg.ContentType = "document"
	}

	return types.InboundEvent{
		ChannelID:        channelID,
		ExternalThreadID: roomID,
		Contact:          cRef,
		Message:          normMsg,
		Timestamp:        t,
	}
}

var _ types.ChannelAdapter = (*Adapter)(nil)
