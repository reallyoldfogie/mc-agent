package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newInventoryTestAgent builds an agent with the given entity IDs tracked and
// nothing else wired up. The inventory cache only needs the entity map and the
// window mapping, so no client or version handler is required.
func newInventoryTestAgent(entityIDs ...int32) *agent {
	entities := make(map[int32]*trackedEntity, len(entityIDs))
	for _, entityID := range entityIDs {
		entities[entityID] = &trackedEntity{EntityID: entityID}
	}
	return &agent{entities: entities, pendingEntityContainerID: -1}
}

// windowSlots builds a container window: containerCount entity slots followed
// by the mandatory 36-slot player inventory section.
func windowSlots(containerCount int) []models.InventorySlot {
	slots := make([]models.InventorySlot, containerCount+models.PlayerInventorySlotCount)
	for i := range slots {
		slots[i] = models.InventorySlot{Present: true, Count: int32(i + 1)}
	}
	return slots
}

func TestStoreEntityWindowContentsKeepsOnlyEntitySlots(t *testing.T) {
	const (
		donkeyEntityID int32 = 7
		windowID       byte  = 3
		chestSlots           = 15
	)

	testAgent := newInventoryTestAgent(donkeyEntityID)
	testAgent.registerEntityWindow(windowID, donkeyEntityID)
	testAgent.storeEntityWindowContents(windowID, windowSlots(chestSlots))

	inventory, found := testAgent.GetEntityInventory(donkeyEntityID)
	require.True(t, found)
	require.Len(t, inventory.Slots, chestSlots, "player inventory section must be excluded")
	assert.True(t, inventory.Live, "window is still open")
	assert.False(t, inventory.UpdatedAt.IsZero())

	// Leading slots, in order, are the entity's own.
	assert.EqualValues(t, 1, inventory.Slots[0].Count)
	assert.EqualValues(t, chestSlots, inventory.Slots[chestSlots-1].Count)
}

func TestStoreEntityWindowContentsIgnoresUnknownWindow(t *testing.T) {
	const entityID int32 = 7

	testAgent := newInventoryTestAgent(entityID)
	// Neither a registered window nor an open in flight: this is somebody
	// else's window (a chest block, say), so nothing should be attributed.
	testAgent.storeEntityWindowContents(4, windowSlots(15))

	_, found := testAgent.GetEntityInventory(entityID)
	assert.False(t, found)
}

// TestStoreEntityWindowContentsBindsInFlightOpen covers the ordering that broke
// this feature in practice.
//
// The server delivers a window's contents as part of opening it, and that is the
// same event the open call blocks on, so ContainerSetContent routinely arrives
// before the window ID is available to record. Attribution must still work.
func TestStoreEntityWindowContentsBindsInFlightOpen(t *testing.T) {
	const (
		entityID   int32 = 7
		windowID   byte  = 3
		chestSlots       = 15
	)

	testAgent := newInventoryTestAgent(entityID)

	// Open in flight, window ID not yet known — the state the agent is in while
	// blocked inside the container helper.
	testAgent.beginEntityContainerOpen(entityID)
	testAgent.storeEntityWindowContents(windowID, windowSlots(chestSlots))

	inventory, found := testAgent.GetEntityInventory(entityID)
	require.True(t, found, "contents arriving before the window ID was recorded must still be attributed")
	assert.Len(t, inventory.Slots, chestSlots)

	// The binding should now be recorded, so later slot updates land too.
	mappedEntity, mapped := testAgent.entityForWindow(windowID)
	require.True(t, mapped)
	assert.Equal(t, entityID, mappedEntity)
}

// TestStoreEntityWindowContentsIgnoredAfterOpenCompletes guards the other side:
// once the open is no longer in flight, an unmapped window must not be bound to
// whatever entity happened to be opened last.
func TestStoreEntityWindowContentsIgnoredAfterOpenCompletes(t *testing.T) {
	const entityID int32 = 7

	testAgent := newInventoryTestAgent(entityID)
	testAgent.beginEntityContainerOpen(entityID)
	testAgent.endEntityContainerOpen()

	testAgent.storeEntityWindowContents(9, windowSlots(15))

	_, found := testAgent.GetEntityInventory(entityID)
	assert.False(t, found, "an unrelated window opened later must not bind to a finished open")
}

// TestReleaseEntityWindowsClearsPendingOpen stops a stale in-flight marker from
// capturing a subsequent unrelated container.
func TestReleaseEntityWindowsClearsPendingOpen(t *testing.T) {
	const entityID int32 = 7

	testAgent := newInventoryTestAgent(entityID)
	testAgent.beginEntityContainerOpen(entityID)
	testAgent.releaseEntityWindows()

	testAgent.storeEntityWindowContents(5, windowSlots(15))

	_, found := testAgent.GetEntityInventory(entityID)
	assert.False(t, found)
}

func TestStoreEntityWindowContentsRejectsUnsplittableWindow(t *testing.T) {
	const (
		entityID int32 = 7
		windowID byte  = 3
	)

	testAgent := newInventoryTestAgent(entityID)
	testAgent.registerEntityWindow(windowID, entityID)

	// Too few slots to contain a player inventory section. Storing a mis-split
	// snapshot would be worse than storing none.
	tooSmall := make([]models.InventorySlot, models.PlayerInventorySlotCount-1)
	testAgent.storeEntityWindowContents(windowID, tooSmall)

	_, found := testAgent.GetEntityInventory(entityID)
	assert.False(t, found)
}

