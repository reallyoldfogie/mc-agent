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
	t.Run("MaxAgeFilter", func(t *testing.T) { testMaxAgeFilter(t, factory) })
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
