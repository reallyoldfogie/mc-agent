package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	"github.com/reallyoldfogie/mc-agent/memorystore/jsonstore"
	"github.com/reallyoldfogie/mc-agent/models"
)

// TestOpenWorldMemoryDefaultsToNone guards the 2026-11-19 default
// change (docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md's "Data store
// options"): a models.AgentConfig that leaves MemoryStoreBackend unset
// must get no store at all - not silently "json" - and openWorldMemory
// must never touch MemoryStoreDir in that case, confirmed here by
// checking the directory stays empty rather than just asserting the
// returned Store is nil.
func TestOpenWorldMemoryDefaultsToNone(t *testing.T) {
	dir := t.TempDir()
	store, err := openWorldMemory(models.AgentConfig{MemoryStoreDir: dir}, "test")
	if err != nil {
		t.Fatalf("openWorldMemory with unset backend: %v", err)
	}
	if store != nil {
		t.Fatalf("openWorldMemory with unset backend = non-nil Store, want nil")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Fatalf("openWorldMemory with the default backend created %v in %s, want nothing", entries, dir)
	}
}

func TestOpenWorldMemoryExplicitNoneReturnsNilStore(t *testing.T) {
	store, err := openWorldMemory(models.AgentConfig{MemoryStoreBackend: "none"}, "test")
	if err != nil {
		t.Fatalf("openWorldMemory with explicit none: %v", err)
	}
	if store != nil {
		t.Fatalf("openWorldMemory with explicit none = non-nil Store, want nil")
	}
}

// TestOpenWorldMemoryJSONBackendStillOptsIn is the regression guard
// alongside the default change above: explicitly requesting "json"
// must still work exactly as before, including actually persisting to
// MemoryStoreDir.
func TestOpenWorldMemoryJSONBackendStillOptsIn(t *testing.T) {
	dir := t.TempDir()
	store, err := openWorldMemory(models.AgentConfig{MemoryStoreBackend: "json", MemoryStoreDir: dir}, "test")
	if err != nil {
		t.Fatalf("openWorldMemory with explicit json: %v", err)
	}
	if store == nil {
		t.Fatalf("openWorldMemory with explicit json = nil Store, want a real one")
	}
	defer store.Close()
	if err := store.RecordReservation(memorystore.Reservation{Position: memorystore.Position{X: 1, Y: 2, Z: 3}, StructureID: "t"}); err != nil {
		t.Fatalf("RecordReservation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "test.json")); err != nil {
		t.Fatalf("expected test.json to exist after a write: %v", err)
	}
}

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

// TestReconcileBlockSighting_ForgetsWhenChanged exercises the operator's
// direct ask, 2026-11-18: a block change this agent merely *observes* (not
// one it caused) must still invalidate a stale sighting at that position.
func TestReconcileBlockSighting_ForgetsWhenChanged(t *testing.T) {
	store := newTestWorldMemory(t)
	a := &agent{worldMemory: store}
	pos := memorystore.Position{X: 1, Y: 2, Z: 3}

	if err := store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:iron_ore", Position: pos}); err != nil {
		t.Fatalf("RecordSighting: %v", err)
	}

	a.reconcileBlockSighting(pos, "minecraft:air") // someone/something else changed it

	_, found, err := store.SightingAtPosition(pos)
	if err != nil {
		t.Fatalf("SightingAtPosition: %v", err)
	}
	if found {
		t.Fatalf("SightingAtPosition found = true after an observed change to something else, want false")
	}
}

