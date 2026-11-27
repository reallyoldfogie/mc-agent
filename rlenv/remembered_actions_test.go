package rlenv_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

func TestActionMineRemembered_LegalOnlyWhenLiveLOSFails(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:iron_ore", 1, 1, 1) // live-visible
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:iron_ore"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMineRemembered]; legal {
		t.Fatalf("ActionMineRemembered legal = true, want false (live LOS already found it)")
	}
}

// TestActionMineRemembered_IllegalWhenNothingRemembered is the fix found
// live, 2026-11-25, spectating: with nothing visible AND nothing ever
// remembered, ActionMineRemembered used to stay legal anyway -
// guaranteed to fail every single time it was tried (one agent logged
// >139,000 consecutive "I don't remember seeing X" lines over 7.5 hours,
// firing every 14-30ms with zero movement between attempts). It must be
// masked off instead, so the policy is pushed toward exploring
// (ActionGoToTarget) rather than offered a free, zero-progress no-op.
func TestActionMineRemembered_IllegalWhenNothingRemembered(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true) // nothing live-visible
	// setMineRemembered never called: mineRememberedFound stays false,
	// so NearestRememberedBlock (backing mineRememberedCandidateExists)
	// reports nothing either.
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:iron_ore"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMineRemembered]; legal {
		t.Fatalf("ActionMineRemembered legal = true, want false (nothing visible, nothing remembered - guaranteed to fail)")
	}
}

func TestActionMineRemembered_IllegalWithoutMineTargetBlock(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMineRemembered]; legal {
		t.Fatalf("ActionMineRemembered legal = true, want false (Config.MineTargetBlock unset)")
	}
}

func TestActionMineRemembered_DispatchesAndRewards(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true) // nothing live-visible
	agent.setMineRemembered("minecraft:iron_ore", true)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:iron_ore"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMineRemembered]; !legal {
		t.Fatalf("ActionMineRemembered legal = false, want true")
	}

	result, err := env.Step(context.Background(), rlenv.ActionMineRemembered)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (remembered block was mined)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (mineRememberedRewardBonus)", result.Reward)
	}
	if len(agent.mineRememberedCalls) != 1 || agent.mineRememberedCalls[0] != "minecraft:iron_ore" {
		t.Fatalf("mineRememberedCalls = %v, want exactly one call for minecraft:iron_ore", agent.mineRememberedCalls)
	}
}

func TestActionMineRemembered_NothingRememberedDoesNotEndEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	agent.setMineRemembered("minecraft:iron_ore", false) // nothing remembered either
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:iron_ore"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMineRemembered)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (nothing remembered, nothing mined)")
	}
}

// TestActionMineRemembered_RejectsPositionAfterUnreachableFailure is the
// fix found live, 2026-11-27: a remembered position that's still fully
// encased in undug stone fails with models.ErrNoInteractPosition every
// time it's tried (see MineBlockAt's own ApproachOptions.
// SkipUnreachableFallback doc comment), but nothing stopped
// ActionMineRemembered from being offered as legal again the very next
// step - and consecutiveStepTimeouts' shared give-up budget wasn't a
// reliable backstop either, since unrelated, successfully-resolving
// actions interleaved with the repeated failures kept resetting it before
// it ever reached its threshold. One such failure must be enough to stop
// masking ActionMineRemembered legal for that exact position again this
// episode.
func TestActionMineRemembered_RejectsPositionAfterUnreachableFailure(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true) // nothing live-visible
	agent.setMineRemembered("minecraft:coal_ore", true)
	agent.setMineRememberedErr(fmt.Errorf("approach block to mine: %w", models.ErrNoInteractPosition))
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:coal_ore"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMineRemembered]; !legal {
		t.Fatalf("ActionMineRemembered legal = false, want true (before the failed attempt)")
	}

	result, err := env.Step(context.Background(), rlenv.ActionMineRemembered)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (a failed approach doesn't end the episode)")
	}

	if legal := env.ActionMask()[rlenv.ActionMineRemembered]; legal {
		t.Fatalf("ActionMineRemembered legal = true, want false (this exact position already failed with ErrNoInteractPosition - stop offering it)")
	}
}

// fetchRememberedTestConfig poses a standalone-placement task with
// UsePlaceIngredientMemory enabled, matching placeIngredientMemoryTestConfig
// — ActionFetchRemembered's own legality reuses placeIngredientValid
// directly.
func fetchRememberedTestConfig() rlenv.Config {
	cfg := testConfig()
	cfg.PlaceTargetItem = "minecraft:dirt"
	cfg.UsePlaceIngredientMemory = true
	return cfg
}

func TestActionFetchRemembered_LegalWhenPlaceIngredientRemembered(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setPlaceTarget("minecraft:dirt", 0) // holds none yet, but InventoryCount now tracks it
	inner.setFetchRememberedFound(true)
	agent := &fakeIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 1, Y: 2, Z: 3}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), fetchRememberedTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionFetchRemembered]; !legal {
		t.Fatalf("ActionFetchRemembered legal = false, want true")
	}

	result, err := env.Step(context.Background(), rlenv.ActionFetchRemembered)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (remembered item was fetched)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (fetchRememberedRewardBonus)", result.Reward)
	}
	if len(agent.fetchRememberedCalls) != 1 || agent.fetchRememberedCalls[0] != "minecraft:dirt" {
		t.Fatalf("fetchRememberedCalls = %v, want exactly one call for minecraft:dirt", agent.fetchRememberedCalls)
	}
}

func TestActionFetchRemembered_IllegalWithoutUsePlaceIngredientMemory(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setFetchRememberedFound(true)
	agent := &fakeIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 1, Y: 2, Z: 3}, found: true}

	cfg := fetchRememberedTestConfig()
	cfg.UsePlaceIngredientMemory = false

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionFetchRemembered]; legal {
		t.Fatalf("ActionFetchRemembered legal = true, want false (UsePlaceIngredientMemory disabled)")
	}
}

func TestActionFetchRemembered_IllegalWhenAlreadyHeld(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setPlaceTarget("minecraft:dirt", 1) // already holds it
	inner.setFetchRememberedFound(true)
	agent := &fakeIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 1, Y: 2, Z: 3}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), fetchRememberedTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionFetchRemembered]; legal {
		t.Fatalf("ActionFetchRemembered legal = true, want false (already holds the target item)")
	}
}
