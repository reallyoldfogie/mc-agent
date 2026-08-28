package models

import pk "github.com/Tnze/go-mc/net/packet"

// MovementMirror consumes serverbound packets and may synthesize clientbound packets.
type MovementMirror interface {
	HandleServerbound(pk.Packet)
	SetEntityMeta(entityID int32, name string, uuid [16]byte)
	SetEntityType(entityType int32)
	HandlePlayerInfo(pk.Packet)
	NotifyLoginSeen()

	// EmitEquipment synthesizes a ClientboundEntityEquipment packet for the agent's own
	// entity and records it into the replay stream. This is needed because the server
	// only sends EntityEquipment to other players, not to the player who changed equipment.
	// slot: any EquipmentSlotType (hand, armor, or body)
	// itemID: protocol item ID (0 for empty slot)
	// count: item count (0 for empty slot)
	EmitEquipment(entityID int32, slot EquipmentSlotType, itemID int32, count int32)
}
