package pathfinding_test

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestGetPossibleVehicleMoves_BoatAvoidsMagma is
// WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 7: vanilla sinks (and damages) a boat that touches
// magma beneath shallow water, so a boat route should never be generated onto a water tile with
// magma directly beneath it, even though the tile itself is otherwise perfectly floatable water.
func TestGetPossibleVehicleMoves_BoatAvoidsMagma(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	stoneID := registry.GetStateID("minecraft:stone", nil)
	magmaID := registry.GetStateID("minecraft:magma_block", nil)
	waterID := registry.GetStateID("minecraft:water", nil)

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 4, 4, 63, stoneID). // Safe stone floor everywhere...
		WaterDirect(0, 64, 0, 4, 64, 4, waterID).  // ...with a shallow 1-block-deep water pool on top
		Build()
	// ...except directly beneath (2,63,2), which is magma instead of stone.
	world.SetBlock(2, 63, 2, magmaID)

	shapeMgr := mctesting.NewMockShapeManager()
	caps := models.GetVehicleCapabilities(models.VehicleTypeBoat)
	validator := pathfinding.NewVehicleMovementValidator(world, shapeMgr, caps, nil)

	from := models.V3{X: 1, Y: 64, Z: 2}
	moves := validator.GetPossibleVehicleMoves(from, models.V3{}, 999, nil)

	magmaPos := models.V3{X: 2, Y: 64, Z: 2}
	safePos := models.V3{X: 1, Y: 64, Z: 1}

	for _, move := range moves {
		if move.Position == magmaPos {
			t.Errorf("expected no boat move onto (%v, magma beneath), got one: %+v", magmaPos, move)
		}
	}

	foundSafeMove := false
	for _, move := range moves {
		if move.Position == safePos {
			foundSafeMove = true
			break
		}
	}
	if !foundSafeMove {
		t.Errorf("expected a boat move onto %v (safe water, stone beneath), got moves: %+v", safePos, moves)
	}
}
