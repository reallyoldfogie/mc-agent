package models

import "testing"

// Candidates are the four orthogonal neighbours first, each with its floor
// one block below, so the nearest free cell is always tried before a far one.
func TestPlacementCandidatesAreNearestFirstWithFloorsBelow(t *testing.T) {
	c := PlacementCandidates(10, -60, 20)
	if len(c) != 12 {
		t.Fatalf("len = %d, want 12", len(c))
	}
	for i, want := range []V3{{X: 11, Y: -60, Z: 20}, {X: 9, Y: -60, Z: 20}, {X: 10, Y: -60, Z: 21}, {X: 10, Y: -60, Z: 19}} {
		if c[i].Cell != want {
			t.Errorf("candidate %d cell = %v, want %v", i, c[i].Cell, want)
		}
	}
	prev := 0.0
	for i, p := range c {
		if p.Floor.X != p.Cell.X || p.Floor.Z != p.Cell.Z || p.Floor.Y != p.Cell.Y-1 {
			t.Errorf("candidate %d floor %v is not directly below cell %v", i, p.Floor, p.Cell)
		}
		d := abs(p.Cell.X-10) + abs(p.Cell.Z-20)
		if d+1e-9 < prev {
			t.Errorf("candidate %d (%v) is nearer than an earlier one", i, p.Cell)
		}
		prev = d
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
