// Package storetest is a backend-agnostic conformance suite for
// memorystore.Store implementations. Both built-in backends
// (jsonstore, boltstore) run the exact same suite against their own
// Factory — see each backend's own _test.go — so the two stay provably
// interchangeable behind the memorystore.Store interface, and a third
// backend added later (per the plan's "Extensibility") gets the same
// conformance check for free by calling Run too.
package storetest

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	"github.com/stretchr/testify/require"
)

// Run exercises factory's Store against the behaviors Parts 1/4 of
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md actually need. t.TempDir()
// backs every subtest, so each gets an isolated store directory.
func Run(t *testing.T, factory memorystore.Factory) {
	t.Run("RecordAndQuerySighting", func(t *testing.T) { testRecordAndQuerySighting(t, factory) })
	t.Run("RefreshesExistingSighting", func(t *testing.T) { testRefreshesExistingSighting(t, factory) })
	t.Run("NearestPicksClosest", func(t *testing.T) { testNearestPicksClosest(t, factory) })
	t.Run("ContainerOnlyFilter", func(t *testing.T) { testContainerOnlyFilter(t, factory) })
	t.Run("ExcludeContainerFilter", func(t *testing.T) { testExcludeContainerFilter(t, factory) })
	t.Run("DifferentItemAtSameSlotSupersedes", func(t *testing.T) { testDifferentItemAtSameSlotSupersedes(t, factory) })
	t.Run("ForgetContainerSlotRemovesSighting", func(t *testing.T) { testForgetContainerSlotRemovesSighting(t, factory) })
	t.Run("ForgetSightingRemovesLooseSighting", func(t *testing.T) { testForgetSightingRemovesLooseSighting(t, factory) })
	t.Run("SightingAtPositionFindsLooseSighting", func(t *testing.T) { testSightingAtPositionFindsLooseSighting(t, factory) })
	t.Run("SightingAtPositionIgnoresContainerSightings", func(t *testing.T) { testSightingAtPositionIgnoresContainerSightings(t, factory) })
	t.Run("MaxAgeFilter", func(t *testing.T) { testMaxAgeFilter(t, factory) })
	t.Run("SightingsNearReturnsEverythingInBoxRegardlessOfName", func(t *testing.T) { testSightingsNearReturnsEverythingInBoxRegardlessOfName(t, factory) })
	t.Run("SightingsNearExcludesOutOfBoxOnEachAxis", func(t *testing.T) { testSightingsNearExcludesOutOfBoxOnEachAxis(t, factory) })
	t.Run("SightingsNearHonorsContainerFiltersAndMaxAge", func(t *testing.T) { testSightingsNearHonorsContainerFiltersAndMaxAge(t, factory) })
	t.Run("ReservationIsAddOnly", func(t *testing.T) { testReservationIsAddOnly(t, factory) })
	t.Run("PersistsAcrossReopen", func(t *testing.T) { testPersistsAcrossReopen(t, factory) })
}

func open(t *testing.T, factory memorystore.Factory, dir string) memorystore.Store {
	t.Helper()
	store, err := factory(memorystore.Config{Dir: dir, AgentName: "TestBot"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testRecordAndQuerySighting(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	pos := memorystore.Position{X: 10, Y: 64, Z: -5}
	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:iron_ore",
		Position:        pos,
		ObservedAt:      time.Now(),
	}))

	got, found, err := store.NearestSighting("minecraft:iron_ore", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, pos, got.Position)

	_, found, err = store.NearestSighting("minecraft:diamond_ore", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.False(t, found)
}

func testRefreshesExistingSighting(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())
	pos := memorystore.Position{X: 1, Y: 2, Z: 3}

	first := time.Now().Add(-time.Hour)
	second := time.Now()
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:coal_ore", Position: pos, ObservedAt: first}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:coal_ore", Position: pos, ObservedAt: second}))

	got, found, err := store.NearestSighting("minecraft:coal_ore", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found)
	require.WithinDuration(t, second, got.ObservedAt, time.Second, "re-observing the same spot must refresh, not duplicate, the record")
}

