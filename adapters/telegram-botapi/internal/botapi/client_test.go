package botapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestClientRedactsTokenFromTransportErrors(t *testing.T) {
	t.Parallel()
	const token = "123456:super-secret-token"
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
	client, err := New("https://api.telegram.test", httpClient)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetMe(context.Background(), token)
	if err == nil {
		t.Fatal("expected request error")
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestClientReturnsSanitizedAPIError(t *testing.T) {
	t.Parallel()
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":false,"error_code":401,"description":"token 123:secret is Unauthorized"}`)), Request: request}, nil
	})}
	client, _ := New("https://api.telegram.test", httpClient)
	_, err := client.GetMe(context.Background(), "123:secret")
	var apiErr *Error
	if !errors.As(err, &apiErr) || !apiErr.Permanent() {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "123:secret") {
		t.Fatalf("error leaked token: %v", err)
	}
}
