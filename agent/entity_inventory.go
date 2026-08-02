package agent

import (
	"log"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Entity container inventory tracking.
//
// Container packets (ClientboundContainerSetContent, ClientboundSetSlot) only
// identify a window by its ID; nothing on the wire says which entity that
// window belongs to. We know the association only because we opened the
// container ourselves, so the mapping is recorded at open time and consulted
// when contents arrive.
//
// Contents are cached per entity so callers can ask what is in a donkey's chest
// without re-opening it. The cache is explicitly a snapshot: see
// models.EntityInventory for the staleness contract.

// registerEntityWindow records that windowID is showing entityID's container.
func (a *agent) registerEntityWindow(windowID byte, entityID int32) {
	a.entityWindowsMu.Lock()
	defer a.entityWindowsMu.Unlock()
	if a.entityWindows == nil {
		a.entityWindows = make(map[byte]int32)
	}
	a.entityWindows[windowID] = entityID
	log.Printf("[entityInventory] Window %d now tracking entity %d's container", windowID, entityID)
}

// entityForWindow returns the entity whose container is shown in windowID.
func (a *agent) entityForWindow(windowID byte) (int32, bool) {
	a.entityWindowsMu.RLock()
	defer a.entityWindowsMu.RUnlock()
	entityID, ok := a.entityWindows[windowID]
	return entityID, ok
}

// releaseEntityWindows marks every tracked entity container as no longer live
// and forgets the window mapping.
//
// Called when a container closes. The cached contents are deliberately kept —
// that is the whole point of the cache — but they stop being authoritative the
// moment the window is gone, because the server no longer reports changes.
func (a *agent) releaseEntityWindows() {
	a.entityWindowsMu.Lock()
	trackedEntityIDs := make([]int32, 0, len(a.entityWindows))
	for windowID, entityID := range a.entityWindows {
		trackedEntityIDs = append(trackedEntityIDs, entityID)
		delete(a.entityWindows, windowID)
	}
	a.entityWindowsMu.Unlock()

	if len(trackedEntityIDs) == 0 {
		return
	}

	a.entitiesMu.Lock()
	for _, entityID := range trackedEntityIDs {
		if entity, ok := a.entities[entityID]; ok && entity.Inventory != nil {
			entity.Inventory.Live = false
		}
	}
	a.entitiesMu.Unlock()

	log.Printf("[entityInventory] Released %d entity container window(s); cached contents are now snapshots", len(trackedEntityIDs))
}

// forgetEntityWindows removes the window mapping for entities that no longer
// exist. Window IDs are recycled by the server, so a mapping left pointing at a
// purged entity could cause a later, unrelated container's contents to be
// attributed to it.
func (a *agent) forgetEntityWindows(entityIDs []int32) {
	if len(entityIDs) == 0 {
		return
	}

	purged := make(map[int32]struct{}, len(entityIDs))
	for _, entityID := range entityIDs {
		purged[entityID] = struct{}{}
	}

	a.entityWindowsMu.Lock()
	defer a.entityWindowsMu.Unlock()
	for windowID, entityID := range a.entityWindows {
		if _, gone := purged[entityID]; gone {
			delete(a.entityWindows, windowID)
			log.Printf("[entityInventory] Dropped window %d mapping; entity %d no longer tracked", windowID, entityID)
		}
	}
}

// storeEntityWindowContents records a full container refresh for the entity
// showing in windowID.
//
// windowSlots is the entire window, which is laid out as the container's own
// slots followed by the player's inventory. Only the container portion is
// stored; our own inventory is not the entity's business.
func (a *agent) storeEntityWindowContents(windowID byte, windowSlots []models.InventorySlot) {
	entityID, ok := a.entityForWindow(windowID)
	if !ok {
		return // Not an entity container we are tracking.
	}

	containerSlots, _, splitOK := models.SplitContainerSlots(windowSlots)
	if !splitOK {
		// Too small to have the expected [container][player] layout. Storing a
		// mis-split snapshot would be worse than storing nothing.
		log.Printf("[entityInventory] Window %d for entity %d has %d slots, fewer than the %d-slot player section; skipping",
			windowID, entityID, len(windowSlots), models.PlayerInventorySlotCount)
		return
	}

	// Copy: the parsed slice is owned by the packet handler and the window
	// portion we keep is a sub-slice of it.
	storedSlots := make([]models.InventorySlot, len(containerSlots))
	copy(storedSlots, containerSlots)

	a.entitiesMu.Lock()
	if entity, exists := a.entities[entityID]; exists {
		entity.Inventory = &models.EntityInventory{
			Slots:     storedSlots,
			UpdatedAt: time.Now(),
			Live:      true,
		}
	}
	a.entitiesMu.Unlock()

	log.Printf("[entityInventory] Entity %d container refreshed: %d entity slots (window had %d total)",
		entityID, len(storedSlots), len(windowSlots))
}

// updateEntityWindowSlot records a single-slot change for the entity showing in
// windowID.
//
// slotIndex is a window-relative index. Indices at or beyond the container's
// own slot count belong to the player inventory section and are ignored.
func (a *agent) updateEntityWindowSlot(windowID byte, slotIndex int16, item models.InventorySlot) {
	entityID, ok := a.entityForWindow(windowID)
	if !ok {
		return
	}

	a.entitiesMu.Lock()
	defer a.entitiesMu.Unlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Inventory == nil {
		// No baseline snapshot yet. A single slot update on its own cannot
		// establish where the container/player boundary sits, so wait for the
		// full ContainerSetContent that the server sends when the window opens.
		return
	}

	if slotIndex < 0 || int(slotIndex) >= len(entity.Inventory.Slots) {
		return // Player inventory section, or out of range.
	}

	entity.Inventory.Slots[slotIndex] = item
	entity.Inventory.UpdatedAt = time.Now()
}

// GetEntityInventory returns the cached snapshot of an entity's container
// contents, and whether one has ever been captured.
//
// The snapshot is only guaranteed current while Live is true (the window is
// open). Once closed, treat it as "what was there at UpdatedAt" — the server
// stops reporting changes, so a hopper or another player can empty the chest
// without us hearing about it. Re-open the container when accuracy matters.
func (a *agent) GetEntityInventory(entityID int32) (models.EntityInventory, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Inventory == nil {
		return models.EntityInventory{}, false
	}

	// Return a copy so callers cannot mutate the cache, and so the slot slice
	// does not race with a concurrent update.
	snapshotSlots := make([]models.InventorySlot, len(entity.Inventory.Slots))
	copy(snapshotSlots, entity.Inventory.Slots)

	return models.EntityInventory{
		Slots:     snapshotSlots,
		UpdatedAt: entity.Inventory.UpdatedAt,
		Live:      entity.Inventory.Live,
	}, true
}
