package wecom

import (
	"github.com/Kantemba/clawy/pkg/bus"
	"github.com/Kantemba/clawy/pkg/channels"
	"github.com/Kantemba/clawy/pkg/config"
)

func init() {
	channels.RegisterFactory(
		config.ChannelWeCom,
		func(channelName, channelType string, cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
			bc := cfg.Channels[channelName]
			decoded, err := bc.GetDecoded()
			if err != nil {
				return nil, err
			}
			c, ok := decoded.(*config.WeComSettings)
			if !ok {
				return nil, channels.ErrSendFailed
			}
			return NewChannel(bc, c, b)
		},
	)
}
