package rlenv_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// walkGroundStateID mirrors models/interact_position_test.go's own
// grassStateID convention: an arbitrary non-zero, solid block state ID a
// mctesting.MockShapeManager treats as solid ground.
const walkGroundStateID = 9

// fakeWalkabilityAgent wraps *fakeAgent (this package's existing live-agent
// fake — position, movement, health, mine/craft simulation) with a
// mctesting.MockWorld/MockShapeManager pair, so it satisfies both
// rlenv.LiveAgent (via the embedded *fakeAgent) and rlenv.WalkabilityAgent
// (via the two methods below) — exercising Environment.Reset's real
// ground-snap/fallback logic (rlenv/walkability.go) end to end rather than
// calling its unexported helpers directly. mc-agent/testing can't be
// imported from rlenv's own internal test package (it transitively imports
// rlenv itself via mc-agent/config, an import cycle) — see
// rlenv/walkability_test.go's own comment — so this lives here instead,
// in the external rlenv_test package, which has no such problem.
type fakeWalkabilityAgent struct {
	*fakeAgent
	world    models.World
	shapeMgr models.BlockShapeManager
}

func (f fakeWalkabilityAgent) GetWorld() models.World                      { return f.world }
func (f fakeWalkabilityAgent) BlockShapeManager() models.BlockShapeManager { return f.shapeMgr }

func newWalkabilityTestEnvironment(t *testing.T, agent fakeWalkabilityAgent, cfg rlenv.Config) *rlenv.Environment {
	t.Helper()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	return env
}

func TestResetGroundSnapsGotoTargetToRaisedTerrain(t *testing.T) {
	// Bot starts at (0,0,0) standing on flat ground (surface at y=-1).
	// TargetOffset (5,0,0) naively poses (5,0,0), but the real terrain at
	// that column has a 3-block-higher surface (y=2) — the exact bug
	// docs/plans/08-parallel-environments-and-scaling.md's own "Status"
	// traced live: a fixed offset with no terrain awareness landing
	// somewhere the pathfinder can never reach.
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-5, -5, 10, 5, -1, walkGroundStateID).
		SetBlockDirect(5, -1, 0, 0). // clear the flat ground's own surface block in this one column — the raised block below replaces it, not stacks with it.
		SetBlockDirect(5, 2, 0, walkGroundStateID).
		Build()
	agent := fakeWalkabilityAgent{fakeAgent: newFakeAgent(0, 0, 0), world: world, shapeMgr: mctesting.NewMockShapeManager()}

	env := newWalkabilityTestEnvironment(t, agent, rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 0.5,
		StepTimeout:      200 * time.Millisecond,
	})

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	// observation.go: index 0 = dx, index 1 = dy = targetY - current Y.
	// The naive (unsnapped) target would have dy=0; ground-snapping onto
	// the raised surface (standing at y=3 on top of the block at y=2)
	// means dy should read 3, not 0.
	if dx := obs.Values[0]; dx != 5 {
		t.Fatalf("dx = %v, want 5 (X is untouched by ground-snapping in this scenario)", dx)
	}
	if dy := obs.Values[1]; dy != 3 {
		t.Fatalf("dy = %v, want 3 (ground-snapped onto the raised surface at Y=2)", dy)
	}
}

func TestResetErrorsWhenGotoTargetHasNoWalkableCellAnywhereNearby(t *testing.T) {
	// Solid rock fills every column within both search radii around the
	// target — Reset should fail loudly rather than silently pose an
	// unreachable target (the bug this whole fix closes: previously this
	// situation produced no error at all, just an episode where every Step
	// failed identically with zero learning signal).
	registry := mctesting.NewSimpleBlockRegistry()
	wb := mctesting.NewWorldBuilder(registry).FlatGroundDirect(-5, -5, 0, 5, -1, walkGroundStateID)
	const margin = 2
	for dx := -margin; dx <= margin+4; dx++ {
		for dz := -margin - 4; dz <= margin+4; dz++ {
			wb = wb.WallDirect(5+float64(dx), float64(dz), 5+float64(dx), float64(dz), -20, 20, walkGroundStateID)
		}
	}
	world := wb.Build()
	agent := fakeWalkabilityAgent{fakeAgent: newFakeAgent(0, 0, 0), world: world, shapeMgr: mctesting.NewMockShapeManager()}

	env := newWalkabilityTestEnvironment(t, agent, rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 0.5,
		StepTimeout:      200 * time.Millisecond,
	})

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatal("Reset with no walkable cell anywhere near the target: want error, got nil")
	}
}

