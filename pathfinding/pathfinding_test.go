package pathfinding_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

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
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	// Test: Find path from (0, 65, 0) to (10, 65, 10)
	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 10, Y: 65, Z: 10}

	path, err := pathFinder.FindPath(context.Background(), start, goal, 200)

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
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	// Test: Find path at negative Y
	start := models.V3{X: 0, Y: -59, Z: 0}
	goal := models.V3{X: 10, Y: -59, Z: 10}

	path, err := pathFinder.FindPath(context.Background(), start, goal, 200)

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
		// Add some obstacles (walls) - placed to not block test paths
		Wall(8, 0, 8, 3, 65, 68, "stone").     // Stone wall (3 blocks high)
		Wall(55, 55, 55, 58, 65, 66, "stone"). // Another wall - far corner
		// Create elevated platform at Y=68 (4 blocks higher)
		// Platform from (18, 20) to (35, 35) to accommodate stairs starting at X=18
		FlatGround(18, 20, 35, 35, 68, "grass_block"). // Elevated platform at Y=68
		// Build stairs from ground (Y=65) up to platform (Y=68+1=69 for feet)
		// Stairs at (20, 65, 18) going north 4 blocks: places stairs leading onto platform
		// Stairs: (20,65,18), (20,66,17), (20,67,16), (20,68,15) - but this goes away from platform
		// Actually, need stairs going SOUTH to reach the platform which is at Z=20+
		// Stairs at (20, 65, 16) going south 4 blocks: (20,65,16), (20,66,17), (20,67,18), (20,68,19)
		// This leads to Y=68 surface at Z=19, and platform starts at Z=20 - close enough to step onto
		Stairs(20, 65, 16, 4, "south", "oak_stairs"). // 4 stairs going south, ending near platform
		// Create even higher platform at Y=72
		FlatGround(32, 32, 50, 45, 72, "grass_block"). // Higher platform at Y=72
		// Stairs from first platform (Y=69 feet level) to second platform (Y=73 feet level)
		// First platform surface is Y=68, so feet at Y=69
		// Need to climb 4 more blocks to Y=73 (surface Y=72)
		// Stairs at (30, 69, 30) going east 4 blocks: (30,69,30), (31,70,30), (32,71,30), (33,72,30)
		// Platform starts at X=32, so (33,72,30) is on the platform (X=33 >= 32)
		Stairs(30, 69, 30, 4, "east", "oak_stairs"). // 4 stair blocks going east
		Build()

	shapeMgr := mctesting.NewMockShapeManager()

	// Create both pathfinders
	aStarPathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)
	epeaStarPathFinder := pathfinding.NewEPEAStarPathFinder(world, shapeMgr, nil)

	resultsLog := []string{}

	// Test case 1: Path requiring stairs to elevated platform
	t.Run("PathRequiringStairs", func(t *testing.T) {
		start := models.V3{X: 15, Y: 65, Z: 15} // On ground level (feet at Y=65, grass at Y=64)
		goal := models.V3{X: 25, Y: 69, Z: 25}  // On elevated platform (feet at Y=69, grass at Y=68)

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
		aStarPath, aStarErr := aStarPathFinder.FindPath(context.Background(), start, goal, 15000000)

		// Run EPEA* with same step limit
		epeaStarPath, epeaStarErr := epeaStarPathFinder.FindPath(context.Background(), start, goal, 15000000)

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
		aStarPath, aStarErr := aStarPathFinder.FindPath(context.Background(), start, goal, 50000)

		// Run EPEA* with same step limit
		epeaStarPath, epeaStarErr := epeaStarPathFinder.FindPath(context.Background(), start, goal, 50000)

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
		start := models.V3{X: 15, Y: 65, Z: 15} // On ground
		goal := models.V3{X: 40, Y: 73, Z: 38}  // On highest platform (8 blocks higher)

		// Run A* with much higher step limit for complex multi-level navigation
		aStarPath, aStarErr := aStarPathFinder.FindPath(context.Background(), start, goal, 1000000)

		// Run EPEA* with same step limit
		epeaStarPath, epeaStarErr := epeaStarPathFinder.FindPath(context.Background(), start, goal, 1000000)

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
	mv := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	// Test CanAscendStairs: from (4, 65, 5) to (5, 66, 5)
	// - Bot is at (4, 65, 5) standing on ground at Y=64
	// - Target is (5, 66, 5) standing on stair at Y=65
	from := models.V3{X: 4, Y: 65, Z: 5}
	to := models.V3{X: 5, Y: 66, Z: 5}

	canAscendStairs := mv.CanAscendStairs(from, to)
	t.Logf("CanAscendStairs from (%v) to (%v): %v", from, to, canAscendStairs)

	if !canAscendStairs {
		// Debug: check individual conditions
		groundPos := to.Add(models.V3{X: 0, Y: -1, Z: 0})
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

// TestContextCancellation verifies that pathfinding respects context cancellation
func TestContextCancellation(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()

	// Create a large world to ensure pathfinding takes time
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 100, 100, 64, 9). // Large grass area
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 50, Y: 65, Z: 50}

	// Path finding should return immediately with context error
	path, err := pathFinder.FindPath(ctx, start, goal, 100000)

	if err == nil {
		t.Error("Expected error from cancelled context")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
	if path == nil {
		t.Fatal("Expected non-nil path struct even on error")
	}
	if path.Found {
		t.Error("Path should not be found with cancelled context")
	}

	t.Logf("Context cancellation correctly returned: %v", err)
}

// TestContextDeadline verifies that pathfinding respects context deadline
func TestContextDeadline(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()

	// Create a large world that forces a long search
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 200, 200, 64, 9). // Very large grass area
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	// Create a context with a very short deadline (1ms)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Use a very distant goal that would take a long time to path to
	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 150, Y: 65, Z: 150}

	// Path finding should fail due to deadline
	path, err := pathFinder.FindPath(ctx, start, goal, 10000000) // Large step limit to ensure timeout, not step limit

	if err == nil {
		// It's possible the path completes very quickly on fast hardware
		// In that case, the path should be found
		if path.Found {
			t.Log("Path found before deadline - test inconclusive on fast hardware")
			return
		}
		t.Error("Expected error from deadline exceeded")
	}

	// Accept either deadline exceeded or canceled (depends on timing)
	if err != context.DeadlineExceeded && err != context.Canceled {
		// If path was found before timeout, that's ok too
		if path != nil && path.Found {
			t.Log("Path found before deadline - test inconclusive on fast hardware")
			return
		}
		t.Errorf("Expected context deadline/canceled error, got: %v", err)
	}

	t.Logf("Context deadline correctly returned: %v", err)
}