func testNearestPicksClosest(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	far := memorystore.Position{X: 100, Y: 0, Z: 0}
	near := memorystore.Position{X: 1, Y: 0, Z: 0}
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:iron_ore", Position: far, ObservedAt: time.Now()}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:iron_ore", Position: near, ObservedAt: time.Now()}))

	got, found, err := store.NearestSighting("minecraft:iron_ore", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, near, got.Position)
}

func testContainerOnlyFilter(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	loose := memorystore.Position{X: 1, Y: 0, Z: 0}
	inChest := memorystore.Position{X: 2, Y: 0, Z: 0}
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:bread", Position: loose, ObservedAt: time.Now()}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:bread",
		Position:        inChest,
		ObservedAt:      time.Now(),
		Container:       &memorystore.ContainerRef{ContainerPos: memorystore.Position{X: 2, Y: 0, Z: 0}, SlotIndex: 3},
	}))

	got, found, err := store.NearestSighting("minecraft:bread", memorystore.Position{}, memorystore.SightingQuery{ContainerOnly: true})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, inChest, got.Position)
	require.NotNil(t, got.Container)
}

func testExcludeContainerFilter(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	loose := memorystore.Position{X: 1, Y: 0, Z: 0}
	inChest := memorystore.Position{X: 2, Y: 0, Z: 0}
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:iron_ore", Position: loose, ObservedAt: time.Now()}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:iron_ore",
		Position:        inChest,
		ObservedAt:      time.Now(),
		Container:       &memorystore.ContainerRef{ContainerPos: inChest, SlotIndex: 0},
	}))

	// A caller meaning to go mine a block (e.g. refreshMineTarget's
	// memory fallback) can't act on a result sitting inside a container.
	got, found, err := store.NearestSighting("minecraft:iron_ore", memorystore.Position{}, memorystore.SightingQuery{ExcludeContainer: true})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, loose, got.Position)
	require.Nil(t, got.Container)
}

// testDifferentItemAtSameSlotSupersedes guards the exact bug this plan's
// operator flagged live: a chest slot's identity is the slot, not
// whichever item currently occupies it. Recording a second, different
// item at the same (ContainerPos, SlotIndex) must replace the first
// record, never leave both findable at once.
func testDifferentItemAtSameSlotSupersedes(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())
	containerPos := memorystore.Position{X: 5, Y: 5, Z: 5}
	ref := &memorystore.ContainerRef{ContainerPos: containerPos, SlotIndex: 2}

	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:iron_ingot", Position: containerPos, ObservedAt: time.Now(), Container: ref,
	}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:gold_ingot", Position: containerPos, ObservedAt: time.Now(), Container: ref,
	}))

	_, found, err := store.NearestSighting("minecraft:iron_ingot", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.False(t, found, "the stale iron_ingot record must not still be findable once gold_ingot replaced it at the same slot")

	got, found, err := store.NearestSighting("minecraft:gold_ingot", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, containerPos, got.Position)
}

// testForgetContainerSlotRemovesSighting covers the complementary case:
// a slot that became empty (no replacement item to record under) must
// be explicitly forgettable, or the old "item present" record would
// never go away.
func testForgetContainerSlotRemovesSighting(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())
	containerPos := memorystore.Position{X: 9, Y: 9, Z: 9}
	ref := &memorystore.ContainerRef{ContainerPos: containerPos, SlotIndex: 1}

	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:bread", Position: containerPos, ObservedAt: time.Now(), Container: ref,
	}))
	_, found, err := store.NearestSighting("minecraft:bread", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found)

	require.NoError(t, store.ForgetContainerSlot(containerPos, 1))

	_, found, err = store.NearestSighting("minecraft:bread", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.False(t, found, "a forgotten slot must not still be findable")

	// Forgetting an already-empty/never-recorded slot is a no-op, not an
	// error.
	require.NoError(t, store.ForgetContainerSlot(containerPos, 1))
	require.NoError(t, store.ForgetContainerSlot(memorystore.Position{X: 1, Y: 2, Z: 3}, 99))
}

