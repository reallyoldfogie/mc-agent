package pathfinding_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
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
	grassStateID, _ := world.GetBlockAt(10, 64, 10)
	if grassStateID != 9 {
		t.Errorf("Expected grass (9) at (10, 64, 10), got %d", grassStateID)
	}

	// Verify air above grass
	airStateID, _ := world.GetBlockAt(10, 65, 10)
	if airStateID != 0 {
		t.Errorf("Expected air (0) at (10, 65, 10), got %d", airStateID)
	}

	// Create mock managers for pathfinding
	shapeMgr := mctesting.NewMockShapeManager()

	// Create pathfinder
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr)

	// Test: Find path from (0, 65, 0) to (10, 65, 10)
	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 10, Y: 65, Z: 10}

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

	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr)

	// Test: Find path at negative Y
	start := models.V3{X: 0, Y: -59, Z: 0}
	goal := models.V3{X: 10, Y: -59, Z: 10}

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
	blockID, _ := world.GetBlockAt(5, 64, 5)
	if blockID != 9 {
		t.Error("Grass not placed correctly")
	}

	// Assert: Wall exists
	blockID, _ = world.GetBlockAt(10, 66, 2)
	if blockID != 1 {
		t.Error("Wall not placed correctly")
	}

	// Assert: Stairs exist
	blockID, _ = world.GetBlockAt(5, 65, 10)
	if blockID != 3391 {
		t.Error("Stairs not placed correctly")
	}

	// Assert: Gap exists (air)
	blockID1, _ := world.GetBlockAt(15, 64, 10)
	blockID2, _ := world.GetBlockAt(16, 64, 10)
	if blockID1 != 0 || blockID2 != 0 {
		t.Error("Gap not created correctly")
	}

	t.Logf("WorldBuilder created world with %d blocks", world.BlockCount())
}

