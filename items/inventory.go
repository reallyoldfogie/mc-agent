package items

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// MouseButton represents which mouse button is used for clicking
type MouseButton int

const (
	LeftButton   MouseButton = 0
	RightButton  MouseButton = 1
	MiddleButton MouseButton = 2
)

// ClickMode represents the type of inventory click operation
type ClickMode int

const (
	// ClickModeNormal - Regular click (left/right mouse button)
	// Button 0 = left click (pickup/place all), Button 1 = right click (pickup half/place one)
	ClickModeNormal ClickMode = 0

	// ClickModeShift - Shift + click for quick transfer between inventories
	// Button 0 = shift+left, Button 1 = shift+right (both do same thing)
	ClickModeShift ClickMode = 1

	// ClickModeHotbar - Number key press (1-9) to swap with hotbar
	// Button 0-8 = hotbar slot to swap with
	ClickModeHotbar ClickMode = 2

	// ClickModeCreativeMiddle - Middle click in creative mode (clone item)
	// Button 2 = middle click
	ClickModeCreativeMiddle ClickMode = 3

	// ClickModeDrop - Drop key (Q) or Ctrl+Q
	// Button 0 = drop one item (Q), Button 1 = drop entire stack (Ctrl+Q)
	ClickModeDrop ClickMode = 4

	// ClickModeDrag - Click and drag to distribute items
	// Button 0 = left drag (distribute evenly), Button 1 = right drag (place one each)
	// Button 2 = middle drag (creative only)
	ClickModeDrag ClickMode = 5

	// ClickModeDoubleClick - Double click to collect all matching items
	// Button 0 = double click
	ClickModeDoubleClick ClickMode = 6
)

// DragState represents the state of a drag operation
type DragState int

const (
	DragStart DragState = 0 // Start dragging (button down)
	DragAdd   DragState = 1 // Add slot to drag (mouse over slot while dragging)
	DragEnd   DragState = 2 // End dragging (button up, distribute items)
)

// InventoryManager handles inventory and container interactions
// This manages window IDs, state tracking, and sends container click packets
type inventoryManager struct {
	mu                   sync.Mutex
	screen               screenClicker
	cursor               models.ItemStack // Item currently held by cursor
	windowID             byte             // Currently open window (0 = player inventory)
	waitForUpdates       bool
	updateWaitDelay      time.Duration
	pendingUpdateVersion int64
	awaitingScreenUpdate bool
}

// NewInventoryManager creates a new InventoryManager instance
func NewInventoryManager(screenMgr mcscreen.Manager) models.InventoryManager {
	im := newInventoryManagerWithClicker(screenManagerAdapter{manager: screenMgr})
	im.waitForUpdates = true
	im.updateWaitDelay = 5 * time.Second
	return im
}

// newInventoryManagerWithClicker creates an InventoryManager with a custom clicker (useful for tests).
func newInventoryManagerWithClicker(screen screenClicker) *inventoryManager {
	im := &inventoryManager{
		screen:          screen,
		cursor:          models.ItemStack{}, // Empty cursor
		windowID:        0,                  // Player inventory by default
		waitForUpdates:  false,
		updateWaitDelay: 2 * time.Second,
	}
	im.SyncCursorFromScreen()
	return im
}

// SetWindow sets the currently open window ID
// Call this when opening a container (chest, furnace, etc.)
// windowID 0 = player inventory (always open)
func (im *inventoryManager) SetWindow(windowID byte) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.windowID = windowID
}

// GetWindow returns the currently open window ID
func (im *inventoryManager) GetWindow() byte {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.windowID
}

// SetCursorItem sets the item held by cursor (call when server sends cursor updates)
func (im *inventoryManager) SetCursorItem(item models.ItemStack) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.cursor = item
}

// GetCursorItem returns the item currently held by cursor
func (im *inventoryManager) GetCursorItem() models.ItemStack {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.cursor
}

// SetWaitForUpdates controls whether multi-click helpers wait for screen updates before continuing.
func (im *inventoryManager) SetWaitForUpdates(wait bool) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.waitForUpdates = wait
}

// SetUpdateWaitDelay sets how long to wait for screen updates when waiting is enabled.
func (im *inventoryManager) SetUpdateWaitDelay(delay time.Duration) {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.updateWaitDelay = delay
}

// SyncCursorFromScreen updates the local cursor state using the screen manager's cursor.
func (im *inventoryManager) SyncCursorFromScreen() {
	if im.screen == nil {
		return
	}
	im.cursor = itemStackFromSlot(im.screen.CursorSlot())
}

// ClickSlot performs a raw inventory click operation for a specific window ID.
// This is the low-level function - most users should use higher-level functions like
// LeftClickSlot, RightClickSlot, ShiftClickSlot, etc.
func (im *inventoryManager) ClickSlot(windowID byte, slot int16, button MouseButton, mode ClickMode) error {
	return im.clickWithChanges(windowID, slot, byte(button), mode, nil, nil, false)
}

