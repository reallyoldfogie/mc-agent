// Package v1_21_9 provides version-specific packet handling for Minecraft 1.21.9.
package v1_21_9

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/davecgh/go-spew/spew"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// entityHandler implements common.EntityHandler for 1.21.9.
type entityHandler struct {
	packetMgr protocol_models.PacketMgr
}

// ParseAddEntity parses a SpawnEntity packet (clientbound ID 1).
// Returns entity information including position, rotation, type, objectData (projectile owner ID), and velocity.
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
	velX = float64(pkt.Velocity.X) // / 8000.0 // previous version divided by 8000. We need to see if we still need to.
	velY = float64(pkt.Velocity.Y) // / 8000.0 // previous version divided by 8000. We need to see if we still need to.
	velZ = float64(pkt.Velocity.Z) // / 8000.0 // previous version divided by 8000. We need to see if we still need to.

	log.Printf("[1.21.9][ParseAddEntity] returning %d %d %d %v %.2f %.2f %.2f %d %d vel=(%.4f,%.4f,%.4f) <nil>", entityID, entityType, objectData, uuid, x, y, z, yaw, pitch, velX, velY, velZ)

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

	log.Printf("[1.21.9][ParseMoveEntityPos] %d received %s", entityID, spew.Sdump(pkt))
	log.Printf("[1.21.9][ParseMoveEntityPos] returning %d %d %d %d %v <nil>", entityID, dx, dy, dz, onGround)

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

	log.Printf("[1.21.9][ParseMoveEntityPosRot] returning %d %d %d %d %d %d %v <nil>", entityID, dx, dy, dz, yaw, pitch, onGround)

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

	log.Printf("[1.21.9][ParseTeleportEntity] returning %d %.2f %.2f %.2f %d %d %v <nil>", entityID, x, y, z, yaw, pitch, onGround)

	return entityID, x, y, z, yaw, pitch, onGround, nil
}

// ParseSyncEntityPosition parses a SyncEntityPosition packet (clientbound ID 31).
// Returns entity ID, absolute position, velocity deltas, rotation, and onGround.
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

	log.Printf("[1.21.9][ParseSyncEntityPosition] returning %d pos=(%.2f,%.2f,%.2f) vel=(%.4f,%.4f,%.4f) rot=(%d,%d) ground=%v <nil>",
		entityID, x, y, z, dx, dy, dz, yaw, pitch, onGround)

	return entityID, x, y, z, dx, dy, dz, yaw, pitch, onGround, nil
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

	log.Printf("[1.21.9][ParseRemoveEntities] returning %v <nil>", entityIDs)

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

	log.Printf("[1.21.9][ParseEntityEvent] returning %d %d <nil>", entityID, eventID)

	return entityID, eventID, nil
}

// ParseSetEntityMetadata parses an entity metadata update packet.
// Returns all metadata entries with handler IDs extracted from the protocol packet.
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
		// Extract the handler type from the metadata entry
		// The Type field contains the handler type string (e.g., "byte", "float", "vector3")
		handlerID := models.HandlerTypeFromString(entry.Type.Value)

		entries[i] = models.MetadataEntry{
			Key:       int32(entry.Key),
			HandlerID: handlerID,
			Value:     entry.Value,
		}

		log.Printf("[1.21.9][ParseSetEntityMetadata] Entry %d: key=%d handler=%s value=%T",
			i, entry.Key, handlerID.String(), entry.Value)
	}

	log.Printf("[1.21.9][ParseSetEntityMetadata] returning entityID=%d with %d metadata entries <nil>", entityID, len(entries))
	return entityID, entries, nil
}

