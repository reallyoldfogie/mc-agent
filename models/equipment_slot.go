package models

// EquipmentSlotType identifies a slot in the ClientboundEntityEquipment packet
// (also known as ClientboundSetEquipment; both names resolve to the same packet).
type EquipmentSlotType int32

// Equipment slot indices.
//
// These are taken from the generated protocol data's equippable-slot mapping
// (mc-protocol-go data/<version>/basetypes/mappers.go,
// SlotComponentDataEquippableSlotMappings), which is identical across every
// supported version except for the saddle slot.
//
// Do not confuse this enum with the attribute-modifier slot-group enum in the
// same file, which numbers things differently (it includes "any", "hand" and
// "armor" grouping entries and puts saddle at 10).
const (
	EquipmentSlotMainHand EquipmentSlotType = 0
	EquipmentSlotOffHand  EquipmentSlotType = 1
	EquipmentSlotFeet     EquipmentSlotType = 2
	EquipmentSlotLegs     EquipmentSlotType = 3
	EquipmentSlotChest    EquipmentSlotType = 4
	EquipmentSlotHead     EquipmentSlotType = 5
	EquipmentSlotBody     EquipmentSlotType = 6

	// EquipmentSlotSaddle exists only in 1.21.5 and later. Before that the
	// saddle was not an equipment slot at all: it lived in the mount's own
	// inventory as a SaddleItem NBT compound and was never sent to the client
	// in the equipment packet. See docs/horse-nbt-data.md.
	EquipmentSlotSaddle EquipmentSlotType = 7
)

// String returns the protocol name for the slot, matching the generated
// mapping's values.
func (slot EquipmentSlotType) String() string {
	switch slot {
	case EquipmentSlotMainHand:
		return "main_hand"
	case EquipmentSlotOffHand:
		return "off_hand"
	case EquipmentSlotFeet:
		return "feet"
	case EquipmentSlotLegs:
		return "legs"
	case EquipmentSlotChest:
		return "chest"
	case EquipmentSlotHead:
		return "head"
	case EquipmentSlotBody:
		return "body"
	case EquipmentSlotSaddle:
		return "saddle"
	default:
		return "unknown"
	}
}

// MinSaddleSlotVersion is the first Minecraft version in which the saddle is
// carried in the equipment packet rather than the mount's NBT inventory.
// Callers use it to decide whether the absence of a saddle slot means
// "not saddled" or merely "cannot tell from here".
const MinSaddleSlotVersion = "1.21.5"
