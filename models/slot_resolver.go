package models

// SlotResolver resolves a slot into an item ID and count.
type SlotResolver interface {
	ResolveSlot(id int, index int16) (itemID int32, count int, ok bool)
}
