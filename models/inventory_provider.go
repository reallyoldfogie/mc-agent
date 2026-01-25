package models

// InventoryProvider interface for querying inventory contents.
type InventoryProvider interface {
	GetSlot(slotIndex int) (itemID int32, count int, ok bool)
	GetSlotCount() int
}
