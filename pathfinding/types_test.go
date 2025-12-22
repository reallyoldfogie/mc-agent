package pathfinding

import (
	"testing"
)

// TestMovementType_String verifies all movement types have string representations
func TestMovementType_String(t *testing.T) {
	tests := []struct {
		mt   MovementType
		want string
	}{
		{Traverse, "Traverse"},
		{Ascend, "Ascend"},
		{Descend, "Descend"},
		{Jump2, "Jump2"},
		{DiagonalTraverse, "DiagonalTraverse"},
		{DiagonalAscend, "DiagonalAscend"},
		{Swim, "Swim"},
		{Climb, "Climb"},
		{SwimUp, "SwimUp"},
		{SwimDown, "SwimDown"},
		// New Phase 5 movement types
		{DescendLadderNorth, "DescendLadderNorth"},
		{DescendLadderSouth, "DescendLadderSouth"},
		{DescendLadderEast, "DescendLadderEast"},
		{DescendLadderWest, "DescendLadderWest"},
		{Drop2North, "Drop2North"},
		{Drop2South, "Drop2South"},
		{Drop2East, "Drop2East"},
		{Drop2West, "Drop2West"},
		{TraverseNorthEast, "TraverseNorthEast"},
		{TraverseNorthWest, "TraverseNorthWest"},
		{TraverseSouthEast, "TraverseSouthEast"},
		{TraverseSouthWest, "TraverseSouthWest"},
		{SneakThrough, "SneakThrough"},
		{SneakTraverse, "SneakTraverse"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mt.String(); got != tt.want {
				t.Errorf("MovementType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMovementType_BaseCost verifies all movement types have valid costs
func TestMovementType_BaseCost(t *testing.T) {
	tests := []struct {
		mt   MovementType
		want float64
	}{
		{Traverse, 1.0},
		{Ascend, 1.5},
		{Descend, 1.2},
		{Jump2, 2.0},
		{DiagonalTraverse, 1.414},
		{DiagonalAscend, 2.0},
		{Swim, 2.0},
		{Climb, 1.8},
		{SwimUp, 2.5},
		{SwimDown, 1.5},
		// New Phase 5 movement types
		{DescendLadderNorth, 1.8},
		{DescendLadderSouth, 1.8},
		{DescendLadderEast, 1.8},
		{DescendLadderWest, 1.8},
		{Drop2North, 1.5},
		{Drop2South, 1.5},
		{Drop2East, 1.5},
		{Drop2West, 1.5},
		{TraverseNorthEast, 1.414},
		{TraverseNorthWest, 1.414},
		{TraverseSouthEast, 1.414},
		{TraverseSouthWest, 1.414},
		{SneakThrough, 3.0},
		{SneakTraverse, 3.0},
	}

	for _, tt := range tests {
		t.Run(tt.mt.String(), func(t *testing.T) {
			got := tt.mt.BaseCost()
			if got != tt.want {
				t.Errorf("MovementType.BaseCost() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMovementType_AllTypesHaveCosts verifies no movement type returns default cost
func TestMovementType_AllTypesHaveCosts(t *testing.T) {
	// Test all movement types from 0 to SneakTraverse
	for mt := Traverse; mt <= SneakTraverse; mt++ {
		cost := mt.BaseCost()
		if cost <= 0 {
			t.Errorf("Movement type %v has invalid cost: %v", mt, cost)
		}
		if mt.String() == "Unknown" {
			t.Errorf("Movement type %v is unknown", mt)
		}
	}
}
