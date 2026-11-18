package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	"github.com/reallyoldfogie/mc-agent/memorystore/jsonstore"
	"github.com/reallyoldfogie/mc-agent/models"
)

func newTestWorldMemory(t *testing.T) memorystore.Store {
	t.Helper()
	store, err := jsonstore.New(memorystore.Config{Dir: t.TempDir(), AgentName: "test"})
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// TestIsReservedPosition_ReflectsRecordedReservations exercises the Part 4
// enforcement path end to end: a position RecordReservation claims must
// then report reserved via isReservedPosition (the check
// FindVisibleBlock now runs per
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md), while an unrelated position
// must not.
func TestIsReservedPosition_ReflectsRecordedReservations(t *testing.T) {
	a := &agent{worldMemory: newTestWorldMemory(t)}

	if a.isReservedPosition(1, 2, 3) {
		t.Fatalf("isReservedPosition = true before any reservation, want false")
	}

	a.RecordReservation(models.V3{X: 1, Y: 2, Z: 3}, "tower-a")
	a.wg.Wait() // RecordReservation writes in its own goroutine

	if !a.isReservedPosition(1, 2, 3) {
		t.Fatalf("isReservedPosition = false after RecordReservation, want true")
	}
	if a.isReservedPosition(9, 9, 9) {
		t.Fatalf("isReservedPosition = true for an unrelated position, want false")
	}
}

func TestIsReservedPosition_NilStoreFailsOpen(t *testing.T) {
	a := &agent{}
	if a.isReservedPosition(1, 2, 3) {
		t.Fatalf("isReservedPosition = true with no world-memory store, want false (fail open)")
	}
}

// TestRecordBrokenBlock_ForgetsTheMinedSighting exercises the fix for the
// gap the operator asked about directly, 2026-11-17: a block this agent
// mines must stop being findable at the old position in its own
// world-memory store, the loose-sighting counterpart to the container
// "slot became empty" fix.
func TestRecordBrokenBlock_ForgetsTheMinedSighting(t *testing.T) {
	store := newTestWorldMemory(t)
	a := &agent{worldMemory: store}

	if err := store.RecordSighting(memorystore.Sighting{
		ItemOrBlockName: "minecraft:iron_ore",
		Position:        memorystore.Position{X: 3, Y: 4, Z: 5},
	}); err != nil {
		t.Fatalf("RecordSighting: %v", err)
	}

	a.recordBrokenBlock(3, 4, 5, "minecraft:iron_ore")
	a.wg.Wait() // forgetBlockSighting writes in its own goroutine

	_, found, err := store.NearestSighting("minecraft:iron_ore", memorystore.Position{}, memorystore.SightingQuery{})
	if err != nil {
		t.Fatalf("NearestSighting: %v", err)
	}
	if found {
		t.Fatalf("NearestSighting found = true after recordBrokenBlock, want false (mined, so no longer there)")
	}
}

func TestRecordBrokenBlock_NilStoreIsNoOp(t *testing.T) {
	a := &agent{}
	a.recordBrokenBlock(3, 4, 5, "minecraft:iron_ore") // must not panic with no worldMemory
	a.wg.Wait()
}
