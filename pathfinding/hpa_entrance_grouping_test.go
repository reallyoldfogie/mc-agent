package pathfinding

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestEntranceGroupingOptimized verifies that the optimized grouping produces correct results
func TestEntranceGroupingOptimized(t *testing.T) {
	// Create a line of 10 contiguous entrances
	entrances := make([]*Entrance, 10)
	for i := range 10 {
		entrances[i] = &Entrance{
			Pos1:     models.V3{X: float64(i), Y: 0, Z: 0},
			Pos2:     models.V3{X: float64(i), Y: 0, Z: 1},
			Cluster1: ClusterID{X: 0, Y: 0, Z: 0},
			Cluster2: ClusterID{X: 0, Y: 0, Z: 1},
		}
	}

	grouped := groupEntrances(entrances)

	// Should be grouped into a single entrance
	if len(grouped) != 1 {
		t.Errorf("Expected 1 grouped entrance, got %d", len(grouped))
	}

	// Verify all positions are stored in GroupedPositions
	if len(grouped[0].GroupedPositions[0]) != 10 {
		t.Errorf("Expected 10 grouped positions in Cluster1, got %d", len(grouped[0].GroupedPositions[0]))
	}

	// Verify representative is near center (centroid optimization)
	rep := grouped[0].Pos1
	if rep.X < 3 || rep.X > 6 {
		t.Errorf("Expected representative near center (3-6), got X=%.0f", rep.X)
	}
}

// TestEntranceGroupingMultipleComponents verifies separate groups stay separate
func TestEntranceGroupingMultipleComponents(t *testing.T) {
	// Create two separate groups with a gap
	entrances := make([]*Entrance, 0)

	// Group 1: positions 0-2
	for i := range 3 {
		entrances = append(entrances, &Entrance{
			Pos1:     models.V3{X: float64(i), Y: 0, Z: 0},
			Pos2:     models.V3{X: float64(i), Y: 0, Z: 1},
			Cluster1: ClusterID{X: 0, Y: 0, Z: 0},
			Cluster2: ClusterID{X: 0, Y: 0, Z: 1},
		})
	}

	// Gap at position 3

	// Group 2: positions 4-6
	for i := 4; i < 7; i++ {
		entrances = append(entrances, &Entrance{
			Pos1:     models.V3{X: float64(i), Y: 0, Z: 0},
			Pos2:     models.V3{X: float64(i), Y: 0, Z: 1},
			Cluster1: ClusterID{X: 0, Y: 0, Z: 0},
			Cluster2: ClusterID{X: 0, Y: 0, Z: 1},
		})
	}

	grouped := groupEntrances(entrances)

	// Should be 2 separate groups
	if len(grouped) != 2 {
		t.Errorf("Expected 2 grouped entrances, got %d", len(grouped))
	}

	// Each group should have correct number of positions
	totalPositions := 0
	for _, g := range grouped {
		totalPositions += len(g.GroupedPositions[0])
	}
	if totalPositions != 6 {
		t.Errorf("Expected 6 total grouped positions, got %d", totalPositions)
	}
}

// BenchmarkEntranceGrouping measures performance of the optimized grouping
func BenchmarkEntranceGrouping(b *testing.B) {
	// Create a large grid of entrances (32x32 face)
	entrances := make([]*Entrance, 0, 1024)
	for x := range 32 {
		for y := range 32 {
			entrances = append(entrances, &Entrance{
				Pos1:     models.V3{X: float64(x), Y: float64(y), Z: 0},
				Pos2:     models.V3{X: float64(x), Y: float64(y), Z: 1},
				Cluster1: ClusterID{X: 0, Y: 0, Z: 0},
				Cluster2: ClusterID{X: 0, Y: 0, Z: 1},
			})
		}
	}

	b.ResetTimer()
	for range b.N {
		_ = groupEntrances(entrances)
	}
}
