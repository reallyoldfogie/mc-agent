package pathfinding_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestReproHPASlowness reproduces docs/bugs/hpa-star-slowness: a flat,
// obstacle-free 5-block hop that should be trivial for A*.
func TestReproHPASlowness(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 20, 20, -1, 9).
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: -6, Y: 0, Z: 0}
	goal := models.V3{X: -1, Y: 0, Z: 0}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	startTime := time.Now()
	path, err := pathFinder.FindPath(ctx, start, goal, 20000)
	elapsed := time.Since(startTime)

	t.Logf("FindPath took %s, err=%v", elapsed, err)
	if err != nil {
		t.Fatalf("expected path to be found quickly, got err=%v after %s", err, elapsed)
	}
	if path == nil || !path.Found {
		t.Fatalf("expected path.Found=true, got %+v", path)
	}
	t.Logf("Path found with %d steps in %s", len(path.Steps), elapsed)
}
