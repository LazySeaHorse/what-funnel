package control

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whatfunnel/whatfunnel/adapters/telegram-botapi/internal/session"
	"github.com/whatfunnel/whatfunnel/packages/go-common/messaging"
)

type fakeController struct {
	credential string
	createErr  error
}

func (c *fakeController) Create(_ context.Context, channelID, credential string) (session.Snapshot, error) {
	c.credential = credential
	return session.Snapshot{ChannelID: channelID, State: messaging.ConnectionConnected, RemoteAccountID: "@safe_bot"}, c.createErr
}

func (*fakeController) Retry(context.Context, string, string) (session.Snapshot, error) {
	return session.Snapshot{}, nil
}
func (*fakeController) Snapshot(string) (session.Snapshot, error) { return session.Snapshot{}, nil }
func (*fakeController) Logout(context.Context, string) error      { return nil }
func (*fakeController) Download(context.Context, string, string) (session.MediaFile, error) {
	return session.MediaFile{}, nil
}

func TestCreateNeverReturnsBotToken(t *testing.T) {
	const token = "123456:browser-must-not-see-this"
	controller := &fakeController{}
	handler, err := NewHandler(controller, "internal-secret")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/connections", strings.NewReader(`{"channel_id":"channel-1","credential":"`+token+`"}`))
	request.Header.Set("Authorization", "Bearer internal-secret")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if controller.credential != token {
		t.Fatal("controller did not receive credential")
	}
	if strings.Contains(response.Body.String(), token) {
		t.Fatal("response exposed bot token")
	}
}

func TestCreateFailureRedactsControllerError(t *testing.T) {
	const token = "654321:secret-in-error"
	controller := &fakeController{createErr: errors.New("upstream rejected " + token)}
	handler, err := NewHandler(controller, "internal-secret")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/connections", strings.NewReader(`{"channel_id":"channel-1","credential":"`+token+`"}`))
	request.Header.Set("Authorization", "Bearer internal-secret")
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), token) {
		t.Fatal("error response exposed bot token")
	}
}
