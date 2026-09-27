package combat

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		want Category
	}{
		{"player", Player}, {"minecraft:player", Player}, {"Creeper", Hostile},
		{"minecraft:skeleton", Hostile}, {"cow", Neutral},
	}
	for _, test := range tests {
		if got := Classify(test.name); got != test.want {
			t.Errorf("Classify(%q) = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestRankPrefersPlayerThenHostileThenNeutral(t *testing.T) {
	targets := []Target{
		{EntityID: 3, Category: Neutral, Distance: 1, Visible: true, MaxHealth: 10, Health: 10},
		{EntityID: 2, Category: Hostile, Distance: 10, Visible: true, MaxHealth: 20, Health: 20},
		{EntityID: 1, Category: Player, Distance: 20, Visible: true, MaxHealth: 20, Health: 20},
	}
	ranked := Rank(targets, true)
	if len(ranked) != 3 || ranked[0].EntityID != 1 || ranked[1].EntityID != 2 || ranked[2].EntityID != 3 {
		t.Fatalf("unexpected ranking: %#v", ranked)
	}
}

func TestRankExcludesUnsafeTargetsAndNeutralByDefault(t *testing.T) {
	targets := []Target{
		{EntityID: 1, Category: Player, Visible: false},
		{EntityID: 2, Category: Hostile, Visible: true, Removed: true},
		{EntityID: 3, Category: Neutral, Visible: true},
		{EntityID: 4, Category: Hostile, Visible: true, Distance: 4},
	}
	ranked := Rank(targets, false)
	if len(ranked) != 1 || ranked[0].EntityID != 4 {
		t.Fatalf("unexpected filtered ranking: %#v", ranked)
	}
}

func TestPriorityPrefersLowerHealthAtSameDistance(t *testing.T) {
	healthy := Target{Category: Hostile, Distance: 4, MaxHealth: 20, Health: 20}
	weak := Target{Category: Hostile, Distance: 4, MaxHealth: 20, Health: 2}
	if Priority(weak) <= Priority(healthy) {
		t.Fatalf("low-health target should have higher priority")
	}
}
