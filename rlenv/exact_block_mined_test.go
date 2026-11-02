package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// fakeDenseOreAgent wraps fakeAgent to satisfy both rlenv.BuriedTargetAgent
// and rlenv.BrokenBlocksReporter at once - needed to prove the new
// exact-coordinate fallback catches what the existence-radius check alone
// cannot: exists stays true the whole time (simulating dense ore seeding,
// where destroying one instance never makes "does any instance still
// exist" go false), while pending supplies the one confirmed break that
// should still be credited.
type fakeDenseOreAgent struct {
	*fakeAgent
	exists  bool
	pending []models.BrokenBlock
}

func (f *fakeDenseOreAgent) BlockExistsInRadius(_ context.Context, x, y, z float64, blockName string, radius int) (bool, error) {
	return f.exists, nil
}

func (f *fakeDenseOreAgent) DrainBrokenBlocks() []models.BrokenBlock {
	drained := f.pending
	f.pending = nil
	return drained
}

// TestExactBrokenBlockCreditsMinedEvenWhenOtherInstancesStillExistNearby
// reproduces the exact bug found live, 2026-11-02 (docs/plans/21-buried-
// mining-success.md's own triage): with 150 ore instances densely seeded,
// BuriedTargetAgent.BlockExistsInRadius's own "does any instance still
// exist nearby" question stays true even after the real target gets
// destroyed, because plenty of other instances remain in range - so a
// bot could repeatedly, genuinely destroy real target ore and never once
// be credited. The new exact-coordinate fallback must catch this even
// though the existence check alone would miss it entirely.
func TestExactBrokenBlockCreditsMinedEvenWhenOtherInstancesStillExistNearby(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true) // never LOS-resolved - the exact scenario found live
	agent := &fakeDenseOreAgent{fakeAgent: inner, exists: true}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// An ordinary step first: exists=true, nothing broken yet - must not
	// end the episode or pay the mine bonus.
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true before anything was actually broken")
	}

	// The pathfinder incidentally destroys the real target ore - exists
	// stays true throughout (other instances remain in range), so
	// BlockExistsInRadius alone would never notice. The exact-coordinate
	// fallback should credit it anyway.
	agent.pending = []models.BrokenBlock{{X: 5, Y: 0, Z: 0, BlockName: "minecraft:coal_ore"}}
	result, err = env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false after an exact broken-block match, want true - BlockExistsInRadius alone (exists=true throughout, simulating dense seeding) would miss this entirely")
	}
	if result.Reward <= 5 {
		t.Fatalf("Reward = %v, want > 5 (should include mineRewardBonus)", result.Reward)
	}
}

// TestBrokenBlocksDrainedEveryStepRegardlessOfTask confirms Step drains
// BrokenBlocksReporter on every call, not just while buriedTargetActive()
// - otherwise an unrelated task's own incidental breaks (e.g.
// chain_gather's tree) pile up unread in the agent's own buffer for the
// rest of its lifetime.
func TestBrokenBlocksDrainedEveryStepRegardlessOfTask(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	agent := &fakeBrokenBlocksAgent{fakeAgent: inner}
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
	if agent.drainCalls != 1 {
		t.Fatalf("drainCalls = %d, want 1 - must drain every Step regardless of task, or the buffer grows unbounded", agent.drainCalls)
	}
}

// TestBrokenBlocksOnlyCreditDuringBuriedSearch confirms a broken block
// matching MineTargetBlock does not, by itself, end an ordinary
// (non-buried) mine episode via this new fallback - that path is
// deliberately gated on buriedTargetActive(), same scoping as
// digProgressBonus/depthProgressBonus.
func TestBrokenBlocksOnlyCreditDuringBuriedSearch(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	agent := &fakeBrokenBlocksAgent{fakeAgent: inner}
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone" // ordinary (non-buried) mine task
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.pending = []models.BrokenBlock{{X: 1, Y: 1, Z: 1, BlockName: "minecraft:stone"}}
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true via the exact-broken-block fallback outside a buried search, want false - that path is gated on buriedTargetActive()")
	}
}

// TestBrokenBlocksNoOpWithoutAgentSupport confirms an agent that doesn't
// implement BrokenBlocksReporter is left alone, not failed over - same
// convention as BuriedTargetAgent/BlocksMinedCounter.
func TestBrokenBlocksNoOpWithoutAgentSupport(t *testing.T) {
	agent := newFakeAgent(0, 0, 0) // plain fakeAgent: no DrainBrokenBlocks
	agent.setMineBlockHidden(true)
	env := newTestEnvironment(t, agent, buriedTestConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for i := 0; i < 3; i++ {
		result, err := env.Step(context.Background(), rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if result.Done {
			t.Fatalf("Step %d: Done = true with no BrokenBlocksReporter support, nothing should trigger completion", i)
		}
	}
}
