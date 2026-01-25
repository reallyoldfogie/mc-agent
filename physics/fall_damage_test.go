package physics

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	testStoneBlockID uint32 = 1
	testGrassBlockID uint32 = 3
)

func TestCalculateFallDamage_SafeDistance(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	shapeProvider.SetPassable(testStoneBlockID, false)

	tests := []struct {
		name         string
		fallDistance float64
		expectDamage float64
	}{
		{
			name:         "No fall",
			fallDistance: 0.0,
			expectDamage: 0.0,
		},
		{
			name:         "1 block fall (safe)",
			fallDistance: 1.0,
			expectDamage: 0.0,
		},
		{
			name:         "3 blocks fall (at safe limit)",
			fallDistance: 3.0,
			expectDamage: 0.0,
		},
		{
			name:         "2.5 blocks fall (safe)",
			fallDistance: 2.5,
			expectDamage: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			damage := CalculateFallDamage(tt.fallDistance, testStoneBlockID, shapeProvider)
			assert.Equal(t, tt.expectDamage, damage, "Safe falls should deal no damage")
		})
	}
}

func TestCalculateFallDamage_DangerousDistance(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	shapeProvider.SetPassable(testStoneBlockID, false)

	tests := []struct {
		name         string
		fallDistance float64
		expectDamage float64
	}{
		{
			name:         "4 blocks fall (1 damage)",
			fallDistance: 4.0,
			expectDamage: 1.0,
		},
		{
			name:         "5 blocks fall (2 damage)",
			fallDistance: 5.0,
			expectDamage: 2.0,
		},
		{
			name:         "10 blocks fall (7 damage)",
			fallDistance: 10.0,
			expectDamage: 7.0,
		},
		{
			name:         "23 blocks fall (20 damage - lethal)",
			fallDistance: 23.0,
			expectDamage: 20.0,
		},
		{
			name:         "3.5 blocks fall (0.5 damage)",
			fallDistance: 3.5,
			expectDamage: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			damage := CalculateFallDamage(tt.fallDistance, testStoneBlockID, shapeProvider)
			assert.InDelta(t, tt.expectDamage, damage, 0.01, "Damage should match formula: max(0, distance - 3)")
		})
	}
}

func TestCalculateFallDamage_WaterLanding(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	// BlockWater is defined in state_test.go as constant 2

	tests := []struct {
		name         string
		fallDistance float64
	}{
		{"Water from 1 block", 1.0},
		{"Water from 10 blocks", 10.0},
		{"Water from 50 blocks", 50.0},
		{"Water from 100 blocks", 100.0},
		{"Water from 256 blocks", 256.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			damage := CalculateFallDamage(tt.fallDistance, BlockWater, shapeProvider)
			assert.Equal(t, 0.0, damage, "Water should negate ALL fall damage")
		})
	}
}

func TestCalculateFallDamage_DamageFormula(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	shapeProvider.SetPassable(testStoneBlockID, false)

	// Test exact formula: max(0, fallDistance - SafeFallDistance) * DamagePerBlock
	for fallDistance := 0.0; fallDistance <= 30.0; fallDistance += 0.5 {
		damage := CalculateFallDamage(fallDistance, testStoneBlockID, shapeProvider)
		expected := math.Max(0, fallDistance-SafeFallDistance) * DamagePerBlock
		assert.InDelta(t, expected, damage, 0.001,
			"Damage formula should match for fall distance %.1f", fallDistance)
	}
}

func TestGetDamageReduction(t *testing.T) {
	shapeProvider := newMockShapeProvider()

	// Test normal blocks (no reduction)
	tests := []struct {
		name     string
		blockID  uint32
		expected float64
	}{
		{"Stone block", 1, 0.0},
		{"Grass block", 2, 0.0},
		{"Unknown block", 999, 0.0},
		{"Block 0 (air)", 0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reduction := GetDamageReduction(tt.blockID, shapeProvider)
			assert.Equal(t, tt.expected, reduction,
				"Normal blocks should have no damage reduction")
		})
	}
}

