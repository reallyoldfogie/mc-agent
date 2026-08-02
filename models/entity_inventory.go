package models

import "time"

// PlayerInventorySlotCount is how many slots any container window appends for
// the player's own inventory: 27 main slots followed by the 9-slot hotbar.
//
// Container windows are laid out as [container slots...][player inventory...],
// and the player section is always this size, so subtracting it from the total
// slot count yields the number of slots that belong to the container itself.
// That is how an entity's own inventory is separated from ours without needing
// to know the specific window type.
const PlayerInventorySlotCount = 36

// EntityInventory is a cached snapshot of an entity's own container contents,
// for example a donkey's or llama's chest, or a chest boat's storage.
//
// This is a snapshot, not a live view. The server only reports container
// contents while the window is open, so once the window closes the contents can
// change without us hearing about it (a hopper empties the chest, another
// player loots it). Callers must treat a snapshot with Live == false as a hint
// about what was there at UpdatedAt, not as current truth, and re-open the
// container when accuracy matters.
type EntityInventory struct {
	// Slots holds only the entity's own slots. The player-inventory section
	// that the window appends is excluded, so index 0 is the entity's first
	// slot.
	Slots []InventorySlot

	// UpdatedAt is when this snapshot was last refreshed from the server.
	UpdatedAt time.Time

	// Live reports whether the container window was still open as of the last
	// update. While true the snapshot tracks the server; once false it is
	// frozen and starts going stale immediately.
	Live bool
}

// Age returns how long it has been since this snapshot was refreshed.
func (inv EntityInventory) Age(now time.Time) time.Duration {
	return now.Sub(inv.UpdatedAt)
}

// IsEmpty reports whether every known slot is empty. An inventory that has
// never been observed also reads as empty, so check the snapshot exists first.
func (inv EntityInventory) IsEmpty() bool {
	for _, slot := range inv.Slots {
		if slot.Count > 0 {
			return false
		}
	}
	return true
}

// SplitContainerSlots divides a full container window's slot list into the
// container's own slots and the trailing player-inventory section.
//
// Returns ok == false when the window is too small to contain a player
// inventory section, which means the layout is not the expected
// [container][player] shape and the split would be meaningless.
func SplitContainerSlots(windowSlots []InventorySlot) (containerSlots, playerSlots []InventorySlot, ok bool) {
	if len(windowSlots) < PlayerInventorySlotCount {
		return nil, nil, false
	}
	boundary := len(windowSlots) - PlayerInventorySlotCount
	return windowSlots[:boundary], windowSlots[boundary:], true
}