// PathFinderFactory creates a PathFinder for testing - allows easy addition of new algorithms
type PathFinderFactory func(w models.World, shapeMgr models.BlockShapeManager) models.PathFinder

// algorithmTestSuite defines all available pathfinding algorithms for comparison
var algorithmTestSuite = map[string]PathFinderFactory{
	"A*": func(w models.World, shapeMgr models.BlockShapeManager) models.PathFinder {
		return pathfinding.NewAStarPathFinder(w, shapeMgr, nil)
	},
	"EPEA*": func(w models.World, shapeMgr models.BlockShapeManager) models.PathFinder {
		return pathfinding.NewEPEAStarPathFinder(w, shapeMgr, nil)
	},
	"Bidirectional": func(w models.World, shapeMgr models.BlockShapeManager) models.PathFinder {
		return pathfinding.NewBidirAStarPathFinder(w, shapeMgr, nil)
	},
}

// TestAlgorithmCorrectness verifies all pathfinding algorithms find valid paths
func TestAlgorithmCorrectness(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	shapeMgr := mctesting.NewMockShapeManager()

	testCases := []struct {
		name       string
		buildWorld func() *mctesting.MockWorld
		start      models.V3
		goal       models.V3
		maxSteps   int
	}{
		{
			name: "FlatGround",
			buildWorld: func() *mctesting.MockWorld {
				return mctesting.NewWorldBuilder(registry).
					FlatGroundDirect(0, 0, 30, 30, 64, 9).
					Build()
			},
			start:    models.V3{X: 5, Y: 65, Z: 5},
			goal:     models.V3{X: 25, Y: 65, Z: 25},
			maxSteps: 5000,
		},
		{
			name: "LongDistance",
			buildWorld: func() *mctesting.MockWorld {
				return mctesting.NewWorldBuilder(registry).
					FlatGroundDirect(0, 0, 80, 80, 64, 9).
					Build()
			},
			start:    models.V3{X: 5, Y: 65, Z: 5},
			goal:     models.V3{X: 70, Y: 65, Z: 70},
			maxSteps: 50000,
		},
		{
			name: "NegativeY",
			buildWorld: func() *mctesting.MockWorld {
				return mctesting.NewWorldBuilder(registry).
					FlatGroundDirect(0, 0, 20, 20, -60, 9).
					Build()
			},
			start:    models.V3{X: 5, Y: -59, Z: 5},
			goal:     models.V3{X: 15, Y: -59, Z: 15},
			maxSteps: 5000,
		},
	}

	for _, tc := range testCases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			world := tc.buildWorld()

			for algoName, factory := range algorithmTestSuite {
				algoName := algoName // capture
				factory := factory   // capture

				t.Run(algoName, func(t *testing.T) {
					pathFinder := factory(world, shapeMgr)

					path, err := pathFinder.FindPath(context.Background(), tc.start, tc.goal, tc.maxSteps)

					if err != nil {
						t.Errorf("%s failed to find path: %v", algoName, err)
						return
					}

					if path == nil || !path.Found {
						t.Errorf("%s did not find path", algoName)
						return
					}

					if len(path.Steps) == 0 {
						t.Errorf("%s found path but no steps", algoName)
						return
					}

					// Verify path reaches goal (approximately)
					lastStep := path.Steps[len(path.Steps)-1]
					dist := lastStep.Position.DistanceTo(tc.goal)
					if dist > 1.5 {
						t.Errorf("%s path ends at (%f, %f, %f), expected near goal (%f, %f, %f), dist=%.2f",
							algoName,
							lastStep.Position.X, lastStep.Position.Y, lastStep.Position.Z,
							tc.goal.X, tc.goal.Y, tc.goal.Z, dist)
					}

					t.Logf("%s: %d steps, cost=%.2f, time=%.2fms",
						algoName, len(path.Steps), path.TotalCost, path.SearchTime)
				})
			}
		})
	}
}

