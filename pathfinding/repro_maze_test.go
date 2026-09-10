package pathfinding_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestReproHPASlownessEnclosedGoal stresses the "no path exists" branch of
// docs/bugs/hpa-star-slowness: an enclosed room with the goal outside it.
// The reachable state space (floor of the room) is small and bounded, so a
// correct search should exhaust it in well under 1000 expansions - not
// thousands, and not by coincidentally hitting a round maxSteps ceiling.
func TestReproHPASlownessEnclosedGoal(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	// Floor only inside the room - no floor outside it, so CanJump2's
	// "clip over the wall" loophole (it never checks the block being jumped
	// over, only the landing spot - see movement.go's CanJump2, noted as a
	// separate finding in the investigation doc) can't land anywhere either.
	wb := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-5, -5, 5, 5, -1, 9)

	// Enclose the 10x10 room (x:-5..5, z:-5..5) with walls 3 blocks tall.
	wb = wb.WallDirect(-5, -5, 5, -5, 0, 2, 9)
	wb = wb.WallDirect(-5, 5, 5, 5, 0, 2, 9)
	wb = wb.WallDirect(-5, -5, -5, 5, 0, 2, 9)
	wb = wb.WallDirect(5, -5, 5, 5, 0, 2, 9)

	world := wb.Build()
	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 0, Y: 0, Z: 0} // inside the enclosed room
	goal := models.V3{X: 10, Y: 0, Z: 0} // outside the room - unreachable

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()
	path, err := pathFinder.FindPath(ctx, start, goal, 20000)
	elapsed := time.Since(startTime)

	t.Logf("FindPath took %s, err=%v, path=%+v", elapsed, err, path)
	if err == nil {
		t.Fatalf("expected pathfinding to fail (goal is walled off), got success: %+v", path)
	}
}
