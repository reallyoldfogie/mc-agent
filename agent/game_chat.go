package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Tnze/go-mc/chat"
	pk "github.com/Tnze/go-mc/net/packet"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"

	"github.com/reallyoldfogie/mc-agent/models"
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
	a.logf("Disconnected: %s", reason)
}

// onHealthChange handles player health changes.
func (a *agent) onHealthChange(health float32, food int32, saturation float32) {
	a.setHealth(health, food, saturation)
	a.logf("Health: %.2f, Food: %d, Saturation: %.2f", health, food, saturation)
}

// onDeath handles player death events.
func (a *agent) onDeath() {
	a.logf("Died and respawn scheduled")
	// Pause physics position updates while dead to prevent corrupting server-side playerdata
	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()
	if notifier, ok := moveExec.(interface{ NotifyDead() }); ok {
		notifier.NotifyDead()
	}
	// Auto-respawn after 5 seconds when respawner is available
	// Prefer auto-created player subsystem, fall back to injected teleport
	var r models.Respawner
	if rr, ok := any(a.player).(models.Respawner); ok {
		r = rr
	} else if rr, ok := any(a.teleport).(models.Respawner); ok {
		r = rr
	}
	if r != nil {
		go func() { time.Sleep(5 * time.Second); _ = r.Respawn() }()
	}
}

// onTeleported handles local player teleport events.
func (a *agent) onTeleported(x, y, z float64, yaw, pitch float64) {
	a.setPosition(models.V3{X: x, Y: y, Z: z}, yaw, pitch)
}

// HandleTeleported is a wrapper matching basic.EventsListener.Teleported signature.
func (a *agent) HandleTeleported(x, y, z float64, yaw, pitch float64, _ byte, teleportID int32) error {
	a.setPosition(models.V3{X: x, Y: y, Z: z}, yaw, pitch)
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

// chatMessageMaxLength is Minecraft's protocol-enforced maximum length, in
// characters, for a single serverbound chat message
// (net.minecraft.network.packet.c2s.play.ChatMessageC2SPacket reads/writes
// the message field with a hard-coded 256-character bound). A server
// disconnects the client outright ("Internal Exception:
// io.netty.handler.codec.DecoderException: Failed to decode packet
// 'serverbound/minecraft:chat'") if it ever receives a longer one - this
// isn't a timing-sensitive edge case, it's the server's decoder
// unconditionally rejecting the packet. Confirmed live via
// docs/bugs/rare-packet-decode-disconnect/investigation-2026-09-13.md: a
// Craft error message that enumerated every valid ingredient-tag
// alternative (e.g. every plank variant for a "stick" recipe) reached 371
// characters and disconnected the bot mid-training-run. SendChat is the
// single choke point every chat send (both the version-handler path and
// the fallback bot/msg/chat.go Manager) goes through, so guarding it here
// protects every caller without needing every message-construction site
// (e.g. agent/craft.go's placeCraftIngredient) to independently know about
// this limit.
const chatMessageMaxLength = 256

// splitChatMessage returns message unchanged (as a single-element slice) if
// it's already within chatMessageMaxLength characters, or splits it into
// multiple chunks - each within the limit - otherwise, so the full content
// still reaches chat as several messages instead of being cut short.
// Splits by rune, not byte, so a multi-byte UTF-8 character is never
// divided across chunks. Prefers to break at the last space within a
// chunk's budget so words aren't split mid-word, falling back to a hard
// break only when a single word alone exceeds the limit (or no space
// exists in the first half of the budget, to avoid producing a run of
// needlessly tiny chunks from one stray early space).
func splitChatMessage(message string) []string {
	runes := []rune(message)
	if len(runes) <= chatMessageMaxLength {
		return []string{message}
	}

	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= chatMessageMaxLength {
			chunks = append(chunks, string(runes))
			break
		}

		splitAt := chatMessageMaxLength
		if idx := lastSpaceIndex(runes[:chatMessageMaxLength]); idx > chatMessageMaxLength/2 {
			splitAt = idx
		}

		if chunk := strings.TrimRight(string(runes[:splitAt]), " "); chunk != "" {
			chunks = append(chunks, chunk)
		}
		runes = []rune(strings.TrimLeft(string(runes[splitAt:]), " "))
	}
	return chunks
}

// lastSpaceIndex returns the index of the last space character in runes, or
// -1 if none is present.
func lastSpaceIndex(runes []rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == ' ' {
			return i
		}
	}
	return -1
}

