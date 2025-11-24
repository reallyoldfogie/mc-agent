package types

import (
	world_entity "github.com/Tnze/go-mc/bot/world/entity"
)

// InventoryAccess -
type InventoryAccess interface {
	GetInventoryItem(slotID int) *world_entity.Slot
}
