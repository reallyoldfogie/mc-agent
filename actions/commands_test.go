package actions

import "testing"

// TestParsePositiveIntArg backs Mine's "mine <blockName> [radius]" — see
// its own doc comment on why an explicit override was added
// (2026-09-10, mine's own hardcoded default radius used to be entirely
// disconnected from a caller's idea of a reasonable search volume).
func TestParsePositiveIntArg(t *testing.T) {
	cases := []struct {
		in     string
		want   int
		wantOK bool
	}{
		{"4", 4, true},
		{"1", 1, true},
		{"0", 0, false},
		{"-1", 0, false},
		{"not-a-number", 0, false},
		{"", 0, false},
		{"3.5", 0, false},
	}
	for _, c := range cases {
		got, ok := parsePositiveIntArg(c.in)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Fatalf("parsePositiveIntArg(%q) = (%d, %v), want (%d, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}
