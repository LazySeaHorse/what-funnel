package adapterclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestClientCreate(t *testing.T) {
	t.Parallel()

	client, err := New("http://whatsapp-adapter:8085", "shared-secret")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	client.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer shared-secret" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		if request.URL.Path != "/v1/connections" {
			t.Errorf("path = %q, want /v1/connections", request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["credential"] != "bot-token" {
			t.Errorf("credential = %q", body["credential"])
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"channel_id":"channel-1","state":"awaiting_scan","qr_data":"qr"}`,
			)),
		}, nil
	})

	snapshot, err := client.Create(context.Background(), "channel-1", "bot-token")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if snapshot.State != messaging.ConnectionAwaitingScan {
		t.Errorf("state = %q, want %q", snapshot.State, messaging.ConnectionAwaitingScan)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		secret  string
	}{
		{name: "relative url", baseURL: "/adapter", secret: "secret"},
		{name: "file url", baseURL: "file:///adapter", secret: "secret"},
		{name: "empty secret", baseURL: "http://adapter:8085"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(test.baseURL, test.secret); err == nil {
				t.Fatal("New() error = nil, want error")
			}
		})
	}
}
