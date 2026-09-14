package rlenv

import "testing"

// ringOffsets is pure (no World/BlockShapeManager access), so it's tested
// directly here in the internal package. findWalkableTarget/groundSnap
// need a fake models.World/models.BlockShapeManager to exercise — see
// TestReset*Walkab*/TestReset*GroundSnap* in environment_test.go
// (rlenv_test, external package) instead: mc-agent/testing (this
// package's own natural source for that fake, mctesting.MockWorld/
// MockShapeManager) imports mc-agent/config, which imports mc-agent/rlenv
// itself, so importing it from *this* internal package's own test binary
// would be a real import cycle. rlenv_test has no such problem — it's a
// different package from rlenv, so it can depend on both rlenv and
// anything that (transitively) depends on rlenv without contradiction.
func TestRingOffsetsExcludesOriginAndStaysWithinRadius(t *testing.T) {
	const r = 4
	offsets := ringOffsets(r)

	seen := map[[2]int]bool{}
	lastRadius := 0
	for _, off := range offsets {
		if off == [2]int{0, 0} {
			t.Fatal("ringOffsets included the origin (0,0), which findWalkableTarget's caller already checks separately")
		}
		if seen[off] {
			t.Fatalf("ringOffsets produced duplicate offset %v", off)
		}
		seen[off] = true

		radius := max(abs(off[0]), abs(off[1]))
		if radius > r {
			t.Fatalf("offset %v has Chebyshev radius %d, want <= %d", off, radius, r)
		}
		if radius < lastRadius {
			t.Fatalf("offset %v (radius %d) came after a larger radius %d — expected closest-ring-first order", off, radius, lastRadius)
		}
		lastRadius = radius
	}

	want := (2*r+1)*(2*r+1) - 1
	if len(offsets) != want {
		t.Fatalf("ringOffsets(%d) returned %d offsets, want %d", r, len(offsets), want)
	}
}
