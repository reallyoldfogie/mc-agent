package pathfinding_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestFindPath_FractionalGoalNearGoalRadiusBoundary reproduces a real
// failure live-confirmed against mc-rsi-trainer's shared-server training
// run: rlenv.Config.Jitter draws a continuous, arbitrary-valued goal
// every episode, and roughly a fifth of all possible continuous goals
// (circles of radius 0.5 around nodes spaced 1.0 apart only cover
// ~78.5% of the plane) have no reachable node within the default 0.5
// goalRadius at all -- independent of any floating-point rounding. The
// exact coordinates below (goal (264.2, Y, 3.9) from start
// (249.5, Y, 5.5), here on Y=0 flat ground rather than the live run's
// Y=-60 since the bug is about X/Z alignment, not Y) previously
// exhausted the full step/time budget (thousands of nodes, several
// seconds) before failing; it must now resolve in well under a second.
func TestFindPath_FractionalGoalNearGoalRadiusBoundary(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 300, 20, -1, 9).
		Build()
	shapeMgr := mctesting.NewMockShapeManager()

	start := models.V3{X: 249.5, Y: 0, Z: 5.5}
	goal := models.V3{X: 264.2, Y: 0, Z: 3.9}

	tests := []struct {
		name       string
		pathFinder models.PathFinder
	}{
		{"A*", pathfinding.NewAStarPathFinder(world, shapeMgr, nil)},
		{"BidirectionalA*", pathfinding.NewBidirAStarPathFinder(world, shapeMgr, nil)},
		{"EPEA*", pathfinding.NewEPEAStarPathFinder(world, shapeMgr, nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			startTime := time.Now()
			path, err := tc.pathFinder.FindPath(ctx, start, goal, 20000)
			elapsed := time.Since(startTime)

			t.Logf("FindPath took %s, err=%v", elapsed, err)
			if err != nil {
				t.Fatalf("expected path to be found quickly, got err=%v after %s", err, elapsed)
			}
			if path == nil || !path.Found {
				t.Fatalf("expected path.Found=true, got %+v", path)
			}
			if elapsed > time.Second {
				t.Fatalf("expected fail-fast resolution (<1s), took %s — the goal-snap fix may not be firing", elapsed)
			}
		})
	}
}
