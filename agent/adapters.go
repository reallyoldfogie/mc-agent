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

// GetMountedEntityYaw returns the yaw of a mounted entity in degrees.
// The entity tracker stores yaw as a packed int8 (0–255 covering 0–360 degrees).
func (a *agent) GetMountedEntityYaw(entityID int32) (yaw float32, found bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed {
		return 0, false
	}

	// Convert packed angle (int8, 0–255 = 0°–360°) to degrees.
	yaw = float32(entity.Yaw) * 360.0 / 256.0
	return yaw, true
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
// Returns true if the entity type's local name (after the namespace prefix)
// ends with "_boat" or "_raft" — e.g. oak_boat, oak_chest_boat, bamboo_raft,
// bamboo_chest_raft, pale_oak_boat, etc.
//
// The suffix is matched against the local part of the identifier rather than
// the full string because the namespace "minecraft" itself contains the
// substring "raft" (mineCRAFT), so a naive Contains check would classify every
// minecraft-namespaced entity as a boat.
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

	// Strip the namespace prefix (e.g., "minecraft:") so the suffix check is
	// applied to the local name only.
	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	// In 1.21.1, boats use a single entity type "boat" / "chest_boat".
	// In 1.21.2+, boats are per-wood-type: "oak_boat", "birch_chest_boat", etc.
	rval := localName == "boat" || localName == "chest_boat" ||
		strings.HasSuffix(localName, "_boat") || strings.HasSuffix(localName, "_raft")

	log.Printf("[IsMountedEntityBoat] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// GetEntityAttribute retrieves an entity attribute value by name.
// Returns the attribute value and a found flag. Common attributes include:
// - "generic.movement_speed" (horse speed, etc.)
// - "generic.max_health" (health cap)
// - "generic.attack_damage" (etc.)
func (a *agent) GetEntityAttribute(entityID int32, attributeName string) (float64, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed || entity.Attributes == nil {
		return 0, false
	}

	value, ok := entity.Attributes[attributeName]
	return value, ok
}
