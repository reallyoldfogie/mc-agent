package models

import mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"

// ScreenAccess defines core screen/inventory lookups.
type ScreenAccess interface {
	GetScreen(windowID int) Screen
	GetInventory() mcscreen.Inventory
	GetCursor() Slot
}
