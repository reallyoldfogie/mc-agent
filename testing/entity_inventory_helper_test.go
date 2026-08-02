package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedEntityContainerSlot puts a known item stack into an entity container slot
// via RCON, so a cached snapshot can be checked against a value we chose rather
// than against an empty container (which an entirely broken cache would also
// satisfy).
//
// entitySelector is a target selector such as
// `@e[type=minecraft:oak_chest_boat,limit=1,sort=nearest]`.
func seedEntityContainerSlot(t *testing.T, env *StandaloneTestEnv, entitySelector string, slot int, itemID string, count int) {
	t.Helper()

	cmd := fmt.Sprintf("item replace entity %s container.%d with %s %d", entitySelector, slot, itemID, count)
	resp, err := env.Inst.RCON.Exec(env.Ctx, cmd)
	require.NoError(t, err, "seed entity container slot")
	t.Logf("%s => %s", cmd, resp)
	time.Sleep(300 * time.Millisecond)
}

// assertEntityInventoryCachedAfterClose checks the Phase 3.6c contract on a
// container that has just been opened and closed.
//
// The point of the cache is that contents remain readable after the window is
// gone, so this is asserted *after* CloseContainer. The snapshot must also stop
// claiming to be current at that point, because the server no longer reports
// changes to a closed container.
//
// wantSlotZeroCount > 0 additionally asserts the contents. Pass 0 to check only
// the snapshot contract, for containers where nothing was seeded.
func assertEntityInventoryCachedAfterClose(
	t *testing.T,
	agent models.Agent,
	entityID int32,
	wantSlotZeroCount int32,
) {
	t.Helper()

	inventory, found := agent.GetEntityInventory(entityID)
	require.True(t, found,
		"entity %d should have a cached inventory snapshot after its container was opened; "+
			"if this fails the window contents were never attributed to the entity",
		entityID)

	t.Logf("Cached inventory for entity %d: %d slots, updatedAt=%s, live=%v",
		entityID, len(inventory.Slots), inventory.UpdatedAt.Format(time.RFC3339Nano), inventory.Live)
	for i, slot := range inventory.Slots {
		if slot.Count > 0 {
			t.Logf("  slot %d: itemID=%d count=%d", i, slot.ItemID, slot.Count)
		}
	}

	assert.False(t, inventory.Live,
		"a closed container can no longer track the server, so the snapshot must not claim to be live")
	assert.False(t, inventory.UpdatedAt.IsZero(),
		"snapshot must carry the time it was captured so callers can judge staleness")

	// The player-inventory section must have been stripped. A window carrying
	// only the 36 player slots would leave this empty, and anything that failed
	// to strip them would leave at least 36.
	assert.NotEmpty(t, inventory.Slots,
		"snapshot should contain the entity's own slots")
	assert.Less(t, len(inventory.Slots), models.PlayerInventorySlotCount,
		"snapshot should hold only the entity's slots; a count at or above %d means the player inventory section was not stripped",
		models.PlayerInventorySlotCount)

	if wantSlotZeroCount > 0 {
		require.NotEmpty(t, inventory.Slots, "expected seeded contents")
		assert.EqualValues(t, wantSlotZeroCount, inventory.Slots[0].Count,
			"slot 0 should hold the stack we seeded before opening the container")
		assert.True(t, inventory.Slots[0].Present, "seeded slot should be marked present")
	}
}