// ClickSlotWithChanges performs a raw click with explicit changed slot predictions.
func (im *inventoryManager) ClickSlotWithChanges(windowID byte, slot int16, button MouseButton, mode ClickMode, changedSlots []models.ChangedSlot) error {
	return im.clickWithChanges(windowID, slot, byte(button), mode, changedSlots, nil, false)
}

func (im *inventoryManager) clickWithChanges(windowID byte, slot int16, button byte, mode ClickMode, changedSlots []models.ChangedSlot, carryOverride *models.ItemStack, overrideCarried bool) error {
	if im.screen == nil {
		return errors.New("inventory manager requires a screen manager")
	}

	changed := make(mcscreen.ChangedSlots)
	for _, cs := range changedSlots {
		slotData := slotFromItemStack(cs.Item)
		changed[int(cs.SlotIndex)] = slotData
	}

	var carriedSlot *mcscreen.Slot
	if overrideCarried {
		if carryOverride != nil && !carryOverride.IsEmpty() {
			carriedSlot = slotFromItemStack(*carryOverride)
		}
	} else {
		carriedSlot = slotPointerFromItemStack(im.cursor)
	}

	// Only capture version if not already captured by caller (e.g., LeftClickSlot/RightClickSlot)
	// This allows callers to capture BEFORE applying predictions
	if !im.awaitingScreenUpdate {
		im.pendingUpdateVersion = im.screen.ServerUpdateVersion()
		im.awaitingScreenUpdate = true
	}

	return im.screen.ContainerClick(int(windowID), slot, button, int32(mode), changed, carriedSlot)
}

// LeftClickSlot picks up an entire stack or places the entire held stack
// If cursor is empty: picks up the entire stack from the slot
// If cursor has items: places the entire held stack into the slot
func (im *inventoryManager) LeftClickSlot(slot int16, slotItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.leftClickSlot(slot, slotItem)
}

func (im *inventoryManager) leftClickSlot(slot int16, slotItem models.ItemStack) error {
	im.SyncCursorFromScreen()
	slotItem = im.slotItemFor(slot, slotItem)

	carryBefore := im.cursor

	slotNext, cursorNext := simulateNormalClick(slotItem, im.cursor, LeftButton)
	changedSlots := buildChangedSlots(slot, slotItem, slotNext)

	// DEBUG: Log the click prediction
	if debugPath := os.Getenv("MC_AGENT_CLICK_DEBUG_PATH"); debugPath != "" {
		if f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			fmt.Fprintf(f, "[PREDICT] LeftClickSlot slot=%d\n", slot)
			fmt.Fprintf(f, "  slotItem: id=%d count=%d\n", slotItem.ItemID, slotItem.Count)
			fmt.Fprintf(f, "  carryBefore: id=%d count=%d (empty=%v)\n", carryBefore.ItemID, carryBefore.Count, carryBefore.IsEmpty())
			fmt.Fprintf(f, "  slotNext: id=%d count=%d\n", slotNext.ItemID, slotNext.Count)
			fmt.Fprintf(f, "  cursorNext: id=%d count=%d\n", cursorNext.ItemID, cursorNext.Count)
			_ = f.Close()
		}
	}

	// CRITICAL: Capture version BEFORE prediction to ensure we can detect server response
	// If we capture after prediction, the screen manager won't increment version when
	// server confirms because the value already matches the prediction
	im.pendingUpdateVersion = im.screen.ServerUpdateVersion()
	im.awaitingScreenUpdate = true

	// DEBUG: Log version capture
	if debugPath := os.Getenv("MC_AGENT_CLICK_DEBUG_PATH"); debugPath != "" {
		if f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			fmt.Fprintf(f, "[VERSION-CAPTURE] beforeVersion=%d (before applyPredictedClick)\n", im.pendingUpdateVersion)
			_ = f.Close()
		}
	}

	// Apply prediction BEFORE sending - keeps original timing
	im.applyPredictedClick(slot, slotNext, cursorNext)

	// Pass cursorNext (predicted cursor state after click) not carryBefore
	// The Minecraft protocol expects the client's prediction of cursor state after the click
	// - except when that prediction is empty: depositing the entire held
	// stack onto a slot (cursor non-empty going in, empty coming out).
	// carriedItemForClick falls back to carryBefore there - see its doc
	// comment for why.
	carried := carriedItemForClick(carryBefore, cursorNext)
	return im.clickWithChanges(im.windowID, slot, byte(LeftButton), ClickModeNormal, changedSlots, &carried, true)
}

