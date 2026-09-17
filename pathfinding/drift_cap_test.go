package pathfinding_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
)

// TestGetPossibleMovesEnforcesCumulativeDriftCap is a regression test for
// a live-confirmed bug: GetPossibleMoves' distance-based pruning only
// ever compared a candidate move's distance-to-goal against the
// *currently expanding node's* distance-to-goal (fromDist+DriftCap) -
// never against the search's actual start-goal line. A long chain of
// individually-small missteps (each locally "allowed") could therefore
// drift arbitrarily far off the direct route before that per-step check
// ever caught it. Confirmed live: a real FindPath call chasing a goal
// only ~4 blocks away on the Z axis wandered across a ~40-block Z range
// on flat, unobstructed terrain, burning its entire step/time budget
// without reaching a goal ~16 blocks away in total.
//
// This test constructs a candidate move that the *old* per-step check
// would have allowed (it's only a small step further from goal than
// `from` already is) but that clearly overshoots any reasonable
// straight-line detour budget, and asserts it's now excluded.
func TestGetPossibleMovesEnforcesCumulativeDriftCap(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-10, -30, 40, 30, 64, 9). // wide-open flat grass area
		Build()
	shapeMgr := mctesting.NewMockShapeManager()
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)
	mv.ResetBlockCache()

	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 20, Y: 65, Z: 0}
	// `from` sits right at the edge of the admissible ellipse for
	// StartDist=20/DriftCap=4 (distToStart+distToGoal == 23.32, just
	// under the 24 budget). A move one block further off-axis in Z
	// (to Z=7) would push that sum to 24.42 - just over budget, and
	// therefore excluded by the fix - while the *old*, purely local
	// check (toDist <= fromDist+DriftCap) would have allowed it, since
	// it's only ~0.55 blocks farther from goal than `from` already is.
	from := models.V3{X: 10, Y: 65, Z: 6}

	prune := &pathfinding.MovePruneConfig{
		Start:     start,
		StartDist: start.DistanceTo(goal),
		DriftCap:  4.0,
	}

	moves := mv.GetPossibleMoves(from, goal, prune)
	require.NotEmpty(t, moves, "expected at least one possible move from a wide-open flat position")

	excludedPos := models.V3{X: 10, Y: 65, Z: 7}
	for _, move := range moves {
		require.NotEqualf(t, excludedPos, move.Position,
			"move to %+v should have been excluded by the cumulative drift-cap check (start+goal distance 24.42 > budget 24.0), "+
				"but the old per-step-only check would have allowed it", excludedPos)
	}

	budget := prune.StartDist + prune.DriftCap
	for _, move := range moves {
		total := move.Position.DistanceTo(start) + move.Position.DistanceTo(goal)
		require.LessOrEqualf(t, total, budget+1e-9,
			"move to (%.1f,%.1f,%.1f) has start+goal distance %.2f, exceeding the %.2f budget (StartDist=%.2f + DriftCap=%.2f) - "+
				"the cumulative drift-cap check regressed",
			move.Position.X, move.Position.Y, move.Position.Z, total, budget, prune.StartDist, prune.DriftCap)
	}
}

// TestFindPathDoesNotExhaustBudgetOnWideOpenFlatTerrain is an end-to-end
// regression test for the same bug: on wide-open, obstruction-free flat
// terrain, FindPath must never need anywhere near the thousands of node
// expansions the live bug burned (2000-5300+ steps observed) for what is
// a trivial, direct hop.
func TestFindPathDoesNotExhaustBudgetOnWideOpenFlatTerrain(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-40, -40, 80, 80, 64, 9). // large open flat grass area
		Build()
	shapeMgr := mctesting.NewMockShapeManager()
	pf := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 30, Y: 65, Z: 0}

	path, err := pf.FindPath(context.Background(), start, goal, 500)
	require.NoError(t, err)
	require.NotNil(t, path)
	require.True(t, path.Found)
	require.LessOrEqualf(t, len(path.Steps), 40,
		"expected a near-direct path (roughly %d steps) on wide-open flat terrain, got %d steps",
		int(start.DistanceTo(goal)), len(path.Steps))
}
