package control

import (
	"context"
	"errors"
	"strings"

	"github.com/whatfunnel/whatfunnel/adapters/whatsapp-whatsmeow/internal/session"
	"github.com/whatfunnel/whatfunnel/packages/go-common/adapterkit"
)

type (
	Handler  = adapterkit.Handler
	Snapshot = session.Snapshot
)

type Controller interface {
	Create(ctx context.Context, channelID string) (session.Snapshot, error)
	Retry(ctx context.Context, channelID string) (session.Snapshot, error)
	Snapshot(channelID string) (session.Snapshot, error)
	Logout(ctx context.Context, channelID string) error
	Download(ctx context.Context, channelID, providerRef string) (session.MediaFile, error)
}

type controllerAdapter struct {
	target Controller
}

func (c *controllerAdapter) Create(ctx context.Context, channelID, _ string) (adapterkit.Snapshot, error) {
	return c.target.Create(ctx, channelID)
}

func (c *controllerAdapter) Retry(ctx context.Context, channelID, _ string) (adapterkit.Snapshot, error) {
	return c.target.Retry(ctx, channelID)
}

func (c *controllerAdapter) Snapshot(channelID string) (adapterkit.Snapshot, error) {
	return c.target.Snapshot(channelID)
}

func (c *controllerAdapter) Logout(ctx context.Context, channelID string) error {
	return c.target.Logout(ctx, channelID)
}

func (c *controllerAdapter) Download(ctx context.Context, channelID, providerRef string) (adapterkit.MediaFile, error) {
	return c.target.Download(ctx, channelID, providerRef)
}

func NewHandler(controller Controller, secret string) (*Handler, error) {
	if controller == nil || strings.TrimSpace(secret) == "" {
		return nil, errors.New("whatsapp control: invalid handler configuration")
	}
	return adapterkit.NewHandler(&controllerAdapter{target: controller}, adapterkit.HandlerConfig{
		ProviderName: "WhatsApp",
		SharedSecret: secret,
	})
}
