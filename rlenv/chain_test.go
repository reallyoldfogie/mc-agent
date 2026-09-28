package rlenv

import "testing"

func TestChainLogsNeededMatchesTheRecipes(t *testing.T) {
	cases := []struct {
		stage int
		goal  string
		logs  int
	}{
		{ChainGather, "", 1},
		{ChainTable, "", 1}, // 4 planks = 1 log
		{ChainPlace, "", 1},
		{ChainUse, "minecraft:bowl", 2},           // 4 + 3 = 7 planks
		{ChainUse, "minecraft:chest", 3},          // 4 + 8 = 12 planks
		{ChainUse, "minecraft:wooden_pickaxe", 3}, // 4 + 3 + 2 = 9 planks
	}
	for _, c := range cases {
		if got := ChainLogsNeeded(c.stage, c.goal); got != c.logs {
			t.Errorf("ChainLogsNeeded(%d, %q) = %d, want %d", c.stage, c.goal, got, c.logs)
		}
		if got := ChainTreeHeight(c.stage, c.goal); got != c.logs+1 {
			t.Errorf("ChainTreeHeight(%d, %q) = %d, want %d (one spare)", c.stage, c.goal, got, c.logs+1)
		}
	}
}

func TestNextChainCraft(t *testing.T) {
	inv := func(logs, planks, sticks, tables int) chainInventory {
		return chainInventory{logs: logs, planks: planks, sticks: sticks, tables: tables}
	}
	const bowl, chest, pick = "minecraft:bowl", "minecraft:chest", "minecraft:wooden_pickaxe"
	cases := []struct {
		name   string
		stage  int
		goal   string
		inv    chainInventory
		placed bool
		want   string
	}{
		{"gather crafts nothing", ChainGather, "", inv(2, 0, 0, 0), false, ""},
		{"nothing held", ChainTable, "", inv(0, 0, 0, 0), false, ""},
		{"a log becomes planks", ChainTable, "", inv(1, 0, 0, 0), false, chainPlanks},
		{"planks below four still need a log", ChainTable, "", inv(1, 2, 0, 0), false, chainPlanks},
		{"four planks become the table", ChainTable, "", inv(0, 4, 0, 0), false, chainTable},
		{"the table wins over more planks", ChainTable, "", inv(1, 4, 0, 0), false, chainTable},
		{"stage 2 is done once the table is held", ChainTable, "", inv(0, 0, 0, 1), false, ""},
		{"stage 3 stops at the table too", ChainPlace, "", inv(1, 0, 0, 1), false, ""},
		{"bowl: table held but unplaced waits for Place", ChainUse, bowl, inv(1, 0, 0, 1), false, ""},
		{"bowl: placed, planks short, log held", ChainUse, bowl, inv(1, 0, 0, 0), true, chainPlanks},
		{"bowl: placed, enough planks", ChainUse, bowl, inv(0, 3, 0, 0), true, bowl},
		{"bowl: placed, nothing left to craft from", ChainUse, bowl, inv(0, 2, 0, 0), true, ""},
		{"chest needs eight planks", ChainUse, chest, inv(1, 4, 0, 0), true, chainPlanks},
		{"chest with eight planks", ChainUse, chest, inv(0, 8, 0, 0), true, chest},
		{"pickaxe: sticks first", ChainUse, pick, inv(0, 5, 0, 0), true, chainStick},
		{"pickaxe: then the pickaxe", ChainUse, pick, inv(0, 3, 4, 0), true, pick},
		{"pickaxe: sticks need planks, so a log first", ChainUse, pick, inv(1, 1, 0, 0), true, chainPlanks},
	}
	for _, c := range cases {
		if got := nextChainCraft(c.stage, c.goal, c.inv, c.placed); got != c.want {
			t.Errorf("%s: nextChainCraft = %q, want %q", c.name, got, c.want)
		}
	}
}

