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

// TestMineSucceedsViaInventoryDeltaWhenEntityChaseFindsNothing reproduces
// the dominant live cause of collector_truncation found 2026-11-06
// (docs/plans/21-buried-mining-success.md, collectDrops' own diagnostic
// log): CollectNearbyItems' own entity-chasing result says nothing was
// collected, but the bot's inventory count of the drop item increased
// anyway within the same step - vanilla's own passive pickup (automatic
// once the bot stands close enough) beat it there. The step must still
// credit success: collected == 0 is not evidence nothing was gathered.
func TestMineSucceedsViaInventoryDeltaWhenEntityChaseFindsNothing(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:coal_ore", 1, 0, 0)
	agent.collectNearbyItemsFail = true
	agent.silentPickupItem = "minecraft:coal"
	agent.silentPickupCount = 1
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:coal_ore"
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
		t.Fatalf("Done = false even though the bot's own coal count increased this step, want true - collectDrops' own entity-chasing result finding nothing is not proof nothing was collected")
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

// TestMineSucceedsViaBreakPositionFallbackWhenBotHasMovedOn reproduces the
// mechanism confirmed live, 2026-11-07, behind collector_truncation's
// smaller remaining bucket (docs/plans/21-buried-mining-success.md): a
// single step's own clamped goto hop can mine through several blocks
// before the agent settles (an 8-block dig in one step, confirmed live),
// leaving the break - and its drop - several blocks behind where the
// bot-anchored search (CollectNearbyItems) then centers, finding nothing
// even though the drop is genuinely sitting right where the block broke.
// collectDrops' own break-position fallback (models.ItemCollectorAt) must
// search from there instead once the bot-anchored search comes up empty.
func TestMineSucceedsViaBreakPositionFallbackWhenBotHasMovedOn(t *testing.T) {
	inner := newFakeAgent(10, 0, 0) // far from the break, as if the hop already moved on
	inner.setMineBlockHidden(true)
	inner.collectNearbyItemsFail = true // the bot-anchored search finds nothing
	breakPos := models.V3{X: 5, Y: 0, Z: 0}
	inner.collectAtWantAnchor = &breakPos
	inner.collectAtResult = 1 // the break-anchored fallback finds the drop
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
		t.Fatalf("Done = false even though the break-position fallback found the drop, want true")
	}
	if agent.collectDropsCalls != 1 {
		t.Fatalf("collectDropsCalls = %d, want 1 - the bot-anchored search must still run first", agent.collectDropsCalls)
	}
	if agent.collectAtCalls != 1 {
		t.Fatalf("collectAtCalls = %d, want 1 - the break-position fallback must run once the bot-anchored search finds nothing", agent.collectAtCalls)
	}
}

// TestMineWithholdsSuccessWhenBreakPositionFallbackAlsoFindsNothing is
// BreakPositionFallbackWhenBotHasMovedOn's opposite: when neither the
// bot-anchored search nor the break-position fallback finds anything (the
// drop is genuinely gone, not just behind), success must still be
// withheld exactly as it was before the fallback existed.
func TestMineWithholdsSuccessWhenBreakPositionFallbackAlsoFindsNothing(t *testing.T) {
	inner := newFakeAgent(10, 0, 0)
	inner.setMineBlockHidden(true)
	inner.collectNearbyItemsFail = true
	// collectAtWantAnchor left nil: the fallback is available but finds
	// nothing either, mirroring collectNearbyItemsFail's own "found
	// nothing, not an error" contract.
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
		t.Fatalf("Done = true even though neither search found the drop, want false")
	}
	if agent.collectAtCalls != 1 {
		t.Fatalf("collectAtCalls = %d, want 1 - the fallback must still be attempted", agent.collectAtCalls)
	}
}

// TestBrokenBlocksDrainRestoresNonTargetBlocksToo reproduces a gap found
// live, 2026-11-07, while investigating collector_truncation's smaller
// remaining bucket (docs/plans/21-buried-mining-success.md): the
// BrokenBlocksReporter drain only ever tracked its *first* target-matching
// block for restoration (e.minedBlocks) - a filler block (stone, dirt)
// broken incidentally in the same drain, or a second target instance, was
// silently dropped and never restored at the next Reset. Both must be
// restored: restoreMinedBlocks is a generic "put back whatever broke"
// mechanism, not one that should care why a block broke.
func TestBrokenBlocksDrainRestoresNonTargetBlocksToo(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	agent := &fakeBrokenBlocksAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.pending = []models.BrokenBlock{
		{X: 1, Y: 0, Z: 0, BlockName: "minecraft:stone"},
		{X: 5, Y: 0, Z: 0, BlockName: "minecraft:coal_ore"},
		{X: 2, Y: 0, Z: 0, BlockName: "minecraft:stone"},
	}
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("second Reset: %v", err)
	}

	want := []restoredBlock{
		{1, 0, 0, "minecraft:stone"},
		{5, 0, 0, "minecraft:coal_ore"},
		{2, 0, 0, "minecraft:stone"},
	}
	if len(agent.restoredBlocks) != len(want) {
		t.Fatalf("restored = %v, want all %d broken blocks restored: %v", agent.restoredBlocks, len(want), want)
	}
	for _, w := range want {
		found := false
		for _, r := range agent.restoredBlocks {
			if r == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("restored = %v, missing %v", agent.restoredBlocks, w)
		}
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
