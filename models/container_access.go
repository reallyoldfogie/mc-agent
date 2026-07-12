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
}
