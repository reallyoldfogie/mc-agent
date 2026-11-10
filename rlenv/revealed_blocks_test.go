package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// TestRevealedBlockFallbackSetsEverSeenEvenWithoutLiveSighting reproduces
// the bug found live, 2026-11-10: spectating a bot walk past ore newly
// exposed in its own tunnel wall/floor. A single dispatch can mine
// through several blocks before refreshMineTarget's own once-per-Step
// FindVisibleBlock scan runs, and by the time it does, the bot may have
// moved past the exposure (out of MineSearchRadius or no longer in line
// of sight). RevealedBlocksReporter's drain must still mark the target
// as ever-seen from the recorded coordinate, even though the live scan
// below finds nothing.
func TestRevealedBlockFallbackSetsEverSeenEvenWithoutLiveSighting(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true) // the live FindVisibleBlock scan never resolves anything
	agent := &fakeRevealedBlocksAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible (ever-seen) = %v after Reset, want 0 (nothing revealed yet)", visible)
	}

	// Simulate a block revealed mid-dispatch, well outside what the live
	// end-of-step scan (MineSearchRadius 10, agent still at the origin)
	// would itself find, since the agent never moved.
	agent.pending = []models.BrokenBlock{{X: 5, Y: 0, Z: 0, BlockName: "minecraft:coal_ore"}}
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[12]; visible != 1 {
		t.Fatalf("mineVisible (ever-seen) = %v, want 1 - RevealedBlocksReporter's fallback should have set it from the recorded sighting alone", visible)
	}
	wantDx := float32(5.5) // recorded block center (5,0,0)+0.5 minus the bot's own position (0,0,0)
	if dx := result.Observation.Values[9]; dx != wantDx {
		t.Fatalf("mineDx = %v, want %v (the revealed block's recorded position)", dx, wantDx)
	}
}

// TestRevealedBlockFallbackOverriddenByLiveSighting confirms the live
// FindVisibleBlock scan always wins over the revealed-block fallback when
// both resolve something this Step - the fallback exists only to cover
// the gap where the live scan finds nothing, not to compete with it.
func TestRevealedBlockFallbackOverriddenByLiveSighting(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlock("minecraft:coal_ore", 3, 0, 4) // visible from the start
	agent := &fakeRevealedBlocksAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// A decoy revealed sighting at a different position - the live scan
	// should still win since it resolves the real, currently-visible block.
	agent.pending = []models.BrokenBlock{{X: 9, Y: 0, Z: 0, BlockName: "minecraft:coal_ore"}}
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	wantDx, wantDz := float32(3), float32(4) // the live block's position (3,0,4), not the decoy's (9,0,0)
	if dx := result.Observation.Values[9]; dx != wantDx {
		t.Fatalf("mineDx = %v, want %v - the live sighting must win over the revealed-block decoy", dx, wantDx)
	}
	if dz := result.Observation.Values[11]; dz != wantDz {
		t.Fatalf("mineDz = %v, want %v - the live sighting must win over the revealed-block decoy", dz, wantDz)
	}
}

// TestRevealedBlockNameMismatchIgnored confirms a revealed block that
// doesn't match this episode's mine target is drained (so the agent's
// own buffer doesn't grow unbounded) but never marks the target as
// ever-seen.
func TestRevealedBlockNameMismatchIgnored(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	agent := &fakeRevealedBlocksAgent{fakeAgent: inner}
	cfg := buriedTestConfig() // target: minecraft:coal_ore
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.pending = []models.BrokenBlock{{X: 5, Y: 0, Z: 0, BlockName: "minecraft:stone"}}
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[12]; visible != 0 {
		t.Fatalf("mineVisible (ever-seen) = %v, want 0 - a non-matching revealed block must not mark the target as seen", visible)
	}
	if agent.drainCalls != 2 { // once at Reset, once at Step
		t.Fatalf("drainCalls = %d, want 2 - the buffer must still be drained even on a name mismatch", agent.drainCalls)
	}
}

// TestRevealedBlocksDrainedEveryStepRegardlessOfTask mirrors
// TestBrokenBlocksDrainedEveryStepRegardlessOfTask: the agent's own
// revealedBlocks buffer must be drained every Step regardless of task,
// not just while buriedTargetActive(), or an unrelated task's own
// incidental reveals pile up unread for the agent's whole lifetime.
func TestRevealedBlocksDrainedEveryStepRegardlessOfTask(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	agent := &fakeRevealedBlocksAgent{fakeAgent: inner}
	env, err := rlenv.New(agent, actions.NewRegistry(), testConfig()) // ordinary goto task, not buried
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.drainCalls < 2 { // once at Reset, once at Step
		t.Fatalf("drainCalls = %d, want >= 2 (Reset + Step) - draining must happen regardless of task", agent.drainCalls)
	}
}
