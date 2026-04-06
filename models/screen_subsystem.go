package models

import (
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// ScreenSubsystem provides access to inventory and container state.
type ScreenSubsystem interface {
	GetScreenByID(windowID int) mcscreen.Container
	GetPlayerInventory() mcscreen.Inventory
	GetCursorSlot() mcscreen.Slot
}
