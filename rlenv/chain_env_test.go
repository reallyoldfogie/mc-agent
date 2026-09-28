package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// newChainEnv builds a chain episode over the fake agent's chain simulation:
// a far tree, seeded at Reset, an empty inventory, real recipes.
func newChainEnv(t *testing.T, stage int, goal string) (*fakeAgent, *rlenv.Environment) {
	t.Helper()
	agent := newFakeAgent(0, -60, 0)
	agent.chainInv = map[string]int{}
	agent.chainBlocks = map[[3]int]string{}
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{12, 0, 0}
	cfg.ArrivalThreshold = 1.5
	cfg.MineTargetBlock = "minecraft:oak_log"
	cfg.MineSearchRadius = 8
	cfg.CraftTargetItem = goal
	cfg.ChainStage = stage
	cfg.CollectDrops = true
	cfg.SeedAtGoal = true
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	return agent, newTestEnvironment(t, agent, cfg)
}

func step(t *testing.T, env *rlenv.Environment, a rl.Action) rl.StepResult {
	t.Helper()
	r, err := env.Step(context.Background(), a)
	if err != nil {
		t.Fatalf("Step(%d): %v", a, err)
	}
	return r
}

func legal(env *rlenv.Environment, a rl.Action) bool { return env.ActionMask()[a] }

// A stage-4 bowl episode, driven by hand through every action: the tree is
// seeded and the inventory cleared, milestones pay once as the chain
// progresses, the mask follows what is possible, and only the bowl ends it.
func TestChainUseEpisodeRunsTheWholeChain(t *testing.T) {
	agent, env := newChainEnv(t, rlenv.ChainUse, "minecraft:bowl")
	ctx := context.Background()
	obs, err := env.Reset(ctx)
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Seeding: an empty inventory, drops cleared at the tree, and a tree of
	// two logs plus a spare (a bowl needs 7 planks = 2 logs) beside the goal.
	if agent.clearedInventory != 1 {
		t.Errorf("inventory cleared %d times at Reset, want 1", agent.clearedInventory)
	}
	if len(agent.clearedDropAreas) == 0 {
		t.Error("stray drops were not cleared at the tree")
	}
	logs := 0
	for _, c := range agent.seedBlockAtCalls {
		if c.name == "minecraft:oak_log" {
			logs++
		}
	}
	if logs != 3 {
		t.Errorf("seeded %d logs, want 3 (2 needed + 1 spare)", logs)
	}
	if len(obs.Values) != 21 {
		t.Fatalf("observation has %d values, want 21", len(obs.Values))
	}
	for i := 17; i <= 20; i++ {
		if obs.Values[i] != 0 {
			t.Errorf("chain feature %d = %v at the start, want 0", i, obs.Values[i])
		}
	}
	if legal(env, rlenv.ActionCraft) || legal(env, rlenv.ActionPlace) {
		t.Fatalf("craft/place legal with nothing held: mask=%v", env.ActionMask())
	}

	r := step(t, env, rlenv.ActionGoToTarget) // arrive at the tree
	if r.Done {
		t.Fatal("arriving ended a chain episode")
	}
	if legal(env, rlenv.ActionGoToTarget) {
		t.Error("goto still legal after arriving with a tree in reach")
	}

	// Mining pays a milestone, not the +10 mine bonus, and never ends the episode.
	r = step(t, env, rlenv.ActionMine)
	if r.Done || r.Reward > 5 {
		t.Fatalf("first log: done=%v reward=%v, want a small milestone, not the mine bonus", r.Done, r.Reward)
	}
	if got := r.Observation.Values[17]; got != 1 {
		t.Errorf("chainLogs = %v after the first log, want 1", got)
	}
	r = step(t, env, rlenv.ActionMine)
	if r.Done || r.Observation.Values[17] != 2 {
		t.Fatalf("second log: done=%v logs=%v", r.Done, r.Observation.Values[17])
	}
	if !legal(env, rlenv.ActionCraft) {
		t.Fatal("craft not legal holding logs")
	}

	// craft #1: planks; craft #2: the table. Both are the resolver's choices.
	step(t, env, rlenv.ActionCraft)
	if agent.chainInv["minecraft:oak_planks"] != 4 || agent.chainInv["minecraft:oak_log"] != 1 {
		t.Fatalf("after craft #1: %v, want a log turned into 4 planks", agent.chainInv)
	}
	step(t, env, rlenv.ActionCraft)
	if agent.chainInv["minecraft:crafting_table"] != 1 {
		t.Fatalf("after craft #2: %v, want a crafting table", agent.chainInv)
	}
	if legal(env, rlenv.ActionCraft) {
		t.Error("craft legal with the table held but unplaced: the goal needs it placed first")
	}
	if !legal(env, rlenv.ActionPlace) {
		t.Fatal("place not legal holding a table")
	}

	r = step(t, env, rlenv.ActionPlace)
	if r.Done || r.Observation.Values[20] != 1 {
		t.Fatalf("place: done=%v tablePlaced=%v, want placed and not done", r.Done, r.Observation.Values[20])
	}
	if legal(env, rlenv.ActionPlace) {
		t.Error("place still legal after placing")
	}

	step(t, env, rlenv.ActionCraft) // the spare log -> planks
	r = step(t, env, rlenv.ActionCraft)
	if !r.Done || r.Reward < 9 {
		t.Fatalf("bowl: done=%v reward=%v, want the completion bonus and done", r.Done, r.Reward)
	}

	// The next episode cleans up: the placed table and the tree are removed
	// and the chain state starts over.
	obs, err = env.Reset(ctx)
	if err != nil {
		t.Fatalf("second Reset: %v", err)
	}
	removedTable := false
	for _, b := range agent.restoredBlocks {
		if b.name == "minecraft:air" && agent.chainBlocks[[3]int{b.x, b.y, b.z}] == "" {
			removedTable = removedTable || (b.x == 12 && b.y == -60 && b.z == 1)
		}
	}
	if !removedTable {
		t.Errorf("the placed table was not removed at the next Reset: restored=%v", agent.restoredBlocks)
	}
	for i := 17; i <= 20; i++ {
		if obs.Values[i] != 0 {
			t.Errorf("chain feature %d = %v after Reset, want 0", i, obs.Values[i])
		}
	}
}

