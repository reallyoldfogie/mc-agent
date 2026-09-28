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

func TestStandingCellAbsorbsFeetYDrift(t *testing.T) {
	cases := []struct {
		pos     V3
		x, y, z int
	}{
		{V3{X: 10.5, Y: -60, Z: 20.5}, 10, -60, 20},
		{V3{X: 10.5, Y: -60.0000000001, Z: 20.5}, 10, -60, 20}, // drift below the integer
		{V3{X: 10.5, Y: -59.9999999, Z: 20.5}, 10, -60, 20},
		{V3{X: -296.5, Y: -60.0002, Z: -299.5}, -297, -60, -300},
		{V3{X: 10.5, Y: -60.4, Z: 20.5}, 10, -61, 20}, // genuinely lower: stays lower
	}
	for _, c := range cases {
		x, y, z := StandingCell(c.pos)
		if x != c.x || y != c.y || z != c.z {
			t.Errorf("StandingCell(%v) = (%d,%d,%d), want (%d,%d,%d)", c.pos, x, y, z, c.x, c.y, c.z)
		}
	}
}
