package models

import "github.com/reallyoldfogie/mc-bot-go/bot"

// EventBus registers and dispatches packet handlers.
type EventBus interface {
	AddGeneric(listeners ...bot.PacketHandler)
	AddListener(listeners ...bot.PacketHandler)
}