// SendChat sends a chat message via the version handler or fallback chat
// manager, splitting it into multiple messages first if it exceeds
// chatMessageMaxLength (see splitChatMessage). Implements ChatOperations
// interface. Returns the first error encountered, if any, but still
// attempts every chunk rather than aborting partway through.
func (a *agent) SendChat(message string) error {
	var firstErr error
	for _, chunk := range splitChatMessage(message) {
		if err := a.sendChatMessage(chunk); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SendCommand sends a server command through the agent's player connection.
// Unlike SendChat("/command"), this uses Minecraft's dedicated command
// packet, so the server executes it as this player rather than through RCON.
// command must not include the leading slash.
func (a *agent) SendCommand(command string) error {
	vh := a.versionHandler
	c := a.client
	if vh == nil || c == nil || c.Conn() == nil {
		return fmt.Errorf("send command: agent is not connected")
	}
	return vh.Play().Chat().SendCommand(c.Conn(), strings.TrimPrefix(command, "/"))
}

// sendChatMessage sends a single message - already within
// chatMessageMaxLength - via the version handler or fallback chat manager.
func (a *agent) sendChatMessage(message string) error {
	// versionHandler and client are no-lock fields (set-once in Init, read-only after)
	vh := a.versionHandler
	c := a.client

	// Prefer version handler if both it and a valid connection are available
	if vh != nil && c != nil {
		conn := c.Conn()
		if conn != nil {
			return vh.Play().Chat().SendChat(conn, message)
		}
	}

	// Fall back to injected chat manager
	a.fallbackHandlersMu.RLock()
	fallback := a.chat
	a.fallbackHandlersMu.RUnlock()

	if fallback != nil {
		return fallback.SendMessage(message)
	}
	// If no fallback is set, log and return (allows tests to work without explicit chat setup)
	a.logf("SendChat (no network): %s", message)
	return nil
}

// OnSystemChat handles system chat messages from the server. RCON's /say
// broadcasts as a system chat message (ClientboundSystemChat), not a
// disguised/profileless one - confirmed live: sending a chat command via
// RCON /say only ever arrives here, never through OnDisguisedChat - so
// this also extracts and dispatches bot commands the same way
// OnPlayerChat/OnDisguisedChat do.
func (a *agent) OnSystemChat(c chat.Message, overlay bool) error {
	a.logf("System Chat: %#v, Overlay: %v", c, overlay)
	a.emitChatEvent(c)

	text, ok := a.extractCommandFromMessage(c)
	if !ok {
		return nil
	}

	_ = a.SendChat("Received: " + text)
	a.handleChatCommand(text)
	return nil
}

// extractCommandFromMessage checks if a chat message is directed at this bot
// using the >>>botName<<< flag pattern and returns the command text if found.
func (a *agent) extractCommandFromMessage(msg chat.Message) (string, bool) {
	botName := ""
	if a.client != nil {
		botName = a.cfg.Name
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
	a.logf("%sPlayer: %v", prefix, msg)
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
	a.logf("Disguised: %v", msg)
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

// initChatCommandHandlers registers the clientbound chat packet types
// (system, signed player, and disguised/profileless) so real chat messages
// reach the >>>botName<<< command pipeline (extractCommandFromMessage /
// handleChatCommand). Previously nothing registered these packet IDs at
// all - onSystemChatPacket/onPlayerChatPacket/onDisguisedChatPacket had
// working parse logic (covered by their own unit tests) but were never
// wired to the live connection, so no in-game or RCON chat message could
// ever trigger a bot command. Mirrors initClientInformationHandler's
// AddListener pattern.
//
// Guarded by chatHandlersInitialized because the surrounding Init() setup
// block can run more than once per agent (initHeldSlotTracking guards
// itself the same way, via a.heldSlotUpdates != nil, for the same reason).
//
// Each handler is also wrapped with dedupeChatPacket: confirmed live via a
// raw byte dump that a single RCON `say` can arrive at this client as two
// separate ClientboundProfilelessChat packets, byte-for-byte identical,
// milliseconds apart (a real server-side/RCON behavior, not a registration
// bug - the packets have distinct addresses but identical payloads).
// Deduping on the raw bytes rather than the decoded command text is what
// makes this safe: a genuine chat message a player or test sends twice on
// purpose is a new, distinct packet even when the text matches (a
// different signature/timestamp at minimum), so this can never suppress a
// real repeated command - only an exact repeat of the wire bytes, which a
// legitimate second send can't produce. An earlier version of this fix
// deduped on the decoded command text within a time window instead, which
// broke tests that intentionally send the same no-op-safe command twice in
// a row (e.g. stopFollow, to check both the normal and "not following"
// responses) - text can legitimately repeat; these bytes can't.
func (a *agent) initChatCommandHandlers() {
	if a.client == nil || a.packetMgr == nil || a.chatHandlersInitialized {
		return
	}
	a.chatHandlersInitialized = true

	a.client.Events().AddListener(bot.PacketHandler{
		ID: a.packetMgr.GetClientboundPacketID("ClientboundSystemChat"),
		F:  a.dedupeChatPacket(a.onSystemChatPacket),
	})
	a.client.Events().AddListener(bot.PacketHandler{
		ID: a.packetMgr.GetClientboundPacketID("ClientboundPlayerChat"),
		F:  a.dedupeChatPacket(a.onPlayerChatPacket),
	})
	a.client.Events().AddListener(bot.PacketHandler{
		ID: a.packetMgr.GetClientboundPacketID("ClientboundProfilelessChat"),
		F:  a.dedupeChatPacket(a.onDisguisedChatPacket),
	})
}

// chatPacketDedupWindow bounds how long an exact byte-for-byte repeat of
// the immediately-preceding chat packet (of the same clientbound type) is
// dropped as a duplicate delivery.
const chatPacketDedupWindow = 2 * time.Second

// dedupeChatPacket wraps a raw chat packet handler so an exact repeat of
// the previous packet's bytes within chatPacketDedupWindow is dropped
// before reaching it. See initChatCommandHandlers's doc comment for why
// this operates on raw bytes rather than decoded text.
func (a *agent) dedupeChatPacket(next func(pk.Packet) error) func(pk.Packet) error {
	var mu sync.Mutex
	var lastData string
	var lastAt time.Time
	return func(p pk.Packet) error {
		data := string(p.Data)
		mu.Lock()
		now := time.Now()
		if data == lastData && now.Sub(lastAt) < chatPacketDedupWindow {
			mu.Unlock()
			a.logf("[handleChatCommand] Dropping duplicate chat packet (%d bytes) within dedup window", len(p.Data))
			return nil
		}
		lastData = data
		lastAt = now
		mu.Unlock()
		return next(p)
	}
}

// onSystemChatPacket handles raw ClientboundSystemChat packets using version-specific parsing.
func (a *agent) onSystemChatPacket(p pk.Packet) error {
	// versionHandler is a no-lock field (set-once in Init, read-only after)
	vh := a.versionHandler

	if vh == nil {
		return nil
	}

	message, overlay, err := vh.Play().Chat().ParseSystemChat(p)
	if err != nil {
		return err
	}
	return a.OnSystemChat(chat.Message{Text: message}, overlay)
}

// onPlayerChatPacket handles raw ClientboundPlayerChat packets using version-specific parsing.
func (a *agent) onPlayerChatPacket(p pk.Packet) error {
	// versionHandler and playerList are no-lock fields (set-once in Init, read-only after)
	vh := a.versionHandler
	pl := a.playerList

	if vh == nil {
		return nil
	}

	senderUUID, message, err := vh.Play().Chat().ParsePlayerChat(p)
	if err != nil {
		return err
	}

	// Resolve sender info from playerlist (best-effort; fall back to minimal struct)
	info := playerlist.PlayerInfo{}
	if pl != nil {
		for id, pi := range pl.Get() {
			var cu [16]byte
			copy(cu[:], id[:])
			if cu == senderUUID {
				if pi != nil {
					info = *pi
				}
				break
			}
		}
	}
	return a.OnPlayerChat(info, chat.Message{Text: message}, false)
}

// onDisguisedChatPacket handles raw ClientboundProfilelessChat packets using version-specific parsing.
func (a *agent) onDisguisedChatPacket(p pk.Packet) error {
	// versionHandler is a no-lock field (set-once in Init, read-only after)
	vh := a.versionHandler

	if vh == nil {
		return nil
	}

	message, err := vh.Play().Chat().ParseDisguisedChat(p)
	if err != nil {
		return err
	}
	return a.OnDisguisedChat(chat.Message{Text: message})
}
