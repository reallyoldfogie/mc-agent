package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// TestBlindDestructionEndsEpisodeWhenAgentSupportsExistenceCheck covers
// BuriedTargetAgent (found live, mc-rsi-trainer, digging mine_buried's
// overburden with an Efficiency V tool): the pathfinder can destroy the
// buried target before FindVisibleBlock ever resolves it even once, which
// the ordinary prevMineVisible-gated `mined` check has no way to notice -
// it diffs one specific, already-resolved coordinate's block name, and
// with no coordinate ever resolved it has nothing to diff. An agent that
// implements BuriedTargetAgent gives Step a second, LOS-free way to
// detect the same destruction.
func TestBlindDestructionEndsEpisodeWhenAgentSupportsExistenceCheck(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true) // never resolvable via FindVisibleBlock
	agent := &fakeBuriedTargetAgent{fakeAgent: inner, exists: true}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.blockExistsInRadiusCalls) != 1 {
		t.Fatalf("BlockExistsInRadius calls after Reset = %d, want 1 (baseline read)", len(agent.blockExistsInRadiusCalls))
	}
	if got := agent.blockExistsInRadiusCalls[0]; got.blockName != "minecraft:coal_ore" || got.radius != 10 {
		t.Fatalf("BlockExistsInRadius call = %+v, want blockName=minecraft:coal_ore radius=10", got)
	}

	// Still there - an ordinary step must not end the episode or pay the
	// mine bonus.
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true on a step where the buried target still exists")
	}

	// Simulate the pathfinder blindly destroying it mid-episode - the
	// only thing that changed is BlockExistsInRadius's own next answer,
	// exactly like a real destruction would (no FindVisibleBlock call
	// ever having resolved it - inner.setMineBlockHidden(true) above
	// guarantees mineEverSeen stays false throughout).
	agent.exists = false
	result, err = env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false the step the buried target's existence check flips to false — blind destruction should still end the episode")
	}
	if result.Reward <= 5 {
		t.Fatalf("Reward = %v, want > 5 (should include mineRewardBonus)", result.Reward)
	}
}

// TestBlindDestructionNoOpWithoutAgentSupport confirms
// refreshMineBuriedExistence degrades to a no-op (never ends the episode
// on its own) when the agent doesn't implement BuriedTargetAgent - the
// same "an agent that doesn't support an optional capability is left
// alone" convention as ToolSeedAgent/EnchantedToolSeedAgent, not a
// failure.
func TestBlindDestructionNoOpWithoutAgentSupport(t *testing.T) {
	agent := newFakeAgent(0, 0, 0) // plain fakeAgent: no BlockExistsInRadius
	agent.setMineBlockHidden(true)
	cfg := buriedTestConfig()
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for i := 0; i < 3; i++ {
		result, err := env.Step(context.Background(), rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if result.Done {
			t.Fatalf("Step %d: Done = true with no BuriedTargetAgent support and no visible target - nothing should trigger completion", i)
		}
	}
}
