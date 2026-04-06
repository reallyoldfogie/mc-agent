package agent

import (
	"log"
	"strings"

	pk "github.com/Tnze/go-mc/net/packet"

	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// Bot client adapters to satisfy our narrow interfaces without leaking dependencies.

// type botClientAdapter struct {
// 	c *bot.Client
// }

// func (b *botClientAdapter) JoinServerWithOptions(ctx context.Context, address string, opts JoinOptions) error {
// 	var rec bot.PacketRecorder
// 	var mirror bot.MovementMirror
// 	if opts.ReplayRecorder != nil {
// 		rec = packetRecorderAdapter{rec: opts.ReplayRecorder}
// 	}
// 	if opts.MovementMirror != nil {
// 		mirror = movementMirrorAdapter{m: opts.MovementMirror}
// 	}
// 	return b.c.JoinServerWithOptions(ctx, address, bot.JoinOptions{
// 		ProtocolVersion:      opts.ProtocolVersion,
// 		ReplayRecorder:       rec,
// 		MovementMirror:       mirror,
// 		RegistryDataCallback: opts.RegistryDataCallback,
// 	})
// }

// func (b *botClientAdapter) Events() EventBus {
// 	return &botEventBusAdapter{events: &b.c.Events}
// }

// func (b *botClientAdapter) Name() string { return b.c.Name }

// func (b *botClientAdapter) HandleGame(ctx context.Context) error { return b.c.HandleGame(ctx) }

// func (b *botClientAdapter) WritePacket(p pk.Packet) error { return b.c.Conn.WritePacket(p) }

// // BotClient returns the underlying *bot.Client for internal use by movement executor, etc.
// func (b *botClientAdapter) BotClient() *bot.Client { return b.c }

// botEventBusAdapter wraps bot.Events and converts PacketHandler shapes.
type botEventBusAdapter struct {
	events bot.Events
}

func (e *botEventBusAdapter) AddGeneric(listeners ...bot.PacketHandler) {
	converted := make([]bot.PacketHandler, 0, len(listeners))
	for _, l := range listeners {
		h := l
		converted = append(converted, bot.PacketHandler{ID: protocol_models.ClientboundPacketID(h.ID), Priority: h.Priority, F: func(p pk.Packet) error { return h.F(p) }})
	}
	e.events.AddGeneric(converted...)
}

func (e *botEventBusAdapter) AddListener(listeners ...bot.PacketHandler) {
	converted := make([]bot.PacketHandler, 0, len(listeners))
	for _, l := range listeners {
		h := l
		converted = append(converted, bot.PacketHandler{ID: protocol_models.ClientboundPacketID(h.ID), Priority: h.Priority, F: func(p pk.Packet) error { return h.F(p) }})
	}
	e.events.AddListener(converted...)
}

// NewClientFromBot wraps a concrete bot.Client as an Agent Client.
// func NewClientFromBot(c *bot.Client) Client { return &botClientAdapter{c: c} }

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

// GetMountedEntityPosition returns the position of a mounted entity by ID.
// This implements the MountedEntityPositionGetter interface for use by the movement executor.
func (a *agent) GetMountedEntityPosition(entityID int32) (x, y, z float64, found bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed {
		return 0, 0, 0, false
	}

	return entity.X, entity.Y, entity.Z, true
}

// GetMountedEntityType returns the entity type ID of a mounted entity by ID.
// Returns the entity type and found flag.
func (a *agent) GetMountedEntityType(entityID int32) (int32, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed {
		return 0, false
	}

	return entity.EntityType, true
}

// IsMountedEntityBoat checks if a mounted entity is a boat or raft by type ID.
// Returns true if the entity is a boat or raft variant (e.g., boat, oak_boat, chest_boat, bamboo_raft).
func (a *agent) IsMountedEntityBoat(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	// Check if entity type name contains "boat" or "raft" (handles boat, oak_boat, chest_boat, bamboo_raft, etc.)
	rval := strings.Contains(entityTypeName, "boat") || strings.Contains(entityTypeName, "raft")

	log.Printf("[IsMountedEntityBoat] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}
