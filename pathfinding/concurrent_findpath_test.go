package pathfinding_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// countingWorld wraps a models.World and atomically counts GetBlockAt
// calls - used to detect cross-call block-cache interference (see
// TestConcurrentFindPathCallsDoNotThrashEachOthersCache below).
type countingWorld struct {
	models.World
	getBlockAtCalls atomic.Int64
}

func (cw *countingWorld) GetBlockAt(x, y, z float64) (uint32, bool) {
	cw.getBlockAtCalls.Add(1)
	return cw.World.GetBlockAt(x, y, z)
}

// TestConcurrentFindPathCallsDoNotThrashEachOthersCache is a regression
// test for a live-confirmed bug: aStarPathFinder (and bidir/EPEA*)
// previously held one shared *MovementValidator per pathfinder instance,
// whose blockCache was wiped by ResetBlockCache() at the top of every
// FindPath call. A single agent's pathfinder is invoked concurrently
// from several independent callers in practice (real-movement dispatch,
// rlenv.Reset's own reachable() sanity checks, and
// PhysicsMovementExecutor's stuck-recovery callback), so two overlapping
// FindPath calls on the same instance would repeatedly invalidate each
// other's memoization - not a data race (the cache was mutex-protected),
// but a severe, confirmed-live performance regression: cheap
// tens-of-milliseconds searches degraded into calls that blew past a
// 3-second budget repeatedly on flat, otherwise-trivial terrain.
//
// FindPath now constructs a fresh, private MovementValidator per call, so
// concurrent calls on the same pathfinder instance can no longer
// interfere with each other's caching at all. This test proves that
// property directly and deterministically: the total number of raw
// GetBlockAt calls made by N concurrent, identical FindPath calls must
// equal exactly N times what one call alone needs - any cross-call cache
// invalidation would inflate that count.
func TestConcurrentFindPathCallsDoNotThrashEachOthersCache(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	baseWorld := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 40, 0, 64, 9). // long flat strip, grass (state 9)
		Build()
	shapeMgr := mctesting.NewMockShapeManager()

	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 30, Y: 65, Z: 0}

	// Solo baseline: one call's own raw block-query cost.
	soloWorld := &countingWorld{World: baseWorld}
	soloFinder := pathfinding.NewAStarPathFinder(soloWorld, shapeMgr, nil)
	path, err := soloFinder.FindPath(context.Background(), start, goal, 5000)
	if err != nil {
		t.Fatalf("solo FindPath failed: %v", err)
	}
	if path == nil || !path.Found {
		t.Fatal("solo FindPath found no path")
	}
	soloCalls := soloWorld.getBlockAtCalls.Load()
	if soloCalls == 0 {
		t.Fatal("expected solo FindPath to make at least one GetBlockAt call")
	}

	// Concurrent runs: many goroutines calling FindPath on the SAME
	// pathfinder instance at once, with the same start/goal so their
	// searches genuinely overlap in the positions they query.
	const concurrency = 8
	concurrentWorld := &countingWorld{World: baseWorld}
	sharedFinder := pathfinding.NewAStarPathFinder(concurrentWorld, shapeMgr, nil)

	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	founds := make([]bool, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := sharedFinder.FindPath(context.Background(), start, goal, 5000)
			errs[i] = err
			founds[i] = p != nil && p.Found
		}(i)
	}
	wg.Wait()

	for i := 0; i < concurrency; i++ {
		if errs[i] != nil {
			t.Fatalf("concurrent FindPath %d failed: %v", i, errs[i])
		}
		if !founds[i] {
			t.Fatalf("concurrent FindPath %d found no path", i)
		}
	}

	wantCalls := soloCalls * int64(concurrency)
	gotCalls := concurrentWorld.getBlockAtCalls.Load()
	if gotCalls != wantCalls {
		t.Errorf("concurrent GetBlockAt calls = %d, want exactly %d (solo=%d x concurrency=%d) - "+
			"a mismatch means concurrent FindPath calls are still interfering with each other's block cache",
			gotCalls, wantCalls, soloCalls, concurrency)
	}
}
