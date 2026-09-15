package pathfinding_test

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
	"github.com/stretchr/testify/require"
)

// TestCanSwim_SolidToWater tests entering water horizontally from solid ground.
func TestCanSwim_SolidToWater(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	waterID := registry.GetStateID("minecraft:water", nil)
	stoneID := registry.GetStateID("minecraft:stone", nil)

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 10, 10, 64, stoneID). // Stone floor at Y=64
		WaterDirect(1, 65, 2, 4, 66, 8, waterID).    // Water pool from X=1-4, Y=65-66, Z=2-8
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	validator := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	testCases := []struct {
		name     string
		from     models.V3
		to       models.V3
		expected bool
	}{
		{
			name:     "stone to water same level",
			from:     models.V3{X: 0, Y: 65, Z: 5},
			to:       models.V3{X: 1, Y: 65, Z: 5},
			expected: true,
		},
		{
			name:     "water to water horizontal",
			from:     models.V3{X: 2, Y: 65, Z: 5},
			to:       models.V3{X: 3, Y: 65, Z: 5},
			expected: true,
		},
		{
			name:     "water to air rejected",
			from:     models.V3{X: 3, Y: 65, Z: 5},
			to:       models.V3{X: 5, Y: 65, Z: 5},
			expected: false,
		},
		{
			name:     "vertical movement rejected by CanSwim",
			from:     models.V3{X: 2, Y: 65, Z: 5},
			to:       models.V3{X: 2, Y: 66, Z: 5},
			expected: false,
		},
		{
			name:     "diagonal water swim",
			from:     models.V3{X: 2, Y: 65, Z: 5},
			to:       models.V3{X: 3, Y: 65, Z: 6},
			expected: true,
		},
		{
			name:     "out of bounds to air",
			from:     models.V3{X: 1, Y: 65, Z: 5},
			to:       models.V3{X: 2, Y: 65, Z: 1},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.CanSwim(tc.from, tc.to)
			if result != tc.expected {
				// Debug: check what blocks are at these positions
				fromBlock, _ := world.GetBlockAt(tc.from.X, tc.from.Y, tc.from.Z)
				toBlock, _ := world.GetBlockAt(tc.to.X, tc.to.Y, tc.to.Z)
				t.Logf("Debug: from block=%d (water=%v), to block=%d (water=%v)",
					fromBlock, shapeMgr.IsWater(fromBlock),
					toBlock, shapeMgr.IsWater(toBlock))
			}
			require.Equal(t, tc.expected, result,
				"CanSwim(%v, %v) expected %v but got %v",
				tc.from, tc.to, tc.expected, result)
		})
	}
}

// TestCanSwimUp_SolidToWater tests entering water while moving upward from solid ground.
func TestCanSwimUp_SolidToWater(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	waterID := registry.GetStateID("minecraft:water", nil)
	stoneID := registry.GetStateID("minecraft:stone", nil)

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 10, 10, 64, stoneID). // Stone floor at Y=64
		WaterDirect(1, 65, 2, 4, 66, 8, waterID).    // Water from Y=65-66 only
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	validator := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	testCases := []struct {
		name     string
		from     models.V3
		to       models.V3
		expected bool
	}{
		{
			name:     "stone upward into water",
			from:     models.V3{X: 1, Y: 65, Z: 5},
			to:       models.V3{X: 1, Y: 66, Z: 5},
			expected: true,
		},
		{
			name:     "water upward in water",
			from:     models.V3{X: 2, Y: 65, Z: 5},
			to:       models.V3{X: 2, Y: 66, Z: 5},
			expected: true,
		},
		{
			name:     "upward to air rejected",
			from:     models.V3{X: 1, Y: 67, Z: 5},
			to:       models.V3{X: 1, Y: 68, Z: 5},
			expected: false,
		},
		{
			name:     "upward adjacent is allowed by CanSwimUp",
			from:     models.V3{X: 1, Y: 65, Z: 5},
			to:       models.V3{X: 2, Y: 66, Z: 5},
			expected: true, // CanSwimUp allows same horizontal OR adjacent (dx^2+dz^2 <= 1)
		},
		{
			name:     "upward diagonal too far",
			from:     models.V3{X: 1, Y: 65, Z: 5},
			to:       models.V3{X: 3, Y: 66, Z: 5},
			expected: false, // dx=2, dy=1, dz=0 -> dx^2+dz^2 = 4 > 1, rejected
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.CanSwimUp(tc.from, tc.to)
			require.Equal(t, tc.expected, result,
				"CanSwimUp(%v, %v) expected %v but got %v",
				tc.from, tc.to, tc.expected, result)
		})
	}
}