// carriedItemForClick returns the "carried item" value to send in a
// ContainerClick packet for a normal left/right click. Protocol convention
// is the client's prediction of cursor state AFTER the click (cursorNext) -
// correct for pickup (empty->full) and partial-place (full->still-full)
// clicks. But when cursorNext is empty because the click deposits the
// *entire* held stack (cursor non-empty going in), sending "nothing
// carried" produces an internally-inconsistent packet: it claims a slot is
// receiving an item while also claiming nothing was held to place there.
// The server rejects this silently, correcting the client only via a later,
// unrelated full ContainerSetContent resync - by which point callers like
// MoveSingle have already returned success on the wrong prediction.
//
// Found live via MC_AGENT_CLICK_DEBUG_PATH tracing MoveSingle's "return
// unneeded remainder to source slot" step (docs/plans/CRAFTING_TABLE_3X3_PLAN.md-adjacent
// investigation, 2026-09-06): a stack of 3 logs picked up to place 1 in a
// crafting grid predicted the source slot would receive its 2 leftover
// logs back, but the server's next full resync showed that slot empty -
// the 2 logs were gone from every tracked slot, not just delayed.
func carriedItemForClick(carryBefore, cursorNext models.ItemStack) models.ItemStack {
	if cursorNext.IsEmpty() && !carryBefore.IsEmpty() {
		return carryBefore
	}
	return cursorNext
}

// RightClickSlot picks up half a stack or places one item
// If cursor is empty: picks up half the stack (rounded up) from the slot
// If cursor has items: places one item from held stack into the slot
func (im *inventoryManager) RightClickSlot(slot int16, slotItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.rightClickSlot(slot, slotItem)
}

func (im *inventoryManager) rightClickSlot(slot int16, slotItem models.ItemStack) error {
	im.SyncCursorFromScreen()
	slotItem = im.slotItemFor(slot, slotItem)

	carryBefore := im.cursor

	slotNext, cursorNext := simulateNormalClick(slotItem, im.cursor, RightButton)
	changedSlots := buildChangedSlots(slot, slotItem, slotNext)

	// CRITICAL: Capture version BEFORE prediction to ensure we can detect server response
	im.pendingUpdateVersion = im.screen.ServerUpdateVersion()
	im.awaitingScreenUpdate = true

	// Apply prediction BEFORE sending - keeps original timing
	im.applyPredictedClick(slot, slotNext, cursorNext)

	// Pass cursorNext (predicted cursor state) not carryBefore - except when
	// cursorNext is empty (placing the last single item off a cursor stack
	// of exactly 1); see carriedItemForClick's doc comment (LeftClickSlot,
	// above) for why - the same gap applies here structurally, just not yet
	// reproduced against a live server for this button.
	carried := carriedItemForClick(carryBefore, cursorNext)
	return im.clickWithChanges(im.windowID, slot, byte(RightButton), ClickModeNormal, changedSlots, &carried, true)
}

// ShiftClickSlot performs a shift+click quick transfer
// Moves items between player inventory and open container
// For example: shift+click in chest moves to player inventory, shift+click in player inventory moves to chest
func (im *inventoryManager) ShiftClickSlot(slot int16, slotItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.shiftClickSlot(slot, slotItem)
}

func (im *inventoryManager) shiftClickSlot(slot int16, slotItem models.ItemStack) error {
	if slotItem.IsEmpty() {
		return nil // Nothing to transfer
	}

	// Note: Server handles the actual item movement logic
	// We just need to send the shift+click packet
	// Changed slots are determined by server and sent back to us
	changedSlots := []models.ChangedSlot{
		{SlotIndex: slot, Item: models.ItemStack{}}, // Simplified - slot becomes empty (server will correct if needed)
	}

	// Apply the source-slot-becomes-empty prediction to local screen state,
	// matching LeftClickSlot/RightClickSlot's applyPredictedClick. Without
	// this, nothing ever clears the slot locally: vanilla servers commonly
	// skip sending a ClientboundContainerSetSlot correction for a
	// shift-click's source slot when the prediction is already correct
	// (confirmed against a live server - no such packet ever arrives), so
	// the local state would otherwise stay stale indefinitely, and anything
	// reading it (item-in-hand tracking, the replay mirror) would too.
	if setter, ok := im.screen.(screenSlotSetter); ok {
		setter.SetSlotAt(int(im.windowID), int(slot), *slotFromItemStack(models.ItemStack{}))
	}

	return im.clickWithChanges(im.windowID, slot, byte(LeftButton), ClickModeShift, changedSlots, nil, false)
}

