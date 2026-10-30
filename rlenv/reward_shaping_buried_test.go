package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// TestPerQuadrantArrivalBonusPaysOnceEachNotJustFirst confirms Step pays
// arrivalBonus once per newly-explored quadrant during a buried-target
// search (see markExploredQuadrant's return value and arrivalBonus's own
// doc comment), not just the first-ever arrival in the episode: a bot that
// already collected the bonus reaching quadrant 1 still needs a reason to
// keep searching quadrants 2-4 once the ore isn't there.
func TestPerQuadrantArrivalBonusPaysOnceEachNotJustFirst(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	cfg := buriedTestConfig()
	cfg.MineSearchRadius = 4 // small radius - matches TestUnknownDirectionTracksExploredQuadrants
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.setPosition(2, -2, 2)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Reward < 5 {
		t.Fatalf("Reward for first-ever arrival at a quadrant = %v, want >= 5 (should include arrivalBonus)", result.Reward)
	}

	// Same point, second time - already explored, must not pay again.
	result, err = env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Reward >= 5 {
		t.Fatalf("Reward for re-visiting an already-explored quadrant = %v, want < 5 (bonus must not repeat)", result.Reward)
	}

	// A different, still-unexplored quadrant - must pay again.
	agent.setPosition(-2, -2, 2)
	result, err = env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Reward < 5 {
		t.Fatalf("Reward for first-ever arrival at a second, different quadrant = %v, want >= 5 (should include arrivalBonus again)", result.Reward)
	}
}

// TestDigProgressBonusRewardsConfirmedBlockBreaksDuringBuriedSearch
// confirms Step rewards actual digging (BlocksMinedCounter) during a
// buried-target search, not just movement toward it - see
// digProgressBonus's own doc comment for why: distance shaping plus the
// per-quadrant arrival bonus alone still can't distinguish a bot that
// genuinely digs through overburden from one that paces toward
// currentGoToTarget without ever breaking a block.
func TestDigProgressBonusRewardsConfirmedBlockBreaksDuringBuriedSearch(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	agent := &fakeBlocksMinedAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// No digging yet this step - ordinary small (likely negative) reward.
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Reward >= 1 {
		t.Fatalf("Reward with no blocks mined this step = %v, want < 1", result.Reward)
	}

	// Simulate 3 confirmed block breaks between steps - overburden-clearing
	// during pathfinding counts the same as a deliberate ActionMine, see
	// MineBlockAt's own doc comment.
	agent.count += 3
	result, err = env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Reward < 1 {
		t.Fatalf("Reward after 3 confirmed block breaks = %v, want >= 1 (should include digProgressBonus*3 = 1.5)", result.Reward)
	}
}

// TestDigProgressBonusNotAppliedOutsideBuriedSearch confirms the bonus is
// gated on buriedTargetActive() - an agent implementing BlocksMinedCounter
// for an ordinary (non-buried) task must not have its digging rewarded
// through this path, matching distanceRewardScale/arrivalBonus's own
// "belongs to the goto task only" scoping.
func TestDigProgressBonusNotAppliedOutsideBuriedSearch(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	agent := &fakeBlocksMinedAgent{fakeAgent: inner}
	cfg := testConfig() // plain goto task, not buried
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.count += 5
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Reward >= 1 {
		t.Fatalf("Reward = %v with BlocksMinedCounter incrementing outside a buried-target search, want < 1 (digProgressBonus must not fire)", result.Reward)
	}
}