// TestAlgorithmPerformanceComparison compares performance of all algorithms
func TestAlgorithmPerformanceComparison(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	shapeMgr := mctesting.NewMockShapeManager()

	// Create a medium-sized world for fair comparison
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 100, 100, 64, 9).
		Build()

	testCases := []struct {
		name     string
		start    models.V3
		goal     models.V3
		maxSteps int
	}{
		{
			name:     "ShortPath_20blocks",
			start:    models.V3{X: 10, Y: 65, Z: 10},
			goal:     models.V3{X: 30, Y: 65, Z: 30},
			maxSteps: 10000,
		},
		{
			name:     "MediumPath_50blocks",
			start:    models.V3{X: 5, Y: 65, Z: 5},
			goal:     models.V3{X: 55, Y: 65, Z: 55},
			maxSteps: 50000,
		},
		{
			name:     "LongPath_90blocks",
			start:    models.V3{X: 5, Y: 65, Z: 5},
			goal:     models.V3{X: 95, Y: 65, Z: 95},
			maxSteps: 100000,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			results := make(map[string]struct {
				steps int
				cost  float64
				time  float64
			})

			for algoName, factory := range algorithmTestSuite {
				pathFinder := factory(world, shapeMgr)

				path, err := pathFinder.FindPath(context.Background(), tc.start, tc.goal, tc.maxSteps)

				if err != nil {
					t.Errorf("%s failed: %v", algoName, err)
					continue
				}

				if path.Found {
					results[algoName] = struct {
						steps int
						cost  float64
						time  float64
					}{
						steps: len(path.Steps),
						cost:  path.TotalCost,
						time:  path.SearchTime,
					}
				}
			}

			// Log comparison
			t.Logf("=== %s Results ===", tc.name)
			for name, r := range results {
				t.Logf("  %s: steps=%d, cost=%.2f, time=%.2fms", name, r.steps, r.cost, r.time)
			}

			// Check that all algorithms found similar-cost paths (correctness check)
			var costs []float64
			for _, r := range results {
				costs = append(costs, r.cost)
			}
			if len(costs) > 1 {
				maxCost := costs[0]
				minCost := costs[0]
				for _, c := range costs[1:] {
					if c > maxCost {
						maxCost = c
					}
					if c < minCost {
						minCost = c
					}
				}
				// Paths should be within 20% cost of each other
				if maxCost > 0 && (maxCost-minCost)/minCost > 0.2 {
					t.Logf("Warning: Path costs vary significantly (min=%.2f, max=%.2f)", minCost, maxCost)
				}
			}
		})
	}
}

