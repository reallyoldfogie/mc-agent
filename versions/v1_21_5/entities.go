// Package v1_21_5 provides version-specific packet handling for Minecraft 1.21.5.
package v1_21_5

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// entityHandler implements common.EntityHandler for 1.21.5.
type entityHandler struct {
	packetMgr protocol_models.PacketMgr
}

// ParseAddEntity parses a SpawnEntity packet (clientbound ID 1).
// Returns entity information including position, rotation, type, object data, and velocity.
// Velocity is encoded as fixed-point and needs to be divided by 8000.0.
func (e *entityHandler) ParseAddEntity(p pk.Packet) (entityID, entityType, objectData int32, uuid [16]byte, x, y, z float64, yaw, pitch int8, velX, velY, velZ float64, err error) {
	pkt := cb.NewSpawnEntity()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, uuid, 0, 0, 0, 0, 0, 0, 0, 0, common.ErrPacketParse{PacketName: "SpawnEntity", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	entityType = int32(pkt.Type)
	objectData = int32(pkt.ObjectData)
	uuid = [16]byte(pkt.ObjectUUID)
	x = float64(pkt.X)
	y = float64(pkt.Y)
	z = float64(pkt.Z)
	// Protocol uses bytes for angles (0-255 maps to 0-360 degrees)
	yaw = int8(pkt.Yaw)
	pitch = int8(pkt.Pitch)
	// Velocity is encoded as fixed-point and needs to be divided by 8000
	velX = float64(pkt.VelocityX) / 8000.0
	velY = float64(pkt.VelocityY) / 8000.0
	velZ = float64(pkt.VelocityZ) / 8000.0

	return entityID, entityType, objectData, uuid, x, y, z, yaw, pitch, velX, velY, velZ, nil
}

// ParseMoveEntityPos parses a RelEntityMove packet (clientbound ID 46).
// Returns delta movement values which need to be divided by 4096 to get block coordinates.
func (e *entityHandler) ParseMoveEntityPos(p pk.Packet) (entityID int32, dx, dy, dz int16, onGround bool, err error) {
	pkt := cb.NewRelEntityMove()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, false, common.ErrPacketParse{PacketName: "RelEntityMove", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	dx = int16(pkt.DX)
	dy = int16(pkt.DY)
	dz = int16(pkt.DZ)
	onGround = bool(pkt.OnGround)

	return entityID, dx, dy, dz, onGround, nil
}

// ParseMoveEntityPosRot parses an EntityMoveLook packet (clientbound ID 47).
// Returns delta movement values and rotation angles.
func (e *entityHandler) ParseMoveEntityPosRot(p pk.Packet) (entityID int32, dx, dy, dz int16, yaw, pitch int8, onGround bool, err error) {
	pkt := cb.NewEntityMoveLook()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, 0, 0, false, common.ErrPacketParse{PacketName: "EntityMoveLook", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	dx = int16(pkt.DX)
	dy = int16(pkt.DY)
	dz = int16(pkt.DZ)
	yaw = int8(pkt.Yaw)
	pitch = int8(pkt.Pitch)
	onGround = bool(pkt.OnGround)

	return entityID, dx, dy, dz, yaw, pitch, onGround, nil
}

// ParseTeleportEntity parses an EntityTeleport packet (clientbound ID 118).
// Returns absolute position values (not deltas).
func (e *entityHandler) ParseTeleportEntity(p pk.Packet) (entityID int32, x, y, z float64, yaw, pitch int8, onGround bool, err error) {
	pkt := cb.NewEntityTeleport()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, 0, 0, false, common.ErrPacketParse{PacketName: "EntityTeleport", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	x = float64(pkt.X)
	y = float64(pkt.Y)
	z = float64(pkt.Z)
	yaw = int8(pkt.Yaw)
	pitch = int8(pkt.Pitch)
	onGround = bool(pkt.OnGround)

	return entityID, x, y, z, yaw, pitch, onGround, nil
}

// ParseRemoveEntities parses an EntityDestroy packet (clientbound ID 70).
// Returns a list of entity IDs to remove.
func (e *entityHandler) ParseRemoveEntities(p pk.Packet) (entityIDs []int32, err error) {
	pkt := cb.NewEntityDestroy()
	if err = pkt.Scan(p); err != nil {
		return nil, common.ErrPacketParse{PacketName: "EntityDestroy", Cause: err}
	}

	// Convert from []pk.VarInt to []int32
	ids := pkt.EntityIds.Get()
	entityIDs = make([]int32, len(ids))
	for i, id := range ids {
		entityIDs[i] = int32(id)
	}

	return entityIDs, nil
}

// ParseEntityEvent parses an EntityStatus packet (clientbound ID 30).
// Returns the entity ID and event/status ID.
func (e *entityHandler) ParseEntityEvent(p pk.Packet) (entityID int32, eventID int8, err error) {
	pkt := cb.NewEntityStatus()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, common.ErrPacketParse{PacketName: "EntityStatus", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	eventID = int8(pkt.EntityStatus)

	return entityID, eventID, nil
}

// ParseSetEntityMetadata parses an entity metadata update packet.
// Returns all metadata entries as-is for the caller to interpret.
// Entity metadata indices vary by entity type. See:
// https://minecraft.wiki/w/Java_Edition_protocol/Entity_metadata#Entity_Metadata
func (e *entityHandler) ParseSetEntityMetadata(p pk.Packet) (entityID int32, entries []models.MetadataEntry, err error) {
	pkt := cb.NewEntityMetadata()
	if err = pkt.Scan(p); err != nil {
		return 0, nil, common.ErrPacketParse{PacketName: "EntityMetadata", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	entries = make([]models.MetadataEntry, len(pkt.Metadata.Entries))

	for i, entry := range pkt.Metadata.Entries {
		handlerID := models.HandlerTypeFromString(entry.Type.Value)
		entries[i] = models.MetadataEntry{
			Key:       int32(entry.Key),
			HandlerID: handlerID,
			Value:     entry.Value,
		}
	}

	return entityID, entries, nil
}
func (e *entityHandler) SendInteract(conn models.PacketWriter, entityID int32, hand models.Hand, sneaking bool) error {
	pkt := sb.NewUseEntity()
	pkt.Target = pk.VarInt(entityID)
	pkt.Mouse = pk.VarInt(common.InteractionTypeInteract) // Type 0: Interact

	// For type 0, X/Y/Z are void, Hand is VarInt
	pkt.X = &protocol_models.Void{}
	pkt.Y = &protocol_models.Void{}
	pkt.Z = &protocol_models.Void{}
	handVal := pk.VarInt(hand)
	pkt.Hand = &handVal
	pkt.Sneaking = pk.Boolean(sneaking)

	log.Printf("[v1.21.5 Entity] SendInteract: entityID=%d hand=%d sneaking=%v", entityID, hand, sneaking)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseEntity", Cause: err}
	}
	return nil
}

// SendInteractAt sends an entity interaction packet at a specific position.
// This is interaction type 2 (interact at position).
func (e *entityHandler) SendInteractAt(conn models.PacketWriter, entityID int32, targetX, targetY, targetZ float32, hand models.Hand, sneaking bool) error {
	pkt := sb.NewUseEntity()
	pkt.Target = pk.VarInt(entityID)
	pkt.Mouse = pk.VarInt(common.InteractionTypeInteractAt) // Type 2: InteractAt

	// For type 2, X/Y/Z are Float, Hand is VarInt
	xVal := pk.Float(targetX)
	yVal := pk.Float(targetY)
	zVal := pk.Float(targetZ)
	pkt.X = &xVal
	pkt.Y = &yVal
	pkt.Z = &zVal
	handVal := pk.VarInt(hand)
	pkt.Hand = &handVal
	pkt.Sneaking = pk.Boolean(sneaking)

	log.Printf("[v1.21.5 Entity] SendInteractAt: entityID=%d pos=(%.2f,%.2f,%.2f) hand=%d sneaking=%v",
		entityID, targetX, targetY, targetZ, hand, sneaking)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseEntity", Cause: err}
	}
	return nil
}

// SendAttack sends an attack packet to hit an entity (left-click).
// This is interaction type 1 (attack).
func (e *entityHandler) SendAttack(conn models.PacketWriter, entityID int32, sneaking bool) error {
	pkt := sb.NewUseEntity()
	pkt.Target = pk.VarInt(entityID)
	pkt.Mouse = pk.VarInt(common.InteractionTypeAttack) // Type 1: Attack

	// For type 1, X/Y/Z and Hand are all void
	pkt.X = &protocol_models.Void{}
	pkt.Y = &protocol_models.Void{}
	pkt.Z = &protocol_models.Void{}
	pkt.Hand = &protocol_models.Void{}
	pkt.Sneaking = pk.Boolean(sneaking)

	log.Printf("[v1.21.5 Entity] SendAttack: entityID=%d sneaking=%v", entityID, sneaking)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseEntity", Cause: err}
	}
	return nil
}

// ParseSyncEntityPosition parses a SyncEntityPosition packet (clientbound ID 96).
// Returns entity ID and absolute position values.
func (e *entityHandler) ParseSyncEntityPosition(p pk.Packet) (entityID int32, x, y, z float64, dx, dy, dz float64, yaw, pitch int8, onGround bool, err error) {
	pkt := cb.NewSyncEntityPosition()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, 0, 0, false, common.ErrPacketParse{PacketName: "SyncEntityPosition", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	x = float64(pkt.X)
	y = float64(pkt.Y)
	z = float64(pkt.Z)
	// Velocity deltas are encoded as doubles
	dx = float64(pkt.Dx)
	dy = float64(pkt.Dy)
	dz = float64(pkt.Dz)
	yaw = int8(pkt.Yaw)
	pitch = int8(pkt.Pitch)
	onGround = bool(pkt.OnGround)

	return entityID, x, y, z, dx, dy, dz, yaw, pitch, onGround, nil
}

// ParseEntityVelocityUpdate parses an entity velocity update packet (EntityVelocity).
// Returns entity ID and velocity components in blocks per tick.
// Velocity is encoded as fixed-point and needs to be divided by 8000.0.
func (e *entityHandler) ParseEntityVelocityUpdate(p pk.Packet) (entityID int32, velX, velY, velZ float64, err error) {
	pkt := cb.NewEntityVelocity()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, common.ErrPacketParse{PacketName: "EntityVelocity", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	// Velocity is encoded as fixed-point divided by 8000
	velX = float64(pkt.VelocityX) / 8000.0
	velY = float64(pkt.VelocityY) / 8000.0
	velZ = float64(pkt.VelocityZ) / 8000.0

	return entityID, velX, velY, velZ, nil
}

// ParseEntityEquipment parses an entity equipment packet.
// Returns entity ID and a slice of equipment entries (slot + item pairs).
func (e *entityHandler) ParseEntityEquipment(p pk.Packet) (entityID int32, equipment []models.EquipmentEntry, err error) {
	pkt := cb.NewEntityEquipment()
	if err = pkt.Scan(p); err != nil {
		return 0, nil, common.ErrPacketParse{PacketName: "EntityEquipment", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	equipment = make([]models.EquipmentEntry, 0, len(pkt.Equipments.Values))

	// Equipments is a TopBitSetTerminatedArray of EquipmentEntry objects
	// Each entry contains both slot index and item data
	for _, entry := range pkt.Equipments.Values {
		if entry == nil {
			continue
		}

		// Convert protocol Slot to common Slot format
		item := models.InventorySlot{
			Present: int32(entry.Item.ItemCount) != 0,
			ItemID:  0, // TODO: Extract from entry.Item complex structure
			Count:   int32(entry.Item.ItemCount),
			NBT:     nil, // TODO: Extract from entry.Item complex structure
		}

		equipment = append(equipment, models.EquipmentEntry{
			InventorySlot: int32(entry.Slot),
			Item:          item,
		})

		log.Printf("[v1.21.5 Entity] ParseEntityEquipment: entityID=%d, slot=%d, itemCount=%d",
			entityID, entry.Slot, entry.Item.ItemCount)
	}

	return entityID, equipment, nil
}

// ParseEntityHeadRotation parses an entity head rotation packet.
// Returns entity ID and head yaw (0-255, where 256 represents a full rotation).
func (e *entityHandler) ParseEntityHeadRotation(p pk.Packet) (entityID int32, headYaw int8, err error) {
	pkt := cb.NewEntityHeadRotation()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, common.ErrPacketParse{PacketName: "EntityHeadRotation", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	headYaw = int8(pkt.HeadYaw)

	return entityID, headYaw, nil
}

// ParseEntityLook parses an entity look packet (rotation only, no position change).
// Returns entity ID, yaw, pitch, and onGround status.
func (e *entityHandler) ParseEntityLook(p pk.Packet) (entityID int32, yaw, pitch int8, onGround bool, err error) {
	pkt := cb.NewEntityLook()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, false, common.ErrPacketParse{PacketName: "EntityLook", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	yaw = int8(pkt.Yaw)
	pitch = int8(pkt.Pitch)
	onGround = bool(pkt.OnGround)

	return entityID, yaw, pitch, onGround, nil
}

// ParseDamageEvent parses a DamageEvent packet.
// Returns entity ID, source type ID, source cause entity ID, source direct entity ID,
// and optional source position. Entity IDs of 0 mean "no entity" (protocol sends ID+1).
func (e *entityHandler) ParseDamageEvent(p pk.Packet) (entityID int32, sourceTypeID int32, sourceCauseID int32, sourceDirectID int32, sourceX, sourceY, sourceZ float64, hasSourcePosition bool, err error) {
	pkt := cb.NewDamageEvent()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, 0, 0, 0, false, common.ErrPacketParse{PacketName: "DamageEvent", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	sourceTypeID = int32(pkt.SourceTypeId)
	// Protocol sends entity ID + 1, where 0 means "no entity"
	sourceCauseID = int32(pkt.SourceCauseId) - 1
	sourceDirectID = int32(pkt.SourceDirectId) - 1

	if pos := pkt.SourcePosition.Pointer(); pos != nil {
		hasSourcePosition = true
		sourceX = float64(pos.X)
		sourceY = float64(pos.Y)
		sourceZ = float64(pos.Z)
	}

	log.Printf("[%s][ParseDamageEvent] entityID=%d sourceType=%d causedBy=%d directBy=%d hasPos=%v pos=(%.2f,%.2f,%.2f)",
		"1.21.5", entityID, sourceTypeID, sourceCauseID, sourceDirectID, hasSourcePosition, sourceX, sourceY, sourceZ)

	return entityID, sourceTypeID, sourceCauseID, sourceDirectID, sourceX, sourceY, sourceZ, hasSourcePosition, nil
}

// ParseSetPassengers parses a ClientboundSetPassengers packet.
// Returns the vehicle entity ID and a list of passenger entity IDs.
func (e *entityHandler) ParseSetPassengers(p pk.Packet) (vehicleEntityID int32, passengerEntityIDs []int32, err error) {
	pkt := cb.NewSetPassengers()
	if err = pkt.Scan(p); err != nil {
		return 0, nil, common.ErrPacketParse{PacketName: "SetPassengers", Cause: err}
	}

	vehicleEntityID = int32(pkt.EntityId)

	// Convert passengers array to int32 slice
	passengers := pkt.Passengers.Get()
	passengerEntityIDs = make([]int32, len(passengers))
	for i, passengerID := range passengers {
		passengerEntityIDs[i] = int32(passengerID)
	}

	log.Printf("[v1.21.5 Entity] ParseSetPassengers: vehicleID=%d, passengers=%v", vehicleEntityID, passengerEntityIDs)

	return vehicleEntityID, passengerEntityIDs, nil
}


// ParseEntityUpdateAttributes parses an entity attributes update packet.
func (e *entityHandler) ParseEntityUpdateAttributes(p pk.Packet) (int32, map[string]float64, error) {
	pkt := cb.NewEntityUpdateAttributes()
	if err := pkt.Scan(p); err != nil {
		return 0, nil, common.ErrPacketParse{PacketName: "EntityUpdateAttributes", Cause: err}
	}
	attrs := make(map[string]float64)
	// Extract attribute values from the properties array
	for _, prop := range pkt.Properties.Get() {
		// Key is the attribute name string, Value is the attribute value (float64)
		attrs[prop.Key.Value] = float64(prop.Value)
	}
	return int32(pkt.EntityId), attrs, nil
}
