package models

// SlotResolver resolves a slot into an item ID and count.
type SlotResolver interface {
	ResolveSlot(id, index int) (itemID int, count int, ok bool)
}
