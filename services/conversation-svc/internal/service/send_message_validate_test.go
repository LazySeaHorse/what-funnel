package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/whatfunnel/whatfunnel/packages/go-common/types"
)

func TestSendMessageParamsValidate_IdempotencyKeyLength(t *testing.T) {
	params := SendMessageParams{Sender: types.MessageSenderHuman}

	params.IdempotencyKey = strings.Repeat("k", maxIdempotencyKeyLength)
	if err := params.validate(); err != nil {
		t.Fatalf("key at the limit must be accepted: %v", err)
	}

	params.IdempotencyKey = strings.Repeat("k", maxIdempotencyKeyLength+1)
	if err := params.validate(); !errors.Is(err, ErrValidation) {
		t.Fatalf("oversized key must be a validation error, got %v", err)
	}
}