// ParseEntityVelocityUpdate parses an entity velocity update packet (EntityVelocity).
// Returns entity ID and velocity components in blocks per tick.
// Velocity is encoded as fixed-point and needs to be divided by 8000.0.
func (e *entityHandler) ParseEntityVelocityUpdate(p pk.Packet) (entityID int32, velX, velY, velZ float64, err error) {
	pkt := cb.NewEntityVelocity()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, common.ErrPacketParse{PacketName: "EntityVelocity", Cause: err}
	}

	log.Printf("[1.21.9][ParseEntityVelocityUpdate] Raw parsed packet: %#v", *pkt)

	entityID = int32(pkt.EntityId)
	// Velocity is encoded as fixed-point divided by 8000
	velX = float64(pkt.Velocity.X) // / 8000.0 // previous version divided by 8000. We need to see if we still need to.
	velY = float64(pkt.Velocity.Y) // / 8000.0 // previous version divided by 8000. We need to see if we still need to.
	velZ = float64(pkt.Velocity.Z) // / 8000.0 // previous version divided by 8000. We need to see if we still need to.

	log.Printf("[1.21.9][ParseEntityVelocityUpdate] returning %d %.6f %.6f %.6f <nil>", entityID, velX, velY, velZ)

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
			ItemID:  slotItemID(entry.Item),
			Count:   int32(entry.Item.ItemCount),
			NBT:     nil, // TODO: Extract from entry.Item complex structure
		}

		equipment = append(equipment, models.EquipmentEntry{
			InventorySlot: int32(entry.Slot),
			Item:          item,
		})

		log.Printf("[v1.21.9 Entity][ParseEntityEquipment]: entityID=%d, slot=%d, itemCount=%d",
			entityID, entry.Slot, entry.Item.ItemCount)
	}

	return entityID, equipment, nil
}

// ParseEntityEffect parses a status-effect-applied packet
// (ClientboundEntityEffect, aliased ClientboundUpdateMobEffect).
func (e *entityHandler) ParseEntityEffect(p pk.Packet) (entityID, effectID, amplifier, durationTicks int32, ambient, showParticles, showIcon bool, err error) {
	pkt := cb.NewEntityEffect()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, false, false, false, common.ErrPacketParse{PacketName: "EntityEffect", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	effectID = int32(pkt.EffectId)
	amplifier = int32(pkt.Amplifier)
	durationTicks = int32(pkt.Duration)
	flags := byte(pkt.Flags)
	ambient = flags&0x01 != 0
	showParticles = flags&0x02 != 0
	showIcon = flags&0x04 != 0

	log.Printf("[ParseEntityEffect] entityID=%d effectID=%d amplifier=%d duration=%d ambient=%v particles=%v icon=%v",
		entityID, effectID, amplifier, durationTicks, ambient, showParticles, showIcon)

	return entityID, effectID, amplifier, durationTicks, ambient, showParticles, showIcon, nil
}

// ParseRemoveEntityEffect parses a status-effect-removed packet
// (ClientboundRemoveEntityEffect, aliased ClientboundRemoveMobEffect).
func (e *entityHandler) ParseRemoveEntityEffect(p pk.Packet) (entityID, effectID int32, err error) {
	pkt := cb.NewRemoveEntityEffect()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, common.ErrPacketParse{PacketName: "RemoveEntityEffect", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	effectID = int32(pkt.EffectId)

	log.Printf("[ParseRemoveEntityEffect] entityID=%d effectID=%d", entityID, effectID)

	return entityID, effectID, nil
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

	log.Printf("[1.21.9][ParseEntityHeadRotation] returning %d %d <nil>", entityID, headYaw)

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

	log.Printf("[1.21.9][ParseEntityLook] returning %d %d %d %v <nil>", entityID, yaw, pitch, onGround)

	return entityID, yaw, pitch, onGround, nil
}

// SendInteract sends an entity interaction packet (right-click with hand).
func (e *entityHandler) SendInteract(conn models.PacketWriter, entityID int32, hand models.Hand, sneaking bool) error {
	pkt := sb.NewUseEntity()
	pkt.Target = pk.VarInt(entityID)
	pkt.Mouse = pk.VarInt(common.InteractionTypeInteract)
	pkt.X = &protocol_models.Void{}
	pkt.Y = &protocol_models.Void{}
	pkt.Z = &protocol_models.Void{}
	handVal := pk.VarInt(hand)
	pkt.Hand = &handVal
	pkt.Sneaking = pk.Boolean(sneaking)

	log.Printf("[v1.21.9 Entity] SendInteract: entityID=%d hand=%d sneaking=%v", entityID, hand, sneaking)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseEntity", Cause: err}
	}
	return nil
}

// SendInteractAt sends an entity interaction packet at a specific position.
func (e *entityHandler) SendInteractAt(conn models.PacketWriter, entityID int32, targetX, targetY, targetZ float32, hand models.Hand, sneaking bool) error {
	pkt := sb.NewUseEntity()
	pkt.Target = pk.VarInt(entityID)
	pkt.Mouse = pk.VarInt(common.InteractionTypeInteractAt)
	xVal := pk.Float(targetX)
	yVal := pk.Float(targetY)
	zVal := pk.Float(targetZ)
	pkt.X = &xVal
	pkt.Y = &yVal
	pkt.Z = &zVal
	handVal := pk.VarInt(hand)
	pkt.Hand = &handVal
	pkt.Sneaking = pk.Boolean(sneaking)

	log.Printf("[v1.21.9 Entity] SendInteractAt: entityID=%d pos=(%.2f,%.2f,%.2f) hand=%d sneaking=%v",
		entityID, targetX, targetY, targetZ, hand, sneaking)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseEntity", Cause: err}
	}
	return nil
}

