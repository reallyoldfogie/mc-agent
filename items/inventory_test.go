package items

import (
	"testing"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/require"
)

type clickCall struct {
	windowID int
	slot     int16
	button   byte
	mode     int32
	changed  mcscreen.ChangedSlots
	carried  *mcscreen.Slot
}

type fakeScreen struct {
	calls         []clickCall
	cursor        mcscreen.Slot
	slots         map[int]mcscreen.Slot
	err           error
	updateVersion int64
}

func (f *fakeScreen) ContainerClick(id int, slot int16, button byte, mode int32, slots mcscreen.ChangedSlots, carried *mcscreen.Slot) error {
	f.calls = append(f.calls, clickCall{
		windowID: id,
		slot:     slot,
		button:   button,
		mode:     mode,
		changed:  slots,
		carried:  carried,
	})
	if f.slots != nil {
		for idx, slotData := range slots {
			if slotData == nil {
				delete(f.slots, idx)
				continue
			}
			f.slots[idx] = *slotData
		}
	}
	f.updateVersion++
	return f.err
}

func (f *fakeScreen) CursorSlot() mcscreen.Slot {
	return f.cursor
}

func (f *fakeScreen) SlotAt(windowID int, slot int) (mcscreen.Slot, bool) {
	if f.slots == nil {
		return mcscreen.Slot{}, false
	}
	slotData, ok := f.slots[slot]
	return slotData, ok
}

func (f *fakeScreen) SetSlotAt(windowID int, slot int, data mcscreen.Slot) bool {
	if slot < 0 {
		return false
	}
	if f.slots == nil {
		f.slots = map[int]mcscreen.Slot{}
	}
	f.slots[slot] = data
	return true
}

func (f *fakeScreen) SetCursorSlot(slot mcscreen.Slot) {
	f.cursor = slot
}

func (f *fakeScreen) ServerUpdateVersion() int64 {
	return f.updateVersion
}

func TestSlotConversionComponents(t *testing.T) {
	stack := models.ItemStack{
		ItemID: 7,
		Count:  3,
		Components: []models.ItemComponent{
			{Type: 1, Data: pk.String("hi")},
		},
		RemoveComponents: []int32{4},
	}

	slot := slotFromItemStack(stack)
	roundTrip := itemStackFromSlot(*slot)

	require.Equal(t, stack.ItemID, roundTrip.ItemID)
	require.Equal(t, stack.Count, roundTrip.Count)
	require.Equal(t, stack.Components, roundTrip.Components)
	require.Equal(t, stack.RemoveComponents, roundTrip.RemoveComponents)
}

func TestLeftClickSlotPickUp(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	slotItem := models.ItemStack{ItemID: 5, Count: 4}
	screen.slots = map[int]mcscreen.Slot{
		10: *slotFromItemStack(slotItem),
	}
	err := im.LeftClickSlot(10, slotItem)
	require.NoError(t, err)
	require.Equal(t, slotItem, im.cursor)
	require.Len(t, screen.calls, 1)

	call := screen.calls[0]
	require.Equal(t, int(im.windowID), call.windowID)
	require.Equal(t, int16(10), call.slot)
	require.Equal(t, byte(LeftButton), call.button)
	require.Equal(t, int32(ClickModeNormal), call.mode)
	require.NotNil(t, call.changed[10])
	require.Equal(t, int32(0), int32(call.changed[10].Count))
}

func TestLeftClickSlotPlace(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	cursor := models.ItemStack{ItemID: 5, Count: 4}
	slotItem := models.ItemStack{ItemID: 1, Count: 2}
	screen.cursor = *slotFromItemStack(cursor)
	screen.slots = map[int]mcscreen.Slot{
		2: *slotFromItemStack(slotItem),
	}

	err := im.LeftClickSlot(2, slotItem)
	require.NoError(t, err)
	require.Equal(t, slotItem, im.cursor)
	require.Len(t, screen.calls, 1)

	call := screen.calls[0]
	require.Equal(t, int32(5), int32(call.changed[2].ID))
}

func TestRightClickSlotPickHalf(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	slotItem := models.ItemStack{ItemID: 5, Count: 5}
	screen.slots = map[int]mcscreen.Slot{
		3: *slotFromItemStack(slotItem),
	}
	err := im.RightClickSlot(3, slotItem)
	require.NoError(t, err)
	require.Equal(t, int8(3), im.cursor.Count)
	require.Len(t, screen.calls, 1)

	call := screen.calls[0]
	require.Equal(t, int32(2), int32(call.changed[3].Count))
}

