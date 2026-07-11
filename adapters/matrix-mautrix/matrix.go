package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

// Credentials holds session keys needed to connect to Synapse.
type Credentials struct {
	HomeserverURL string `json:"homeserver_url"`
	UserID        string `json:"user_id"`
	AccessToken   string `json:"access_token"`
}

// Adapter implements types.ChannelAdapter for Matrix client-server API.
type Adapter struct {
	mu     sync.RWMutex
	creds  map[string]Credentials
	status map[string]types.ChannelStatus
	client *http.Client
}

// New creates a new Matrix Adapter instance.
func New() *Adapter {
	return &Adapter{
		creds:  make(map[string]Credentials),
		status: make(map[string]types.ChannelStatus),
		client: &http.Client{Timeout: 45 * time.Second},
	}
}

// Configure registers credentials and status for a channel.
func (a *Adapter) Configure(channelID string, creds Credentials) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.creds[channelID] = creds
	a.status[channelID] = types.ChannelStatus{
		Status: "disconnected",
		Detail: "Configured, not started",
	}
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
func (a *Adapter) Start(ctx context.Context, publish func(types.InboundEvent)) error {
	a.mu.RLock()
	channelIDs := make([]string, 0, len(a.creds))
	for cid := range a.creds {
		channelIDs = append(channelIDs, cid)
	}
	a.mu.RUnlock()

	var wg sync.WaitGroup
	for _, cid := range channelIDs {
		wg.Add(1)
		go func(channelID string) {
			defer wg.Done()
			a.syncLoop(ctx, channelID, publish)
		}(cid)
	}

	<-ctx.Done()
	wg.Wait()
	return nil
}

// SendMessage delivers a normalized message to a Matrix room.
func (a *Adapter) SendMessage(ctx context.Context, channelID, externalThreadID string, msg types.NormalizedMessage) error {
	a.mu.RLock()
	creds, exists := a.creds[channelID]
	a.mu.RUnlock()
	if !exists {
		return fmt.Errorf("channel %s credentials not configured in matrix adapter", channelID)
	}

	if creds.HomeserverURL == "mock" || creds.HomeserverURL == "" {
		return nil
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
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sendURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to send message, Matrix response code %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (a *Adapter) syncLoop(ctx context.Context, channelID string, publish func(types.InboundEvent)) {
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
			time.Sleep(5 * time.Second)
			continue
		}
		failCount = 0

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			a.SetStatus(channelID, "error", fmt.Sprintf("sync HTTP error %d: %s", resp.StatusCode, string(bodyBytes)))
			time.Sleep(5 * time.Second)
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
			time.Sleep(2 * time.Second)
			continue
		}

		since = syncResp.NextBatch

		for roomID, roomData := range syncResp.Rooms.Join {
			for _, ev := range roomData.Timeline.Events {
				if ev.Sender == creds.UserID {
					continue
				}

				if ev.Type != "m.room.message" && ev.Type != "m.reaction" && ev.Type != "net.maunium.whatsapp.contact" {
					continue
				}

				inboundEv := a.NormalizeEvent(channelID, roomID, ev.EventID, ev.Sender, ev.Type, ev.OriginServerTs, ev.Content)
				publish(inboundEv)
			}
		}
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
