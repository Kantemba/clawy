package telegram

import (
	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
)

func init() {
	channels.RegisterFactory(
		config.ChannelTelegram,
		func(channelName, channelType string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
			bc := cfg.Channels[channelName]
			decoded, err := bc.GetDecoded()
			if err != nil {
				return nil, err
			}
			c, ok := decoded.(*config.TelegramSettings)
			if !ok {
				return nil, channels.ErrSendFailed
			}
			return NewTelegramChannel(bc, c, b)
		},
	)
}