// Simulates a whole stage-4 episode with the resolver making every craft
// decision, to show the recipes and the resolver agree end to end.
func TestChainResolverCompletesEveryGoalFromItsLogs(t *testing.T) {
	for goal, g := range chainGoals {
		inv := chainInventory{logs: ChainLogsNeeded(ChainUse, goal)}
		placed := false
		for step := 0; step < 30; step++ {
			if chainPlaceLegal(ChainUse, inv, placed) {
				inv.tables--
				placed = true
				continue
			}
			next := nextChainCraft(ChainUse, goal, inv, placed)
			switch next {
			case "":
				t.Fatalf("%s: resolver stuck at %+v placed=%v after %d steps", goal, inv, placed, step)
			case chainPlanks:
				inv.logs--
				inv.planks += chainPlanksPerLog
			case chainTable:
				inv.planks -= chainTablePlanks
				inv.tables++
			case chainStick:
				inv.planks -= chainPlanksPerSticks
				inv.sticks += chainSticksPerCraft
			case goal:
				if inv.planks < g.planks || inv.sticks < g.sticks {
					t.Fatalf("%s: resolver crafted the goal without its ingredients: %+v", goal, inv)
				}
				goto crafted
			}
		}
		t.Fatalf("%s: never crafted the goal", goal)
	crafted:
	}
}

func TestChainPlaceLegal(t *testing.T) {
	if chainPlaceLegal(ChainTable, chainInventory{tables: 1}, false) {
		t.Error("placing must be illegal at stage 2, which ends at the table")
	}
	if !chainPlaceLegal(ChainPlace, chainInventory{tables: 1}, false) {
		t.Error("placing must be legal holding a table at stage 3")
	}
	if chainPlaceLegal(ChainUse, chainInventory{tables: 1}, true) {
		t.Error("placing must be illegal once a table is placed")
	}
	if chainPlaceLegal(ChainUse, chainInventory{}, false) {
		t.Error("placing must be illegal with no table held")
	}
}

func TestChainMilestonesPayOnceAndTheStageEndsOnItsOwnGoal(t *testing.T) {
	var s chainState
	if r, done := s.advance(ChainUse, chainInventory{}, false, false); r != 0 || done {
		t.Fatalf("empty: reward=%v done=%v", r, done)
	}
	if r, done := s.advance(ChainUse, chainInventory{logs: 1}, false, false); r != chainMilestoneBonus || done {
		t.Fatalf("first log: reward=%v done=%v, want one milestone", r, done)
	}
	if r, _ := s.advance(ChainUse, chainInventory{logs: 2}, false, false); r != 0 {
		t.Fatalf("more logs paid again: %v", r)
	}
	if r, _ := s.advance(ChainUse, chainInventory{planks: 4}, false, false); r != chainMilestoneBonus {
		t.Fatalf("planks: %v", r)
	}
	// crafting and un-crafting the same thing must not farm it
	if r, _ := s.advance(ChainUse, chainInventory{logs: 1}, false, false); r != 0 {
		t.Fatalf("logs reappearing paid again: %v", r)
	}
	if r, _ := s.advance(ChainUse, chainInventory{tables: 1}, false, false); r != chainMilestoneBonus {
		t.Fatalf("table item: %v", r)
	}
	if r, done := s.advance(ChainUse, chainInventory{}, true, false); r != chainMilestoneBonus || done {
		t.Fatalf("placed: reward=%v done=%v", r, done)
	}
	if r, done := s.advance(ChainUse, chainInventory{}, true, true); r != chainCompletionBonus || !done {
		t.Fatalf("goal: reward=%v done=%v, want the completion bonus and done", r, done)
	}
}

func TestChainStagesEndOnTheirOwnMilestone(t *testing.T) {
	cases := []struct {
		stage int
		inv   chainInventory
		placd bool
	}{
		{ChainGather, chainInventory{logs: 1}, false},
		{ChainTable, chainInventory{tables: 1}, false},
		{ChainPlace, chainInventory{}, true},
	}
	for _, c := range cases {
		var s chainState
		_, done := s.advance(c.stage, c.inv, c.placd, false)
		if !done {
			t.Errorf("stage %d did not end on its own milestone", c.stage)
		}
	}
	// stage 3 must not end merely because the table was crafted
	var s chainState
	if _, done := s.advance(ChainPlace, chainInventory{tables: 1}, false, false); done {
		t.Error("stage 3 ended when the table was crafted, before it was placed")
	}
	// the sum of intermediate milestones stays below the completion bonus
	if 3*chainMilestoneBonus >= chainCompletionBonus {
		t.Error("intermediate milestones must sum to less than the completion bonus")
	}
}