// SwapWithHotbar swaps the clicked slot with a hotbar slot (number key 1-9)
// hotbarSlot: 0-8 (corresponding to keys 1-9)
func (im *inventoryManager) SwapWithHotbar(slot int16, slotItem models.ItemStack, hotbarSlot int, hotbarItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if hotbarSlot < 0 || hotbarSlot > 8 {
		return nil // Invalid hotbar slot
	}

	// Swap the two slots
	changedSlots := []models.ChangedSlot{
		{SlotIndex: slot, Item: hotbarItem},            // Clicked slot gets hotbar item
		{SlotIndex: int16(hotbarSlot), Item: slotItem}, // Hotbar slot gets clicked item
	}

	return im.clickWithChanges(im.windowID, slot, byte(hotbarSlot), ClickModeHotbar, changedSlots, nil, false)
}

// DropItem drops one item from the slot (Q key)
func (im *inventoryManager) DropItem(slot int16, slotItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if slotItem.IsEmpty() {
		return nil // Nothing to drop
	}

	newCount := slotItem.Count - 1
	changedSlots := []models.ChangedSlot{
		{SlotIndex: slot, Item: models.ItemStack{
			ItemID: slotItem.ItemID,
			Count:  newCount,
			NBT:    slotItem.NBT,
		}},
	}

	if newCount <= 0 {
		// Slot becomes empty
		changedSlots[0].Item = models.ItemStack{}
	}

	return im.clickWithChanges(im.windowID, slot, byte(LeftButton), ClickModeDrop, changedSlots, nil, false)
}

// DropStack drops the entire stack from the slot (Ctrl+Q)
func (im *inventoryManager) DropStack(slot int16, slotItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if slotItem.IsEmpty() {
		return nil // Nothing to drop
	}

	changedSlots := []models.ChangedSlot{
		{SlotIndex: slot, Item: models.ItemStack{}}, // Slot becomes empty
	}

	return im.clickWithChanges(im.windowID, slot, byte(RightButton), ClickModeDrop, changedSlots, nil, false)
}

// DoubleClick collects all matching items into the clicked slot (double-click)
// This gathers all items of the same type from the inventory into one stack
func (im *inventoryManager) DoubleClick(slot int16, slotItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if slotItem.IsEmpty() {
		return nil // Nothing to collect
	}

	// Note: Server handles collecting logic
	// We just send the double-click packet
	changedSlots := []models.ChangedSlot{
		{SlotIndex: slot, Item: slotItem}, // Simplified - server will update
	}

	return im.clickWithChanges(im.windowID, slot, byte(LeftButton), ClickModeDoubleClick, changedSlots, nil, false)
}

// StartDrag begins a drag operation
// dragType: 0 = left drag (distribute evenly), 1 = right drag (place one each), 2 = middle (creative)
func (im *inventoryManager) StartDrag(dragType byte) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.startDrag(dragType)
}

func (im *inventoryManager) startDrag(dragType byte) error {
	// Drag start: slot=-999, button=dragType*4+0 (DragStart encoding)
	button := dragType*4 + byte(DragStart)
	return im.clickWithChanges(im.windowID, -999, button, ClickModeDrag, []models.ChangedSlot{}, nil, false)
}

// AddDragSlot adds a slot to the current drag operation
// Call this for each slot you want to include in the drag
func (im *inventoryManager) AddDragSlot(slot int16, dragType byte) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.addDragSlot(slot, dragType)
}

func (im *inventoryManager) addDragSlot(slot int16, dragType byte) error {
	// Drag add: slot=target, button=dragType*4+1 (DragAdd encoding)
	button := dragType*4 + byte(DragAdd)
	return im.clickWithChanges(im.windowID, slot, button, ClickModeDrag, []models.ChangedSlot{}, nil, false)
}

// EndDrag completes the drag operation and distributes items
// The server calculates how items are distributed based on drag type and slots added
func (im *inventoryManager) EndDrag(dragType byte, changedSlots []models.ChangedSlot) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.endDrag(dragType, changedSlots)
}

func (im *inventoryManager) endDrag(dragType byte, changedSlots []models.ChangedSlot) error {
	// Drag end: slot=-999, button=dragType*4+2 (DragEnd encoding)
	button := dragType*4 + byte(DragEnd)
	return im.clickWithChanges(im.windowID, -999, button, ClickModeDrag, changedSlots, nil, false)
}

// MoveItem moves an entire item stack from one slot to another within the same window
// This is a high-level helper that uses pickup+place logic
func (im *inventoryManager) MoveItem(fromSlot, toSlot int16, fromItem, toItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.moveItem(fromSlot, toSlot, fromItem, toItem)
}

