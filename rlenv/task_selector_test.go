package rlenv_test

import (
	"context"
	"math/rand"
	"testing"

	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// This file tests Config.TaskSelector and the goal-conditioning
// observation block — docs/plans/06-per-episode-task-selection-and-goal-conditioning.md
// in ../mc-rsi-trainer, promoting
// testing/rl_train_test.go's newAlternatingMineOrCraftSeeder
// proof-of-concept into this package's own reusable capability. See that
// document's "Done when" for what these are meant to prove.

// TestTaskSelectorNilLeavesStaticConfigUnchanged is the core backward-
// compatibility guarantee this feature depends on: an Environment built
// without a TaskSelector must behave byte-for-byte like it did before this
// field existed. If this test ever fails, every existing static-Config
// caller (including every other test in this package) is at risk.
func TestTaskSelectorNilLeavesStaticConfigUnchanged(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)

	for episode := 0; episode < 3; episode++ {
		obs, err := env.Reset(context.Background())
		if err != nil {
			t.Fatalf("episode %d: Reset: %v", episode, err)
		}
		mask := env.ActionMask()
		if !mask[rlenv.ActionGoToTarget] {
			t.Fatalf("episode %d: ActionGoToTarget legal = false, want true (no TaskSelector set)", episode)
		}
		if got := obs.Values[14]; got != 1 {
			t.Fatalf("episode %d: goalGoToActive = %v, want 1 (no TaskSelector set)", episode, got)
		}
		if got := obs.Values[15]; got != 1 {
			t.Fatalf("episode %d: goalMineActive = %v, want 1 (Config.MineTargetBlock is set)", episode, got)
		}
		if got := obs.Values[16]; got != 0 {
			t.Fatalf("episode %d: goalCraftActive = %v, want 0 (Config.CraftTargetItem is unset)", episode, got)
		}
	}
}

// TestTaskSelectorOverridesTaskFieldsPerEpisode verifies the actual
// per-episode swap this document exists for: alternating between a
// goto-only episode and a mine-only episode, checked against both
// ActionMask (the enforcement mechanism) and the goal-conditioning
// observation block (the policy-visible signal) — the same pairing
// TestActionMaskMineLegalOnlyWhenConfiguredAndVisible already establishes
// as this package's own convention for "don't just trust the mask's
// internal logic, cross-check what it actually reports."
func TestTaskSelectorOverridesTaskFieldsPerEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 5, 0, 0)

	cfg := testConfig()
	cfg.TargetOffset = [3]float64{5, 0, 0}
	cfg.TaskSelector = func(episode int, _ *rand.Rand) rlenv.TaskOverride {
		if episode%2 == 0 {
			return rlenv.TaskOverride{TargetOffset: [3]float64{5, 0, 0}}
		}
		return rlenv.TaskOverride{GoToTargetDisabled: true, MineTargetBlock: "minecraft:stone", MineSearchRadius: 20}
	}
	env := newTestEnvironment(t, agent, cfg)

	// Episode 0: goto-only.
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("episode 0: Reset: %v", err)
	}
	mask := env.ActionMask()
	if !mask[rlenv.ActionGoToTarget] || mask[rlenv.ActionMine] {
		t.Fatalf("episode 0: mask = %v, want GoToTarget legal, Mine illegal", mask)
	}
	if obs.Values[14] != 1 || obs.Values[15] != 0 {
		t.Fatalf("episode 0: goal block = %v, want goalGoToActive=1 goalMineActive=0", obs.Values[14:17])
	}

	// Episode 1: mine-only — GoToTarget must now be illegal, both via the
	// mask and via a direct dispatch attempt (mirrors this package's own
	// "cross-check the mask against what Step actually does" convention).
	obs, err = env.Reset(context.Background())
	if err != nil {
		t.Fatalf("episode 1: Reset: %v", err)
	}
	mask = env.ActionMask()
	if mask[rlenv.ActionGoToTarget] || !mask[rlenv.ActionMine] {
		t.Fatalf("episode 1: mask = %v, want GoToTarget illegal, Mine legal", mask)
	}
	if obs.Values[14] != 0 || obs.Values[15] != 1 {
		t.Fatalf("episode 1: goal block = %v, want goalGoToActive=0 goalMineActive=1", obs.Values[14:17])
	}
	moveCallsBefore := agent.moveToWithChatCalls
	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("episode 1: Step(ActionGoToTarget): %v", err)
	}
	if agent.moveToWithChatCalls != moveCallsBefore {
		t.Fatalf("episode 1: MoveTo was dispatched despite ActionGoToTarget being masked illegal")
	}

	// Episode 2: back to goto-only — proves this isn't a one-way switch.
	obs, err = env.Reset(context.Background())
	if err != nil {
		t.Fatalf("episode 2: Reset: %v", err)
	}
	if mask := env.ActionMask(); !mask[rlenv.ActionGoToTarget] || mask[rlenv.ActionMine] {
		t.Fatalf("episode 2: mask = %v, want GoToTarget legal, Mine illegal (back to goto-only)", mask)
	}
	if obs.Values[14] != 1 || obs.Values[15] != 0 {
		t.Fatalf("episode 2: goal block = %v, want goalGoToActive=1 goalMineActive=0", obs.Values[14:17])
	}
}