// SendAttack sends an attack packet to hit an entity (left-click).
func (e *entityHandler) SendAttack(conn models.PacketWriter, entityID int32, sneaking bool) error {
	pkt := sb.NewUseEntity()
	pkt.Target = pk.VarInt(entityID)
	pkt.Mouse = pk.VarInt(common.InteractionTypeAttack)
	pkt.X = &protocol_models.Void{}
	pkt.Y = &protocol_models.Void{}
	pkt.Z = &protocol_models.Void{}
	pkt.Hand = &protocol_models.Void{}
	pkt.Sneaking = pk.Boolean(sneaking)

	log.Printf("[v1.21.9 Entity] SendAttack: entityID=%d sneaking=%v", entityID, sneaking)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseEntity", Cause: err}
	}
	return nil
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
		"1.21.9", entityID, sourceTypeID, sourceCauseID, sourceDirectID, hasSourcePosition, sourceX, sourceY, sourceZ)

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

	log.Printf("[v1.21.9 Entity] ParseSetPassengers: vehicleID=%d, passengers=%v", vehicleEntityID, passengerEntityIDs)

	return vehicleEntityID, passengerEntityIDs, nil
}

// ParseEntityUpdateAttributes parses an entity attributes update packet.
func (e *entityHandler) ParseEntityUpdateAttributes(p pk.Packet) (int32, map[string]models.AttributeValue, error) {
	pkt := cb.NewEntityUpdateAttributes()
	if err := pkt.Scan(p); err != nil {
		return 0, nil, common.ErrPacketParse{PacketName: "EntityUpdateAttributes", Cause: err}
	}
	attrs := make(map[string]models.AttributeValue)
	// Extract each attribute's base value and live modifiers from the
	// properties array. Modifiers were previously discarded here (only
	// prop.Value was read) — see models.AttributeValue.Compute for applying
	// them the way vanilla does.
	for _, prop := range pkt.Properties.Get() {
		modifiers := make([]models.AttributeModifier, 0, len(prop.Modifiers.Get()))
		for _, mod := range prop.Modifiers.Get() {
			modifiers = append(modifiers, models.AttributeModifier{
				Amount:    float64(mod.Amount),
				Operation: models.AttributeOperation(mod.Operation),
			})
		}
		attrs[correctAttributeKey(prop.Key.Value)] = models.AttributeValue{
			Base:      float64(prop.Value),
			Modifiers: modifiers,
		}
	}
	return int32(pkt.EntityId), attrs, nil
}

// correctAttributeKey works around a stale mc-protocol-go attribute-ID
// mapping table for ClientboundEntityUpdateAttributes: this version's
// generated EntityUpdateAttributesPropertiesArrayTypeKeyMappings table is
// missing several attributes Mojang added over time (at minimum
// player.mining_efficiency, generic.movement_efficiency,
// generic.oxygen_bonus, and player.sneaking_speed for this version), which
// shifts every later attribute's numeric wire ID and makes the parser
// report the WRONG name for that ID — confirmed directly against decompiled
// net/minecraft/entity/attribute/EntityAttributes.java's registration
// order. See mc-protocol-go/docs/bugs for the full analysis with citations,
// tracked for an eventual upstream fix via a PrismarineJS/minecraft-data
// patch. TEMPORARY — delete this function and its call site once that
// lands.
//
// Scoped to just generic.movement_speed, the one attribute this codebase
// currently reads that's affected (Speed/Slowness, mount speed resolution),
// not a full correction of every attribute the same corruption shifts.
func correctAttributeKey(wireLabel string) string {
	if wireLabel == "generic.scale" {
		return "generic.movement_speed"
	}
	return wireLabel
}