func TestChainGatherEndsOnTheFirstLogInTheInventory(t *testing.T) {
	_, env := newChainEnv(t, rlenv.ChainGather, "")
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	step(t, env, rlenv.ActionGoToTarget)
	r := step(t, env, rlenv.ActionMine)
	if !r.Done || r.Reward < 9 {
		t.Fatalf("done=%v reward=%v, want the completion bonus when the log is in hand", r.Done, r.Reward)
	}
}

// Stage 3 stops at the placed table, and does not end when the table is only
// crafted.
func TestChainPlaceEndsOnlyWhenTheTableIsPlaced(t *testing.T) {
	agent, env := newChainEnv(t, rlenv.ChainPlace, "minecraft:crafting_table")
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	step(t, env, rlenv.ActionGoToTarget)
	step(t, env, rlenv.ActionMine)
	step(t, env, rlenv.ActionCraft)
	r := step(t, env, rlenv.ActionCraft)
	if agent.chainInv["minecraft:crafting_table"] != 1 || r.Done {
		t.Fatalf("table crafted: inv=%v done=%v, want held and not done", agent.chainInv, r.Done)
	}
	r = step(t, env, rlenv.ActionPlace)
	if !r.Done || r.Reward < 9 {
		t.Fatalf("placed: done=%v reward=%v, want the completion bonus", r.Done, r.Reward)
	}
}

// A placement the agent could not complete leaves the table in hand and the
// episode going, and place stays legal to try again.
func TestChainFailedPlacementCanBeRetried(t *testing.T) {
	agent, env := newChainEnv(t, rlenv.ChainPlace, "minecraft:crafting_table")
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatal(err)
	}
	step(t, env, rlenv.ActionGoToTarget)
	step(t, env, rlenv.ActionMine)
	step(t, env, rlenv.ActionCraft)
	step(t, env, rlenv.ActionCraft)
	agent.failNextPlace = true
	r := step(t, env, rlenv.ActionPlace)
	if r.Done || r.Observation.Values[20] != 0 || !legal(env, rlenv.ActionPlace) {
		t.Fatalf("failed placement: done=%v placed=%v placeLegal=%v", r.Done, r.Observation.Values[20], legal(env, rlenv.ActionPlace))
	}
	if r = step(t, env, rlenv.ActionPlace); !r.Done {
		t.Fatal("the retry did not complete the stage")
	}
}

// Outside a chain episode the new action is illegal and the chain features
// stay zero, so the older tasks behave as before.
func TestPlaceIsIllegalOutsideChainEpisodes(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if legal(env, rlenv.ActionPlace) {
		t.Error("place legal in a plain goto episode")
	}
	for i := 17; i <= 20; i++ {
		if obs.Values[i] != 0 {
			t.Errorf("chain feature %d = %v outside a chain episode", i, obs.Values[i])
		}
	}
}

func TestNewRejectsAnOutOfRangeChainStage(t *testing.T) {
	cfg := testConfig()
	cfg.ChainStage = 5
	if _, err := rlenv.New(newFakeAgent(0, 0, 0), nil, cfg); err == nil {
		t.Fatal("ChainStage 5 accepted")
	}
}