func TestGetDamageReduction_SpecialBlocks(t *testing.T) {
	shapeProvider := newMockShapeProvider()

	hayBaleID := uint32(10)
	bedID := uint32(11)
	honeyBlockID := uint32(12)
	slimeBlockID := uint32(13)
	powderSnowID := uint32(14)

	shapeProvider.SetHayBale(hayBaleID, true)
	shapeProvider.SetBed(bedID, true)
	shapeProvider.SetHoneyBlock(honeyBlockID, true)
	shapeProvider.SetSlimeBlock(slimeBlockID, true)
	shapeProvider.SetPowderSnow(powderSnowID, true)

	tests := []struct {
		name     string
		blockID  uint32
		expected float64
	}{
		{"Hay bale (80% reduction)", hayBaleID, HayBaleReduction},
		{"Bed (50% reduction)", bedID, BedReduction},
		{"Honey block (80% reduction)", honeyBlockID, HoneyBlockReduction},
		{"Slime block (100% reduction)", slimeBlockID, 1.0},
		{"Powder snow (100% reduction)", powderSnowID, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reduction := GetDamageReduction(tt.blockID, shapeProvider)
			assert.Equal(t, tt.expected, reduction)
		})
	}
}

func TestCalculateFallDamage_HayBaleReduction(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	hayBaleID := uint32(10)
	shapeProvider.SetPassable(hayBaleID, false)
	shapeProvider.SetHayBale(hayBaleID, true)

	tests := []struct {
		fallDistance   float64
		expectedDamage float64
	}{
		{10.0, 1.4}, // (10-3) * 1.0 * (1-0.8) = 7 * 0.2 = 1.4
		{23.0, 4.0}, // (23-3) * 1.0 * (1-0.8) = 20 * 0.2 = 4.0
		{4.0, 0.2},  // (4-3) * 1.0 * (1-0.8) = 1 * 0.2 = 0.2
	}

	for _, tt := range tests {
		damage := CalculateFallDamage(tt.fallDistance, hayBaleID, shapeProvider)
		assert.InDelta(t, tt.expectedDamage, damage, 0.01,
			"Hay bale should reduce damage by 80%%")
	}
}

func TestCalculateFallDamage_BedReduction(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	bedID := uint32(11)
	shapeProvider.SetPassable(bedID, false)
	shapeProvider.SetBed(bedID, true)

	tests := []struct {
		fallDistance   float64
		expectedDamage float64
	}{
		{10.0, 3.5},  // (10-3) * 1.0 * (1-0.5) = 7 * 0.5 = 3.5
		{23.0, 10.0}, // (23-3) * 1.0 * (1-0.5) = 20 * 0.5 = 10.0
		{4.0, 0.5},   // (4-3) * 1.0 * (1-0.5) = 1 * 0.5 = 0.5
	}

	for _, tt := range tests {
		damage := CalculateFallDamage(tt.fallDistance, bedID, shapeProvider)
		assert.InDelta(t, tt.expectedDamage, damage, 0.01,
			"Bed should reduce damage by 50%%")
	}
}

func TestCalculateFallDamage_SlimeBlock(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	slimeBlockID := uint32(13)
	shapeProvider.SetPassable(slimeBlockID, false)
	shapeProvider.SetSlimeBlock(slimeBlockID, true)

	tests := []struct {
		fallDistance   float64
		expectedDamage float64
	}{
		{10.0, 0.0},  // Slime blocks = 0 damage
		{23.0, 0.0},  // Any distance
		{100.0, 0.0}, // Even extreme falls
	}

	for _, tt := range tests {
		damage := CalculateFallDamage(tt.fallDistance, slimeBlockID, shapeProvider)
		assert.Equal(t, tt.expectedDamage, damage,
			"Slime block should negate all fall damage")
	}
}

func TestIsWaterBlock(t *testing.T) {
	shapeProvider := newMockShapeProvider()

	tests := []struct {
		name     string
		blockID  uint32
		expected bool
	}{
		{"Water block", BlockWater, true},
		{"Stone block", testStoneBlockID, false},
		{"Unknown block (air)", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isWater := IsWaterBlock(tt.blockID, shapeProvider)
			assert.Equal(t, tt.expected, isWater)
		})
	}
}

func TestIsSafeLanding_SafeCases(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	shapeProvider.SetPassable(testStoneBlockID, false)

	tests := []struct {
		name         string
		fallDistance float64
		blockID      uint32
		expectSafe   bool
	}{
		{"No fall", 0.0, testStoneBlockID, true},
		{"Safe distance", 3.0, testStoneBlockID, true},
		{"Small damage (1 heart)", 4.0, testStoneBlockID, true},
		{"Moderate damage (1.9 hearts)", 4.9, testStoneBlockID, true},
		{"High damage (2 hearts)", 5.0, testStoneBlockID, false},
		{"Lethal fall", 23.0, testStoneBlockID, false},
		{"Water from any height", 100.0, BlockWater, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe := IsSafeLanding(tt.fallDistance, tt.blockID, shapeProvider)
			assert.Equal(t, tt.expectSafe, safe)
		})
	}
}