// testForgetSightingRemovesLooseSighting covers the loose-block
// counterpart to testForgetContainerSlotRemovesSighting: the memory this
// plan's operator asked about directly — a block the bot mines must stop
// being findable at that position (agent.recordBrokenBlock's own hook),
// and a different sighting of the same item elsewhere must be
// unaffected.
func testForgetSightingRemovesLooseSighting(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())
	minedPos := memorystore.Position{X: 3, Y: 4, Z: 5}
	elsewherePos := memorystore.Position{X: 30, Y: 40, Z: 50}

	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:iron_ore", Position: minedPos, ObservedAt: time.Now(),
	}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:iron_ore", Position: elsewherePos, ObservedAt: time.Now(),
	}))

	require.NoError(t, store.ForgetSighting("minecraft:iron_ore", minedPos))

	got, found, err := store.NearestSighting("minecraft:iron_ore", minedPos, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found, "the other sighting must still be findable")
	require.Equal(t, elsewherePos, got.Position)

	// Forgetting an already-gone/never-recorded sighting is a no-op, not
	// an error.
	require.NoError(t, store.ForgetSighting("minecraft:iron_ore", minedPos))
	require.NoError(t, store.ForgetSighting("minecraft:nonexistent", memorystore.Position{X: 1, Y: 2, Z: 3}))
}

// testSightingAtPositionFindsLooseSighting covers the position-keyed
// lookup agent.observeBlockChange needs: given only a position (no item
// name), find whatever loose sighting is there, or report nothing is.
func testSightingAtPositionFindsLooseSighting(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())
	pos := memorystore.Position{X: 7, Y: 8, Z: 9}

	_, found, err := store.SightingAtPosition(pos)
	require.NoError(t, err)
	require.False(t, found, "nothing recorded yet")

	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:diamond_ore", Position: pos, ObservedAt: time.Now(),
	}))

	got, found, err := store.SightingAtPosition(pos)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "minecraft:diamond_ore", got.ItemOrBlockName)

	require.NoError(t, store.ForgetSighting("minecraft:diamond_ore", pos))
	_, found, err = store.SightingAtPosition(pos)
	require.NoError(t, err)
	require.False(t, found, "forgotten sighting must not still be findable by position")
}

// testSightingAtPositionIgnoresContainerSightings: a container's own
// sighting at ContainerPos must not be mistaken for a loose sighting at
// that same position — SightingAtPosition is the loose-sighting query
// only (see ForgetSighting's own "two different items can't occupy the
// same world position" reasoning: a container's contents are a distinct
// kind of fact about that position, tracked separately).
func testSightingAtPositionIgnoresContainerSightings(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())
	pos := memorystore.Position{X: 1, Y: 1, Z: 1}

	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:bread", Position: pos, ObservedAt: time.Now(),
		Container: &memorystore.ContainerRef{ContainerPos: pos, SlotIndex: 0},
	}))

	_, found, err := store.SightingAtPosition(pos)
	require.NoError(t, err)
	require.False(t, found, "a container sighting must not answer a loose-sighting position query")
}

func testMaxAgeFilter(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	stale := memorystore.Position{X: 1, Y: 0, Z: 0}
	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:wheat",
		Position:        stale,
		ObservedAt:      time.Now().Add(-48 * time.Hour),
	}))

	_, found, err := store.NearestSighting("minecraft:wheat", memorystore.Position{}, memorystore.SightingQuery{MaxAge: time.Hour})
	require.NoError(t, err)
	require.False(t, found, "a sighting older than MaxAge must be excluded")

	_, found, err = store.NearestSighting("minecraft:wheat", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found, "MaxAge of zero must not filter by age at all")
}

