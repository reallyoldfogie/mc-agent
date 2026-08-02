package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEquipmentSlotIndices pins the wire indices against the generated protocol
// mapping (mc-protocol-go SlotComponentDataEquippableSlotMappings). If a future
// Minecraft version renumbers these, saddle detection would silently read the
// wrong slot, so the values are asserted literally rather than derived.
func TestEquipmentSlotIndices(t *testing.T) {
	assert.Equal(t, EquipmentSlotType(0), EquipmentSlotMainHand)
	assert.Equal(t, EquipmentSlotType(1), EquipmentSlotOffHand)
	assert.Equal(t, EquipmentSlotType(2), EquipmentSlotFeet)
	assert.Equal(t, EquipmentSlotType(3), EquipmentSlotLegs)
	assert.Equal(t, EquipmentSlotType(4), EquipmentSlotChest)
	assert.Equal(t, EquipmentSlotType(5), EquipmentSlotHead)
	assert.Equal(t, EquipmentSlotType(6), EquipmentSlotBody)
	assert.Equal(t, EquipmentSlotType(7), EquipmentSlotSaddle)
}

// TestEquipmentSlotSaddleIsNotBody guards the easiest mistake here: body (6)
// and saddle (7) are adjacent, and the happy ghast harness will need one of
// them. Reading body when we mean saddle would report every armoured horse as
// saddled.
func TestEquipmentSlotSaddleIsNotBody(t *testing.T) {
	assert.NotEqual(t, EquipmentSlotBody, EquipmentSlotSaddle)
	assert.Equal(t, "body", EquipmentSlotBody.String())
	assert.Equal(t, "saddle", EquipmentSlotSaddle.String())
}

func TestEquipmentSlotStringNames(t *testing.T) {
	// Names match the protocol mapping's values exactly.
	expected := map[EquipmentSlotType]string{
		EquipmentSlotMainHand: "main_hand",
		EquipmentSlotOffHand:  "off_hand",
		EquipmentSlotFeet:     "feet",
		EquipmentSlotLegs:     "legs",
		EquipmentSlotChest:    "chest",
		EquipmentSlotHead:     "head",
		EquipmentSlotBody:     "body",
		EquipmentSlotSaddle:   "saddle",
	}
	for slot, name := range expected {
		assert.Equal(t, name, slot.String())
	}

	assert.Equal(t, "unknown", EquipmentSlotType(99).String())
	assert.Equal(t, "unknown", EquipmentSlotType(-1).String())
}

func TestMinSaddleSlotVersion(t *testing.T) {
	// The saddle became an equipment slot in 1.21.5; before that it was a
	// SaddleItem NBT compound in the mount's inventory. See docs/horse-nbt-data.md.
	assert.Equal(t, "1.21.5", MinSaddleSlotVersion)
}
