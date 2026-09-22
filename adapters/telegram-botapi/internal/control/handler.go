package control

import (
	"github.com/whatfunnel/whatfunnel/packages/go-common/adapterkit"
)

type (
	Handler    = adapterkit.Handler
	Controller = adapterkit.Controller
)

func NewHandler(controller Controller, secret string) (*Handler, error) {
	return adapterkit.NewHandler(controller, adapterkit.HandlerConfig{
		ProviderName: "Telegram",
		SharedSecret: secret,
	})
}
