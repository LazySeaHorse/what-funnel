package main

import (
	"context"
	"errors"
	"testing"
)

func TestConsumerResult(t *testing.T) {
	consumerErr := errors.New("consumer failed")
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		err     error
		wantErr string
	}{
		{
			name: "cancellation is a clean stop",
			ctx:  canceledCtx,
			err:  context.Canceled,
		},
		{
			name:    "consumer failure is reported with stream",
			ctx:     context.Background(),
			err:     consumerErr,
			wantErr: "consume stream test.stream: consumer failed",
		},
		{
			name:    "unexpected clean return is an error",
			ctx:     context.Background(),
			wantErr: "consume stream test.stream stopped unexpectedly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := consumerResult(tt.ctx, "test.stream", tt.err)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("consumerResult() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("consumerResult() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
