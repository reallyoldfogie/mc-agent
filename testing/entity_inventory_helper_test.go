package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForEntityTypeID polls the agent's registry for an entity type name.
//
// The registry arrives during the configuration phase, so a lookup issued too
// early legitimately misses. Failing immediately turned that into a flaky
// "entity type should be in registry" assertion.
func waitForEntityTypeID(t *testing.T, env *StandaloneTestEnv, entityTypeName string, timeout time.Duration) int32 {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if typeID, ok := env.Agent.Agent.GetEntityTypeID(entityTypeName); ok {
			return typeID
		}
		if time.Now().After(deadline) {
			env.Agent.Agent.DumpRegistry("minecraft:entity_type")
			require.FailNowf(t, "entity type never appeared in registry",
				"%s not found after %v", entityTypeName, timeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// waitForNearestEntityByType polls until an entity of the given type is tracked
// near the supplied position.
//
// An RCON summon returns as soon as the server has spawned the entity, which is
// before the spawn packet has reached us and been processed. Asserting straight
// after the summon therefore races the network, which is what produced the
// intermittent "should find X entity" failures.
//
// maxEntitySearchDistance caps how far a type-match can be from the search
// point before it's treated as not found yet and polling continues. World
// generation (e.g. a mineshaft's chest minecart) can produce a genuine,
// unrelated entity of the same type elsewhere in the loaded area; without a
// cap, a stray match like that gets returned instead of the one the test just
// spawned nearby, and the later interaction times out because it's too far
// away to reach.
const maxEntitySearchDistance = 15.0

// entityTrackingAttempts bounds how many full timeout windows
// waitForNearestEntityByType polls before failing.
//
// RCON already confirms the entity exists server-side before this is ever
// called (every call site verifies the spawn via RCON first), so a timeout
// here means the spawn packet was slow or dropped in transit, not that the
// entity doesn't exist. Re-arming the deadline once absorbs a slow packet
// without hiding a real regression: an entity that truly never gets tracked
// still fails, just after two windows instead of one.
const entityTrackingAttempts = 2

func waitForNearestEntityByType(t *testing.T, env *StandaloneTestEnv, entityTypeID int32, x, y, z float64, timeout time.Duration) int32 {
	t.Helper()

	for attempt := 1; attempt <= entityTrackingAttempts; attempt++ {
		if entityID, found := pollForNearestEntityByType(t, env, entityTypeID, x, y, z, timeout); found {
			return entityID
		}
		if attempt < entityTrackingAttempts {
			t.Logf("entity type %d not tracked within %v (attempt %d/%d); retrying wait",
				entityTypeID, timeout, attempt, entityTrackingAttempts)
		}
	}

	require.FailNowf(t, "entity never became tracked",
		"no entity of type %d within %.1f blocks of (%.1f, %.1f, %.1f) after %d attempts of %v",
		entityTypeID, maxEntitySearchDistance, x, y, z, entityTrackingAttempts, timeout)
	return 0 // unreachable: FailNowf stops the goroutine
}

// pollForNearestEntityByType polls once for up to timeout, returning
// (0, false) instead of failing the test if nothing turns up.
func pollForNearestEntityByType(t *testing.T, env *StandaloneTestEnv, entityTypeID int32, x, y, z float64, timeout time.Duration) (int32, bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		entityID, distance, found := env.Agent.FindNearestEntityByType(entityTypeID, x, y, z)
		if found && distance <= maxEntitySearchDistance {
			t.Logf("Found entity type %d as ID %d at distance %.2f blocks", entityTypeID, entityID, distance)
			return entityID, true
		}
		if found {
			t.Logf("Entity type %d found as ID %d at distance %.2f blocks, past the %.1f-block cap — treating as not found yet",
				entityTypeID, entityID, distance, maxEntitySearchDistance)
		}
		if time.Now().After(deadline) {
			return 0, false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

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
