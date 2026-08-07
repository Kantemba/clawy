//go:build !whatsapp_native

package whatsapp

import (
	"fmt"

	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
)

// NewWhatsAppNativeChannel returns an error when the binary was not built with -tags whatsapp_native.
// Build with: go build -tags whatsapp_native ./cmd/...
func NewWhatsAppNativeChannel(
	bc *config.Channel,
	name string,
	cfg *config.WhatsAppSettings,
	bus *bus.MessageBus,
	storePath string,
) (channels.Channel, error) {
	_ = bc
	_ = name
	_ = cfg
	_ = bus
	_ = storePath
	return nil, fmt.Errorf("whatsapp native not compiled in; build with -tags whatsapp_native")
}