// TestReconcileBlockSighting_RefreshesWhenUnchanged confirms a block that
// still matches what's remembered gets its confidence refreshed (a newer
// ObservedAt), not left to decay just because it happened to be
// re-observed via a packet rather than direct vision.
func TestReconcileBlockSighting_RefreshesWhenUnchanged(t *testing.T) {
	store := newTestWorldMemory(t)
	a := &agent{worldMemory: store}
	pos := memorystore.Position{X: 1, Y: 2, Z: 3}
	old := time.Now().Add(-time.Hour)

	if err := store.RecordSighting(memorystore.Sighting{ItemOrBlockName: "minecraft:iron_ore", Position: pos, ObservedAt: old}); err != nil {
		t.Fatalf("RecordSighting: %v", err)
	}

	a.reconcileBlockSighting(pos, "minecraft:iron_ore") // still the same block

	got, found, err := store.SightingAtPosition(pos)
	if err != nil {
		t.Fatalf("SightingAtPosition: %v", err)
	}
	if !found {
		t.Fatalf("SightingAtPosition found = false, want true (still there, just refreshed)")
	}
	if !got.ObservedAt.After(old) {
		t.Fatalf("ObservedAt = %v, want refreshed to after %v", got.ObservedAt, old)
	}
}

// TestReconcileBlockSighting_NoOpWhenNothingRemembered confirms the
// common case (almost every observed block change is at a position this
// store has no opinion about) does nothing, not even a write attempt.
func TestReconcileBlockSighting_NoOpWhenNothingRemembered(t *testing.T) {
	store := newTestWorldMemory(t)
	a := &agent{worldMemory: store}

	a.reconcileBlockSighting(memorystore.Position{X: 9, Y: 9, Z: 9}, "minecraft:dirt")

	_, found, err := store.SightingAtPosition(memorystore.Position{X: 9, Y: 9, Z: 9})
	if err != nil {
		t.Fatalf("SightingAtPosition: %v", err)
	}
	if found {
		t.Fatalf("SightingAtPosition found = true, want false (nothing was ever recorded here)")
	}
}

func TestRememberedBlocksNearReturnsNilWithoutWorldMemory(t *testing.T) {
	a := &agent{}
	blocks, err := a.RememberedBlocksNear(16)
	if err != nil {
		t.Fatalf("RememberedBlocksNear without a store: %v", err)
	}
	if blocks != nil {
		t.Fatalf("RememberedBlocksNear without a store = %v, want nil", blocks)
	}
}

// TestRecordManualSighting_IsFindableAfterward confirms the seeding side
// of rlenv.MemoryEditAgent writes a sighting that subsequent lookups (the
// same path a genuine perception event would have taken) actually find.
func TestRecordManualSighting_IsFindableAfterward(t *testing.T) {
	store := newTestWorldMemory(t)
	a := &agent{worldMemory: store}

	pos := models.V3{X: 12, Y: -5, Z: 34}
	if err := a.RecordManualSighting(pos, "minecraft:coal_ore"); err != nil {
		t.Fatalf("RecordManualSighting: %v", err)
	}

	got, found, err := store.SightingAtPosition(memorystore.Position{X: 12, Y: -5, Z: 34})
	if err != nil {
		t.Fatalf("SightingAtPosition: %v", err)
	}
	if !found {
		t.Fatalf("SightingAtPosition after RecordManualSighting found = false, want true")
	}
	if got.ItemOrBlockName != "minecraft:coal_ore" {
		t.Fatalf("SightingAtPosition ItemOrBlockName = %q, want minecraft:coal_ore", got.ItemOrBlockName)
	}
}

func TestRecordManualSighting_NoOpWithoutWorldMemory(t *testing.T) {
	a := &agent{}
	if err := a.RecordManualSighting(models.V3{X: 1, Y: 2, Z: 3}, "minecraft:coal_ore"); err != nil {
		t.Fatalf("RecordManualSighting without a store: %v", err)
	}
}

