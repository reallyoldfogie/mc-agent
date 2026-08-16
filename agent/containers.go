package agent

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// startPositionHeartbeat starts a background goroutine that sends position packets at the specified TPS.
// This ensures the server sees continuous movement packets, which is required for certain interactions
// (e.g., opening loom/beacon containers in Minecraft 1.21.5+).
//
// The heartbeat runs until stopPositionHeartbeat() is called or the agent's context is cancelled.
func (a *agent) startPositionHeartbeat(tps int) {
	a.posHeartbeatMu.Lock()
	defer a.posHeartbeatMu.Unlock()

	if a.posHeartbeatActive {
		return // Already running
	}

	if a.moveExec == nil {
		log.Printf("[Agent %s] Cannot start position heartbeat: movement executor not set", a.cfg.Name)
		return
	}

	a.posHeartbeatActive = true
	a.posHeartbeatStop = make(chan struct{})

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		ticker := time.NewTicker(time.Second / time.Duration(tps))
		defer ticker.Stop()

		log.Printf("[Agent %s] Position heartbeat started at %d TPS", a.cfg.Name, tps)

		for {
			select {
			case <-a.posHeartbeatStop:
				log.Printf("[Agent %s] Position heartbeat stopped", a.cfg.Name)
				return
			case <-a.ctx.Done():
				log.Printf("[Agent %s] Position heartbeat stopped (context cancelled)", a.cfg.Name)
				return
			case <-ticker.C:
				// Send current position to server
				pos, yaw, pitch, initialized := a.GetPosition()
				if initialized && a.moveExec != nil {
					log.Printf("[YAW DEBUG] HEARTBEAT sending: pos=(%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f", pos.X, pos.Y, pos.Z, yaw, pitch)
					if err := a.moveExec.SendPositionAndRotation(pos.X, pos.Y, pos.Z, yaw, pitch, true); err != nil {
						// Don't spam logs on errors, just continue
						// log.Printf("[Agent] Position heartbeat send error: %v", err)
					}
				}
			}
		}
	}()
}

// stopPositionHeartbeat stops the position heartbeat goroutine.
func (a *agent) stopPositionHeartbeat() {
	a.posHeartbeatMu.Lock()
	defer a.posHeartbeatMu.Unlock()

	if !a.posHeartbeatActive {
		return // Not running
	}

	close(a.posHeartbeatStop)
	a.posHeartbeatActive = false
	a.posHeartbeatStop = nil
}

// OpenContainer opens a container at the specified position and waits for the server to respond.
// This method:
// 1. Looks at the container
// 2. Calls the container helper to open the container
//
// Note: Continuous position packets are sent automatically by the agent's background heartbeat
// (started in Start() at 20 TPS, matching vanilla Minecraft client behavior).
//
// Returns the window ID assigned by the server, or error if timeout/failure.
func (a *agent) OpenContainer(pos models.V3, face models.BlockFace, timeout time.Duration, cursorX, cursorY, cursorZ float32) (byte, error) {
	ch := a.containerHelper
	if ch == nil {
		return 0, fmt.Errorf("container helper not initialized")
	}

	moveExec := a.moveExec
	if moveExec == nil {
		return 0, fmt.Errorf("movement executor not initialized")
	}

	face = a.chooseOpenFace(pos, face)

	// Look at the container before opening
	log.Printf("[Agent %s] Looking at container at (%.1f, %.1f, %.1f)", a.cfg.Name, pos.X, pos.Y, pos.Z)
	if err := moveExec.LookAt(pos.X, pos.Y, pos.Z, true); err != nil {
		return 0, fmt.Errorf("look at container: %w", err)
	}

	// Small delay to ensure rotation packet is processed
	time.Sleep(2 * time.Second)

	// Open the container using helper
	log.Printf("[Agent %s] Opening container at (%.1f, %.1f, %.1f) face=%d", a.cfg.Name, pos.X, pos.Y, pos.Z, face)
	windowID, err := ch.OpenContainer(pos, face, timeout, cursorX, cursorY, cursorZ)
	if err != nil {
		return 0, fmt.Errorf("open container: %w", err)
	}

	log.Printf("[Agent %s] Container opened successfully with window ID %d", a.cfg.Name, windowID)
	time.Sleep(2 * time.Second)
	return windowID, nil
}

