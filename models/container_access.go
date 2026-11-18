package models

import "time"

// ContainerAccess defines shared container interaction methods.
type ContainerAccess interface {
	OpenContainer(pos V3, face BlockFace, timeout time.Duration, cursorX, cursorY, cursorZ float32) (byte, error)
	OpenEntityContainer(entityID int32, timeout time.Duration) (byte, error)
	CloseContainer() error
	TakeItemFromChest(windowID byte, chestSlot int16) error
	PutItemInChest(windowID byte, playerInventorySlot int16, chestSlot int16) error
	FindItemInPlayerInventory(windowID byte, itemID int32) int16
	FindEmptyChestSlot(windowID byte) int16
	GetChestRows(windowID byte) int
	GetContainerSlotCount(windowID byte) int

	// ChestSlots returns windowID's chest-section slots (excluding the
	// player's own inventory section), or nil if windowID isn't a
	// currently-open chest — see the real implementation's own doc
	// comment (items/container_helper.go) for why this exists.
	ChestSlots(windowID byte) []ItemStack

	// ReportSlotObserved reports a single passively-observed chest-slot
	// change for windowID — one this agent's own
	// TakeItemFromChest/PutItemInChest didn't necessarily cause, e.g. a
	// concurrent player's own edit to a shared chest, or (1.21.9+) a
	// copper golem moving items inside one while this agent also has it
	// open. A no-op if windowID isn't a currently-open chest this agent
	// has a recorded position for, or slotIndex falls in the window's
	// player-inventory section rather than its chest section — see the
	// real implementation's own doc comment.
	ReportSlotObserved(windowID byte, slotIndex int16, item InventorySlot)

	// ReportWindowObserved is ReportSlotObserved's bulk counterpart, for
	// a full-window resync rather than one slot at a time.
	ReportWindowObserved(windowID byte, slots []InventorySlot)
}

// EntityInventoryAccess exposes cached entity container contents.
//
// This is deliberately separate from ContainerAccess. Everything in
// ContainerAccess operates on a currently-open window and is implemented by the
// container helper, which has no entity model. The entity inventory cache is
// agent-level state keyed by entity ID and outlives the window, so it belongs
// on its own interface.
type EntityInventoryAccess interface {
	// GetEntityInventory returns the cached contents of an entity's own
	// container (donkey/mule/llama chest, chest boat, chest minecart), and
	// whether one has ever been captured.
	//
	// The snapshot survives the window closing, at which point it also stops
	// being authoritative, so honour EntityInventory.Live and UpdatedAt.
	GetEntityInventory(entityID int32) (EntityInventory, bool)
}