// TestForgetSightingsNear_RemovesLooseAndContainerSightingsInBoxOnly
// confirms the clearing side of rlenv.MemoryEditAgent: both a loose and a
// container sighting inside the box are gone afterward, while one just
// outside it on a single axis survives (SightingsNear's own box-not-
// sphere shape - see memorystore.Store.SightingsNear's doc comment).
func TestForgetSightingsNear_RemovesLooseAndContainerSightingsInBoxOnly(t *testing.T) {
	store := newTestWorldMemory(t)
	a := &agent{worldMemory: store}

	loose := memorystore.Position{X: 5, Y: 0, Z: 5}
	containerPos := memorystore.Position{X: -5, Y: 0, Z: -5}
	outside := memorystore.Position{X: 20, Y: 0, Z: 0}

	for _, s := range []memorystore.Sighting{
		{ItemOrBlockName: "minecraft:coal_ore", Position: loose, ObservedAt: time.Now()},
		{ItemOrBlockName: "minecraft:iron_ingot", Position: containerPos, ObservedAt: time.Now(), Container: &memorystore.ContainerRef{ContainerPos: containerPos, SlotIndex: 2}},
		{ItemOrBlockName: "minecraft:coal_ore", Position: outside, ObservedAt: time.Now()},
	} {
		if err := store.RecordSighting(s); err != nil {
			t.Fatalf("RecordSighting(%v): %v", s, err)
		}
	}

	if err := a.ForgetSightingsNear(models.V3{X: 0, Y: 0, Z: 0}, 10); err != nil {
		t.Fatalf("ForgetSightingsNear: %v", err)
	}

	if _, found, _ := store.SightingAtPosition(loose); found {
		t.Fatalf("loose sighting at %v still present after ForgetSightingsNear", loose)
	}
	if _, found, _ := store.SightingAtPosition(containerPos); found {
		t.Fatalf("container sighting at %v still present after ForgetSightingsNear", containerPos)
	}
	if _, found, _ := store.SightingAtPosition(outside); !found {
		t.Fatalf("out-of-box sighting at %v was removed, want it untouched", outside)
	}
}

func TestForgetSightingsNear_NoOpWithoutWorldMemory(t *testing.T) {
	a := &agent{}
	if err := a.ForgetSightingsNear(models.V3{X: 0, Y: 0, Z: 0}, 10); err != nil {
		t.Fatalf("ForgetSightingsNear without a store: %v", err)
	}
}

func TestRememberedBlocksNearErrorsWithoutAPosition(t *testing.T) {
	a := &agent{worldMemory: &fakeMemoryStore{}}
	if _, err := a.RememberedBlocksNear(16); err == nil {
		t.Fatalf("RememberedBlocksNear with no position yet = nil error, want one")
	}
}

func TestRememberedBlocksNearQueriesExcludingContainersAtItsOwnPosition(t *testing.T) {
	store := &fakeMemoryStore{nearResult: []memorystore.Sighting{
		{ItemOrBlockName: "minecraft:stone", Position: memorystore.Position{X: 1, Y: 2, Z: 3}},
	}}
	a := &agent{worldMemory: store, posX: 10, posY: 20, posZ: 30, posInitialized: true}

	blocks, err := a.RememberedBlocksNear(16)
	if err != nil {
		t.Fatalf("RememberedBlocksNear: %v", err)
	}
	if len(blocks) != 1 || blocks[0].BlockName != "minecraft:stone" || blocks[0].X != 1 || blocks[0].Y != 2 || blocks[0].Z != 3 {
		t.Fatalf("RememberedBlocksNear = %+v, want one translated minecraft:stone block", blocks)
	}

	if len(store.nearCalls) != 1 {
		t.Fatalf("SightingsNear called %d times, want 1", len(store.nearCalls))
	}
	call := store.nearCalls[0]
	if call.from != (memorystore.Position{X: 10, Y: 20, Z: 30}) {
		t.Fatalf("SightingsNear queried from %v, want the agent's own current position", call.from)
	}
	if call.radius != 16 {
		t.Fatalf("SightingsNear radius = %d, want 16", call.radius)
	}
	if !call.query.ExcludeContainer {
		t.Fatalf("SightingsNear query = %+v, want ExcludeContainer - a chest's remembered contents aren't a block to render at the chest's position", call.query)
	}
}