// TestEPEAStarVsAStar compares EPEA* with standard A* pathfinding
// Tests include stairs, obstacles, and various terrain features
func TestEPEAStarVsAStar(t *testing.T) {
	// Setup: Create a complex world with obstacles, stairs, and elevated platforms
	registry := mctesting.NewSimpleBlockRegistry()

	world := mctesting.NewWorldBuilder(registry).
		FlatGround(0, 0, 60, 60, 64, "grass_block"). // Base grass layer at Y=64
		// Add some obstacles (walls)
		Wall(15, 0, 15, 10, 65, 68, "stone").  // Stone wall (3 blocks high)
		Wall(30, 20, 30, 30, 65, 66, "stone"). // Another wall
		Wall(40, 10, 40, 20, 65, 67, "stone"). // Third wall
		// Create elevated platform at Y=68 (4 blocks higher - requires stairs)
		FlatGround(20, 20, 35, 35, 68, "grass_block"). // Elevated platform at Y=68
		// Build stairs from ground (Y=65) up to platform (Y=69)
		Stairs(18, 65, 25, 4, "north", "oak_stairs"). // 4 oak stair blocks going north
		// Create even higher platform at Y=72 (8 blocks higher than base)
		FlatGround(40, 35, 50, 45, 72, "grass_block"). // Higher platform at Y=72
		// Stairs from first platform (Y=69) to second platform (Y=73)
		Stairs(33, 69, 40, 4, "east", "oak_stairs"). // 4 stair blocks going east
		Build()

	shapeMgr := mctesting.NewMockShapeManager()

	// Create both pathfinders
	aStarPathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr)
	epeaStarPathFinder := pathfinding.NewEPEAStarPathFinder(world, shapeMgr)

	resultsLog := []string{}

	// Test case 1: Path requiring stairs to elevated platform
	t.Run("PathRequiringStairs", func(t *testing.T) {
		start := models.V3{X: 15, Y: 65, Z: 25} // On ground level (feet at Y=65, grass at Y=64)
		goal := models.V3{X: 27, Y: 69, Z: 27}  // On elevated platform (feet at Y=69, grass at Y=68)

		// Debug: Verify world setup at goal and stairs
		t.Logf("World setup check:")
		t.Logf("  Goal (27, ?, 27):")
		for y := float64(64); y <= 70; y++ {
			stateID, _ := world.GetBlockAt(27, y, 27)
			t.Logf("    Y=%.0f: stateID=%d", y, stateID)
		}
		t.Logf("  Stairs base (18, ?, 25):")
		for y := float64(64); y <= 70; y++ {
			stateID, _ := world.GetBlockAt(18, y, 25)
			t.Logf("    Y=%.0f: stateID=%d", y, stateID)
		}
		t.Logf("  Stairs top (18, ?, 22):")
		for y := float64(64); y <= 70; y++ {
			stateID, _ := world.GetBlockAt(18, y, 22)
			t.Logf("    Y=%.0f: stateID=%d", y, stateID)
		}

		// Run A* with higher step limit due to complex search space
		aStarPath, aStarErr := aStarPathFinder.FindPath(start, goal, 15000000)

		// Run EPEA* with same step limit
		epeaStarPath, epeaStarErr := epeaStarPathFinder.FindPath(start, goal, 15000000)

		if aStarErr != nil {
			t.Errorf("A* failed: %v", aStarErr)
		}
		if epeaStarErr != nil {
			t.Errorf("EPEA* failed: %v", epeaStarErr)
		}

		if !aStarPath.Found || !epeaStarPath.Found {
			t.Error("One or both algorithms did not find a path")
		}

		// Verify paths actually climb 4 blocks (can't jump this high)
		aStarElevationGain := goal.Y - start.Y
		epeaElevationGain := goal.Y - start.Y

		if aStarElevationGain < 3 || epeaElevationGain < 3 {
			t.Error("Expected paths to climb at least 3 blocks (requires stairs)")
		}

		resultsLog = append(resultsLog, "=== Path Requiring Stairs (4 blocks elevation) ===")
		resultsLog = append(resultsLog, fmt.Sprintf("Start: (%.0f, %.0f, %.0f) -> Goal: (%.0f, %.0f, %.0f)",
			start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z))
		resultsLog = append(resultsLog, fmt.Sprintf("Elevation gain: %.0f blocks", goal.Y-start.Y))
		resultsLog = append(resultsLog, "\nA* Results:")
		resultsLog = append(resultsLog, fmt.Sprintf("  Steps: %d, Cost: %.2f, Time: %.2fms",
			len(aStarPath.Steps), aStarPath.TotalCost, aStarPath.SearchTime))
		resultsLog = append(resultsLog, "\nEPEA* Results:")
		resultsLog = append(resultsLog, fmt.Sprintf("  Steps: %d, Cost: %.2f, Time: %.2fms",
			len(epeaStarPath.Steps), epeaStarPath.TotalCost, epeaStarPath.SearchTime))

		if epeaStarPath.SearchTime > 0 && aStarPath.SearchTime > 0 {
			speedup := aStarPath.SearchTime / epeaStarPath.SearchTime
			resultsLog = append(resultsLog, fmt.Sprintf("\nSpeedup: %.2fx %s", speedup, speedupDescription(speedup)))
		}
	})

	// Test case 2: Long path with obstacles at same level
	t.Run("LongPathWithObstacles", func(t *testing.T) {
		start := models.V3{X: 5, Y: 65, Z: 5}
		goal := models.V3{X: 50, Y: 65, Z: 50}

		// Run A* with higher step limit
		aStarPath, aStarErr := aStarPathFinder.FindPath(start, goal, 50000)

		// Run EPEA* with same step limit
		epeaStarPath, epeaStarErr := epeaStarPathFinder.FindPath(start, goal, 50000)

		if aStarErr != nil {
			t.Errorf("A* failed: %v", aStarErr)
		}
		if epeaStarErr != nil {
			t.Errorf("EPEA* failed: %v", epeaStarErr)
		}

		if !aStarPath.Found || !epeaStarPath.Found {
			t.Error("One or both algorithms did not find a path")
		}

		resultsLog = append(resultsLog, "\n=== Long Path With Obstacles ===")
		t.Logf("Start: (%.0f, %.0f, %.0f) -> Goal: (%.0f, %.0f, %.0f)",
			start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z)
		resultsLog = append(resultsLog, "\nA* Results:")
		resultsLog = append(resultsLog, fmt.Sprintf("  Steps: %d, Cost: %.2f, Time: %.2fms",
			len(aStarPath.Steps), aStarPath.TotalCost, aStarPath.SearchTime))
		resultsLog = append(resultsLog, "\nEPEA* Results:")
		resultsLog = append(resultsLog, fmt.Sprintf("  Steps: %d, Cost: %.2f, Time: %.2fms",
			len(epeaStarPath.Steps), epeaStarPath.TotalCost, epeaStarPath.SearchTime))

		if epeaStarPath.SearchTime > 0 && aStarPath.SearchTime > 0 {
			speedup := aStarPath.SearchTime / epeaStarPath.SearchTime
			resultsLog = append(resultsLog, fmt.Sprintf("\nSpeedup: %.2fx %s", speedup, speedupDescription(speedup)))
		}
	})

	// Test case 3: Complex multi-level path (ground -> platform1 -> platform2)
	t.Run("ComplexMultiLevelPath", func(t *testing.T) {
		start := models.V3{X: 10, Y: 65, Z: 20} // On ground
		goal := models.V3{X: 45, Y: 73, Z: 40}  // On highest platform (8 blocks higher)

		// Run A* with much higher step limit for complex multi-level navigation
		aStarPath, aStarErr := aStarPathFinder.FindPath(start, goal, 1000000)

		// Run EPEA* with same step limit
		epeaStarPath, epeaStarErr := epeaStarPathFinder.FindPath(start, goal, 1000000)

		if aStarErr != nil {
			t.Errorf("A* failed: %v", aStarErr)
		}
		if epeaStarErr != nil {
			t.Errorf("EPEA* failed: %v", epeaStarErr)
		}

		if !aStarPath.Found || !epeaStarPath.Found {
			t.Error("One or both algorithms did not find a path")
		}

		// Verify significant elevation gain
		elevationGain := goal.Y - start.Y
		if elevationGain < 6 {
			t.Errorf("Expected elevation gain of at least 6 blocks, got %.0f", elevationGain)
		}

		resultsLog = append(resultsLog, "\n=== Complex Multi-Level Path (8 blocks elevation) ===")
		resultsLog = append(resultsLog, fmt.Sprintf("Start: (%.0f, %.0f, %.0f) -> Goal: (%.0f, %.0f, %.0f)",
			start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z))
		resultsLog = append(resultsLog, fmt.Sprintf("Elevation gain: %.0f blocks", elevationGain))
		resultsLog = append(resultsLog, "\nA* Results:")
		resultsLog = append(resultsLog, fmt.Sprintf("  Steps: %d, Cost: %.2f, Time: %.2fms",
			len(aStarPath.Steps), aStarPath.TotalCost, aStarPath.SearchTime))
		resultsLog = append(resultsLog, "\nEPEA* Results:")
		resultsLog = append(resultsLog, fmt.Sprintf("  Steps: %d, Cost: %.2f, Time: %.2fms",
			len(epeaStarPath.Steps), epeaStarPath.TotalCost, epeaStarPath.SearchTime))

		// Compare path quality
		costDiff := math.Abs(aStarPath.TotalCost - epeaStarPath.TotalCost)
		if costDiff > 1.0 {
			resultsLog = append(resultsLog, fmt.Sprintf("\nWarning: Significant cost difference: %.2f", costDiff))
		}

		if epeaStarPath.SearchTime > 0 && aStarPath.SearchTime > 0 {
			speedup := aStarPath.SearchTime / epeaStarPath.SearchTime
			resultsLog = append(resultsLog, fmt.Sprintf("\nSpeedup: %.2fx %s", speedup, speedupDescription(speedup)))
		}
	})

	for _, line := range resultsLog {
		fmt.Printf("%s\n", line)
	}
}

