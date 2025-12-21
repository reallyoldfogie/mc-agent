package pathfinding_test

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestFlatGroundPathfinding tests basic pathfinding on flat terrain
func TestFlatGroundPathfinding(t *testing.T) {
	// Setup: Create flat grass world using WorldBuilder
	registry := mctesting.NewSimpleBlockRegistry()

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 20, 20, 64, 9). // Grass block (state ID 9)
		Build()

	// Verify world was built correctly
	if world.BlockCount() == 0 {
		t.Fatal("World is empty - FlatGround failed")
	}

	// Verify grass block at expected position
	grassStateID := world.GetBlockAt(10, 64, 10)
	if grassStateID != 9 {
		t.Errorf("Expected grass (9) at (10, 64, 10), got %d", grassStateID)
	}

	// Verify air above grass
	airStateID := world.GetBlockAt(10, 65, 10)
	if airStateID != 0 {
		t.Errorf("Expected air (0) at (10, 65, 10), got %d", airStateID)
	}

	// Create mock managers for pathfinding
	blockMgr := mctesting.NewMockBlockManager()
	shapeMgr := mctesting.NewMockShapeManager()

	// Create pathfinder
	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	// Test: Find path from (0, 65, 0) to (10, 65, 10)
	start := pathfinding.V3{X: 0, Y: 65, Z: 0}
	goal := pathfinding.V3{X: 10, Y: 65, Z: 10}

	path, err := pathFinder.FindPath(start, goal, 200)

	// Assert: Path found
	if err != nil {
		t.Fatalf("Failed to find path: %v", err)
	}

	if path == nil {
		t.Fatal("Path is nil")
	}

	// Assert: Path has steps
	if len(path.Steps) == 0 {
		t.Fatal("Path has no steps")
	}

	// Assert: Path reaches goal (approximately)
	lastStep := path.Steps[len(path.Steps)-1]
	if math.Abs(lastStep.Position.X-goal.X) > 1 || math.Abs(lastStep.Position.Z-goal.Z) > 1 {
		t.Errorf("Path doesn't reach goal. Last step: (%f, %f, %f), Goal: (%f, %f, %f)",
			lastStep.Position.X, lastStep.Position.Y, lastStep.Position.Z,
			goal.X, goal.Y, goal.Z)
	}

	t.Logf("Path found with %d steps from (%f, %f, %f) to (%f, %f, %f)",
		len(path.Steps), start.X, start.Y, start.Z,
		lastStep.Position.X, lastStep.Position.Y, lastStep.Position.Z)
}

// TestNegativeYCoordinates tests pathfinding at negative Y (tests bug fix)
func TestNegativeYCoordinates(t *testing.T) {
	// Setup: Create flat ground at negative Y
	registry := mctesting.NewSimpleBlockRegistry()

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 20, 20, -60, 9). // Y = -60 (grass)
		Build()

	blockMgr := mctesting.NewMockBlockManager()
	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewPathFinder(world, shapeMgr, blockMgr, nil)

	// Test: Find path at negative Y
	start := pathfinding.V3{X: 0, Y: -59, Z: 0}
	goal := pathfinding.V3{X: 10, Y: -59, Z: 10}

	path, err := pathFinder.FindPath(start, goal, 200)

	// Assert: Path found (tests negative Y coordinate fix)
	if err != nil {
		t.Fatalf("Failed to find path at negative Y: %v", err)
	}

	if path == nil || len(path.Steps) == 0 {
		t.Fatal("No path found at negative Y")
	}

	// Assert: All path steps have reasonable Y coordinates
	for i, step := range path.Steps {
		if step.Position.Y < -60 || step.Position.Y > -58 {
			t.Errorf("Step %d has unexpected Y: %f (expected around -59)", i, step.Position.Y)
		}
	}

	t.Logf("Successfully found path at negative Y with %d steps", len(path.Steps))
}

// TestWorldBuilder tests the WorldBuilder API directly
func TestWorldBuilder(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()

	// Test: Build world with multiple features
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 20, 20, 64, 9).     // Grass
		WallDirect(10, 0, 10, 5, 65, 67, 1).       // Stone wall
		StairsDirect(5, 65, 10, 3, "north", 3391). // Oak stairs
		Gap(15, 10, 64, 2).                        // 2-block gap
		Build()

	// Assert: Grass exists
	if world.GetBlockAt(5, 64, 5) != 9 {
		t.Error("Grass not placed correctly")
	}

	// Assert: Wall exists
	if world.GetBlockAt(10, 66, 2) != 1 {
		t.Error("Wall not placed correctly")
	}

	// Assert: Stairs exist
	if world.GetBlockAt(5, 65, 10) != 3391 {
		t.Error("Stairs not placed correctly")
	}

	// Assert: Gap exists (air)
	if world.GetBlockAt(15, 64, 10) != 0 || world.GetBlockAt(16, 64, 10) != 0 {
		t.Error("Gap not created correctly")
	}

	t.Logf("WorldBuilder created world with %d blocks", world.BlockCount())
}

// Helper function
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
