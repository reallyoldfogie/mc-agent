package rlenv_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// fakeMemoryEditAgent wraps *fakeAgent with the two methods
// rlenv.MemoryEditAgent needs, recording every call rather than touching
// a real memorystore.Store — this package tests Environment.Reset's own
// decision logic (when to clear, when to seed, which of the two), not
// the store itself (that's agent/memory_test.go's job).
type fakeMemoryEditAgent struct {
	*fakeAgent
	forgetCalls []forgetCall
	seedCalls   []seedCall
	forgetErr   error
	seedErr     error
}

type forgetCall struct {
	pos    models.V3
	radius int
}

type seedCall struct {
	pos       models.V3
	blockName string
}

func (f *fakeMemoryEditAgent) ForgetSightingsNear(pos models.V3, radius int) error {
	f.forgetCalls = append(f.forgetCalls, forgetCall{pos, radius})
	return f.forgetErr
}

func (f *fakeMemoryEditAgent) RecordManualSighting(pos models.V3, blockName string) error {
	f.seedCalls = append(f.seedCalls, seedCall{pos, blockName})
	return f.seedErr
}

func newMemoryResetTestEnvironment(t *testing.T, agent *fakeMemoryEditAgent, cfg rlenv.Config) *rlenv.Environment {
	t.Helper()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	return env
}

const memoryResetTestStepTimeout = 200 * time.Millisecond

func TestResetClearsStaleMemoryWithinRadiusForBuriedTask(t *testing.T) {
	agent := &fakeMemoryEditAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	env := newMemoryResetTestEnvironment(t, agent, rlenv.Config{
		MineTargetBuried:       true,
		MineTargetBlock:        "minecraft:coal_ore",
		MineSearchRadius:       16,
		ArrivalThreshold:       0.5,
		StepTimeout:            memoryResetTestStepTimeout,
		GoToTargetDisabled:     true,
		MemoryResetClearRadius: 40,
	})

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.forgetCalls) != 1 {
		t.Fatalf("ForgetSightingsNear calls = %d, want 1", len(agent.forgetCalls))
	}
	if agent.forgetCalls[0].radius != 40 {
		t.Fatalf("ForgetSightingsNear radius = %d, want 40", agent.forgetCalls[0].radius)
	}
}

func TestResetSkipsMemoryClearWhenNotBuriedTask(t *testing.T) {
	agent := &fakeMemoryEditAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	env := newMemoryResetTestEnvironment(t, agent, rlenv.Config{
		// MineTargetBuried left false: a plain (non-buried) mine task,
		// where the clone-area-jitter staleness problem doesn't apply.
		MineTargetBlock:        "minecraft:coal_ore",
		MineSearchRadius:       16,
		ArrivalThreshold:       0.5,
		StepTimeout:            memoryResetTestStepTimeout,
		GoToTargetDisabled:     true,
		MemoryResetClearRadius: 40,
	})

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.forgetCalls) != 0 {
		t.Fatalf("ForgetSightingsNear calls = %d, want 0 (not a buried task)", len(agent.forgetCalls))
	}
}

func TestResetSkipsMemoryClearWhenRadiusZero(t *testing.T) {
	agent := &fakeMemoryEditAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	env := newMemoryResetTestEnvironment(t, agent, rlenv.Config{
		MineTargetBuried:   true,
		MineTargetBlock:    "minecraft:coal_ore",
		MineSearchRadius:   16,
		ArrivalThreshold:   0.5,
		StepTimeout:        memoryResetTestStepTimeout,
		GoToTargetDisabled: true,
		// MemoryResetClearRadius left at its 0 default.
	})

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.forgetCalls) != 0 {
		t.Fatalf("ForgetSightingsNear calls = %d, want 0 (MemoryResetClearRadius unset)", len(agent.forgetCalls))
	}
}