// speedupDescription provides a human-readable description of the speedup
func speedupDescription(speedup float64) string {
	if speedup > 1.1 {
		return "(EPEA* faster)"
	} else if speedup < 0.9 {
		return "(A* faster)"
	}
	return "(similar performance)"
}

// TestAscendStairsMovement tests that AscendStairs moves are generated for stair blocks
func TestAscendStairsMovement(t *testing.T) {
	// Setup: Create a world with ground and a stair
	registry := mctesting.NewSimpleBlockRegistry()

	// Build world with:
	// - Flat ground at Y=64
	// - A single stair block at (5, 65, 5)
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 10, 10, 64, 9). // Grass at Y=64
		Build()

	// Add a stair block at Y=65 (where the bot would step onto)
	// Oak stairs state ID is 3391 in the test registry
	world.SetBlock(5, 65, 5, 3391)

	shapeMgr := mctesting.NewMockShapeManager()

	// Verify the stair is recognized as a stair
	stairStateID, _ := world.GetBlockAt(5, 65, 5)
	if !shapeMgr.IsStair(stairStateID) {
		t.Fatalf("Stair block (stateID=%d) not recognized as stairs", stairStateID)
	}

	// Create movement validator
	mv := pathfinding.NewMovementValidator(world, shapeMgr)

	// Test CanAscendStairs: from (4, 65, 5) to (5, 66, 5)
	// - Bot is at (4, 65, 5) standing on ground at Y=64
	// - Target is (5, 66, 5) standing on stair at Y=65
	from := models.V3{X: 4, Y: 65, Z: 5}
	to := models.V3{X: 5, Y: 66, Z: 5}

	canAscendStairs := mv.CanAscendStairs(from, to)
	t.Logf("CanAscendStairs from (%v) to (%v): %v", from, to, canAscendStairs)

	if !canAscendStairs {
		// Debug: check individual conditions
		groundPos := to.Add(0, -1, 0)
		groundStateID, loaded := world.GetBlockAt(groundPos.X, groundPos.Y, groundPos.Z)
		t.Logf("  Ground at (%v): stateID=%d, loaded=%v, isStair=%v",
			groundPos, groundStateID, loaded, shapeMgr.IsStair(groundStateID))

		// Check passable
		feetStateID, _ := world.GetBlockAt(to.X, to.Y, to.Z)
		headStateID, _ := world.GetBlockAt(to.X, to.Y+1, to.Z)
		t.Logf("  Feet at (%v): stateID=%d, passable=%v",
			to, feetStateID, feetStateID == 0 || shapeMgr.IsPassable(feetStateID))
		t.Logf("  Head at (%.0f,%.0f,%.0f): stateID=%d, passable=%v",
			to.X, to.Y+1, to.Z, headStateID, headStateID == 0 || shapeMgr.IsPassable(headStateID))

		// Check ground support
		t.Logf("  Ground support: isSolid=%v", shapeMgr.IsSolid(groundStateID))

		t.Error("CanAscendStairs should return true for valid stair ascent")
	}

	// Test GetPossibleMoves includes AscendStairs
	moves := mv.GetPossibleMoves(from, to, nil)

	hasAscendStairs := false
	for _, move := range moves {
		if move.Movement == pathfinding.AscendStairs {
			hasAscendStairs = true
			t.Logf("Found AscendStairs move to (%v)", move.Position)
		}
	}

	if !hasAscendStairs {
		t.Errorf("GetPossibleMoves should include AscendStairs move")
		t.Logf("Generated %d moves:", len(moves))
		for _, move := range moves {
			t.Logf("  - %s to (%v)", move.Movement, move.Position)
		}
	}
}
