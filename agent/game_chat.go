package agent

import (
	"fmt"
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
func (a *agent) onGameStart() {
	if a.chat != nil {
		_ = a.chat.SendMessage("Hello, world")
	}
}

// Exported wrappers for wiring in external packages (basic.EventsListener signatures)
func (a *agent) HandleGameStart() error { a.onGameStart(); return nil }
func (a *agent) HandleDisconnect(reason chat.Message) error {
	a.onDisconnect(reason.String())
	return nil
}
func (a *agent) HandleHealthChange(health float32, food int32, saturation float32) error {
	a.onHealthChange(health, food, saturation)
	return nil
}
func (a *agent) HandleDeath() error { a.onDeath(); return nil }

// onDisconnect handles graceful disconnect processing.
func (a *agent) onDisconnect(reason string) {
	log.Printf("Disconnected: %s", reason)
}

// onHealthChange handles player health changes.
func (a *agent) onHealthChange(health float32, food int32, saturation float32) {
	log.Printf("Health: %.2f, Food: %d, Saturation: %.2f", health, food, saturation)
}

// onDeath handles player death events.
func (a *agent) onDeath() {
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
func (a *agent) onTeleported(x, y, z float64, yaw, pitch float32) {
	a.setPosition(x, y, z, yaw, pitch)
}

// HandleTeleported is a wrapper matching basic.EventsListener.Teleported signature.
func (a *agent) HandleTeleported(x, y, z float64, yaw, pitch float32, _ byte, teleportID int32) error {
	a.setPosition(x, y, z, yaw, pitch)
	// Prefer auto-created player, fall back to injected teleport
	t := a.player
	if t == nil {
		t = a.teleport
	}
	if t != nil {
		_ = t.AcceptTeleportation(pk.VarInt(teleportID))
	}
	return nil
}

// SendChat sends a chat message via the chat subsystem when available.
// Implements ChatOperations interface.
func (a *agent) SendChat(message string) error {
	a.mu.Lock()
	// Prefer auto-created chatMgr, fall back to injected chat
	cm := a.chatMgr
	if cm == nil {
		cm = a.chat
	}
	a.mu.Unlock()

	if cm == nil {
		return fmt.Errorf("chat manager not initialized")
	}
	return cm.SendMessage(message)
}

// OnSystemChat handles system chat messages from the server.
func (a *agent) OnSystemChat(c chat.Message, overlay bool) error {
	log.Printf("System Chat: %#v, Overlay: %v", c, overlay)
	a.emitChatEvent(c)
	return nil
}

// extractCommandFromMessage checks if a chat message is directed at this bot
// using the >>>botName<<< flag pattern and returns the command text if found.
func (a *agent) extractCommandFromMessage(msg chat.Message) (string, bool) {
	botName := ""
	if a.client != nil {
		botName = a.client.Name()
	}
	if botName == "" {
		return "", false
	}
	flag := ">>>" + botName + "<<<"

	// Check main text field first
	if after, ok := strings.CutPrefix(msg.Text, flag); ok {
		return strings.TrimSpace(after), true
	}

	// Check With segments
	for _, with := range msg.With {
		if after, ok := strings.CutPrefix(with.Text, flag); ok {
			return strings.TrimSpace(after), true
		}
	}

	return "", false
}

// OnPlayerChat handles player chat messages.
func (a *agent) OnPlayerChat(senderInfo playerlist.PlayerInfo, msg chat.Message, validated bool) error {
	prefix := ""
	if !validated {
		prefix = "[Not Secure] "
	}
	log.Printf("%sPlayer: %v", prefix, msg)
	a.emitChatEvent(msg)

	// Check if message contains a command for this bot
	text, ok := a.extractCommandFromMessage(msg)
	if !ok {
		return nil
	}

	// Acknowledge and handle command
	_ = a.SendChat("Chat Received: " + text)
	a.handleChatCommand(text)
	return nil
}

// OnDisguisedChat handles disguised chat messages (e.g., from RCON /say).
func (a *agent) OnDisguisedChat(msg chat.Message) error {
	log.Printf("Disguised: %v", msg)
	a.emitChatEvent(msg)

	// Check if message contains a command for this bot
	text, ok := a.extractCommandFromMessage(msg)
	if !ok {
		return nil
	}

	// Acknowledge and handle command
	_ = a.SendChat("Received: " + text)
	a.handleChatCommand(text)
	return nil
}

func (a *agent) emitChatEvent(msg chat.Message) {
	if a.chatEvents == nil {
		return
	}
	text := msg.Text
	if text == "" {
		text = fmt.Sprintf("%v", msg)
	}
	select {
	case a.chatEvents <- text:
	default:
	}
}
