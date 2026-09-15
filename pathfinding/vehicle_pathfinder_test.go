package pathfinding_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// fakeNoVehicleProvider reports no rideable entities in the world - used to isolate the
// boat-placement branch from the pre-existing-vehicle branch in these tests.
type fakeNoVehicleProvider struct{}

func (fakeNoVehicleProvider) FindRideableEntitiesNear(models.V3, float64, bool) []pathfinding.RideableEntity {
	return nil
}

// fakeBoatChecker reports a fixed HasPlaceableBoat answer, letting tests control it directly
// rather than needing a real agent/inventory.
type fakeBoatChecker bool

func (f fakeBoatChecker) HasPlaceableBoat() bool { return bool(f) }

// buildWaterCrossingWorld creates a flat platform with a water strip of the given width between
// two dry shores, solid ground underneath the whole thing (X=0..waterEndX+10, Z=0..4).
func buildWaterCrossingWorld(waterStartX, waterWidth float64) (*mctesting.MockWorld, *mctesting.SimpleBlockRegistry) {
	registry := mctesting.NewSimpleBlockRegistry()
	grassID := registry.GetStateID("minecraft:grass_block", nil)
	waterID := registry.GetStateID("minecraft:water", nil)

	maxX := waterStartX + waterWidth + 10
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, maxX, 4, 64, grassID).
		WaterDirect(waterStartX, 65, 0, waterStartX+waterWidth-1, 65, 4, waterID).
		Build()

	return world, registry
}

func newTestVehicleAwarePathFinder(world models.World, shapeMgr models.BlockShapeManager, hasBoat bool) *pathfinding.VehicleAwarePathFinder {
	base := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)
	vap := pathfinding.NewVehicleAwarePathFinder(base, fakeNoVehicleProvider{}, world, shapeMgr, 32.0, nil).(*pathfinding.VehicleAwarePathFinder)
	vap.SetBoatInventoryChecker(fakeBoatChecker(hasBoat))
	return vap
}

func containsMovement(steps []pathfinding.PathStep, m pathfinding.MovementType) bool {
	for _, s := range steps {
		if s.Movement == m {
			return true
		}
	}
	return false
}

// TestVehicleAwarePathFinder_PlacesBoatForWideCrossing covers
// WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8: with a carried boat available and a wide enough
// water crossing that VehicleSwim's per-tile savings outweigh PlaceVehicle's one-time cost, the
// pathfinder should choose to place and ride a boat rather than swim.
func TestVehicleAwarePathFinder_PlacesBoatForWideCrossing(t *testing.T) {
	const waterWidth = 35 // wide enough for VehicleSwim's savings to clear PlaceVehicle's overhead
	world, _ := buildWaterCrossingWorld(5, waterWidth)
	shapeMgr := mctesting.NewMockShapeManager()

	start := models.V3{X: 2, Y: 65, Z: 2}
	goal := models.V3{X: 5 + waterWidth + 5, Y: 65, Z: 2}

	vapWithBoat := newTestVehicleAwarePathFinder(world, shapeMgr, true)
	pathWithBoat, err := vapWithBoat.FindPath(context.Background(), start, goal, 20000)
	if err != nil {
		t.Fatalf("FindPath (with boat): %v", err)
	}
	if pathWithBoat == nil || !pathWithBoat.Found {
		t.Fatal("expected a path to be found with a boat available")
	}

	vapNoBoat := newTestVehicleAwarePathFinder(world, shapeMgr, false)
	pathNoBoat, err := vapNoBoat.FindPath(context.Background(), start, goal, 20000)
	if err != nil {
		t.Fatalf("FindPath (no boat): %v", err)
	}
	if pathNoBoat == nil || !pathNoBoat.Found {
		t.Fatal("expected a path to be found without a boat")
	}

	t.Logf("with boat: cost=%.2f steps=%d; no boat: cost=%.2f steps=%d",
		pathWithBoat.TotalCost, len(pathWithBoat.Steps), pathNoBoat.TotalCost, len(pathNoBoat.Steps))

	if !containsMovement(pathWithBoat.Steps, pathfinding.PlaceVehicle) {
		t.Error("expected the boat-available path to contain a PlaceVehicle step for this wide a crossing")
	}
	if pathWithBoat.TotalCost >= pathNoBoat.TotalCost {
		t.Errorf("expected boat placement to be cheaper than swimming for a %d-tile crossing: with-boat cost=%.2f, no-boat cost=%.2f",
			waterWidth, pathWithBoat.TotalCost, pathNoBoat.TotalCost)
	}
}

// TestVehicleAwarePathFinder_SkipsBoatForShortCrossing confirms the mechanism doesn't force boat
// placement onto a short crossing a swim handles almost as fast - PlaceVehicle's one-time cost
// should keep it from being selected when the water is only a few tiles wide.
func TestVehicleAwarePathFinder_SkipsBoatForShortCrossing(t *testing.T) {
	const waterWidth = 3
	world, _ := buildWaterCrossingWorld(5, waterWidth)
	shapeMgr := mctesting.NewMockShapeManager()

	start := models.V3{X: 2, Y: 65, Z: 2}
	goal := models.V3{X: 5 + waterWidth + 5, Y: 65, Z: 2}

	vap := newTestVehicleAwarePathFinder(world, shapeMgr, true)
	path, err := vap.FindPath(context.Background(), start, goal, 20000)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if path == nil || !path.Found {
		t.Fatal("expected a path to be found")
	}

	if containsMovement(path.Steps, pathfinding.PlaceVehicle) {
		t.Errorf("did not expect boat placement to be chosen for a %d-tile crossing (cost=%.2f)", waterWidth, path.TotalCost)
	}
}

// TestVehicleAwarePathFinder_NoBoatPlacementWithoutWater confirms a foot path that never touches
// water produces no boat-placement candidate at all, even with a boat available - there's nothing
// to place it onto.
func TestVehicleAwarePathFinder_NoBoatPlacementWithoutWater(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	grassID := registry.GetStateID("minecraft:grass_block", nil)
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 20, 4, 64, grassID).
		Build()
	shapeMgr := mctesting.NewMockShapeManager()

	vap := newTestVehicleAwarePathFinder(world, shapeMgr, true)
	path, err := vap.FindPath(context.Background(), models.V3{X: 2, Y: 65, Z: 2}, models.V3{X: 18, Y: 65, Z: 2}, 5000)
	if err != nil {
		t.Fatalf("FindPath: %v", err)
	}
	if path == nil || !path.Found {
		t.Fatal("expected a path to be found")
	}
	if containsMovement(path.Steps, pathfinding.PlaceVehicle) {
		t.Error("did not expect a PlaceVehicle step on an all-dry path")
	}
}
