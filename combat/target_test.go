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

func TestRankExcludesKnownDeadTargetsBeforeRemovalPacket(t *testing.T) {
	targets := []Target{
		{EntityID: 1, Category: Hostile, Visible: true, Distance: 2, Health: 0, MaxHealth: 20},
		{EntityID: 2, Category: Hostile, Visible: true, Distance: 3, Health: 0, MaxHealth: 0},
		{EntityID: 3, Category: Hostile, Visible: true, Distance: 4, Health: 10, MaxHealth: 20},
	}
	ranked := RankWithPolicy(targets, TargetPolicy{})
	if len(ranked) != 2 || ranked[0].EntityID != 2 || ranked[1].EntityID != 3 {
		t.Fatalf("known dead target should be excluded while unknown health remains eligible: %#v", ranked)
	}
}

func TestRankReacquiresTargetAfterVisibilityReturns(t *testing.T) {
	target := Target{
		EntityID: 9,
		Category: Hostile,
		Visible:  false,
		Distance: 4,
	}
	policy := TargetPolicy{}

	if got := RankWithPolicy([]Target{target}, policy); len(got) != 0 {
		t.Fatalf("invisible target was selected: %+v", got)
	}

	target.Visible = true
	got := RankWithPolicy([]Target{target}, policy)
	if len(got) != 1 || got[0].EntityID != target.EntityID {
		t.Fatalf("target was not reacquired after visibility returned: %+v", got)
	}
}

func TestPriorityPrefersLowerHealthAtSameDistance(t *testing.T) {
	healthy := Target{Category: Hostile, Distance: 4, MaxHealth: 20, Health: 20}
	weak := Target{Category: Hostile, Distance: 4, MaxHealth: 20, Health: 2}
	if Priority(weak) <= Priority(healthy) {
		t.Fatalf("low-health target should have higher priority")
	}
}

func TestRankWithPolicyRequiresExplicitPlayerOptIn(t *testing.T) {
	targets := []Target{
		{EntityID: 1, Category: Player, Distance: 2, Visible: true},
		{EntityID: 2, Category: Hostile, Distance: 3, Visible: true},
	}

	pve := RankWithPolicy(targets, TargetPolicy{})
	if len(pve) != 1 || pve[0].EntityID != 2 {
		t.Fatalf("PvE policy selected %+v, want only hostile target", pve)
	}

	pvp := RankWithPolicy(targets, TargetPolicy{IncludePlayers: true})
	if len(pvp) != 2 || pvp[0].EntityID != 1 {
		t.Fatalf("PvP policy selected %+v, want player first", pvp)
	}
}