func TestRightClickSlotPlaceOneSameItem(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	cursor := models.ItemStack{ItemID: 5, Count: 2}
	slotItem := models.ItemStack{ItemID: 5, Count: 1}
	screen.cursor = *slotFromItemStack(cursor)
	screen.slots = map[int]mcscreen.Slot{
		8: *slotFromItemStack(slotItem),
	}

	err := im.RightClickSlot(8, slotItem)
	require.NoError(t, err)
	require.Equal(t, int8(1), im.cursor.Count)
	require.Len(t, screen.calls, 1)

	call := screen.calls[0]
	require.Equal(t, int32(2), int32(call.changed[8].Count))
}

func TestRightClickSlotSwapDifferentItem(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	cursor := models.ItemStack{ItemID: 5, Count: 2}
	slotItem := models.ItemStack{ItemID: 7, Count: 1}
	screen.cursor = *slotFromItemStack(cursor)
	screen.slots = map[int]mcscreen.Slot{
		1: *slotFromItemStack(slotItem),
	}

	err := im.RightClickSlot(1, slotItem)
	require.NoError(t, err)
	require.Equal(t, slotItem, im.cursor)
	require.Len(t, screen.calls, 1)
}

func TestShiftClickSlot(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	slotItem := models.ItemStack{ItemID: 5, Count: 1}
	screen.slots = map[int]mcscreen.Slot{
		4: *slotFromItemStack(slotItem),
	}
	err := im.ShiftClickSlot(4, slotItem)
	require.NoError(t, err)
	require.Len(t, screen.calls, 1)

	call := screen.calls[0]
	require.Equal(t, int32(ClickModeShift), call.mode)
}

func TestSwapWithHotbarInvalid(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	err := im.SwapWithHotbar(4, models.ItemStack{ItemID: 1, Count: 1}, 9, models.ItemStack{})
	require.NoError(t, err)
	require.Len(t, screen.calls, 0)
}

func TestDropItemAndStack(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	screen.slots = map[int]mcscreen.Slot{
		5: *slotFromItemStack(models.ItemStack{ItemID: 1, Count: 1}),
		6: *slotFromItemStack(models.ItemStack{ItemID: 2, Count: 3}),
	}
	err := im.DropItem(5, models.ItemStack{ItemID: 1, Count: 1})
	require.NoError(t, err)
	err = im.DropStack(6, models.ItemStack{ItemID: 2, Count: 3})
	require.NoError(t, err)
	require.Len(t, screen.calls, 2)
}

func TestMoveSingle(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	fromItem := models.ItemStack{ItemID: 5, Count: 4}
	screen.slots = map[int]mcscreen.Slot{
		1: *slotFromItemStack(fromItem),
		2: *slotFromItemStack(models.ItemStack{}),
	}
	err := im.MoveSingle(1, 2, fromItem, models.ItemStack{})
	require.NoError(t, err)
	require.True(t, im.cursor.IsEmpty())
	require.Len(t, screen.calls, 3)
}

func TestDistributeItems(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	err := im.DistributeItems([]int16{1, 2, 3}, true, nil)
	require.NoError(t, err)
	require.Len(t, screen.calls, 5)

	require.Equal(t, int16(-999), screen.calls[0].slot)
	require.Equal(t, byte(0), screen.calls[0].button)
	require.Equal(t, int16(-999), screen.calls[4].slot)
}

func TestClickSlotWithChanges(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	changes := []models.ChangedSlot{{SlotIndex: 2, Item: models.ItemStack{ItemID: 3, Count: 1}}}
	err := im.ClickSlotWithChanges(0, 2, LeftButton, ClickModeNormal, changes)
	require.NoError(t, err)
	require.Len(t, screen.calls, 1)

	call := screen.calls[0]
	require.Equal(t, int16(2), call.slot)
	require.Equal(t, byte(LeftButton), call.button)
	require.Equal(t, int32(ClickModeNormal), call.mode)
	require.Equal(t, pk.VarInt(3), call.changed[2].ID)
}

func TestClickSlotNoScreen(t *testing.T) {
	im := newInventoryManagerWithClicker(nil)
	err := im.ClickSlot(0, 1, LeftButton, ClickModeNormal)
	require.Error(t, err)
}

func TestSlotItemForFallback(t *testing.T) {
	screen := &fakeScreen{
		slots: map[int]mcscreen.Slot{
			1: *slotFromItemStack(models.ItemStack{ItemID: 9, Count: 2}),
		},
	}
	im := newInventoryManagerWithClicker(screen)
	fallback := models.ItemStack{ItemID: 2, Count: 1}

	require.Equal(t, fallback, im.slotItemFor(-1, fallback))
	require.Equal(t, fallback, im.slotItemFor(2, fallback))
	require.Equal(t, models.ItemStack{ItemID: 9, Count: 2}, im.slotItemFor(1, fallback))
}