func (im *inventoryManager) moveItem(fromSlot, toSlot int16, fromItem, toItem models.ItemStack) error {
	im.SyncCursorFromScreen()

	fromItem = im.slotItemFor(fromSlot, fromItem)
	if fromItem.IsEmpty() {
		return nil // Nothing to move
	}
	toItem = im.slotItemFor(toSlot, toItem)

	beforeCursor := im.cursor
	fromSlotNext, fromCursorNext := simulateNormalClick(fromItem, beforeCursor, LeftButton)

	// Pick up from source
	if err := im.leftClickSlot(fromSlot, fromItem); err != nil {
		return err
	}
	if err := im.waitForSlotAndCursor(fromSlot, fromItem, beforeCursor, fromSlotNext, fromCursorNext); err != nil {
		return err
	}

	im.SyncCursorFromScreen()
	toItem = im.slotItemFor(toSlot, toItem)
	beforeCursor = im.cursor
	toSlotNext, toCursorNext := simulateNormalClick(toItem, beforeCursor, LeftButton)

	// Place at destination
	if err := im.leftClickSlot(toSlot, toItem); err != nil {
		return err
	}
	return im.waitForSlotAndCursor(toSlot, toItem, beforeCursor, toSlotNext, toCursorNext)
}

// MoveSingle moves a single item from one slot to another.
// This performs pickup + place-one + return-remaining to the source slot.
func (im *inventoryManager) MoveSingle(fromSlot, toSlot int16, fromItem, toItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()

	im.SyncCursorFromScreen()
	fromItem = im.slotItemFor(fromSlot, fromItem)
	if fromItem.IsEmpty() {
		return nil
	}
	toItem = im.slotItemFor(toSlot, toItem)

	beforeCursor := im.cursor
	fromSlotNext, fromCursorNext := simulateNormalClick(fromItem, beforeCursor, LeftButton)
	if err := im.leftClickSlot(fromSlot, fromItem); err != nil {
		return err
	}
	if err := im.waitForSlotAndCursor(fromSlot, fromItem, beforeCursor, fromSlotNext, fromCursorNext); err != nil {
		return err
	}

	im.SyncCursorFromScreen()
	toItem = im.slotItemFor(toSlot, toItem)
	beforeCursor = im.cursor
	toSlotNext, toCursorNext := simulateNormalClick(toItem, beforeCursor, RightButton)
	if err := im.rightClickSlot(toSlot, toItem); err != nil {
		return err
	}
	if err := im.waitForSlotAndCursor(toSlot, toItem, beforeCursor, toSlotNext, toCursorNext); err != nil {
		return err
	}

	if !im.cursor.IsEmpty() {
		im.SyncCursorFromScreen()
		fromItem = im.slotItemFor(fromSlot, models.ItemStack{})
		beforeCursor = im.cursor
		finalSlotNext, finalCursorNext := simulateNormalClick(fromItem, beforeCursor, LeftButton)
		if err := im.leftClickSlot(fromSlot, models.ItemStack{}); err != nil {
			return err
		}
		if err := im.waitForSlotAndCursor(fromSlot, fromItem, beforeCursor, finalSlotNext, finalCursorNext); err != nil {
			return err
		}
	}

	return nil
}

// TransferItem moves an item stack between inventories in the same window.
// Cross-window transfers are not supported because Minecraft only allows one open container.
func (im *inventoryManager) TransferItem(fromWindowID byte, fromSlot int16, toWindowID byte, toSlot int16, fromItem, toItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if fromWindowID != toWindowID {
		return errors.New("cannot transfer items between different window IDs")
	}
	return im.moveItem(fromSlot, toSlot, fromItem, toItem)
}

// TransferStack transfers an entire stack between inventories using shift+click
func (im *inventoryManager) TransferStack(slot int16, slotItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.shiftClickSlot(slot, slotItem)
}

// SplitStack splits a stack in half
// Places half in the original slot and half in the destination slot
func (im *inventoryManager) SplitStack(sourceSlot, destSlot int16, sourceItem, destItem models.ItemStack) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if sourceItem.IsEmpty() || sourceItem.Count < 2 {
		return nil // Nothing to split or stack too small
	}

	// Right click source to pick up half
	if err := im.rightClickSlot(sourceSlot, sourceItem); err != nil {
		return err
	}

	// Left click destination to place picked-up half
	return im.leftClickSlot(destSlot, destItem)
}

// DistributeItems distributes items from cursor across multiple slots
// Uses drag operation to place one item per slot (right drag) or distribute evenly (left drag)
func (im *inventoryManager) DistributeItems(slots []int16, evenlyDistribute bool, changedSlots []models.ChangedSlot) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	var dragType byte = 1 // Right drag (one each) by default
	if evenlyDistribute {
		dragType = 0 // Left drag (distribute evenly)
	}

	// Start drag
	if err := im.startDrag(dragType); err != nil {
		return err
	}

	// Add each slot to drag
	for _, slot := range slots {
		if err := im.addDragSlot(slot, dragType); err != nil {
			return err
		}
	}

	// End drag and distribute
	return im.endDrag(dragType, changedSlots)
}