func TestPredictFallDistance_WithGround(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	// Set up ground at Y=64
	for x := 0; x < 5; x++ {
		for z := 0; z < 5; z++ {
			world.SetBlock(x, 64, z, testStoneBlockID)
		}
	}

	shapeProvider.SetPassable(testStoneBlockID, false)

	tests := []struct {
		name             string
		startY           float64
		expectedFall     float64
		expectedLandingY float64
	}{
		{"From Y=70", 70.0, 5.0, 65.0},
		{"From Y=80", 80.0, 15.0, 65.0},
		{"From Y=65", 65.0, 0.0, 65.0},
		{"From Y=100", 100.0, 35.0, 65.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startPos := V3{X: 2.5, Y: tt.startY, Z: 2.5}
			fall, landingY := PredictFallDistance(startPos, world, shapeProvider)

			assert.InDelta(t, tt.expectedFall, fall, 0.1, "Fall distance should match")
			assert.InDelta(t, tt.expectedLandingY, landingY, 0.1, "Landing Y should match")
		})
	}
}

func TestPredictFallDistance_NoGround(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	// No solid blocks, all air (passable)
	startPos := V3{X: 0, Y: 64, Z: 0}
	fall, landingY := PredictFallDistance(startPos, world, shapeProvider)

	assert.Equal(t, 256.0, fall, "Should return max fall distance when no ground")
	assert.Equal(t, -256.0, landingY, "Should return void Y when no ground")
}

func TestGetLandingBlock(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	// Set up ground layers
	world.SetBlock(0, 64, 0, testStoneBlockID)
	world.SetBlock(0, 65, 0, testGrassBlockID)

	shapeProvider.SetPassable(testStoneBlockID, false)
	shapeProvider.SetPassable(testGrassBlockID, false)

	tests := []struct {
		name            string
		pos             V3
		expectedBlockID uint32
	}{
		{
			name:            "Above grass layer",
			pos:             V3{X: 0, Y: 70, Z: 0},
			expectedBlockID: testGrassBlockID,
		},
		{
			name:            "Just above grass",
			pos:             V3{X: 0, Y: 66, Z: 0},
			expectedBlockID: testGrassBlockID,
		},
		{
			name:            "No ground below",
			pos:             V3{X: 10, Y: 70, Z: 10},
			expectedBlockID: 0, // Air
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockID := GetLandingBlock(tt.pos, world, shapeProvider)
			assert.Equal(t, tt.expectedBlockID, blockID)
		})
	}
}

