package agent

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// SetContainerHelper injects a ContainerHelper implementation.
func (a *agent) SetContainerHelper(ch ContainerHelper) {
	a.containerSubsystemMu.Lock()
	defer a.containerSubsystemMu.Unlock()
	a.containerHelper = ch

	// Wire up entity ID provider so container helper can access entity ID
	if ch != nil {
		ch.SetEntityIDProvider(a)
	}
}

// GetContainerHelper returns the container helper if it has been initialized.
func (a *agent) GetContainerHelper() ContainerHelper {
	a.containerSubsystemMu.RLock()
	defer a.containerSubsystemMu.RUnlock()
	return a.containerHelper
}

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
				x, y, z, yaw, pitch, initialized := a.GetPosition()
				if initialized && a.moveExec != nil {
					log.Printf("[YAW DEBUG] HEARTBEAT sending: pos=(%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f", x, y, z, yaw, pitch)
					if err := a.moveExec.SendPositionAndRotation(x, y, z, yaw, pitch, true); err != nil {
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

	// Sequential snapshots: read containerHelper and moveExec from their respective locks
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()

	if ch == nil {
		return 0, fmt.Errorf("container helper not set - call SetContainerHelper first")
	}

	if moveExec == nil {
		return 0, fmt.Errorf("movement executor not set - call SetMovementExecutor first")
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
	x, y, z, _, _, ok := a.GetPosition()
	if !ok {
		return fallback
	}
	centerX := math.Floor(pos.X) + 0.5
	centerY := math.Floor(pos.Y) + 0.5
	centerZ := math.Floor(pos.Z) + 0.5
	dx := centerX - x
	dy := centerY - y
	dz := centerZ - z

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
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	if ch == nil {
		return 0, fmt.Errorf("container helper not set - call SetContainerHelper first")
	}

	// Open the entity container using helper
	log.Printf("[Agent %s] Opening entity container for entity ID %d", a.cfg.Name, entityID)
	windowID, err := ch.OpenEntityContainer(entityID, timeout)
	if err != nil {
		return 0, fmt.Errorf("open entity container: %w", err)
	}

	log.Printf("[Agent %s] Entity container opened successfully with window ID %d", a.cfg.Name, windowID)
	return windowID, nil
}

// CloseContainer closes the currently open container window.
func (a *agent) CloseContainer() error {
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	if ch == nil {
		return fmt.Errorf("container helper not set - call SetContainerHelper first")
	}

	log.Printf("[Agent %s] Closing container", a.cfg.Name)
	if err := ch.CloseContainer(); err != nil {
		return fmt.Errorf("close container: %w", err)
	}

	log.Printf("[Agent %s] Container closed successfully", a.cfg.Name)
	return nil
}

// TakeItemFromChest takes an item from a chest slot and places it in the player's inventory.
func (a *agent) TakeItemFromChest(windowID byte, chestSlot int16) error {
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	if ch == nil {
		return fmt.Errorf("container helper not set - call SetContainerHelper first")
	}

	return ch.TakeItemFromChest(windowID, chestSlot)
}

// PutItemInChest puts an item from the player's inventory into a chest slot.
func (a *agent) PutItemInChest(windowID byte, playerInventorySlot int16, chestSlot int16) error {
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	if ch == nil {
		return fmt.Errorf("container helper not set - call SetContainerHelper first")
	}

	return ch.PutItemInChest(windowID, playerInventorySlot, chestSlot)
}

// FindItemInPlayerInventory finds an item in the player's inventory when a chest is open.
func (a *agent) FindItemInPlayerInventory(windowID byte, itemID int32) int16 {
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	if ch == nil {
		return -1
	}

	return ch.FindItemInPlayerInventory(windowID, itemID)
}

// FindEmptyChestSlot finds an empty slot in a chest.
func (a *agent) FindEmptyChestSlot(windowID byte) int16 {
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	if ch == nil {
		return -1
	}

	return ch.FindEmptyChestSlot(windowID)
}

// GetChestRows returns the number of rows in a chest window.
func (a *agent) GetChestRows(windowID byte) int {
	a.containerSubsystemMu.RLock()
	ch := a.containerHelper
	a.containerSubsystemMu.RUnlock()

	if ch == nil {
		return -1
	}

	return ch.GetChestRows(windowID)
}
