package agent

import (
	"log"
	"strings"
	"time"

	"github.com/Tnze/go-mc/chat"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
)

// Placeholder stubs for game and chat handlers to be filled during migration
// when player/chat/world subsystems are injected.

// onGameStart handles game start events (to be wired via player events).
func (a *Agent) onGameStart() {
	if a.chat != nil {
		_ = a.chat.SendMessage("Hello, world")
	}
}

// Exported wrappers for wiring in external packages (basic.EventsListener signatures)
func (a *Agent) HandleGameStart() error { a.onGameStart(); return nil }
func (a *Agent) HandleDisconnect(reason chat.Message) error {
	a.onDisconnect(reason.String())
	return nil
}
func (a *Agent) HandleHealthChange(health float32, food int32, saturation float32) error {
	a.onHealthChange(health, food, saturation)
	return nil
}
func (a *Agent) HandleDeath() error { a.onDeath(); return nil }

// onDisconnect handles graceful disconnect processing.
func (a *Agent) onDisconnect(reason string) {
	log.Printf("Disconnected: %s", reason)
}

// onHealthChange handles player health changes.
func (a *Agent) onHealthChange(health float32, food int32, saturation float32) {
	log.Printf("Health: %.2f, Food: %d, Saturation: %.2f", health, food, saturation)
}

// onDeath handles player death events.
func (a *Agent) onDeath() {
	log.Printf("Died and respawn scheduled")
	// Auto-respawn after 5 seconds when respawner is set
	a.mu.Lock()
	var r Respawner = nil
	if rr, ok := any(a.teleport).(Respawner); ok {
		r = rr
	}
	a.mu.Unlock()
	if r != nil {
		go func() { time.Sleep(5 * time.Second); _ = r.Respawn() }()
	}
}

// onTeleported handles local player teleport events.
func (a *Agent) onTeleported(x, y, z float64, yaw, pitch float32) {
	a.setPosition(x, y, z, yaw, pitch)
}

// HandleTeleported is a wrapper matching basic.EventsListener.Teleported signature.
func (a *Agent) HandleTeleported(x, y, z float64, yaw, pitch float32, _ byte, teleportID int32) error {
	a.setPosition(x, y, z, yaw, pitch)
	if a.teleport != nil {
		_ = a.teleport.AcceptTeleportation(pk.VarInt(teleportID))
	}
	return nil
}

// SendChat sends a chat message via the chat subsystem when available.
func (a *Agent) SendChat(message string) error {
	if a.chat == nil {
		return nil
	}
	return a.chat.SendMessage(message)
}

// OnSystemChat handles system chat messages from the server.
func (a *Agent) OnSystemChat(c chat.Message, overlay bool) error {
	log.Printf("System Chat: %#v, Overlay: %v", c, overlay)
	return nil
}

// OnPlayerChat handles player chat messages.
func (a *Agent) OnPlayerChat(senderInfo playerlist.PlayerInfo, msg chat.Message, validated bool) error {
	prefix := ""
	if !validated {
		prefix = "[Not Secure] "
	}
	log.Printf("%sPlayer: %v", prefix, msg)

	// Only react to messages directed at this bot using the >>>name<<< flag pattern.
	botName := ""
	if a.client != nil {
		botName = a.client.Name()
	}
	if botName == "" {
		return nil
	}
	flag := ">>>" + botName + "<<<"

	// Find the portion after the flag in any With segment.
	text := ""
	for _, with := range msg.With {
		if after, ok := strings.CutPrefix(with.Text, flag); ok {
			text = strings.TrimSpace(after)
			break
		}
	}
	if text == "" {
		return nil
	}
	// Acknowledge and handle command.
	_ = a.SendChat("Received: " + text)
	a.handleChatCommand(text)
	return nil
}

// OnDisguisedChat handles disguised chat messages.
func (a *Agent) OnDisguisedChat(msg chat.Message) error {
	log.Printf("Disguised: %v", msg)
	return nil
}
