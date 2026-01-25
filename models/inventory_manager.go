package models

import "time"

type InventoryManager interface {
	SetWindow(windowID byte)
	GetWindow() byte
	SetCursor(item ItemStack)
	GetCursor() ItemStack
	SetWaitForUpdates(wait bool)
	SetUpdateWaitDelay(delay time.Duration)
	LeftClickSlot(slot int16, slotItem ItemStack) error
	RightClickSlot(slot int16, slotItem ItemStack) error
	ShiftClickSlot(slot int16, slotItem ItemStack) error
	SwapWithHotbar(slot int16, slotItem ItemStack, hotbarSlot int, hotbarItem ItemStack) error
	DropItem(slot int16, slotItem ItemStack) error
	DropStack(slot int16, slotItem ItemStack) error
	DoubleClick(slot int16, slotItem ItemStack) error
	StartDrag(dragType byte) error
	AddDragSlot(slot int16, dragType byte) error
	EndDrag(dragType byte, changedSlots []ChangedSlot) error
	MoveItem(fromSlot, toSlot int16, fromItem, toItem ItemStack) error
	MoveSingle(fromSlot, toSlot int16, fromItem, toItem ItemStack) error
	TransferItem(fromWindowID byte, fromSlot int16, toWindowID byte, toSlot int16, fromItem, toItem ItemStack) error
	TransferStack(slot int16, slotItem ItemStack) error
	SplitStack(sourceSlot, destSlot int16, sourceItem, destItem ItemStack) error
	DistributeItems(slots []int16, evenlyDistribute bool, changedSlots []ChangedSlot) error
}

// ItemStack represents an item in a slot
type ItemStack struct {
	ItemID           int32           // Item ID (0 = empty/air)
	Count            int8            // Stack size (1-127, 0 = empty)
	NBT              []byte          // Legacy NBT data (unused in 1.21+ component format)
	Components       []ItemComponent // 1.21+ component payloads
	RemoveComponents []int32         // 1.21+ component removals
}

// ItemComponent represents a 1.21+ item component payload.
type ItemComponent struct {
	Type int32
	Data any
}

// IsEmpty returns true if this slot contains no item
func (s ItemStack) IsEmpty() bool {
	return s.ItemID <= 0 || s.Count <= 0
}

// ChangedSlot represents a slot that changed during a click operation
type ChangedSlot struct {
	SlotIndex int16
	Item      ItemStack
}