// TestTaskSelectorReceivesIncrementingEpisodeNumber verifies the episode
// argument TaskSelector receives is 0-indexed and increments once per
// Reset call, matching its own doc comment.
func TestTaskSelectorReceivesIncrementingEpisodeNumber(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	var seenEpisodes []int
	cfg.TaskSelector = func(episode int, _ *rand.Rand) rlenv.TaskOverride {
		seenEpisodes = append(seenEpisodes, episode)
		return rlenv.TaskOverride{TargetOffset: cfg.TargetOffset}
	}
	env := newTestEnvironment(t, agent, cfg)

	for i := 0; i < 3; i++ {
		if _, err := env.Reset(context.Background()); err != nil {
			t.Fatalf("Reset %d: %v", i, err)
		}
	}
	want := []int{0, 1, 2}
	if len(seenEpisodes) != len(want) {
		t.Fatalf("seenEpisodes = %v, want %v", seenEpisodes, want)
	}
	for i, got := range seenEpisodes {
		if got != want[i] {
			t.Fatalf("seenEpisodes = %v, want %v", seenEpisodes, want)
		}
	}
}

// TestTaskSelectorOverrideReachesSeeder verifies Config.Seeder is called
// with this episode's *overridden* Config, not the original static one —
// without this, a curriculum's chosen mine/craft target for the episode
// would never actually get seeded into the world, silently defeating the
// whole mechanism (the exact bug newAlternatingMineOrCraftSeeder's own
// inline closure had to hand-write its own seeding logic to work around,
// since it predates this hook).
func TestTaskSelectorOverrideReachesSeeder(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.TaskSelector = func(episode int, _ *rand.Rand) rlenv.TaskOverride {
		return rlenv.TaskOverride{GoToTargetDisabled: true, MineTargetBlock: "minecraft:stone", MineSearchRadius: 20}
	}
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedNearbyBlockCalls) != 1 {
		t.Fatalf("SeedNearbyBlock calls = %d, want 1", len(agent.seedNearbyBlockCalls))
	}
	if got := agent.seedNearbyBlockCalls[0]; got.blockName != "minecraft:stone" || got.radius != 20 {
		t.Fatalf("SeedNearbyBlock call = %+v, want {minecraft:stone 20} (the TaskSelector-chosen target, not the static Config's, which never set MineTargetBlock at all)", got)
	}
}

// TestGoalConditioningBlockReflectsCombinedConfig verifies the
// goal-conditioning block is a genuine multi-hot, not a strict one-hot —
// Config.TaskSelector composing more than one active task in a single
// TaskOverride (exactly like a static multi-task Config already could)
// must be reflected as more than one active bit, matching
// TaskSelector's own doc comment ("returning more than one composes").
func TestGoalConditioningBlockReflectsCombinedConfig(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.CraftTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if obs.Values[14] != 1 || obs.Values[15] != 1 || obs.Values[16] != 1 {
		t.Fatalf("goal block = %v, want all three active (goto + mine + craft all configured)", obs.Values[14:17])
	}
}

// TestGoalConditioningBlockDistinguishesActiveFromReady verifies
// goalMineActive/goalCraftActive reflect whether the task is *configured*
// this episode, not whether it's currently *visible*/*ready* — that's
// mineVisible (index 12) and craftReady (index 13)'s own job, unchanged
// by this feature. A mine task with nothing currently visible must still
// show goalMineActive=1 (the task is real), while mineVisible=0
// (nothing's there to act on yet).
func TestGoalConditioningBlockDistinguishesActiveFromReady(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone" // configured, but fakeAgent never reports a visible instance
	env := newTestEnvironment(t, agent, cfg)

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got := obs.Values[15]; got != 1 {
		t.Fatalf("goalMineActive = %v, want 1 (task is configured)", got)
	}
	if got := obs.Values[12]; got != 0 {
		t.Fatalf("mineVisible = %v, want 0 (nothing currently visible) — goalMineActive and mineVisible must be independent signals", got)
	}
}