func slotFromItemStack(stack models.ItemStack) *mcscreen.Slot {
	if stack.IsEmpty() {
		return &mcscreen.Slot{Count: 0}
	}
	slot := &mcscreen.Slot{
		ID:    pk.VarInt(stack.ItemID),
		Count: pk.VarInt(stack.Count),
	}
	for _, component := range stack.Components {
		slot.Components = append(slot.Components, mcscreen.SlotComponent{
			Type: pk.VarInt(component.Type),
			Data: component.Data,
		})
	}
	for _, componentType := range stack.RemoveComponents {
		slot.RemoveComponents = append(slot.RemoveComponents, pk.VarInt(componentType))
	}
	return slot
}

func slotPointerFromItemStack(stack models.ItemStack) *mcscreen.Slot {
	if stack.IsEmpty() {
		return nil
	}
	return slotFromItemStack(stack)
}

func itemStackFromSlot(slot mcscreen.Slot) models.ItemStack {
	if slot.Count <= 0 {
		return models.ItemStack{}
	}
	stack := models.ItemStack{
		ItemID: int32(slot.ID),
		Count:  int8(slot.Count),
	}
	for _, component := range slot.Components {
		stack.Components = append(stack.Components, models.ItemComponent{
			Type: int32(component.Type),
			Data: component.Data,
		})
	}
	for _, componentType := range slot.RemoveComponents {
		stack.RemoveComponents = append(stack.RemoveComponents, int32(componentType))
	}
	return stack
}

type screenClicker interface {
	ContainerClick(id int, slot int16, button byte, mode int32, slots mcscreen.ChangedSlots, carried *mcscreen.Slot) error
	CursorSlot() mcscreen.Slot
	SlotAt(windowID int, slot int) (mcscreen.Slot, bool)
	ServerUpdateVersion() int64
}

type screenSlotSetter interface {
	SetCursorSlot(slot mcscreen.Slot)
	SetSlotAt(windowID int, slot int, data mcscreen.Slot) bool
}

type screenManagerAdapter struct {
	manager mcscreen.Manager
}

func (s screenManagerAdapter) ContainerClick(id int, slot int16, button byte, mode int32, slots mcscreen.ChangedSlots, carried *mcscreen.Slot) error {
	if s.manager == nil {
		return errors.New("screen manager is nil")
	}
	return s.manager.ContainerClick(id, slot, button, mode, slots, carried)
}

func (s screenManagerAdapter) CursorSlot() mcscreen.Slot {
	if s.manager == nil {
		return mcscreen.Slot{}
	}
	return s.manager.Cursor()
}

func (s screenManagerAdapter) SetCursorSlot(slot mcscreen.Slot) {
	if s.manager == nil {
		return
	}
	s.manager.SetCursor(slot)
}

func (s screenManagerAdapter) SlotAt(windowID int, slot int) (mcscreen.Slot, bool) {
	if s.manager == nil {
		return mcscreen.Slot{}, false
	}
	if windowID == 0 {
		if slot < 0 || slot >= len(s.manager.Inventory().GetSlots()) {
			return mcscreen.Slot{}, false
		}
		return s.manager.Inventory().GetSlots()[slot], true
	}
	container, ok := s.manager.Screens()[windowID]
	if !ok || slot < 0 {
		return mcscreen.Slot{}, false
	}
	switch c := container.(type) {
	case mcscreen.Inventory:
		if slot >= len(c.GetSlots()) {
			return mcscreen.Slot{}, false
		}
		return c.GetSlots()[slot], true
	case *mcscreen.Chest:
		if slot >= len(c.Slots) {
			return mcscreen.Slot{}, false
		}
		return c.Slots[slot], true
	default:
		return mcscreen.Slot{}, false
	}
}

func (s screenManagerAdapter) SetSlotAt(windowID int, slot int, data mcscreen.Slot) bool {
	if s.manager == nil || slot < 0 {
		return false
	}
	if windowID == 0 {
		inventory := s.manager.Inventory()
		if slot >= len(inventory.GetSlots()) {
			return false
		}
		// OnSetSlot, not GetSlots()[slot] = data: GetSlots() now returns a
		// defensive copy (mc-bot-go/bot/screen/inventory.go, fixed for a
		// concurrent-read data race - see docs/plans/CRAFTING_TABLE_3X3_PLAN.md-adjacent
		// investigation), so mutating its result would silently update a
		// throwaway copy instead of the tracked inventory. OnSetSlot writes
		// through to the real backing state under its own lock.
		if err := inventory.OnSetSlot(slot, data); err != nil {
			return false
		}
		return true
	}
	container, ok := s.manager.Screens()[windowID]
	if !ok {
		return false
	}
	switch c := container.(type) {
	case mcscreen.Inventory:
		if err := c.OnSetSlot(slot, data); err != nil {
			return false
		}
		return true
	case *mcscreen.Chest:
		if slot >= len(c.Slots) {
			return false
		}
		c.Slots[slot] = data
		return true
	default:
		return false
	}
}

