package models

// ScreenAccess defines core screen/inventory lookups.
type ScreenAccess interface {
	GetScreen(windowID int) Screen
	GetInventory() Screen
	GetCursor() Slot
}
