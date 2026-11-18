package rlenv

import (
	"testing"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
)

// TestActionNameCoversEveryAction guards the exact drift this table was
// added to fix (2026-11-14): every action in [0, NumActions) must have a
// real name, not "unknown" — if a future action gets added to the const
// block without a matching actionNames entry, this fails immediately
// instead of silently reporting that action as "unknown" in every metric.
func TestActionNameCoversEveryAction(t *testing.T) {
	for a := rl.Action(0); int(a) < NumActions; a++ {
		if name := ActionName(a); name == "unknown" {
			t.Errorf("action %d has no entry in actionNames - add one alongside its const definition", a)
		}
	}
}

func TestActionNameIsUnknownOutOfRange(t *testing.T) {
	if name := ActionName(rl.Action(NumActions)); name != "unknown" {
		t.Errorf("expected \"unknown\" for an out-of-range action, got %q", name)
	}
}
