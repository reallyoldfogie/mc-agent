package agent

import (
	"context"

	pk "github.com/Tnze/go-mc/net/packet"

	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	"github.com/reallyoldfogie/mc-bot-go/bot/msg"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// Bot client adapters to satisfy our narrow interfaces without leaking dependencies.

type botClientAdapter struct {
	c *bot.Client
}

func (b *botClientAdapter) JoinServerWithOptions(ctx context.Context, address string, opts JoinOptions) error {
	var rec bot.PacketRecorder
	var mirror bot.MovementMirror
	if opts.ReplayRecorder != nil {
		rec = packetRecorderAdapter{rec: opts.ReplayRecorder}
	}
	if opts.MovementMirror != nil {
		mirror = movementMirrorAdapter{m: opts.MovementMirror}
	}
	return b.c.JoinServerWithOptions(ctx, address, bot.JoinOptions{
		ProtocolVersion:      opts.ProtocolVersion,
		ReplayRecorder:       rec,
		MovementMirror:       mirror,
		RegistryDataCallback: opts.RegistryDataCallback,
	})
}

func (b *botClientAdapter) Events() EventBus {
	return &botEventBusAdapter{events: &b.c.Events}
}

func (b *botClientAdapter) Name() string { return b.c.Name }

func (b *botClientAdapter) HandleGame(ctx context.Context) error { return b.c.HandleGame(ctx) }

func (b *botClientAdapter) WritePacket(p pk.Packet) error { return b.c.Conn.WritePacket(p) }

// botEventBusAdapter wraps bot.Events and converts PacketHandler shapes.
type botEventBusAdapter struct {
	events *bot.Events
}

func (e *botEventBusAdapter) AddGeneric(listeners ...PacketHandler) {
	converted := make([]bot.PacketHandler, 0, len(listeners))
	for _, l := range listeners {
		h := l
		converted = append(converted, bot.PacketHandler{ID: protocol_models.ClientboundPacketID(h.ID), Priority: h.Priority, F: func(p pk.Packet) error { return h.F(p) }})
	}
	e.events.AddGeneric(converted...)
}

func (e *botEventBusAdapter) AddListener(listeners ...PacketHandler) {
	converted := make([]bot.PacketHandler, 0, len(listeners))
	for _, l := range listeners {
		h := l
		converted = append(converted, bot.PacketHandler{ID: protocol_models.ClientboundPacketID(h.ID), Priority: h.Priority, F: func(p pk.Packet) error { return h.F(p) }})
	}
	e.events.AddListener(converted...)
}

// NewClientFromBot wraps a concrete bot.Client as an Agent Client.
func NewClientFromBot(c *bot.Client) Client { return &botClientAdapter{c: c} }

// chatAdapter wraps msg.Manager to satisfy Chat.
type chatAdapter struct{ m *msg.Manager }

func (c *chatAdapter) SendMessage(s string) error { return c.m.SendMessage(s) }

func NewChatFromMsg(m *msg.Manager) Chat { return &chatAdapter{m: m} }

// packetRecorderAdapter bridges agent.PacketRecorder to bot.PacketRecorder without
// requiring the concrete recorder type here.
type packetRecorderAdapter struct {
	rec PacketRecorder
}

func (p packetRecorderAdapter) RecordNow(id int32, payload []byte) error {
	return p.rec.RecordNow(id, payload)
}
func (p packetRecorderAdapter) SetSelfID(id int)      { p.rec.SetSelfID(id) }
func (p packetRecorderAdapter) AddPlayer(uuid string) { p.rec.AddPlayer(uuid) }

// movementMirrorAdapter bridges agent.MovementMirror to bot.MovementMirror.
type movementMirrorAdapter struct {
	m MovementMirror
}

func (m movementMirrorAdapter) HandleServerbound(p pk.Packet) { m.m.HandleServerbound(p) }
func (m movementMirrorAdapter) SetEntityMeta(id int32, name string, uuid [16]byte) {
	m.m.SetEntityMeta(id, name, uuid)
}
func (m movementMirrorAdapter) SetEntityType(entityType int32) { m.m.SetEntityType(entityType) }
func (m movementMirrorAdapter) HandlePlayerInfo(p pk.Packet)   { m.m.HandlePlayerInfo(p) }
