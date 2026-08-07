package mqtt

import (
	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
)

func init() {
	channels.RegisterSafeFactory(
		config.ChannelMQTT,
		func(bc *config.Channel, cfg *config.MQTTSettings, b *bus.MessageBus) (channels.Channel, error) {
			return NewMQTTChannel(bc, cfg, b)
		},
	)
}
