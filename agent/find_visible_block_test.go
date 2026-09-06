package agent

import (
	"testing"
)

// chebyshev returns max(|dx|,|dy|,|dz|), matching shellOffsets' definition
// of "shell r".
func chebyshev(off [3]int) int {
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	m := abs(off[0])
	if v := abs(off[1]); v > m {
		m = v
	}
	if v := abs(off[2]); v > m {
		m = v
	}
	return m
}

func TestShellOffsets_ZeroIsJustOrigin(t *testing.T) {
	got := shellOffsets(0)
	if len(got) != 1 || got[0] != ([3]int{0, 0, 0}) {
		t.Fatalf("shellOffsets(0) = %v, want [[0 0 0]]", got)
	}
}

// TestShellOffsets_EveryCellIsExactlyOnTheShell checks that every offset
// shellOffsets(r) returns really does have Chebyshev distance r - a bug in
// the face-range bounds (e.g. an off-by-one) would leak cells from an
// adjacent shell.
func TestShellOffsets_EveryCellIsExactlyOnTheShell(t *testing.T) {
	for r := 0; r <= 6; r++ {
		for _, off := range shellOffsets(r) {
			if d := chebyshev(off); d != r {
				t.Fatalf("shellOffsets(%d) contains %v, whose Chebyshev distance is %d, not %d", r, off, d, r)
			}
		}
	}
}

// TestShellOffsets_NoDuplicates guards against the face-enumeration
// double-counting an edge/corner cell (e.g. a cell where two or three axes
// simultaneously sit at +-r).
func TestShellOffsets_NoDuplicates(t *testing.T) {
	for r := 0; r <= 6; r++ {
		seen := make(map[[3]int]bool)
		for _, off := range shellOffsets(r) {
			if seen[off] {
				t.Fatalf("shellOffsets(%d) contains duplicate offset %v", r, off)
			}
			seen[off] = true
		}
	}
}

// TestShellOffsets_UnionMatchesFullCube is the key correctness property
// FindVisibleBlock's early-exit relies on: scanning shells 0..R must visit
// exactly the same set of cells as the original brute-force scan over the
// full [-R,R]^3 cube - no cell skipped, none visited twice - so switching
// to a shell-by-shell scan can only change *when* a match is found, never
// *whether* one is found.
func TestShellOffsets_UnionMatchesFullCube(t *testing.T) {
	const R = 5
	want := make(map[[3]int]bool)
	for dx := -R; dx <= R; dx++ {
		for dy := -R; dy <= R; dy++ {
			for dz := -R; dz <= R; dz++ {
				want[[3]int{dx, dy, dz}] = true
			}
		}
	}

	got := make(map[[3]int]bool)
	for r := 0; r <= R; r++ {
		for _, off := range shellOffsets(r) {
			if got[off] {
				t.Fatalf("shell %d re-visited offset %v already produced by a smaller shell", r, off)
			}
			got[off] = true
		}
	}

	if len(got) != len(want) {
		t.Fatalf("union of shells 0..%d has %d cells, want %d", R, len(got), len(want))
	}
	for off := range want {
		if !got[off] {
			t.Fatalf("union of shells 0..%d is missing offset %v", R, off)
		}
	}
}

// TestShellOffsets_CountMatchesShellVolumeFormula cross-checks each shell's
// cell count against (2r+1)^3 - (2r-1)^3 (the standard hollow-cube-surface
// formula), an independent arithmetic check on top of the brute-force
// comparison above.
func TestShellOffsets_CountMatchesShellVolumeFormula(t *testing.T) {
	cube := func(n int) int { return n * n * n }
	for r := 1; r <= 8; r++ {
		want := cube(2*r+1) - cube(2*r-1)
		got := len(shellOffsets(r))
		if got != want {
			t.Fatalf("len(shellOffsets(%d)) = %d, want %d", r, got, want)
		}
	}
}