func (a *agent) chooseOpenFace(pos models.V3, fallback models.BlockFace) models.BlockFace {
	botPos, _, _, ok := a.GetPosition()
	if !ok {
		return fallback
	}
	centerX := math.Floor(pos.X) + 0.5
	centerY := math.Floor(pos.Y) + 0.5
	centerZ := math.Floor(pos.Z) + 0.5
	dx := centerX - botPos.X
	dy := centerY - botPos.Y
	dz := centerZ - botPos.Z

	absX := math.Abs(dx)
	absY := math.Abs(dy)
	absZ := math.Abs(dz)

	switch {
	case absY >= absX && absY >= absZ:
		if dy >= 0 {
			return models.FaceUp
		}
		return models.FaceDown
	case absX >= absZ:
		if dx >= 0 {
			return models.FaceEast
		}
		return models.FaceWest
	default:
		if dz >= 0 {
			return models.FaceSouth
		}
		return models.FaceNorth
	}
}

// OpenEntityContainer opens an entity container (horse, donkey, llama, chest boat, chest minecart).
// This method calls the container helper to open the entity container.
//
// Note: Continuous position packets are sent automatically by the agent's background heartbeat
// (started in Start() at 20 TPS, matching vanilla Minecraft client behavior).
//
// Returns the window ID assigned by the server, or error if timeout/failure.
func (a *agent) OpenEntityContainer(entityID int32, timeout time.Duration) (byte, error) {
	ch := a.containerHelper
	if ch == nil {
		return 0, fmt.Errorf("container helper not initialized")
	}

	// Note the entity BEFORE the request goes out. The server sends the window's
	// contents as part of opening it, and that is the very event OpenEntityContainer
	// blocks on, so ContainerSetContent routinely arrives while we are still inside
	// the call below and have no window ID to record yet. Marking the open in
	// flight lets those contents bind themselves once the window ID shows up.
	a.beginEntityContainerOpen(entityID)
	defer a.endEntityContainerOpen()

	// Open the entity container using helper
	log.Printf("[Agent %s] Opening entity container for entity ID %d", a.cfg.Name, entityID)
	windowID, err := ch.OpenEntityContainer(entityID, timeout)
	if err != nil {
		return 0, fmt.Errorf("open entity container: %w", err)
	}

	// Record the mapping explicitly too, for the case where the contents have
	// not arrived yet. registerEntityWindow is idempotent with the in-flight
	// binding above.
	a.registerEntityWindow(windowID, entityID)

	// Total slot count for the window (entity slots + the 36-slot player
	// section) is the fastest way to tell what kind of container the server
	// actually opened — e.g. distinguishing a chested donkey/mule/llama's
	// larger window from a plain horse's saddle-only one.
	log.Printf("[Agent %s] Window %d total slot count: %d", a.cfg.Name, windowID, ch.GetContainerSlotCount(windowID))

	// Report whether contents were actually captured. A container whose window
	// opens but never delivers ContainerSetContent would leave no snapshot, and
	// this is the only place that distinction is visible.
	if _, captured := a.GetEntityInventory(entityID); !captured {
		log.Printf("[Agent %s][WARN] Entity container opened (window %d, entity %d) but no contents have been received yet; inventory snapshot is empty",
			a.cfg.Name, windowID, entityID)
	}

	log.Printf("[Agent %s] Entity container opened successfully with window ID %d (entity %d)", a.cfg.Name, windowID, entityID)
	return windowID, nil
}

// CloseContainer closes the currently open container window.
func (a *agent) CloseContainer() error {
	ch := a.containerHelper
	if ch == nil {
		return fmt.Errorf("container helper not initialized")
	}

	log.Printf("[Agent %s] Closing container", a.cfg.Name)
	if err := ch.CloseContainer(); err != nil {
		return fmt.Errorf("close container: %w", err)
	}

	// Any cached entity container contents stop tracking the server now. The
	// snapshot is kept but flagged not-live so callers know it can go stale.
	a.releaseEntityWindows()

	log.Printf("[Agent %s] Container closed successfully", a.cfg.Name)
	return nil
}

// TakeItemFromChest takes an item from a chest slot and places it in the player's inventory.
func (a *agent) TakeItemFromChest(windowID byte, chestSlot int16) error {
	ch := a.containerHelper
	if ch == nil {
		return fmt.Errorf("container helper not initialized")
	}
	return ch.TakeItemFromChest(windowID, chestSlot)
}

// PutItemInChest puts an item from the player's inventory into a chest slot.
func (a *agent) PutItemInChest(windowID byte, playerInventorySlot int16, chestSlot int16) error {
	ch := a.containerHelper
	if ch == nil {
		return fmt.Errorf("container helper not initialized")
	}
	return ch.PutItemInChest(windowID, playerInventorySlot, chestSlot)
}