func TestResetSkipsWalkabilityCheckWhenGoToTargetDisabled(t *testing.T) {
	// Same fully-blocked target as the error case above, but
	// GoToTargetDisabled means the goto task (and therefore its target)
	// isn't active this episode — Reset must succeed, not spuriously fail
	// a mine/craft-only episode over an irrelevant, unreachable
	// TargetOffset.
	registry := mctesting.NewSimpleBlockRegistry()
	wb := mctesting.NewWorldBuilder(registry).FlatGroundDirect(-5, -5, 0, 5, -1, walkGroundStateID)
	const margin = 2
	for dx := -margin; dx <= margin+4; dx++ {
		for dz := -margin - 4; dz <= margin+4; dz++ {
			wb = wb.WallDirect(5+float64(dx), float64(dz), 5+float64(dx), float64(dz), -20, 20, walkGroundStateID)
		}
	}
	world := wb.Build()
	agent := fakeWalkabilityAgent{fakeAgent: newFakeAgent(0, 0, 0), world: world, shapeMgr: mctesting.NewMockShapeManager()}

	env := newWalkabilityTestEnvironment(t, agent, rlenv.Config{
		TargetOffset:       [3]float64{5, 0, 0},
		GoToTargetDisabled: true,
		MineTargetBlock:    "minecraft:stone",
		ArrivalThreshold:   0.5,
		StepTimeout:        200 * time.Millisecond,
	})

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset with GoToTargetDisabled: want no error (walkability check skipped), got %v", err)
	}
}

func TestResetErrorsWhenGotoTargetIsStandableButUnreachable(t *testing.T) {
	// The exact target (5,0,0) is standable (flat ground below, air
	// above) but the fake pathfinder reports every candidate unreachable
	// — simulating live-confirmed reality: a cell can pass the local
	// standability check while still being disconnected from the origin
	// by something a 3-cell check can't see (water, a wall, a gap). See
	// docs/plans/08-parallel-environments-and-scaling.md's own "Status"
	// for the live run this reproduces (266 "no valid path exists"
	// pathfinder failures despite every ground-snapped target passing
	// standability).
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).FlatGroundDirect(-5, -5, 10, 5, -1, walkGroundStateID).Build()
	fake := newFakeAgent(0, 0, 0)
	fake.findPathUnreachable = func(float64, float64, float64) bool { return true }
	agent := fakeWalkabilityAgent{fakeAgent: fake, world: world, shapeMgr: mctesting.NewMockShapeManager()}

	env := newWalkabilityTestEnvironment(t, agent, rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 0.5,
		StepTimeout:      200 * time.Millisecond,
	})

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatal("Reset with a standable-but-unreachable target and no reachable fallback: want error, got nil")
	}
	if len(fake.findPathCalls) == 0 {
		t.Fatal("expected Reset to actually query FindPath for at least one candidate")
	}
}

