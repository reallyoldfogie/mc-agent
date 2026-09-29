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

func TestCombatHazardClassification(t *testing.T) {
	for _, test := range []struct {
		name   string
		hazard bool
	}{
		{name: "minecraft:creeper", hazard: true},
		{name: "skeleton", hazard: true},
		{name: "zombie", hazard: false},
		{name: "minecraft:spider", hazard: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := combatHazard(test.name); got != test.hazard {
				t.Fatalf("combatHazard(%q)=%v, want %v", test.name, got, test.hazard)
			}
		})
	}
}