func TestUpdateEntityWindowSlot(t *testing.T) {
	const (
		entityID   int32 = 7
		windowID   byte  = 3
		chestSlots       = 15
	)

	setup := func() *agent {
		testAgent := newInventoryTestAgent(entityID)
		testAgent.registerEntityWindow(windowID, entityID)
		testAgent.storeEntityWindowContents(windowID, windowSlots(chestSlots))
		return testAgent
	}

	t.Run("updates a container slot", func(t *testing.T) {
		testAgent := setup()
		testAgent.updateEntityWindowSlot(windowID, 2, models.InventorySlot{Present: true, Count: 64})

		inventory, found := testAgent.GetEntityInventory(entityID)
		require.True(t, found)
		assert.EqualValues(t, 64, inventory.Slots[2].Count)
	})

	t.Run("ignores the player inventory section", func(t *testing.T) {
		testAgent := setup()
		before, _ := testAgent.GetEntityInventory(entityID)

		// Index at the boundary and beyond belongs to our own inventory.
		testAgent.updateEntityWindowSlot(windowID, chestSlots, models.InventorySlot{Present: true, Count: 99})
		testAgent.updateEntityWindowSlot(windowID, chestSlots+10, models.InventorySlot{Present: true, Count: 99})

		after, _ := testAgent.GetEntityInventory(entityID)
		assert.Equal(t, before.Slots, after.Slots)
	})

	t.Run("ignores negative indices", func(t *testing.T) {
		testAgent := setup()
		before, _ := testAgent.GetEntityInventory(entityID)

		testAgent.updateEntityWindowSlot(windowID, -1, models.InventorySlot{Present: true, Count: 99})

		after, _ := testAgent.GetEntityInventory(entityID)
		assert.Equal(t, before.Slots, after.Slots)
	})

	t.Run("does nothing without a baseline snapshot", func(t *testing.T) {
		// A lone slot update cannot establish where the container/player
		// boundary sits, so it must wait for the full window contents.
		testAgent := newInventoryTestAgent(entityID)
		testAgent.registerEntityWindow(windowID, entityID)

		testAgent.updateEntityWindowSlot(windowID, 0, models.InventorySlot{Present: true, Count: 5})

		_, found := testAgent.GetEntityInventory(entityID)
		assert.False(t, found)
	})
}

func TestReleaseEntityWindowsFreezesSnapshot(t *testing.T) {
	const (
		entityID   int32 = 7
		windowID   byte  = 3
		chestSlots       = 15
	)

	testAgent := newInventoryTestAgent(entityID)
	testAgent.registerEntityWindow(windowID, entityID)
	testAgent.storeEntityWindowContents(windowID, windowSlots(chestSlots))

	testAgent.releaseEntityWindows()

	// Contents survive the close — that is the point of the cache — but stop
	// claiming to be current.
	inventory, found := testAgent.GetEntityInventory(entityID)
	require.True(t, found, "closing the window must not discard the snapshot")
	assert.Len(t, inventory.Slots, chestSlots)
	assert.False(t, inventory.Live, "a closed window can no longer track the server")

	// The window mapping is gone, so a recycled window ID cannot write to this
	// entity any more.
	_, stillMapped := testAgent.entityForWindow(windowID)
	assert.False(t, stillMapped)
}

func TestForgetEntityWindowsDropsPurgedEntities(t *testing.T) {
	const (
		keptEntityID   int32 = 7
		purgedEntityID int32 = 8
		keptWindowID   byte  = 3
		purgedWindowID byte  = 4
	)

	testAgent := newInventoryTestAgent(keptEntityID, purgedEntityID)
	testAgent.registerEntityWindow(keptWindowID, keptEntityID)
	testAgent.registerEntityWindow(purgedWindowID, purgedEntityID)

	testAgent.forgetEntityWindows([]int32{purgedEntityID})

	_, purgedMapped := testAgent.entityForWindow(purgedWindowID)
	assert.False(t, purgedMapped, "window for a purged entity must be dropped")

	mappedEntity, keptMapped := testAgent.entityForWindow(keptWindowID)
	require.True(t, keptMapped, "unrelated windows must be left alone")
	assert.Equal(t, keptEntityID, mappedEntity)
}

// TestGetEntityInventoryReturnsCopy guards against callers mutating the cache
// through the returned slice.
func TestGetEntityInventoryReturnsCopy(t *testing.T) {
	const (
		entityID int32 = 7
		windowID byte  = 3
	)

	testAgent := newInventoryTestAgent(entityID)
	testAgent.registerEntityWindow(windowID, entityID)
	testAgent.storeEntityWindowContents(windowID, windowSlots(5))

	firstRead, found := testAgent.GetEntityInventory(entityID)
	require.True(t, found)
	firstRead.Slots[0] = models.InventorySlot{Present: true, Count: 123}

	secondRead, _ := testAgent.GetEntityInventory(entityID)
	assert.NotEqualValues(t, 123, secondRead.Slots[0].Count, "cache must not be mutable by callers")
}

func TestGetEntityInventoryUnknownEntity(t *testing.T) {
	testAgent := newInventoryTestAgent()

	inventory, found := testAgent.GetEntityInventory(999)
	assert.False(t, found)
	assert.Empty(t, inventory.Slots)
}
