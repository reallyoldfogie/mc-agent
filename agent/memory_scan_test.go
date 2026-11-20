package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	"github.com/reallyoldfogie/mc-agent/models"
)

// fakeMemoryStore is a minimal memorystore.Store that only tracks
// RecordSighting calls, for asserting consumeMemoryScanSightings' dedup
// behavior without needing a real backend.
type fakeMemoryStore struct {
	mu         sync.Mutex
	sightings  []memorystore.Sighting
	nearResult []memorystore.Sighting
	nearCalls  []nearCall
}

func (f *fakeMemoryStore) RecordSighting(s memorystore.Sighting) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sightings = append(f.sightings, s)
	return nil
}
func (f *fakeMemoryStore) recordCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sightings)
}
func (f *fakeMemoryStore) NearestSighting(string, memorystore.Position, memorystore.SightingQuery) (memorystore.Sighting, bool, error) {
	return memorystore.Sighting{}, false, nil
}
func (f *fakeMemoryStore) SightingsNear(from memorystore.Position, radius int, query memorystore.SightingQuery) ([]memorystore.Sighting, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nearCalls = append(f.nearCalls, nearCall{from: from, radius: radius, query: query})
	return f.nearResult, nil
}

type nearCall struct {
	from   memorystore.Position
	radius int
	query  memorystore.SightingQuery
}

func (f *fakeMemoryStore) SightingAtPosition(memorystore.Position) (memorystore.Sighting, bool, error) {
	return memorystore.Sighting{}, false, nil
}
func (f *fakeMemoryStore) RecordReservation(memorystore.Reservation) error     { return nil }
func (f *fakeMemoryStore) IsReserved(memorystore.Position) (bool, error)       { return false, nil }
func (f *fakeMemoryStore) ForgetContainerSlot(memorystore.Position, int) error { return nil }
func (f *fakeMemoryStore) ForgetSighting(string, memorystore.Position) error   { return nil }
func (f *fakeMemoryStore) Close() error                                        { return nil }

// drainSightings runs consumeMemoryScanSightings against a.memoryScanSightings
// until it's empty, then cancels ctx and waits for the goroutine to exit —
// giving a test a deterministic point to assert the fake store's state
// without racing the consumer goroutine.
func drainSightings(t *testing.T, a *agent, blocks []models.VisibleBlockInfo) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.consumeMemoryScanSightings(ctx)
		close(done)
	}()
	for _, b := range blocks {
		a.memoryScanSightings <- b
	}
	// Let the consumer drain everything already sent before stopping it -
	// recordVisibleBlockSightings' own non-blocking send means a test
	// pushing straight onto the channel (bypassing that send) never drops
	// anything here, but the consumer is still a separate goroutine.
	deadline := time.After(2 * time.Second)
	for len(a.memoryScanSightings) > 0 {
		select {
		case <-deadline:
			t.Fatalf("consumer did not drain channel in time")
		case <-time.After(time.Millisecond):
		}
	}
	// One more tick to let the last dequeue's RecordSighting complete.
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("consumeMemoryScanSightings did not exit after cancel")
	}
}

func TestConsumeMemoryScanSightingsFiltersAirBlocks(t *testing.T) {
	store := &fakeMemoryStore{}
	a := &agent{worldMemory: store, memoryScanSightings: make(chan models.VisibleBlockInfo, 8)}

	drainSightings(t, a, []models.VisibleBlockInfo{
		{X: 1, Y: 2, Z: 3, BlockName: "minecraft:air"},
		{X: 4, Y: 5, Z: 6, BlockName: "minecraft:cave_air"},
	})

	if got := store.recordCount(); got != 0 {
		t.Fatalf("air blocks must never be recorded, got %d RecordSighting calls", got)
	}
}

func TestConsumeMemoryScanSightingsDedupsUnchangedRepeats(t *testing.T) {
	store := &fakeMemoryStore{}
	a := &agent{worldMemory: store, memoryScanSightings: make(chan models.VisibleBlockInfo, 8)}

	block := models.VisibleBlockInfo{X: 10, Y: 20, Z: 30, BlockName: "minecraft:stone"}
	// The same (position, name) seen three times in a row (three
	// consecutive scans all still finding the same block) must only be
	// written through once.
	drainSightings(t, a, []models.VisibleBlockInfo{block, block, block})

	if got := store.recordCount(); got != 1 {
		t.Fatalf("an unchanged (position, name) repeated 3 times = %d RecordSighting calls, want 1", got)
	}
}

func TestConsumeMemoryScanSightingsWritesThroughOnChange(t *testing.T) {
	store := &fakeMemoryStore{}
	a := &agent{worldMemory: store, memoryScanSightings: make(chan models.VisibleBlockInfo, 8)}

	pos := models.VisibleBlockInfo{X: 1, Y: 1, Z: 1}
	stone := pos
	stone.BlockName = "minecraft:stone"
	ore := pos
	ore.BlockName = "minecraft:iron_ore"

	// Same position, different occupant (e.g. the bot mined the stone
	// and something else is there now) must be recorded as a second,
	// distinct sighting rather than deduped away.
	drainSightings(t, a, []models.VisibleBlockInfo{stone, ore})

	if got := store.recordCount(); got != 2 {
		t.Fatalf("a changed occupant at the same position = %d RecordSighting calls, want 2", got)
	}
}

func TestConsumeMemoryScanSightingsStopsOnContextCancel(t *testing.T) {
	store := &fakeMemoryStore{}
	a := &agent{worldMemory: store, memoryScanSightings: make(chan models.VisibleBlockInfo, 8)}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.consumeMemoryScanSightings(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("consumeMemoryScanSightings did not exit promptly on context cancel")
	}
}

func TestStartMemoryScanLoopIsNoOpWithoutWorldMemory(t *testing.T) {
	a := &agent{}
	// Must not panic (e.g. on a.memoryScanSightings being nil) and must
	// not spawn anything that blocks Close's a.wg.Wait() forever.
	a.startMemoryScanLoop(context.Background())
	a.wg.Wait()
}
