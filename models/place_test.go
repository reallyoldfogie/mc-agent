package models

import "testing"

// Candidates are the four orthogonal neighbours first, each with its floor
// one block below, so the nearest free cell is always tried before a far
// one - all 12 at the bot's own level, then the same 12 one level down.
func TestPlacementCandidatesAreNearestFirstWithFloorsBelow(t *testing.T) {
	c := PlacementCandidates(10, -60, 20)
	if len(c) != 24 {
		t.Fatalf("len = %d, want 24", len(c))
	}
	for i, want := range []V3{{X: 11, Y: -60, Z: 20}, {X: 9, Y: -60, Z: 20}, {X: 10, Y: -60, Z: 21}, {X: 10, Y: -60, Z: 19}} {
		if c[i].Cell != want {
			t.Errorf("candidate %d cell = %v, want %v", i, c[i].Cell, want)
		}
	}
	for tier, y := range []float64{-60, -61} {
		tierC := c[tier*12 : tier*12+12]
		prev := 0.0
		for i, p := range tierC {
			if p.Cell.Y != y {
				t.Errorf("tier %d candidate %d Y = %v, want %v", tier, i, p.Cell.Y, y)
			}
			if p.Floor.X != p.Cell.X || p.Floor.Z != p.Cell.Z || p.Floor.Y != p.Cell.Y-1 {
				t.Errorf("tier %d candidate %d floor %v is not directly below cell %v", tier, i, p.Floor, p.Cell)
			}
			d := abs(p.Cell.X-10) + abs(p.Cell.Z-20)
			if d+1e-9 < prev {
				t.Errorf("tier %d candidate %d (%v) is nearer than an earlier one in its own tier", tier, i, p.Cell)
			}
			prev = d
		}
	}
}

// The second tier - one level below the bot - is what lets a bot standing
// on top of something other than the floor (its own column elevated by a
// leftover block, everywhere around it at the real, normal ground level)
// still find a placeable cell: at a neighbour offset, the bot's own level is
// open air with nothing to place against (no floor there), but one level
// down, right at the neighbour's real floor, both the cell and the block
// above it (the bot's own, open, level) are clear and the ground below is
// solid.
func TestPlacementCandidatesLowerTierReachesTheRealFloor(t *testing.T) {
	// Bot elevated to by=-59 by an unmined block under its own feet only;
	// the world everywhere else is normal (ground at -61, open air at -60
	// and above).
	c := PlacementCandidates(0, -59, 0)
	lower := c[12:]
	if len(lower) != 12 {
		t.Fatalf("lower tier len = %d, want 12", len(lower))
	}
	first := lower[0]
	if first.Cell != (V3{X: 1, Y: -60, Z: 0}) {
		t.Fatalf("lower tier's nearest cell = %v, want the real standing level (-60) beside the bot", first.Cell)
	}
	if first.Floor != (V3{X: 1, Y: -61, Z: 0}) {
		t.Fatalf("lower tier's nearest floor = %v, want the real ground (-61)", first.Floor)
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
