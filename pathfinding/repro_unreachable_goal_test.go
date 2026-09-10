package pathfinding_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestFindPath_FailsFastOnSolidGoal is the direct regression test for
// docs/bugs/hpa-star-slowness's root cause: MoveTo (and therefore
// FindPath) was being asked to reach a solid block's own coordinates,
// which can never satisfy the goal radius. Before the fix, this exhausted
// the entire maxSteps budget (20000 steps took ~100s against a real
// server) before reporting failure. It should now fail in well under a
// second with a clear "not walkable" error, regardless of how large
// maxSteps is.
func TestFindPath_FailsFastOnSolidGoal(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 20, 20, -1, 9).
		SetBlockDirect(4, 0, 0, 9). // solid "crafting table" stand-in
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
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
