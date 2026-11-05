package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// TestMineWithholdsSuccessUntilCollected reproduces the gap found live on
// mine_buried, 2026-11-04: breaking the target alone used to end the
// episode as a success regardless of whether the bot ever kept the drop.
// With Config.CollectDrops set and collection failing (the walk-and-wait
// found nothing it could reach in time), the episode must not credit
// success for the break alone.
func TestMineWithholdsSuccessUntilCollected(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:oak_log", 1, 0, 0)
	agent.collectNearbyItemsFail = true
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:oak_log"
	cfg.CollectDrops = true
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	result, err := env.Step(ctx, rlenv.ActionMine)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true even though CollectNearbyItems collected nothing - a broken block with no drop kept must not count as mined success")
	}
	if agent.collectDropsCalls != 1 {
		t.Fatalf("collectDropsCalls = %d, want 1 - collection must still be attempted", agent.collectDropsCalls)
	}
}

// TestMineSucceedsOnceCollected is WithholdsSuccessUntilCollected's
// opposite: once CollectNearbyItems actually collects the drop, the same
// break does credit success exactly as it would without Config.CollectDrops
// at all.
func TestMineSucceedsOnceCollected(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:oak_log", 1, 0, 0)
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:oak_log"
	cfg.CollectDrops = true
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	result, err := env.Step(ctx, rlenv.ActionMine)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false even though CollectNearbyItems collected the drop, want true")
	}
}

// TestMinedViaNonMineDispatchStillCollects reproduces the actual root cause
// of the live gap (not just its symptom): mined is judged from world state,
// not from which action was dispatched (a goto step that mines through
// blocks along its own path breaks them just as a dedicated ActionMine step
// does - see BuriedTargetAgent's exact-broken-block fallback, exercised
// here via BrokenBlocksReporter). Before this fix, collectDrops only ran
// when dispatch.name == mineActionName, so a break credited through this
// path during a non-mine action (ActionWait stands in for "whatever action
// a goto embedding MineThrough actually was") silently skipped collection
// entirely - which is exactly how roughly half of mine_buried's agents
// ended up without the ore they'd "mined."
func TestMinedViaNonMineDispatchStillCollects(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true) // never LOS-resolved, same as the exact-broken-block precedent
	agent := &fakeBrokenBlocksAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	cfg.CollectDrops = true
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.pending = []models.BrokenBlock{{X: 5, Y: 0, Z: 0, BlockName: "minecraft:coal_ore"}}
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false after an exact broken-block match with collection succeeding, want true")
	}
	if agent.collectDropsCalls != 1 {
		t.Fatalf("collectDropsCalls = %d, want 1 - the break was credited via BrokenBlocksReporter, not an ActionMine dispatch, so a dispatch-name-gated collect call would have stayed at 0", agent.collectDropsCalls)
	}
}

// TestMinedViaNonMineDispatchWithheldWithoutCollection combines the two
// fixes: a break credited through the non-mine-dispatch path (as above)
// still must not succeed if collection then fails.
func TestMinedViaNonMineDispatchWithheldWithoutCollection(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	inner.collectNearbyItemsFail = true
	agent := &fakeBrokenBlocksAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	cfg.CollectDrops = true
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.pending = []models.BrokenBlock{{X: 5, Y: 0, Z: 0, BlockName: "minecraft:coal_ore"}}
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true even though collection failed, want false")
	}
	if agent.collectDropsCalls != 1 {
		t.Fatalf("collectDropsCalls = %d, want 1 - collection must still be attempted even though it then fails", agent.collectDropsCalls)
	}
}