func TestWaitForSlotAndCursorMatch(t *testing.T) {
	screen := &fakeScreen{
		cursor: *slotFromItemStack(models.ItemStack{ItemID: 4, Count: 1}),
		slots: map[int]mcscreen.Slot{
			3: *slotFromItemStack(models.ItemStack{ItemID: 7, Count: 2}),
		},
	}
	im := newInventoryManagerWithClicker(screen)
	im.SetWaitForUpdates(true)
	im.SetUpdateWaitDelay(200 * time.Millisecond)

	err := im.waitForSlotAndCursor(3, models.ItemStack{}, models.ItemStack{}, models.ItemStack{ItemID: 7, Count: 2}, models.ItemStack{ItemID: 4, Count: 1})
	require.NoError(t, err)
	require.Equal(t, int8(1), im.cursor.Count)
}

func TestTransferItemWindowMismatch(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)
	err := im.TransferItem(0, 1, 1, 2, models.ItemStack{ItemID: 1, Count: 1}, models.ItemStack{})
	require.Error(t, err)
}

func TestSplitStack(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)

	err := im.SplitStack(1, 2, models.ItemStack{ItemID: 3, Count: 1}, models.ItemStack{})
	require.NoError(t, err)
	require.Len(t, screen.calls, 0)

	err = im.SplitStack(1, 2, models.ItemStack{ItemID: 3, Count: 4}, models.ItemStack{})
	require.NoError(t, err)
	require.Len(t, screen.calls, 2)
	require.Equal(t, int32(ClickModeNormal), screen.calls[0].mode)
	require.Equal(t, int32(ClickModeNormal), screen.calls[1].mode)
}

func TestMoveItemEmptySource(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)
	err := im.MoveItem(1, 2, models.ItemStack{}, models.ItemStack{})
	require.NoError(t, err)
	require.Len(t, screen.calls, 0)
}

func TestBuildChangedSlots(t *testing.T) {
	before := models.ItemStack{ItemID: 1, Count: 1}
	require.Nil(t, buildChangedSlots(2, before, before))

	after := models.ItemStack{ItemID: 1, Count: 2}
	changed := buildChangedSlots(2, before, after)
	require.Len(t, changed, 1)
	require.Equal(t, int16(2), changed[0].SlotIndex)
	require.Equal(t, after, changed[0].Item)
}

func TestCopyItemStackDeepCopy(t *testing.T) {
	original := models.ItemStack{
		ItemID: 7,
		Count:  2,
		NBT:    []byte{1, 2},
		Components: []models.ItemComponent{
			{Type: 1, Data: pk.VarInt(5)},
		},
		RemoveComponents: []int32{9},
	}
	copied := copyItemStack(original)
	original.NBT[0] = 9
	original.Components[0].Type = 2
	original.RemoveComponents[0] = 10

	require.Equal(t, int8(2), copied.Count)
	require.Equal(t, byte(1), copied.NBT[0])
	require.Equal(t, int32(1), copied.Components[0].Type)
	require.Equal(t, int32(9), copied.RemoveComponents[0])
}

func TestStackHelpers(t *testing.T) {
	a := models.ItemStack{ItemID: 1, Count: 2}
	b := models.ItemStack{ItemID: 1, Count: 2}
	c := models.ItemStack{ItemID: 1, Count: 3}
	require.True(t, stackEqual(a, b))
	require.False(t, stackEqual(a, c))
	require.True(t, sameItem(a, c))
	require.False(t, sameItem(a, models.ItemStack{ItemID: 2, Count: 2}))
	require.True(t, stackMatchesCounts(models.ItemStack{}, models.ItemStack{}))
	require.False(t, stackMatchesCounts(a, c))
}

func TestMaxStackSizeFromComponent(t *testing.T) {
	stack := models.ItemStack{
		ItemID: 1,
		Count:  1,
		Components: []models.ItemComponent{
			{Type: 1, Data: pk.VarInt(16)},
		},
	}
	require.Equal(t, 16, maxStackSize(stack))
}

func TestSimulateNormalClickMerge(t *testing.T) {
	slotItem := models.ItemStack{
		ItemID: 1,
		Count:  7,
		Components: []models.ItemComponent{
			{Type: 1, Data: pk.VarInt(8)},
		},
	}
	cursor := models.ItemStack{
		ItemID: 1,
		Count:  5,
		Components: []models.ItemComponent{
			{Type: 1, Data: pk.VarInt(8)},
		},
	}
	slotNext, cursorNext := simulateNormalClick(slotItem, cursor, LeftButton)
	require.Equal(t, int8(8), slotNext.Count)
	require.Equal(t, int8(4), cursorNext.Count)
}

func TestInventoryManagerConfig(t *testing.T) {
	im := NewInventoryManager(nil).(*inventoryManager)
	require.True(t, im.waitForUpdates)
	require.Equal(t, 5*time.Second, im.updateWaitDelay)

	im.SetWindow(3)
	require.Equal(t, byte(3), im.GetWindow())

	im.SetCursor(models.ItemStack{ItemID: 2, Count: 1})
	require.Equal(t, int8(1), im.GetCursor().Count)
}

