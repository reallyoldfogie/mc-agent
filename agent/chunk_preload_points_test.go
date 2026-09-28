package agent

import "testing"

// TestChunkPreloadPointsCoversTheDestinationAndItsEightChunkNeighbors pins
// the exact grid waitForChunkLoaded polls: the destination itself plus one
// point in each horizontally-adjacent chunk column, all at the
// destination's own Y — nine points total, none of them duplicated.
func TestChunkPreloadPointsCoversTheDestinationAndItsEightChunkNeighbors(t *testing.T) {
	points := chunkPreloadPoints(100, -60, 200)

	if len(points) != 9 {
		t.Fatalf("len(points) = %d, want 9", len(points))
	}
	seen := map[[3]float64]bool{}
	for _, p := range points {
		if p[1] != -60 {
			t.Fatalf("point %v has Y=%v, want -60 (the destination's own)", p, p[1])
		}
		seen[p] = true
	}
	if len(seen) != 9 {
		t.Fatalf("points were not all distinct: %v", points)
	}
	if !seen[[3]float64{100, -60, 200}] {
		t.Fatalf("destination point itself missing from %v", points)
	}
	if !seen[[3]float64{100 + chunkPreloadRadius, -60, 200 + chunkPreloadRadius}] {
		t.Fatalf("a diagonal neighbor missing from %v", points)
	}
}
