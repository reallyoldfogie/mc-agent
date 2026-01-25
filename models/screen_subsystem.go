package models

import "github.com/reallyoldfogie/mc-bot-go/bot/screen"

// ScreenSubsystem provides access to inventory and container state.
type ScreenSubsystem interface {
	GetScreenByID(windowID int) screen.Container
	GetPlayerInventory() *screen.Inventory
	GetCursorSlot() screen.Slot
}