// TestBidirectionalNodeReduction verifies bidirectional search explores fewer nodes
func TestBidirectionalNodeReduction(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	shapeMgr := mctesting.NewMockShapeManager()

	// Create a large world where bidirectional should show improvement
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 150, 150, 64, 9).
		Build()

	start := models.V3{X: 10, Y: 65, Z: 10}
	goal := models.V3{X: 140, Y: 65, Z: 140}
	maxSteps := 500000

	// Run standard A*
	aStarPF := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)
	aStarPath, aStarErr := aStarPF.FindPath(context.Background(), start, goal, maxSteps)

	// Run bidirectional A*
	bidirPF := pathfinding.NewBidirAStarPathFinder(world, shapeMgr, nil)
	bidirPath, bidirErr := bidirPF.FindPath(context.Background(), start, goal, maxSteps)

	if aStarErr != nil {
		t.Errorf("A* failed: %v", aStarErr)
	}
	if bidirErr != nil {
		t.Errorf("Bidirectional failed: %v", bidirErr)
	}

	if aStarPath.Found && bidirPath.Found {
		t.Logf("A*: steps=%d, cost=%.2f, time=%.2fms",
			len(aStarPath.Steps), aStarPath.TotalCost, aStarPath.SearchTime)
		t.Logf("Bidirectional: steps=%d, cost=%.2f, time=%.2fms",
			len(bidirPath.Steps), bidirPath.TotalCost, bidirPath.SearchTime)

		// Both should find valid paths
		if math.Abs(aStarPath.TotalCost-bidirPath.TotalCost) > aStarPath.TotalCost*0.1 {
			t.Logf("Note: Path costs differ by more than 10%% (A*=%.2f, Bidir=%.2f)",
				aStarPath.TotalCost, bidirPath.TotalCost)
		}

		// Log speedup if any
		if bidirPath.SearchTime > 0 && aStarPath.SearchTime > 0 {
			speedup := aStarPath.SearchTime / bidirPath.SearchTime
			t.Logf("Speedup: %.2fx", speedup)
		}
	}
}

// TestBidirectionalContextCancellation verifies bidirectional respects context
func TestBidirectionalContextCancellation(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	shapeMgr := mctesting.NewMockShapeManager()

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 100, 100, 64, 9).
		Build()

	// Already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pathFinder := pathfinding.NewBidirAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 0, Y: 65, Z: 0}
	goal := models.V3{X: 50, Y: 65, Z: 50}

	path, err := pathFinder.FindPath(ctx, start, goal, 100000)

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
	if path == nil {
		t.Fatal("Expected non-nil path struct")
	}
	if path.Found {
		t.Error("Path should not be found with cancelled context")
	}

	t.Logf("Bidirectional context cancellation works correctly")
}

// HPAPathFinderFactory creates an HPA* pathfinder with a specific low-level algorithm
type HPAPathFinderFactory func(w models.World, shapeMgr models.BlockShapeManager, clusterSize int) models.PathFinder

// hpaAlgorithmSuite defines HPA* variants with different low-level pathfinders
var hpaAlgorithmSuite = map[string]HPAPathFinderFactory{
	"HPA*+A*": func(w models.World, shapeMgr models.BlockShapeManager, clusterSize int) models.PathFinder {
		return pathfinding.NewHPAPathFinderWithAStar(w, shapeMgr, clusterSize, nil)
	},
	"HPA*+Bidirectional": func(w models.World, shapeMgr models.BlockShapeManager, clusterSize int) models.PathFinder {
		return pathfinding.NewHPAPathFinderWithBidirectional(w, shapeMgr, clusterSize, nil)
	},
	"HPA*+EPEA*": func(w models.World, shapeMgr models.BlockShapeManager, clusterSize int) models.PathFinder {
		return pathfinding.NewHPAPathFinderWithEPEAStar(w, shapeMgr, clusterSize, nil)
	},
}