func TestSwapWithHotbar(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)
	err := im.SwapWithHotbar(4, models.ItemStack{ItemID: 1, Count: 1}, 3, models.ItemStack{ItemID: 2, Count: 2})
	require.NoError(t, err)
	require.Len(t, screen.calls, 1)
	require.Equal(t, int32(ClickModeHotbar), screen.calls[0].mode)
}

func TestDropStackEmpty(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)
	err := im.DropStack(1, models.ItemStack{})
	require.NoError(t, err)
	require.Len(t, screen.calls, 0)
}

func TestDoubleClick(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)
	err := im.DoubleClick(2, models.ItemStack{})
	require.NoError(t, err)
	require.Len(t, screen.calls, 0)

	err = im.DoubleClick(2, models.ItemStack{ItemID: 3, Count: 1})
	require.NoError(t, err)
	require.Len(t, screen.calls, 1)
	require.Equal(t, int32(ClickModeDoubleClick), screen.calls[0].mode)
}

func TestMoveItem(t *testing.T) {
	screen := &fakeScreen{
		slots: map[int]mcscreen.Slot{
			1: *slotFromItemStack(models.ItemStack{ItemID: 4, Count: 2}),
			2: *slotFromItemStack(models.ItemStack{}),
		},
	}
	im := newInventoryManagerWithClicker(screen)
	err := im.MoveItem(1, 2, models.ItemStack{ItemID: 4, Count: 2}, models.ItemStack{})
	require.NoError(t, err)
	require.Len(t, screen.calls, 2)
}

func TestTransferStack(t *testing.T) {
	screen := &fakeScreen{}
	im := newInventoryManagerWithClicker(screen)
	err := im.TransferStack(2, models.ItemStack{ItemID: 5, Count: 1})
	require.NoError(t, err)
	require.Len(t, screen.calls, 1)
	require.Equal(t, int32(ClickModeShift), screen.calls[0].mode)
}

func TestScreenManagerAdapterNil(t *testing.T) {
	adapter := screenManagerAdapter{}
	err := adapter.ContainerClick(0, 1, 0, 0, nil, nil)
	require.Error(t, err)
	require.Equal(t, mcscreen.Slot{}, adapter.CursorSlot())
	_, ok := adapter.SlotAt(0, 1)
	require.False(t, ok)
}

func TestScreenManagerAdapterSlotAt(t *testing.T) {
	chest := &mcscreen.Chest{Slots: make([]mcscreen.Slot, 9)}
	chest.Slots[2] = *slotFromItemStack(models.ItemStack{ItemID: 3, Count: 1})
	manager := mcscreen.NewManager(nil, nil, nil)
	manager.SetScreens(map[int]mcscreen.Container{
		1: chest,
	})

	inventory := manager.Inventory()
	inventory.Slots[4] = *slotFromItemStack(models.ItemStack{ItemID: 8, Count: 1})
	manager.SetInventory(inventory)

	adapter := screenManagerAdapter{manager: manager}
	slot, ok := adapter.SlotAt(0, 4)
	require.True(t, ok)
	require.Equal(t, pk.VarInt(8), slot.ID)

	slot, ok = adapter.SlotAt(1, 2)
	require.True(t, ok)
	require.Equal(t, pk.VarInt(3), slot.ID)
}

func TestWithCountEmpty(t *testing.T) {
	empty := withCount(models.ItemStack{ItemID: 1, Count: 2}, 0)
	require.True(t, empty.IsEmpty())
}

func TestMaxStackSizeTypes(t *testing.T) {
	cases := []models.ItemStack{
		{Components: []models.ItemComponent{{Type: 1, Data: int(12)}}},
		{Components: []models.ItemComponent{{Type: 1, Data: int32(13)}}},
		{Components: []models.ItemComponent{{Type: 1, Data: int64(14)}}},
		{Components: []models.ItemComponent{{Type: 1, Data: uint32(15)}}},
	}
	for i, stack := range cases {
		require.Equal(t, 12+i, maxStackSize(stack))
	}
	require.Equal(t, 64, maxStackSize(models.ItemStack{}))
}

func TestStackEqualityComponents(t *testing.T) {
	a := models.ItemStack{ItemID: 1, Count: 1, Components: []models.ItemComponent{{Type: 1, Data: pk.VarInt(5)}}}
	b := models.ItemStack{ItemID: 1, Count: 1, Components: []models.ItemComponent{{Type: 2, Data: pk.VarInt(5)}}}
	require.False(t, stackEqual(a, b))
	require.False(t, sameItem(a, b))
}