func (s screenManagerAdapter) ServerUpdateVersion() int64 {
	if s.manager == nil {
		return 0
	}
	return s.manager.ServerUpdateVersion()
}

func (im *inventoryManager) slotItemFor(slot int16, fallback models.ItemStack) models.ItemStack {
	if im.screen == nil || slot < 0 {
		return fallback
	}
	slotData, ok := im.screen.SlotAt(int(im.windowID), int(slot))
	if !ok {
		return fallback
	}
	return itemStackFromSlot(slotData)
}

func (im *inventoryManager) applyPredictedClick(slot int16, slotNext, cursorNext models.ItemStack) {
	im.cursor = cursorNext
	setter, ok := im.screen.(screenSlotSetter)
	if !ok {
		return
	}
	setter.SetCursorSlot(*slotFromItemStack(cursorNext))
	if slot >= 0 {
		setter.SetSlotAt(int(im.windowID), int(slot), *slotFromItemStack(slotNext))
	}
}

func (im *inventoryManager) waitForSlotAndCursor(slot int16, _, _, expectedSlot, expectedCursor models.ItemStack) error {
	if !im.waitForUpdates || im.screen == nil {
		im.awaitingScreenUpdate = false
		return nil
	}
	requireUpdate := im.awaitingScreenUpdate
	beforeVersion := im.pendingUpdateVersion
	deadline := time.Now().Add(im.updateWaitDelay)

	// DEBUG: Log wait parameters
	if debugPath := os.Getenv("MC_AGENT_CLICK_DEBUG_PATH"); debugPath != "" {
		if f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			fmt.Fprintf(f, "[WAIT] slot=%d beforeVersion=%d requireUpdate=%v\n", slot, beforeVersion, requireUpdate)
			fmt.Fprintf(f, "  expectedSlot: id=%d count=%d\n", expectedSlot.ItemID, expectedSlot.Count)
			fmt.Fprintf(f, "  expectedCursor: id=%d count=%d\n", expectedCursor.ItemID, expectedCursor.Count)
			_ = f.Close()
		}
	}

	for time.Now().Before(deadline) {
		currentCursor := itemStackFromSlot(im.screen.CursorSlot())
		if slot >= 0 {
			currentSlot, ok := im.screen.SlotAt(int(im.windowID), int(slot))
			if ok {
				slotStack := itemStackFromSlot(currentSlot)
				currentVersion := im.screen.ServerUpdateVersion()

				// DEBUG: Log current state
				if debugPath := os.Getenv("MC_AGENT_CLICK_DEBUG_PATH"); debugPath != "" {
					if f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
						slotMatch := stackMatchesCounts(slotStack, expectedSlot)
						cursorMatch := stackMatchesCounts(currentCursor, expectedCursor)
						versionOK := !requireUpdate || currentVersion > beforeVersion
						fmt.Fprintf(f, "[WAIT-CHECK] currentVersion=%d slotMatch=%v cursorMatch=%v versionOK=%v\n",
							currentVersion, slotMatch, cursorMatch, versionOK)
						fmt.Fprintf(f, "  currentSlot: id=%d count=%d\n", slotStack.ItemID, slotStack.Count)
						fmt.Fprintf(f, "  currentCursor: id=%d count=%d\n", currentCursor.ItemID, currentCursor.Count)
						_ = f.Close()
					}
				}

				if stackMatchesCounts(slotStack, expectedSlot) && stackMatchesCounts(currentCursor, expectedCursor) {
					if !requireUpdate || currentVersion > beforeVersion {
						// DEBUG: Log success
						if debugPath := os.Getenv("MC_AGENT_CLICK_DEBUG_PATH"); debugPath != "" {
							if f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
								fmt.Fprintf(f, "[WAIT-SUCCESS] version advanced from %d to %d\n", beforeVersion, currentVersion)
								_ = f.Close()
							}
						}
						im.cursor = currentCursor
						im.awaitingScreenUpdate = false
						return nil
					}
				}
			}
		} else if stackMatchesCounts(currentCursor, expectedCursor) {
			if !requireUpdate || im.screen.ServerUpdateVersion() > beforeVersion {
				im.cursor = currentCursor
				im.awaitingScreenUpdate = false
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	im.awaitingScreenUpdate = false

	// DEBUG: Log timeout
	if debugPath := os.Getenv("MC_AGENT_CLICK_DEBUG_PATH"); debugPath != "" {
		if f, err := os.OpenFile(debugPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			finalVersion := im.screen.ServerUpdateVersion()
			fmt.Fprintf(f, "[WAIT-TIMEOUT] beforeVersion=%d finalVersion=%d\n", beforeVersion, finalVersion)
			_ = f.Close()
		}
	}

	return errors.New("timeout waiting for inventory update")
}

func simulateNormalClick(slotItem, cursor models.ItemStack, button MouseButton) (models.ItemStack, models.ItemStack) {
	if button == LeftButton {
		if cursor.IsEmpty() {
			if slotItem.IsEmpty() {
				return slotItem, cursor
			}
			return models.ItemStack{}, slotItem
		}

		if slotItem.IsEmpty() {
			return cursor, models.ItemStack{}
		}

		if sameItem(slotItem, cursor) {
			maxSize := maxStackSize(slotItem)
			space := maxSize - int(slotItem.Count)
			if space <= 0 {
				return slotItem, cursor
			}
			transfer := int(cursor.Count)
			if transfer > space {
				transfer = space
			}
			slotNext := withCount(slotItem, int(slotItem.Count)+transfer)
			cursorNext := withCount(cursor, int(cursor.Count)-transfer)
			return slotNext, cursorNext
		}

		return cursor, slotItem
	}

	if cursor.IsEmpty() {
		if slotItem.IsEmpty() {
			return slotItem, cursor
		}
		half := (int(slotItem.Count) + 1) / 2
		remaining := int(slotItem.Count) - half
		return withCount(slotItem, remaining), withCount(slotItem, half)
	}

	if slotItem.IsEmpty() {
		return withCount(cursor, 1), withCount(cursor, int(cursor.Count)-1)
	}

	if sameItem(slotItem, cursor) {
		maxSize := maxStackSize(slotItem)
		if int(slotItem.Count) >= maxSize {
			return slotItem, cursor
		}
		return withCount(slotItem, int(slotItem.Count)+1), withCount(cursor, int(cursor.Count)-1)
	}

	return cursor, slotItem
}

func buildChangedSlots(slot int16, before, after models.ItemStack) []models.ChangedSlot {
	if stackEqual(before, after) {
		return nil
	}
	return []models.ChangedSlot{{SlotIndex: slot, Item: after}}
}

func withCount(stack models.ItemStack, count int) models.ItemStack {
	if count <= 0 {
		return models.ItemStack{}
	}
	out := copyItemStack(stack)
	out.Count = int8(count)
	return out
}

func copyItemStack(stack models.ItemStack) models.ItemStack {
	out := stack
	if stack.NBT != nil {
		out.NBT = append([]byte(nil), stack.NBT...)
	}
	if len(stack.Components) > 0 {
		out.Components = make([]models.ItemComponent, len(stack.Components))
		copy(out.Components, stack.Components)
	}
	if len(stack.RemoveComponents) > 0 {
		out.RemoveComponents = make([]int32, len(stack.RemoveComponents))
		copy(out.RemoveComponents, stack.RemoveComponents)
	}
	return out
}

func stackEqual(a, b models.ItemStack) bool {
	if a.ItemID != b.ItemID || a.Count != b.Count {
		return false
	}
	if len(a.Components) != len(b.Components) || len(a.RemoveComponents) != len(b.RemoveComponents) {
		return false
	}
	for i := range a.Components {
		if a.Components[i].Type != b.Components[i].Type || !reflect.DeepEqual(a.Components[i].Data, b.Components[i].Data) {
			return false
		}
	}
	for i := range a.RemoveComponents {
		if a.RemoveComponents[i] != b.RemoveComponents[i] {
			return false
		}
	}
	return true
}

func sameItem(a, b models.ItemStack) bool {
	if a.ItemID != b.ItemID {
		return false
	}
	if len(a.Components) != len(b.Components) || len(a.RemoveComponents) != len(b.RemoveComponents) {
		return false
	}
	for i := range a.Components {
		if a.Components[i].Type != b.Components[i].Type || !reflect.DeepEqual(a.Components[i].Data, b.Components[i].Data) {
			return false
		}
	}
	for i := range a.RemoveComponents {
		if a.RemoveComponents[i] != b.RemoveComponents[i] {
			return false
		}
	}
	return true
}

func maxStackSize(stack models.ItemStack) int {
	for _, component := range stack.Components {
		if component.Type != 1 {
			continue
		}
		switch v := component.Data.(type) {
		case pk.VarInt:
			if int(v) > 0 {
				return int(v)
			}
		case int:
			if v > 0 {
				return v
			}
		case int32:
			if v > 0 {
				return int(v)
			}
		case int64:
			if v > 0 {
				return int(v)
			}
		case uint32:
			if v > 0 {
				return int(v)
			}
		}
	}
	return 64
}

func stackMatchesCounts(a, b models.ItemStack) bool {
	if a.IsEmpty() && b.IsEmpty() {
		return true
	}
	return a.ItemID == b.ItemID && a.Count == b.Count
}
