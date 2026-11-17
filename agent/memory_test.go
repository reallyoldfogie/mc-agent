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
