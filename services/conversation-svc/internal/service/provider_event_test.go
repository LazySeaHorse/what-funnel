package service

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

func TestProviderMessageContent(t *testing.T) {
	t.Parallel()

	mediaID := uuid.New()
	data, err := providerMessageContent(messaging.Message{
		Text: "Open WhatsApp to view this message.", NoticeCode: "unsupported",
	}, mediaID)
	if err != nil {
		t.Fatalf("providerMessageContent() error = %v", err)
	}
	var content map[string]any
	if err := json.Unmarshal(data, &content); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}
	if content["media_id"] != mediaID.String() {
		t.Errorf("media_id = %v, want %s", content["media_id"], mediaID)
	}
	if content["notice_code"] != "unsupported" {
		t.Errorf("notice_code = %v, want unsupported", content["notice_code"])
	}
}