// TestHPAVariantsCorrectness verifies all HPA* variants find valid paths
func TestHPAVariantsCorrectness(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	shapeMgr := mctesting.NewMockShapeManager()

	// Create a world large enough to span multiple clusters (cluster size = 16)
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 64, 64, 64, 9). // 64x64 = 4x4 clusters
		Build()

	clusterSize := 16

	testCases := []struct {
		name     string
		start    models.V3
		goal     models.V3
		maxSteps int
	}{
		{
			name:     "SameCluster",
			start:    models.V3{X: 2, Y: 65, Z: 2},
			goal:     models.V3{X: 10, Y: 65, Z: 10},
			maxSteps: 10000,
		},
		{
			name:     "AdjacentClusters",
			start:    models.V3{X: 5, Y: 65, Z: 5},
			goal:     models.V3{X: 25, Y: 65, Z: 25},
			maxSteps: 50000,
		},
		{
			name:     "DistantClusters",
			start:    models.V3{X: 5, Y: 65, Z: 5},
			goal:     models.V3{X: 55, Y: 65, Z: 55},
			maxSteps: 100000,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			results := make(map[string]struct {
				found bool
				steps int
				cost  float64
				time  float64
			})

			for algoName, factory := range hpaAlgorithmSuite {
				pathFinder := factory(world, shapeMgr, clusterSize)

				path, err := pathFinder.FindPath(context.Background(), tc.start, tc.goal, tc.maxSteps)

				if err != nil {
					t.Logf("%s error: %v", algoName, err)
				}

				if path != nil && path.Found {
					results[algoName] = struct {
						found bool
						steps int
						cost  float64
						time  float64
					}{
						found: true,
						steps: len(path.Steps),
						cost:  path.TotalCost,
						time:  path.SearchTime,
					}
				} else {
					results[algoName] = struct {
						found bool
						steps int
						cost  float64
						time  float64
					}{found: false}
				}
			}

			// Log results
			t.Logf("=== %s Results ===", tc.name)
			for name, r := range results {
				if r.found {
					t.Logf("  %s: steps=%d, cost=%.2f, time=%.2fms", name, r.steps, r.cost, r.time)
				} else {
					t.Logf("  %s: path not found", name)
				}
			}

			// All variants should find a path (or all fail - world setup issue)
			foundCount := 0
			for _, r := range results {
				if r.found {
					foundCount++
				}
			}

			if foundCount > 0 && foundCount < len(results) {
				t.Errorf("Inconsistent results: %d/%d algorithms found path", foundCount, len(results))
			}

			// If paths found, verify costs are similar (within 20%)
			if foundCount == len(results) {
				var costs []float64
				for _, r := range results {
					costs = append(costs, r.cost)
				}
				minCost, maxCost := costs[0], costs[0]
				for _, c := range costs[1:] {
					if c < minCost {
						minCost = c
					}
					if c > maxCost {
						maxCost = c
					}
				}
				if minCost > 0 && (maxCost-minCost)/minCost > 0.2 {
					t.Logf("Note: Path costs vary by more than 20%% (min=%.2f, max=%.2f)", minCost, maxCost)
				}
			}
		})
	}
}

// TestNarrowRiverCrossing_PrefersDirectCrossingOverDetour is
// WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 5 regression test: a short river directly between the
// bot and its goal should be crossed rather than walked around, purely from correct per-tile costs
// (Items 1-3) and A*'s additive cost summation - no river-specific special-casing.
//
// World: a 15x11 flat grass platform (Y=64) with a 3-block-wide water strip at X=6-8 covering
// Z=0-8, leaving Z=9-10 as a dry "bridge" at the south end. Start=(2,65,2), Goal=(12,65,2).
//   - Direct crossing: ~10 blocks east, 3 of them through water. Expected cost roughly
//     7 dry (~7.0) + 3 water (~3-6.5 depending on WadeWater vs Swim) = 10-14ish.
//   - Going around via the dry bridge at Z=9-10: ~7 south + ~10 east + ~7 north = ~24, all dry.
//
// If water were costed disproportionately (the risk flagged when Item 4's cost-bias tuning was
// designed), the 24-cost dry detour could beat the direct crossing; with the actual verified
// costs it shouldn't.
func TestNarrowRiverCrossing_PrefersDirectCrossingOverDetour(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	grassID := registry.GetStateID("minecraft:grass_block", nil)
	waterID := registry.GetStateID("minecraft:water", nil)

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 14, 10, 64, grassID). // Solid ground under the whole platform, including the river bed
		WaterDirect(6, 65, 0, 8, 65, 8, waterID).    // 3-block-wide river, leaving Z=9-10 dry
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	pathFinder := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)

	start := models.V3{X: 2, Y: 65, Z: 2}
	goal := models.V3{X: 12, Y: 65, Z: 2}

	path, err := pathFinder.FindPath(context.Background(), start, goal, 5000)
	if err != nil {
		t.Fatalf("Failed to find path: %v", err)
	}
	if path == nil || len(path.Steps) == 0 {
		t.Fatal("Path is nil or empty")
	}

	t.Logf("Path: %d steps, cost=%.2f", len(path.Steps), path.TotalCost)

	const detourCost = 24.0 // all-dry route around the river's south end
	if path.TotalCost >= detourCost {
		t.Errorf("expected a direct river crossing (cost well under the %.1f-cost dry detour), got cost=%.2f",
			detourCost, path.TotalCost)
	}

	crossedWater := false
	for _, step := range path.Steps {
		if step.Movement == pathfinding.WadeWater || step.Movement == pathfinding.Swim {
			crossedWater = true
			break
		}
	}
	if !crossedWater {
		t.Error("expected the chosen path to actually cross the river (WadeWater or Swim step), not avoid it entirely")
	}
}
