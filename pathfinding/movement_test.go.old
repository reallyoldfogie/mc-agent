package pathfinding

import (
	"testing"
)

// TestDiagonalTraverse tests diagonal movement validation
func TestDiagonalTraverse(t *testing.T) {
	mv := NewMovementValidator(nil, nil)

	tests := []struct {
		name     string
		from     V3
		to       V3
		expected bool
	}{
		{
			name:     "Valid diagonal SE",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 100, Z: 1},
			expected: true,
		},
		{
			name:     "Valid diagonal NE",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 100, Z: -1},
			expected: true,
		},
		{
			name:     "Valid diagonal SW",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: -1, Y: 100, Z: 1},
			expected: true,
		},
		{
			name:     "Valid diagonal NW",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: -1, Y: 100, Z: -1},
			expected: true,
		},
		{
			name:     "Invalid - not diagonal (cardinal)",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 100, Z: 0},
			expected: false,
		},
		{
			name:     "Invalid - different Y level",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 101, Z: 1},
			expected: false,
		},
		{
			name:     "Invalid - too far",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 2, Y: 100, Z: 2},
			expected: false,
		},
		{
			name:     "Invalid - same position",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 0, Y: 100, Z: 0},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mv.CanDiagonalTraverse(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("CanDiagonalTraverse(%v, %v) = %v, expected %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

// TestDiagonalAscend tests diagonal jumping validation
func TestDiagonalAscend(t *testing.T) {
	mv := NewMovementValidator(nil, nil)

	tests := []struct {
		name     string
		from     V3
		to       V3
		expected bool
	}{
		{
			name:     "Valid diagonal ascend SE",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 101, Z: 1},
			expected: true,
		},
		{
			name:     "Invalid - not diagonal",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 101, Z: 0},
			expected: false,
		},
		{
			name:     "Invalid - wrong Y difference",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 102, Z: 1},
			expected: false,
		},
		{
			name:     "Invalid - descending",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 99, Z: 1},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mv.CanDiagonalAscend(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("CanDiagonalAscend(%v, %v) = %v, expected %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

// TestJump2 tests 2-block gap jumping
func TestJump2(t *testing.T) {
	mv := NewMovementValidator(nil, nil)

	tests := []struct {
		name     string
		from     V3
		to       V3
		expected bool
	}{
		{
			name:     "Valid 2-block jump East",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 2, Y: 100, Z: 0},
			expected: true,
		},
		{
			name:     "Valid 2-block jump West",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: -2, Y: 100, Z: 0},
			expected: true,
		},
		{
			name:     "Valid 2-block jump North",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 0, Y: 100, Z: -2},
			expected: true,
		},
		{
			name:     "Valid 2-block jump South",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 0, Y: 100, Z: 2},
			expected: true,
		},
		{
			name:     "Valid 2-block jump up East",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 2, Y: 101, Z: 0},
			expected: true,
		},
		{
			name:     "Invalid - diagonal",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 2, Y: 100, Z: 2},
			expected: false,
		},
		{
			name:     "Invalid - only 1 block",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 1, Y: 100, Z: 0},
			expected: false,
		},
		{
			name:     "Invalid - 3 blocks (too far)",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 3, Y: 100, Z: 0},
			expected: false,
		},
		{
			name:     "Invalid - descending",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 2, Y: 99, Z: 0},
			expected: false,
		},
		{
			name:     "Invalid - too high (2 blocks up)",
			from:     V3{X: 0, Y: 100, Z: 0},
			to:       V3{X: 2, Y: 102, Z: 0},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mv.CanJump2(tt.from, tt.to)
			if result != tt.expected {
				t.Errorf("CanJump2(%v, %v) = %v, expected %v",
					tt.from, tt.to, result, tt.expected)
			}
		})
	}
}

// TestMovementTypeCosts tests that all movement types have reasonable costs
func TestMovementTypeCosts(t *testing.T) {
	tests := []struct {
		movement     MovementType
		expectedCost float64
	}{
		{Traverse, 1.0},
		{Ascend, 1.5},
		{Descend, 1.2},
		{Jump2, 2.0},
		{DiagonalTraverse, 1.414}, // sqrt(2)
		{DiagonalAscend, 2.0},
		{Swim, 2.0},
		{Climb, 1.8},
		{SwimUp, 2.5},
		{SwimDown, 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.movement.String(), func(t *testing.T) {
			cost := tt.movement.BaseCost()
			// Allow small floating point error
			if cost < tt.expectedCost-0.001 || cost > tt.expectedCost+0.001 {
				t.Errorf("%s.BaseCost() = %v, expected %v",
					tt.movement.String(), cost, tt.expectedCost)
			}
		})
	}
}

// TestGetPossibleMoves tests that the pathfinder generates expected move counts
func TestGetPossibleMoves(t *testing.T) {
	mv := NewMovementValidator(nil, nil)

	// Test from a high Y position (above 60) where moves should be valid
	from := V3{X: 0, Y: 100, Z: 0}
	moves := mv.GetPossibleMoves(from)

	// We should get moves (exact count depends on validation logic)
	if len(moves) == 0 {
		t.Error("GetPossibleMoves returned no moves")
	}

	// Verify all returned moves have valid types and positive costs
	for i, move := range moves {
		if move.Cost <= 0 {
			t.Errorf("Move %d has invalid cost: %v", i, move.Cost)
		}
		if move.Movement.String() == "Unknown" {
			t.Errorf("Move %d has unknown movement type", i)
		}
	}

	// Count movement types
	moveCounts := make(map[MovementType]int)
	for _, move := range moves {
		moveCounts[move.Movement]++
	}

	t.Logf("Generated %d total moves:", len(moves))
	for mt, count := range moveCounts {
		t.Logf("  %s: %d", mt.String(), count)
	}

	// We should have at least cardinal traversals (4 directions)
	if moveCounts[Traverse] < 4 {
		t.Errorf("Expected at least 4 Traverse moves, got %d", moveCounts[Traverse])
	}

	// We should have diagonal traversals (4 directions)
	if moveCounts[DiagonalTraverse] < 4 {
		t.Errorf("Expected at least 4 DiagonalTraverse moves, got %d", moveCounts[DiagonalTraverse])
	}
}

// TestV3Methods tests the V3 helper methods
func TestV3Methods(t *testing.T) {
	v1 := V3{X: 0, Y: 0, Z: 0}
	v2 := V3{X: 3, Y: 4, Z: 0}

	// Test DistanceTo (3-4-5 right triangle)
	dist := v1.DistanceTo(v2)
	expected := 5.0
	if dist < expected-0.001 || dist > expected+0.001 {
		t.Errorf("DistanceTo() = %v, expected %v", dist, expected)
	}

	// Test ManhattanDistance
	manhattan := v1.ManhattanDistance(v2)
	expectedManhattan := 7
	if manhattan != expectedManhattan {
		t.Errorf("ManhattanDistance() = %v, expected %v", manhattan, expectedManhattan)
	}

	// Test Add
	v3 := v1.Add(1, 2, 3)
	if v3.X != 1 || v3.Y != 2 || v3.Z != 3 {
		t.Errorf("Add(1,2,3) = %v, expected {1,2,3}", v3)
	}

	// Test ToFloat64
	x, y, z := v2.ToFloat64()
	if x != 3.0 || y != 4.0 || z != 0.0 {
		t.Errorf("ToFloat64() = (%v,%v,%v), expected (3.0,4.0,0.0)", x, y, z)
	}
}
