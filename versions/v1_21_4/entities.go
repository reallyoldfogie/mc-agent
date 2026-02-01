// Package v1_21_5 provides version-specific packet handling for Minecraft 1.21.4.
package v1_21_4

import (
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/play/clientbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// entityHandler implements common.EntityHandler for 1.21.4.
type entityHandler struct {
	packetMgr protocol_models.PacketMgr
}

// ParseAddEntity parses a SpawnEntity packet (clientbound ID 1).
// Returns entity information including position, rotation, and type.
func (e *entityHandler) ParseAddEntity(p pk.Packet) (entityID, entityType int32, uuid [16]byte, x, y, z float64, yaw, pitch int8, err error) {
	pkt := cb.NewSpawnEntity()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, uuid, 0, 0, 0, 0, 0, common.ErrPacketParse{PacketName: "SpawnEntity", Cause: err}
	}

	entityID = int32(pkt.EntityId)
	entityType = int32(pkt.Type)
	uuid = [16]byte(pkt.ObjectUUID)
	x = float64(pkt.X)
	y = float64(pkt.Y)
	z = float64(pkt.Z)
	// Protocol uses bytes for angles (0-255 maps to 0-360 degrees)
	yaw = int8(pkt.Yaw)
	pitch = int8(pkt.Pitch)

	return entityID, entityType, uuid, x, y, z, yaw, pitch, nil
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
	entityIDs = make([]int32, len(*ids))
	for i, id := range *ids {
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