// FindItemInPlayerInventory finds an item in the player's inventory when a chest is open.
func (a *agent) FindItemInPlayerInventory(windowID byte, itemID int32) int16 {
	if a.containerHelper == nil {
		return -1
	}
	return a.containerHelper.FindItemInPlayerInventory(windowID, itemID)
}

// FindEmptyChestSlot finds an empty slot in a chest.
func (a *agent) FindEmptyChestSlot(windowID byte) int16 {
	if a.containerHelper == nil {
		return -1
	}
	return a.containerHelper.FindEmptyChestSlot(windowID)
}

// GetChestRows returns the number of rows in a chest window.
func (a *agent) GetChestRows(windowID byte) int {
	if a.containerHelper == nil {
		return -1
	}
	return a.containerHelper.GetChestRows(windowID)
}

// GetContainerSlotCount returns the number of slots in a container window.
func (a *agent) GetContainerSlotCount(windowID byte) int {
	if a.containerHelper == nil {
		return -1
	}
	return a.containerHelper.GetContainerSlotCount(windowID)
}

// InventoryManager pass-through methods.
// Each delegates to the internally-managed InventoryManager.

func (a *agent) SetWindow(windowID byte) {
	if a.invMgr != nil {
		a.invMgr.SetWindow(windowID)
	}
}

func (a *agent) GetWindow() byte {
	if a.invMgr != nil {
		return a.invMgr.GetWindow()
	}
	return 0
}

func (a *agent) SetCursorItem(item models.ItemStack) {
	if a.invMgr != nil {
		a.invMgr.SetCursorItem(item)
	}
}

func (a *agent) GetCursorItem() models.ItemStack {
	if a.invMgr != nil {
		return a.invMgr.GetCursorItem()
	}
	return models.ItemStack{}
}

func (a *agent) SetWaitForUpdates(wait bool) {
	if a.invMgr != nil {
		a.invMgr.SetWaitForUpdates(wait)
	}
}

func (a *agent) SetUpdateWaitDelay(delay time.Duration) {
	if a.invMgr != nil {
		a.invMgr.SetUpdateWaitDelay(delay)
	}
}

func (a *agent) LeftClickSlot(slot int16, slotItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.LeftClickSlot(slot, slotItem)
}

func (a *agent) RightClickSlot(slot int16, slotItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.RightClickSlot(slot, slotItem)
}

func (a *agent) ShiftClickSlot(slot int16, slotItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.ShiftClickSlot(slot, slotItem)
}

func (a *agent) SwapWithHotbar(slot int16, slotItem models.ItemStack, hotbarSlot int, hotbarItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.SwapWithHotbar(slot, slotItem, hotbarSlot, hotbarItem)
}

func (a *agent) DropItem(slot int16, slotItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.DropItem(slot, slotItem)
}

func (a *agent) DropStack(slot int16, slotItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.DropStack(slot, slotItem)
}

func (a *agent) DoubleClick(slot int16, slotItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.DoubleClick(slot, slotItem)
}

func (a *agent) StartDrag(dragType byte) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.StartDrag(dragType)
}

func (a *agent) AddDragSlot(slot int16, dragType byte) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.AddDragSlot(slot, dragType)
}

func (a *agent) EndDrag(dragType byte, changedSlots []models.ChangedSlot) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.EndDrag(dragType, changedSlots)
}

func (a *agent) MoveItem(fromSlot, toSlot int16, fromItem, toItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.MoveItem(fromSlot, toSlot, fromItem, toItem)
}

func (a *agent) MoveSingle(fromSlot, toSlot int16, fromItem, toItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.MoveSingle(fromSlot, toSlot, fromItem, toItem)
}

func (a *agent) TransferItem(fromWindowID byte, fromSlot int16, toWindowID byte, toSlot int16, fromItem, toItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.TransferItem(fromWindowID, fromSlot, toWindowID, toSlot, fromItem, toItem)
}

func (a *agent) TransferStack(slot int16, slotItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.TransferStack(slot, slotItem)
}

func (a *agent) SplitStack(sourceSlot, destSlot int16, sourceItem, destItem models.ItemStack) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.SplitStack(sourceSlot, destSlot, sourceItem, destItem)
}

func (a *agent) DistributeItems(slots []int16, evenlyDistribute bool, changedSlots []models.ChangedSlot) error {
	if a.invMgr == nil {
		return fmt.Errorf("inventory manager not initialized")
	}
	return a.invMgr.DistributeItems(slots, evenlyDistribute, changedSlots)
}
