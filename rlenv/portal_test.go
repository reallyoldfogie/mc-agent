package rlenv_test

import (
	"context"
	"errors"
	"testing"

	"github.com/reallyoldfogie/mc-agent/rlenv"
)

var errUsePortalAtTest = errors.New("simulated: dimension never changed")

// --- ActionUsePortal (docs/plans/NETHER_END_PORTAL_TRANSFER_PLAN.md, local/untracked, Phase 7's rlenv follow-up) ---

func TestStepUsePortalWithRadiusZeroIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.portalFound, agent.portalX, agent.portalY, agent.portalZ = true, 5, 0, 0
	env := newTestEnvironment(t, agent, testConfig()) // PortalSearchRadius unset (0)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionUsePortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.usePortalAtCalls != 0 {
		t.Fatalf("UsePortalAt calls = %d, want 0 (Config.PortalSearchRadius unset, safe no-op like ActionWait)", agent.usePortalAtCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[79]; visible != 0 {
		t.Fatalf("portalVisible = %v, want 0 (no task configured)", visible)
	}
}

func TestStepUsePortalWithNoVisiblePortalDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.PortalSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if legal(env, rlenv.ActionUsePortal) {
		t.Fatalf("ActionUsePortal legal with nothing visible, want illegal (mask=%v)", env.ActionMask())
	}

	result, err := env.Step(context.Background(), rlenv.ActionUsePortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.usePortalAtCalls != 0 {
		t.Fatalf("UsePortalAt calls = %d, want 0 (nothing visible to use)", agent.usePortalAtCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepUsePortalDispatchesWithVisiblePortal(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.portalFound, agent.portalX, agent.portalY, agent.portalZ = true, 5, 0, 0
	cfg := testConfig()
	cfg.PortalSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if !legal(env, rlenv.ActionUsePortal) {
		t.Fatalf("ActionUsePortal illegal with a visible portal, want legal (mask=%v)", env.ActionMask())
	}

	result, err := env.Step(context.Background(), rlenv.ActionUsePortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.usePortalAtCalls != 1 {
		t.Fatalf("UsePortalAt calls = %d, want 1", agent.usePortalAtCalls)
	}
	if visible := result.Observation.Values[79]; visible != 1 {
		t.Fatalf("portalVisible = %v, want 1", visible)
	}
	// portalDx/Dy/Dz at indices 76-78: portal is at (5,0,0), agent at (0,0,0).
	if dx := result.Observation.Values[76]; dx != 5 {
		t.Fatalf("portalDx = %v, want 5", dx)
	}
}

func TestStepUsePortalPropagatesUsePortalAtError(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.portalFound, agent.portalX, agent.portalY, agent.portalZ = true, 5, 0, 0
	agent.usePortalAtErr = errUsePortalAtTest
	cfg := testConfig()
	cfg.PortalSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionUsePortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.usePortalAtCalls != 1 {
		t.Fatalf("UsePortalAt calls = %d, want 1 (dispatch still happens; the error is the agent's own)", agent.usePortalAtCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (portalRewardBonus is paid on an observed dimension change, not on dispatching the action - a failed attempt changes nothing)")
	}
}

// TestStepPortalChangePaysBonusAndEndsEpisode mirrors
// TestStepSleptThroughNightPaysBonusAndEndsEpisode's own shape exactly:
// mutate the fake's tracked world state directly between two Step calls
// (simulating a dimension change caused by anything - not necessarily this
// step's own dispatched action, matching portalRewardBonus's own "judge
// from world-state deltas" philosophy) and confirm the bonus pays and the
// episode ends.
func TestStepPortalChangePaysBonusAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.dimensionTypeID, agent.dimensionName, agent.dimensionInitialized = 0, "minecraft:overworld", true
	env := newTestEnvironment(t, agent, testConfig()) // PortalSearchRadius unset - reward must not depend on it

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// A step where nothing changes: no bonus, no Done.
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true on an ordinary step, want false")
	}

	// Simulate a dimension transfer completing (e.g. the real agent's
	// onRespawn dispatch updating its tracked dimension) between steps.
	agent.dimensionTypeID, agent.dimensionName = 1, "minecraft:the_nether"

	result, err = env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false after an observed dimension change, want true")
	}
	// Allow for the per-step time penalty subtracted from every step
	// regardless of outcome - this just needs to clearly be "the bonus
	// fired," not match it to the exact cent.
	if result.Reward < portalRewardBonusForTest-1 {
		t.Fatalf("reward = %v, want at least portalRewardBonus (%v) minus a small time penalty", result.Reward, portalRewardBonusForTest)
	}
}

// portalRewardBonusForTest mirrors reward.go's own unexported
// portalRewardBonus constant (10.0) - duplicated here rather than
// exported, matching this package's existing convention (see
// sleptThroughNightBonusForTest in sleep_test.go).
const portalRewardBonusForTest = 10.0