func testSightingsNearReturnsEverythingInBoxRegardlessOfName(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:dirt", Position: memorystore.Position{X: 1, Y: 0, Z: 0}, ObservedAt: time.Now()}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:iron_ore", Position: memorystore.Position{X: -1, Y: 0, Z: 0}, ObservedAt: time.Now()}))

	// Unlike NearestSighting, SightingsNear is not scoped to one item
	// name - a caller filling in a 3D view's blind spots wants
	// "everything remembered here," not "the nearest instance of one
	// specific block."
	got, err := store.SightingsNear(memorystore.Position{}, 5, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.Len(t, got, 2)
}

func testSightingsNearExcludesOutOfBoxOnEachAxis(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	inside := memorystore.Position{X: 3, Y: -3, Z: 3}
	outsideX := memorystore.Position{X: 10, Y: 0, Z: 0}
	outsideY := memorystore.Position{X: 0, Y: 10, Z: 0}
	outsideZ := memorystore.Position{X: 0, Y: 0, Z: -10}
	for _, pos := range []memorystore.Position{inside, outsideX, outsideY, outsideZ} {
		require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:stone", Position: pos, ObservedAt: time.Now()}))
	}

	got, err := store.SightingsNear(memorystore.Position{}, 5, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.Len(t, got, 1, "a box bound must exclude a sighting too far on any single axis, even with the other two at the origin")
	require.Equal(t, inside, got[0].Position)
}

func testSightingsNearHonorsContainerFiltersAndMaxAge(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())

	loose := memorystore.Position{X: 1, Y: 0, Z: 0}
	inChest := memorystore.Position{X: 2, Y: 0, Z: 0}
	stale := memorystore.Position{X: -1, Y: 0, Z: 0}
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:bread", Position: loose, ObservedAt: time.Now()}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:bread", Position: inChest, ObservedAt: time.Now(),
		Container: &memorystore.ContainerRef{ContainerPos: inChest, SlotIndex: 0},
	}))
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:wheat", Position: stale, ObservedAt: time.Now().Add(-48 * time.Hour)}))

	got, err := store.SightingsNear(memorystore.Position{}, 5, memorystore.SightingQuery{ExcludeContainer: true, MaxAge: time.Hour})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, loose, got[0].Position)

	got, err = store.SightingsNear(memorystore.Position{}, 5, memorystore.SightingQuery{ContainerOnly: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, inChest, got[0].Position)
}

func testReservationIsAddOnly(t *testing.T, factory memorystore.Factory) {
	store := open(t, factory, t.TempDir())
	pos := memorystore.Position{X: 5, Y: 6, Z: 7}

	reserved, err := store.IsReserved(pos)
	require.NoError(t, err)
	require.False(t, reserved)

	require.NoError(t, store.RecordReservation(memorystore.Reservation{Position: pos, StructureID: "tower-a", PlacedAt: time.Now()}))
	reserved, err = store.IsReserved(pos)
	require.NoError(t, err)
	require.True(t, reserved)

	// Re-recording (e.g. a repaired block placed again) must not error and
	// must not change the reservation's claimed owner.
	require.NoError(t, store.RecordReservation(memorystore.Reservation{Position: pos, StructureID: "tower-b", PlacedAt: time.Now()}))
	reserved, err = store.IsReserved(pos)
	require.NoError(t, err)
	require.True(t, reserved)
}

func testPersistsAcrossReopen(t *testing.T, factory memorystore.Factory) {
	dir := t.TempDir()
	pos := memorystore.Position{X: 42, Y: 43, Z: 44}

	store := open(t, factory, dir)
	require.NoError(t, store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:gold_ore", Position: pos, ObservedAt: time.Now()}))
	require.NoError(t, store.RecordReservation(memorystore.Reservation{Position: pos, StructureID: "hall", PlacedAt: time.Now()}))
	require.NoError(t, store.Close())

	reopened, err := factory(memorystore.Config{Dir: dir, AgentName: "TestBot"})
	require.NoError(t, err)
	defer reopened.Close()

	got, found, err := reopened.NearestSighting("minecraft:gold_ore", memorystore.Position{}, memorystore.SightingQuery{})
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, pos, got.Position)

	reserved, err := reopened.IsReserved(pos)
	require.NoError(t, err)
	require.True(t, reserved)
}
