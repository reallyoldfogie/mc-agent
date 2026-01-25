package pathfinding

import (
	"testing"
)

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