// smallBuriedScanConfig keeps findRealMineTargetPosition's candidate
// space small enough (9*9*5 = 405 cells, comfortably under
// maxMineTargetScanSamples) that it's fully exhaustible every Reset -
// see memory_reset.go's own comment on why a random subset without
// replacement still needs samples >= total to guarantee finding a
// single specific planted block, which these tests depend on
// deterministically.
func smallBuriedScanConfig() rlenv.Config {
	return rlenv.Config{
		MineTargetBuried:        true,
		MineTargetBlock:         "minecraft:coal_ore",
		MineSearchRadius:        4, // buriedExploreDepth falls back to this/2 = 2
		MineBuriedExploreRadius: 8, // buriedExploreRadius = this/2 = 4, xzSpan = 9
		ArrivalThreshold:        0.5,
		StepTimeout:             memoryResetTestStepTimeout,
		GoToTargetDisabled:      true,
	}
}

func TestResetSeedsAccurateMemoryWhenProbForcesIt(t *testing.T) {
	agent := &fakeMemoryEditAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	agent.setMineBlock("minecraft:coal_ore", 3, 0, 3)
	cfg := smallBuriedScanConfig()
	cfg.MemorySeedAccurateProb = 1 // roll is always < 1: always "accurate"
	env := newMemoryResetTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.seedCalls) != 1 {
		t.Fatalf("RecordManualSighting calls = %d, want 1", len(agent.seedCalls))
	}
	if got := agent.seedCalls[0]; got.blockName != "minecraft:coal_ore" || got.pos.X != 3 || got.pos.Z != 3 {
		t.Fatalf("seeded sighting = %+v, want minecraft:coal_ore at (3,_,3) — the real planted block", got)
	}
}

func TestResetSeedsInaccurateMemoryOffsetFromRealPositionWhenProbForcesIt(t *testing.T) {
	agent := &fakeMemoryEditAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	agent.setMineBlock("minecraft:coal_ore", 3, 0, 3)
	cfg := smallBuriedScanConfig()
	// accurateProb=0, inaccurateProb=1: roll is always >= 0 (skips
	// "accurate") and always < 0+1 (skips "none"), so always inaccurate.
	cfg.MemorySeedInaccurateProb = 1
	cfg.MemorySeedInaccurateOffset = 8
	env := newMemoryResetTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.seedCalls) != 1 {
		t.Fatalf("RecordManualSighting calls = %d, want 1", len(agent.seedCalls))
	}
	got := agent.seedCalls[0]
	if got.blockName != "minecraft:coal_ore" {
		t.Fatalf("seeded block name = %q, want minecraft:coal_ore", got.blockName)
	}
	dx, dy, dz := got.pos.X-3, got.pos.Y-0, got.pos.Z-3
	moved := 0
	for _, d := range []float64{dx, dy, dz} {
		if d != 0 {
			moved++
			if d != 8 && d != -8 {
				t.Fatalf("seeded position offset = %v, want exactly +-8 on the one axis that moved", d)
			}
		}
	}
	if moved != 1 {
		t.Fatalf("seeded position %v moved on %d axes relative to the real (3,0,3), want exactly 1", got.pos, moved)
	}
}

func TestResetSeedsNothingWhenRollFallsInRemainingProbability(t *testing.T) {
	agent := &fakeMemoryEditAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	agent.setMineBlock("minecraft:coal_ore", 3, 0, 3)
	cfg := smallBuriedScanConfig()
	// Both left at 0: the entire roll range is "no seed" - unchanged
	// existing behavior.
	env := newMemoryResetTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.seedCalls) != 0 {
		t.Fatalf("RecordManualSighting calls = %d, want 0 (both seed probs 0)", len(agent.seedCalls))
	}
}

func TestResetSeedsNothingWhenNoRealBlockExistsInFootprint(t *testing.T) {
	agent := &fakeMemoryEditAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	// No setMineBlock call: nothing real anywhere for the scan to find.
	cfg := smallBuriedScanConfig()
	cfg.MemorySeedAccurateProb = 1
	env := newMemoryResetTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.seedCalls) != 0 {
		t.Fatalf("RecordManualSighting calls = %d, want 0 (nothing real to seed from)", len(agent.seedCalls))
	}
}