// TestCanSwimDown_SolidToWater tests entering water while moving downward from solid ground.
func TestCanSwimDown_SolidToWater(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	waterID := registry.GetStateID("minecraft:water", nil)
	stoneID := registry.GetStateID("minecraft:stone", nil)

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 10, 10, 62, stoneID). // Stone floor at Y=62
		WaterDirect(1, 60, 2, 4, 65, 8, waterID).    // Water from Y=60-65
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	validator := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	testCases := []struct {
		name     string
		from     models.V3
		to       models.V3
		expected bool
	}{
		{
			name:     "stone downward 1 block into water",
			from:     models.V3{X: 1, Y: 65, Z: 5},
			to:       models.V3{X: 1, Y: 64, Z: 5},
			expected: true,
		},
		{
			name:     "stone downward 2 blocks into water",
			from:     models.V3{X: 1, Y: 65, Z: 5},
			to:       models.V3{X: 1, Y: 63, Z: 5},
			expected: true,
		},
		{
			name:     "water downward in water",
			from:     models.V3{X: 2, Y: 64, Z: 5},
			to:       models.V3{X: 2, Y: 63, Z: 5},
			expected: true,
		},
		{
			name:     "downward to air rejected",
			from:     models.V3{X: 1, Y: 60, Z: 5},
			to:       models.V3{X: 1, Y: 59, Z: 5},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.CanSwimDown(tc.from, tc.to)
			require.Equal(t, tc.expected, result,
				"CanSwimDown(%v, %v) expected %v but got %v",
				tc.from, tc.to, tc.expected, result)
		})
	}
}

// TestCanTraverse_RejectsWater verifies a water tile (even directly over solid ground)
// is no longer treated as a free/dry-land Traverse move - it must go through
// CanWadeWater/CanSwim instead, which carry the correct (higher) cost.
func TestCanTraverse_RejectsWater(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	waterID := registry.GetStateID("minecraft:water", nil)
	stoneID := registry.GetStateID("minecraft:stone", nil)

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 10, 10, 64, stoneID). // Stone floor at Y=64
		WaterDirect(2, 65, 2, 2, 65, 2, waterID).    // Single 1-block-deep puddle over solid ground
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	validator := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	dryFrom := models.V3{X: 0, Y: 65, Z: 5}
	dryTo := models.V3{X: 1, Y: 65, Z: 5}
	require.True(t, validator.CanTraverse(dryFrom, dryTo), "dry ground should still Traverse")

	puddleFrom := models.V3{X: 1, Y: 65, Z: 2}
	puddleTo := models.V3{X: 2, Y: 65, Z: 2}
	require.False(t, validator.CanTraverse(puddleFrom, puddleTo),
		"a puddle over solid ground must not be a free Traverse move")
	require.True(t, validator.CanWadeWater(puddleFrom, puddleTo),
		"the same puddle move should be legal as WadeWater")
}

// TestCanWadeWater_AnyDepthRegardlessOfGroundSupport verifies wading is available both
// over solid ground and over open/deep water, per the verified real-game finding that
// ground support doesn't affect wade speed (see WATER_TRAVERSAL_PATHFINDING_PLAN.md).
func TestCanWadeWater_AnyDepthRegardlessOfGroundSupport(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	waterID := registry.GetStateID("minecraft:water", nil)
	stoneID := registry.GetStateID("minecraft:stone", nil)

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 10, 10, 60, stoneID). // Deep floor, far below the water
		WaterDirect(0, 65, 0, 5, 68, 5, waterID).    // Deep open water column, no nearby floor
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	validator := pathfinding.NewMovementValidator(world, shapeMgr, nil)

	from := models.V3{X: 1, Y: 65, Z: 1}
	to := models.V3{X: 2, Y: 65, Z: 1}
	require.True(t, validator.CanWadeWater(from, to),
		"wading should be legal in deep open water with no ground support below")

	diagFrom := models.V3{X: 1, Y: 65, Z: 1}
	diagTo := models.V3{X: 2, Y: 65, Z: 2}
	require.True(t, validator.CanDiagonalWadeWater(diagFrom, diagTo),
		"diagonal wading should be legal in deep open water with no ground support below")
}
