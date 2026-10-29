package rlenv

import "testing"

func TestCloneAreaZBandSizeFitsUnderVanillaCloneLimit(t *testing.T) {
	// radius 32, default height 4, this run's own depth 8 - the case found
	// live, 2026-10-29, that motivated this whole function.
	xSpan, ySpan := 2*32+1, 4+8+1
	bandSize, ok := cloneAreaZBandSize(xSpan, ySpan)
	if !ok {
		t.Fatalf("cloneAreaZBandSize(%d, %d) ok = false, want true", xSpan, ySpan)
	}
	if bandSize <= 0 {
		t.Fatalf("bandSize = %d, want > 0", bandSize)
	}
	if xSpan*bandSize*ySpan > vanillaCloneBlockLimit {
		t.Fatalf("xSpan*bandSize*ySpan = %d, want <= %d", xSpan*bandSize*ySpan, vanillaCloneBlockLimit)
	}
}

func TestCloneAreaZBandSizeSmallRadiusCoversWholeSpanInOneBand(t *testing.T) {
	// radius 3 - every existing run/test's own small-radius case. A single
	// band must still cover the whole 2*3+1=7 Z span, matching the
	// single-/clone-call behavior this function's caller had before it
	// existed.
	xSpan, ySpan := 2*3+1, 4+3+1
	bandSize, ok := cloneAreaZBandSize(xSpan, ySpan)
	if !ok {
		t.Fatalf("cloneAreaZBandSize(%d, %d) ok = false, want true", xSpan, ySpan)
	}
	if bandSize < xSpan {
		t.Fatalf("bandSize = %d, want >= %d (whole span in one band)", bandSize, xSpan)
	}
}

func TestCloneAreaZBandSizeImpossibleWhenEvenOneRowIsTooBig(t *testing.T) {
	// A pathological combination where a single Z row alone already
	// exceeds vanilla's /clone limit - no amount of Z-splitting can ever
	// make this safe, so ok must come back false rather than some non-zero
	// bandSize the caller might act on.
	xSpan, ySpan := 1000, 1000
	if _, ok := cloneAreaZBandSize(xSpan, ySpan); ok {
		t.Fatalf("cloneAreaZBandSize(%d, %d) ok = true, want false (one row alone is %d blocks, over the %d limit)", xSpan, ySpan, xSpan*ySpan, vanillaCloneBlockLimit)
	}
}
