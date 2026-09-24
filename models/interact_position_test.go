package models_test

import (
	"context"
	"errors"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// fakeInteractAgent implements models.InteractPositionAgent for tests.
// blockedOrigins lets a test simulate a candidate that's walkable but
// can't actually see/interact with the target (e.g. blocked by a wall),
// without needing a real raycaster.
type fakeInteractAgent struct {
	world          models.World
	shapeMgr       models.BlockShapeManager
	blockedOrigins map[models.V3]bool
}

func (f *fakeInteractAgent) GetWorld() models.World                      { return f.world }
func (f *fakeInteractAgent) BlockShapeManager() models.BlockShapeManager { return f.shapeMgr }
func (f *fakeInteractAgent) CanInteractFromPosition(_ context.Context, fromX, fromY, fromZ, _, _, _ float64) (bool, error) {
	if f.blockedOrigins[models.V3{X: fromX, Y: fromY, Z: fromZ}] {
		return false, nil
	}
	return true, nil
}

// grassStateID is the solid, ground-support-providing block ID used by
// mctesting.NewWorldBuilder's FlatGroundDirect/SetBlockDirect helpers
// throughout this codebase's other pathfinding tests.
const grassStateID = 9

func TestFindInteractPosition_AdjacentToSolidBlock(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-5, -5, 5, 5, -1, grassStateID).
		SetBlockDirect(0, 0, 0, grassStateID). // the "target block" itself
		Build()

	agent := &fakeInteractAgent{world: world, shapeMgr: mctesting.NewMockShapeManager()}
	target := models.V3{X: 0, Y: 0, Z: 0}

	pos, ok, err := models.FindInteractPosition(context.Background(), agent, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a walkable interact position adjacent to the target")
	}
	if dist := pos.DistanceTo(target); dist > 1.5 {
		t.Errorf("expected the closest candidate to be picked, got %+v at distance %.2f from target", pos, dist)
	}
}

func TestFindInteractPosition_FullyEnclosed(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	// Solid block at origin, solid rock filling every cell within reach (and
	// one block past it, so there's no gap for the search to slip through
	// or stand on top of) - no walkable position exists anywhere nearby.
	const margin = 1
	bound := int(models.InteractReachDistance) + margin
	wb := mctesting.NewWorldBuilder(registry)
	for dx := -bound; dx <= bound; dx++ {
		for dy := -bound; dy <= bound; dy++ {
			for dz := -bound; dz <= bound; dz++ {
				wb = wb.SetBlockDirect(float64(dx), float64(dy), float64(dz), grassStateID)
			}
		}
	}
	world := wb.Build()

	agent := &fakeInteractAgent{world: world, shapeMgr: mctesting.NewMockShapeManager()}
	target := models.V3{X: 0, Y: 0, Z: 0}

	_, ok, err := models.FindInteractPosition(context.Background(), agent, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected no interact position to be found when every candidate is solid")
	}
}

func TestFindInteractPosition_SkipsBlockedLineOfSight(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-5, -5, 5, 5, -1, grassStateID).
		SetBlockDirect(0, 0, 0, grassStateID).
		Build()

	// The closest candidate (east, distance 1) is walkable but "can't see"
	// the target (simulating a wall in between); a farther candidate
	// should be picked instead.
	nearest := models.V3{X: 1, Y: 0, Z: 0}
	agent := &fakeInteractAgent{
		world:          world,
		shapeMgr:       mctesting.NewMockShapeManager(),
		blockedOrigins: map[models.V3]bool{nearest: true},
	}
	target := models.V3{X: 0, Y: 0, Z: 0}

	pos, ok, err := models.FindInteractPosition(context.Background(), agent, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected a fallback interact position to be found")
	}
	if pos == nearest {
		t.Fatalf("expected the blocked-LOS candidate %+v to be skipped, but it was returned", nearest)
	}
}

// TestTryInteractPositions_FloatingTargetFallsBackToGroundLevel covers a
// block floating above the ground (found live: a seeded crafting table two
// cells above the floor). The spot standing on top of it is the closest
// candidate but unreachable from the floor, so when try rejects it the
// ground-level spots beside it must be offered next.
func TestTryInteractPositions_FloatingTargetFallsBackToGroundLevel(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-5, -5, 5, 5, -1, grassStateID).
		SetBlockDirect(0, 1, 0, grassStateID). // target, one cell of air beneath it
		Build()

	agent := &fakeInteractAgent{world: world, shapeMgr: mctesting.NewMockShapeManager()}
	target := models.V3{X: 0, Y: 1, Z: 0}

	var tried []models.V3
	err := models.TryInteractPositions(context.Background(), agent, target, func(pos models.V3) error {
		tried = append(tried, pos)
		if pos.Y == 0 { // only ground-level spots are "reachable"
			return nil
		}
		return errors.New("no path")
	})
	if err != nil {
		t.Fatalf("expected a ground-level spot to succeed, got %v", err)
	}
	if len(tried) < 2 || tried[0] != (models.V3{X: 0, Y: 2, Z: 0}) {
		t.Fatalf("expected the on-top spot to be tried first and rejected, got %v", tried)
	}
	if last := tried[len(tried)-1]; last.Y != 0 {
		t.Errorf("expected iteration to stop at the first ground-level spot, last tried %+v", last)
	}
	for i := 1; i < len(tried); i++ {
		if tried[i-1].DistanceTo(target) > tried[i].DistanceTo(target) {
			t.Errorf("candidates must be offered closest first: %v", tried)
		}
	}
}

// TestTryInteractPositions_AllCandidatesFailReturnsLastError: it only gives
// up after every qualifying spot has been tried.
func TestTryInteractPositions_AllCandidatesFailReturnsLastError(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-5, -5, 5, 5, -1, grassStateID).
		SetBlockDirect(0, 0, 0, grassStateID).
		Build()
	agent := &fakeInteractAgent{world: world, shapeMgr: mctesting.NewMockShapeManager()}

	calls := 0
	sentinel := errors.New("no path")
	err := models.TryInteractPositions(context.Background(), agent, models.V3{}, func(models.V3) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the try error back, got %v", err)
	}
	if calls < 5 {
		t.Errorf("expected every qualifying candidate to be tried, only %d were", calls)
	}
}

func TestTryInteractPositions_NoCandidatesReturnsErrNoInteractPosition(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	const bound = int(models.InteractReachDistance) + 1
	wb := mctesting.NewWorldBuilder(registry)
	for dx := -bound; dx <= bound; dx++ {
		for dy := -bound; dy <= bound; dy++ {
			for dz := -bound; dz <= bound; dz++ {
				wb = wb.SetBlockDirect(float64(dx), float64(dy), float64(dz), grassStateID)
			}
		}
	}
	agent := &fakeInteractAgent{world: wb.Build(), shapeMgr: mctesting.NewMockShapeManager()}

	err := models.TryInteractPositions(context.Background(), agent, models.V3{}, func(models.V3) error {
		t.Fatal("try must not be called when nothing qualifies")
		return nil
	})
	if !errors.Is(err, models.ErrNoInteractPosition) {
		t.Fatalf("expected ErrNoInteractPosition, got %v", err)
	}
}
