package agent

import (
	"math"
	"testing"
	"time"
)

func TestCombatTickIntervalUsesServerTickRate(t *testing.T) {
	a := &agent{}
	if got, want := a.combatTickInterval(), 50*time.Millisecond; got != want {
		t.Fatalf("unset server rate: got %s, want %s", got, want)
	}

	a.serverTickRateBits.Store(math.Float32bits(40))
	if got, want := a.combatTickInterval(), 25*time.Millisecond; got != want {
		t.Fatalf("40 TPS: got %s, want %s", got, want)
	}

	a.serverTickRateBits.Store(math.Float32bits(10))
	if got, want := a.combatTickInterval(), 100*time.Millisecond; got != want {
		t.Fatalf("10 TPS: got %s, want %s", got, want)
	}
}
