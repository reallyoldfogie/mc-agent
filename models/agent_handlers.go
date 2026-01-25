package models

import (
	"github.com/Tnze/go-mc/chat"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
)

// AgentHandlers contains game event hooks handled by the agent.
type AgentHandlers interface {
	HandleGameStart() error
	HandleDisconnect(reason chat.Message) error
	HandleHealthChange(health float32, food int32, saturation float32) error
	HandleDeath() error
	HandleTeleported(x, y, z float64, yaw, pitch float32, _ byte, teleportID int32) error
	HandleChunkLoad(ChunkPos) error
	HandleChunkUnload(ChunkPos) error
	OnSystemChat(message chat.Message, overlay bool) error
	OnPlayerChat(playerlist.PlayerInfo, chat.Message, bool) error
	OnDisguisedChat(chat.Message) error
	OnScreenSlotChange(id, index int) error
}
