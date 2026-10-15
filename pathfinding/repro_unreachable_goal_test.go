package pathfinding_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestFindPath_FailsFastOnSolidGoal is the direct regression test for
// docs/bugs/hpa-star-slowness's root cause: MoveTo (and therefore
// FindPath) was being asked to reach a solid block's own coordinates,
// which can never satisfy the goal radius. Before that fix, this exhausted
// the entire maxSteps budget (20000 steps took ~100s against a real
// server) before reporting failure. It should still fail in well under a
// second with a clear "not walkable" error, regardless of how large
// maxSteps is - but only for a goal that's genuinely unreachable, not
// merely unexcavated. docs/plans/14's narrow-shaft-navigability fix added
// MineThrough/MineDown/MineDownDiagonal and taught goalUnreachable about
// them, so an ordinary diggable obstruction (the original crafting-table
// stand-in this test used) is now correctly reachable via mining - this
// test's own fixture was updated to an explicitly undiggable block
// (bedrock-like) so it keeps testing the fail-fast invariant itself, not
// an assumption that's since become false.
func TestFindPath_FailsFastOnSolidGoal(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 20, 20, -1, 9).
		SetBlockDirect(4, 0, 0, 9). // solid, undiggable obstruction (bedrock-like)
		Build()

	shapeMgr := &undiggableOverride{MockShapeManager: mctesting.NewMockShapeManager(), undiggableStateID: 9}
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 0, Y: 0, Z: 0}
	goal := models.V3{X: 4, Y: 0, Z: 0} // the solid block itself

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()
	path, err := pathFinder.FindPath(ctx, start, goal, 20000)
	elapsed := time.Since(startTime)

	t.Logf("FindPath took %s, err=%v", elapsed, err)
	if err == nil {
		t.Fatalf("expected an error for a solid, unreachable goal, got success: %+v", path)
	}
	if elapsed > time.Second {
		t.Fatalf("expected fail-fast (<1s), took %s - the upfront reachability check may not be firing", elapsed)
	}
}

// TestFindPath_ReachesADiggableSolidGoalByMining is
// TestFindPath_FailsFastOnSolidGoal's complement: a solid goal that *is*
// diggable (an ordinary block, not bedrock-like) must succeed, not fail
// fast - this is the actual behavior change docs/plans/14's narrow-shaft-
// navigability fix made, and the reason the sibling test above had to
// switch to an explicitly undiggable fixture.
func TestFindPath_ReachesADiggableSolidGoalByMining(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 20, 20, -1, 9).
		SetBlockDirect(4, 0, 0, 9). // solid, ordinary (diggable) obstruction
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 0, Y: 0, Z: 0}
	goal := models.V3{X: 4, Y: 0, Z: 0}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	path, err := pathFinder.FindPath(ctx, start, goal, 20000)
	require.NoError(t, err, "a diggable solid goal should be reachable by mining, not rejected as unreachable")
	require.True(t, path.Found)
	last := path.Steps[len(path.Steps)-1]
	require.Equal(t, pathfinding.MineThrough, last.Movement, "the final step into a solid diggable goal should be a mining move, got %v", last)
}

// TestFindPath_FailsFastOnSolidGoal_EPEAStar mirrors
// TestFindPath_FailsFastOnSolidGoal but exercises EPEA*, which shares the
// same upfront goalUnreachable check as plain A*.
func TestFindPath_FailsFastOnSolidGoal_EPEAStar(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 20, 20, -1, 9).
		SetBlockDirect(4, 0, 0, 9). // solid, undiggable obstruction (bedrock-like) - see TestFindPath_FailsFastOnSolidGoal's own doc comment
		Build()

	shapeMgr := &undiggableOverride{MockShapeManager: mctesting.NewMockShapeManager(), undiggableStateID: 9}
	pathFinder := pathfinding.NewEPEAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 0, Y: 0, Z: 0}
	goal := models.V3{X: 4, Y: 0, Z: 0} // the solid block itself

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()
	path, err := pathFinder.FindPath(ctx, start, goal, 20000)
	elapsed := time.Since(startTime)

	t.Logf("FindPath took %s, err=%v", elapsed, err)
	require.Errorf(t, err, "expected an error for a solid, unreachable goal, got success: %+v", path)
	require.Lessf(t, elapsed, time.Second, "expected fail-fast (<1s), took %s - the upfront reachability check may not be firing", elapsed)
}

// TestFindPath_FailsFastOnSolidGoal_BidirAStar mirrors
// TestFindPath_FailsFastOnSolidGoal but exercises bidirectional A*, which
// shares the same upfront goalUnreachable check as plain A*.
func TestFindPath_FailsFastOnSolidGoal_BidirAStar(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 20, 20, -1, 9).
		SetBlockDirect(4, 0, 0, 9). // solid, undiggable obstruction (bedrock-like) - see TestFindPath_FailsFastOnSolidGoal's own doc comment
		Build()

	shapeMgr := &undiggableOverride{MockShapeManager: mctesting.NewMockShapeManager(), undiggableStateID: 9}
	pathFinder := pathfinding.NewBidirAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 0, Y: 0, Z: 0}
	goal := models.V3{X: 4, Y: 0, Z: 0} // the solid block itself

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()
	path, err := pathFinder.FindPath(ctx, start, goal, 20000)
	elapsed := time.Since(startTime)

	t.Logf("FindPath took %s, err=%v", elapsed, err)
	require.Errorf(t, err, "expected an error for a solid, unreachable goal, got success: %+v", path)
	require.Lessf(t, elapsed, time.Second, "expected fail-fast (<1s), took %s - the upfront reachability check may not be firing", elapsed)
}