func TestResetFallsBackToAReachableNeighborWhenTheExactTargetIsUnreachable(t *testing.T) {
	// The exact target column (5,0,0) is standable but the fake
	// pathfinder reports it specifically unreachable; a nearby column
	// (6,0,0), also standable, is reachable. Reset should fall through
	// to it via the same ring search ground-snapping already used for
	// standability failures — reachability failures use that same
	// fallback path, not a separate one.
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).FlatGroundDirect(-5, -5, 10, 5, -1, walkGroundStateID).Build()
	fake := newFakeAgent(0, 0, 0)
	fake.findPathUnreachable = func(x, _, z float64) bool { return x == 5 && z == 0 }
	agent := fakeWalkabilityAgent{fakeAgent: fake, world: world, shapeMgr: mctesting.NewMockShapeManager()}

	env := newWalkabilityTestEnvironment(t, agent, rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 0.5,
		StepTimeout:      200 * time.Millisecond,
	})

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	// observation.go: index 0 = dx = targetX - current X. The exact
	// target (X offset 5) was rejected as unreachable, so dx must reflect
	// a fallback column, not the original 5.
	if dx := obs.Values[0]; dx == 5 {
		t.Fatalf("dx = %v, want a fallback column's offset, not the rejected exact target's (5)", dx)
	}
}

func TestResetRetriesWithFreshJitterWhenGotoTargetIsUnreachable(t *testing.T) {
	// Wide-open flat ground everywhere any candidate this test's search
	// radii could possibly touch, so every standability check trivially
	// succeeds — isolating the outer retry-with-rejitter loop (the thing
	// under test) from groundSnap's own success/failure, which is a
	// separate concern already covered by the fallback test above.
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).FlatGroundDirect(-50, -50, 50, 50, -1, walkGroundStateID).Build()
	fake := newFakeAgent(0, 0, 0)

	// Block exactly the number of FindPath calls one fully-exhausted
	// findWalkableTarget attempt makes against this flat world (the exact
	// column, plus every ringOffsets(horizontalSearchRadius) neighbor —
	// (2*horizontalSearchRadius+1)^2-1 of them, all of which pass
	// standability here since the world is flat everywhere nearby), then
	// allow every call after that. Deterministically forces Reset's very
	// first attempt to exhaust its entire local search and fail, so the
	// next (freshly rejittered) attempt's first call is the one that
	// succeeds — proving Reset actually retries with a new draw when
	// Config.Jitter is set, rather than giving up the moment one draw's
	// search space is exhausted (the resilience gap confirmed live:
	// docs/plans/08-parallel-environments-and-scaling.md's own "Status").
	const oneAttemptsWorthOfCalls = 1 + 80 // 1 exact column + ringOffsets(4)'s 80 neighbors.
	var calls int
	fake.findPathUnreachable = func(float64, float64, float64) bool {
		calls++
		return calls <= oneAttemptsWorthOfCalls
	}
	agent := fakeWalkabilityAgent{fakeAgent: fake, world: world, shapeMgr: mctesting.NewMockShapeManager()}

	env := newWalkabilityTestEnvironment(t, agent, rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 0.5,
		StepTimeout:      200 * time.Millisecond,
		Jitter:           [3]float64{2, 0, 2},
		JitterSeed:       1,
	})

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: want the retry to eventually succeed against a freshly rejittered draw, got error: %v", err)
	}
	if calls != oneAttemptsWorthOfCalls+1 {
		t.Fatalf("FindPath was called %d times, want exactly %d (one fully-exhausted attempt, then one more successful call on the retry)", calls, oneAttemptsWorthOfCalls+1)
	}
}

func TestResetErrorsAfterExhaustingAllJitterRetriesOnUnreachableTargets(t *testing.T) {
	// Same shape as the retry-succeeds test above, but every single
	// candidate at every retry is unreachable — Reset must still fail
	// loudly once maxJitterRetries is exhausted, not retry forever.
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).FlatGroundDirect(-50, -50, 50, 50, -1, walkGroundStateID).Build()
	fake := newFakeAgent(0, 0, 0)
	fake.findPathUnreachable = func(float64, float64, float64) bool { return true }
	agent := fakeWalkabilityAgent{fakeAgent: fake, world: world, shapeMgr: mctesting.NewMockShapeManager()}

	env := newWalkabilityTestEnvironment(t, agent, rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 0.5,
		StepTimeout:      200 * time.Millisecond,
		Jitter:           [3]float64{2, 0, 2},
		JitterSeed:       1,
	})

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatal("Reset with every candidate on every retry unreachable: want error, got nil")
	}
}
