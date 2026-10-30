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
		t.Fatalf("Done = true, want false (a failed portal attempt is not a terminal condition - no reward/task is tied to it yet)")
	}
}