func TestCalculateFallDamageForDrop(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	world.SetBlock(0, 60, 0, testStoneBlockID)
	// Water at Y=60 with solid ground below at Y=59
	world.SetBlock(5, 60, 0, BlockWater)
	world.SetBlock(5, 59, 0, testStoneBlockID)

	shapeProvider.SetPassable(testStoneBlockID, false)
	shapeProvider.SetPassable(BlockWater, true) // Water is passable

	tests := []struct {
		name           string
		from           V3
		to             V3
		expectedDamage float64
	}{
		{
			name:           "Drop 5 blocks onto stone",
			from:           V3{X: 0, Y: 66, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedDamage: 2.0, // 5 - 3 safe distance
		},
		{
			name:           "Drop 10 blocks onto stone",
			from:           V3{X: 0, Y: 71, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedDamage: 7.0, // 10 - 3 safe distance
		},
		{
			name:           "Drop 100 blocks into water",
			from:           V3{X: 5, Y: 161, Z: 0},
			to:             V3{X: 5, Y: 61, Z: 0},
			expectedDamage: 0.0, // Water negates all damage
		},
		{
			name:           "No fall (going up)",
			from:           V3{X: 0, Y: 61, Z: 0},
			to:             V3{X: 0, Y: 66, Z: 0},
			expectedDamage: 0.0,
		},
		{
			name:           "Short safe fall",
			from:           V3{X: 0, Y: 63, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedDamage: 0.0, // 2 blocks < 3 safe distance
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			damage := CalculateFallDamageForDrop(tt.from, tt.to, world, shapeProvider)
			assert.InDelta(t, tt.expectedDamage, damage, 0.1)
		})
	}
}

func TestIsWaterDrop(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	world.SetBlock(0, 60, 0, testStoneBlockID)
	// Water at Y=60 with solid ground below at Y=59
	world.SetBlock(5, 60, 0, BlockWater)
	world.SetBlock(5, 59, 0, testStoneBlockID)

	shapeProvider.SetPassable(testStoneBlockID, false)
	shapeProvider.SetPassable(BlockWater, true)

	tests := []struct {
		name        string
		from        V3
		to          V3
		expectWater bool
	}{
		{
			name:        "Drop into water",
			from:        V3{X: 5, Y: 100, Z: 0},
			to:          V3{X: 5, Y: 61, Z: 0},
			expectWater: true,
		},
		{
			name:        "Drop onto stone",
			from:        V3{X: 0, Y: 100, Z: 0},
			to:          V3{X: 0, Y: 61, Z: 0},
			expectWater: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isWater := IsWaterDrop(tt.from, tt.to, world, shapeProvider)
			assert.Equal(t, tt.expectWater, isWater)
		})
	}
}

func TestGetDropSafety(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	world.SetBlock(0, 60, 0, testStoneBlockID)
	// Water at Y=60 with solid ground below at Y=59
	world.SetBlock(5, 60, 0, BlockWater)
	world.SetBlock(5, 59, 0, testStoneBlockID)

	shapeProvider.SetPassable(testStoneBlockID, false)
	shapeProvider.SetPassable(BlockWater, true)

	currentHealth := 20.0 // Full health (10 hearts)

	tests := []struct {
		name           string
		from           V3
		to             V3
		expectedSafety DropSafety
	}{
		{
			name:           "No fall (same level)",
			from:           V3{X: 0, Y: 61, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedSafety: DropSafeNoFall,
		},
		{
			name:           "Going up",
			from:           V3{X: 0, Y: 61, Z: 0},
			to:             V3{X: 0, Y: 70, Z: 0},
			expectedSafety: DropSafeNoFall,
		},
		{
			name:           "Water landing from any height",
			from:           V3{X: 5, Y: 161, Z: 0},
			to:             V3{X: 5, Y: 61, Z: 0},
			expectedSafety: DropSafeWater,
		},
		{
			name:           "Short fall (1 heart damage)",
			from:           V3{X: 0, Y: 65, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedSafety: DropSafeShortFall, // 4 blocks = 1 heart < 2 hearts
		},
		{
			name:           "Dangerous fall (5 hearts damage)",
			from:           V3{X: 0, Y: 69, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedSafety: DropDangerous, // 8 blocks = 5 hearts < 10 hearts (50% health)
		},
		{
			name:           "Lethal fall (15 hearts damage)",
			from:           V3{X: 0, Y: 79, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedSafety: DropLethal, // 18 blocks = 15 hearts > 10 hearts (50% health)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safety := GetDropSafety(tt.from, tt.to, currentHealth, world, shapeProvider)
			assert.Equal(t, tt.expectedSafety, safety)
		})
	}
}

func TestGetDropSafety_LowHealth(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	world.SetBlock(0, 60, 0, testStoneBlockID)
	shapeProvider.SetPassable(testStoneBlockID, false)

	lowHealth := 4.0 // 2 hearts

	tests := []struct {
		name           string
		from           V3
		to             V3
		expectedSafety DropSafety
	}{
		{
			name:           "3 heart damage with 2 hearts health",
			from:           V3{X: 0, Y: 67, Z: 0}, // 6 blocks = 3 hearts damage
			to:             V3{X: 0, Y: 61, Z: 0},
			expectedSafety: DropLethal, // 3 > 2 (50% of 4)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safety := GetDropSafety(tt.from, tt.to, lowHealth, world, shapeProvider)
			assert.Equal(t, tt.expectedSafety, safety)
		})
	}
}

func TestPathfindingDropCost_NoFall(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	baseMoveCost := 1.0

	tests := []struct {
		name         string
		from         V3
		to           V3
		expectedCost float64
	}{
		{
			name:         "Same level",
			from:         V3{X: 0, Y: 64, Z: 0},
			to:           V3{X: 1, Y: 64, Z: 0},
			expectedCost: baseMoveCost,
		},
		{
			name:         "Going up",
			from:         V3{X: 0, Y: 64, Z: 0},
			to:           V3{X: 0, Y: 65, Z: 0},
			expectedCost: baseMoveCost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := PathfindingDropCost(tt.from, tt.to, baseMoveCost, world, shapeProvider)
			assert.Equal(t, tt.expectedCost, cost)
		})
	}
}

func TestPathfindingDropCost_WaterDrop(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	// Water at Y=60 with solid ground below at Y=59
	world.SetBlock(0, 60, 0, BlockWater)
	world.SetBlock(0, 59, 0, testStoneBlockID)

	shapeProvider.SetPassable(testStoneBlockID, false)
	shapeProvider.SetPassable(BlockWater, true)

	baseMoveCost := 1.0
	from := V3{X: 0, Y: 100, Z: 0}
	to := V3{X: 0, Y: 61, Z: 0}

	cost := PathfindingDropCost(from, to, baseMoveCost, world, shapeProvider)

	// Water drop should be base * 1.2 (20% penalty)
	expectedCost := baseMoveCost * 1.2
	assert.Equal(t, expectedCost, cost, "Water drop should have 20% penalty")
}

func TestPathfindingDropCost_LandDrop(t *testing.T) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	world.SetBlock(0, 60, 0, testStoneBlockID)
	shapeProvider.SetPassable(testStoneBlockID, false)

	baseMoveCost := 1.0
	damageCostFactor := 10.0

	tests := []struct {
		name           string
		from           V3
		to             V3
		fallDistance   float64
		expectedDamage float64
	}{
		{
			name:           "4 block drop (1 heart)",
			from:           V3{X: 0, Y: 65, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			fallDistance:   4.0,
			expectedDamage: 1.0,
		},
		{
			name:           "10 block drop (7 hearts)",
			from:           V3{X: 0, Y: 71, Z: 0},
			to:             V3{X: 0, Y: 61, Z: 0},
			fallDistance:   10.0,
			expectedDamage: 7.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := PathfindingDropCost(tt.from, tt.to, baseMoveCost, world, shapeProvider)

			// Expected: base + (damage * damageCostFactor)
			expectedCost := baseMoveCost + (tt.expectedDamage * damageCostFactor)
			assert.InDelta(t, expectedCost, cost, 0.1)
		})
	}
}

func TestDropSafetyEnumValues(t *testing.T) {
	assert.Equal(t, DropSafety(0), DropSafeNoFall)
	assert.Equal(t, DropSafety(1), DropSafeWater)
	assert.Equal(t, DropSafety(2), DropSafeShortFall)
	assert.Equal(t, DropSafety(3), DropDangerous)
	assert.Equal(t, DropSafety(4), DropLethal)
}

func TestFallDamageConstants(t *testing.T) {
	assert.Equal(t, 3.0, SafeFallDistance, "Safe fall distance should be 3 blocks")
	assert.Equal(t, 1.0, DamagePerBlock, "Damage per block should be 1 heart")

	// Damage reduction constants
	assert.Equal(t, 0.80, HayBaleReduction)
	assert.Equal(t, 0.50, BedReduction)
	assert.Equal(t, 0.80, HoneyBlockReduction)
	assert.Equal(t, 0.0, SlimeBlockDamage)
	assert.Equal(t, 0.0, PowderSnowDamage)
	assert.Equal(t, 0.0, WaterDamage)
}

// Benchmarks
func BenchmarkCalculateFallDamage(b *testing.B) {
	shapeProvider := newMockShapeProvider()
	shapeProvider.SetPassable(testStoneBlockID, false)

	for i := 0; i < b.N; i++ {
		CalculateFallDamage(10.0, testStoneBlockID, shapeProvider)
	}
}

func BenchmarkPredictFallDistance(b *testing.B) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	// Set up ground
	for x := 0; x < 10; x++ {
		for z := 0; z < 10; z++ {
			world.SetBlock(x, 64, z, testStoneBlockID)
		}
	}
	shapeProvider.SetPassable(testStoneBlockID, false)

	pos := V3{X: 5, Y: 80, Z: 5}

	for i := 0; i < b.N; i++ {
		PredictFallDistance(pos, world, shapeProvider)
	}
}

func BenchmarkGetDropSafety(b *testing.B) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	world.SetBlock(0, 60, 0, testStoneBlockID)
	shapeProvider.SetPassable(testStoneBlockID, false)

	from := V3{X: 0, Y: 70, Z: 0}
	to := V3{X: 0, Y: 61, Z: 0}

	for i := 0; i < b.N; i++ {
		GetDropSafety(from, to, 20.0, world, shapeProvider)
	}
}

func BenchmarkPathfindingDropCost(b *testing.B) {
	shapeProvider := newMockShapeProvider()
	world := newMockWorld()

	world.SetBlock(0, 60, 0, testStoneBlockID)
	shapeProvider.SetPassable(testStoneBlockID, false)

	from := V3{X: 0, Y: 70, Z: 0}
	to := V3{X: 0, Y: 61, Z: 0}

	for i := 0; i < b.N; i++ {
		PathfindingDropCost(from, to, 1.0, world, shapeProvider)
	}
}
