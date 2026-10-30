package rlenv_test

import (
	"context"
	"errors"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

var errLightPortalFrameTest = errors.New("simulated: ignition never confirmed")

// --- ActionLightPortal (docs/plans/NETHER_END_PORTAL_TRANSFER_PLAN.md, local/untracked, Phase 7 follow-up gap) ---

func TestStepLightPortalWithRadiusZeroIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.frameFound = true
	agent.frame = models.PortalFrame{Axis: "x", InteriorMin: models.V3{X: 5, Y: 0, Z: 0}, InteriorMax: models.V3{X: 5, Y: 2, Z: 0}, IgnitionBlock: models.V3{X: 5, Y: 0, Z: 0}}
	env := newTestEnvironment(t, agent, testConfig()) // LightPortalSearchRadius unset (0)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionLightPortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.lightPortalFrameCalls != 0 {
		t.Fatalf("LightPortalFrame calls = %d, want 0 (Config.LightPortalSearchRadius unset, safe no-op like ActionWait)", agent.lightPortalFrameCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[84]; visible != 0 {
		t.Fatalf("frameVisible = %v, want 0 (no task configured)", visible)
	}
}

func TestStepLightPortalWithNoVisibleFrameDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.LightPortalSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if legal(env, rlenv.ActionLightPortal) {
		t.Fatalf("ActionLightPortal legal with nothing visible, want illegal (mask=%v)", env.ActionMask())
	}

	result, err := env.Step(context.Background(), rlenv.ActionLightPortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.lightPortalFrameCalls != 0 {
		t.Fatalf("LightPortalFrame calls = %d, want 0 (nothing visible to light)", agent.lightPortalFrameCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepLightPortalDispatchesWithVisibleFrame(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.frameFound = true
	agent.frame = models.PortalFrame{Axis: "x", InteriorMin: models.V3{X: 5, Y: 0, Z: 0}, InteriorMax: models.V3{X: 5, Y: 2, Z: 0}, IgnitionBlock: models.V3{X: 5, Y: 0, Z: 0}}
	cfg := testConfig()
	cfg.LightPortalSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if !legal(env, rlenv.ActionLightPortal) {
		t.Fatalf("ActionLightPortal illegal with a visible frame, want legal (mask=%v)", env.ActionMask())
	}

	result, err := env.Step(context.Background(), rlenv.ActionLightPortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.lightPortalFrameCalls != 1 {
		t.Fatalf("LightPortalFrame calls = %d, want 1", agent.lightPortalFrameCalls)
	}
	if visible := result.Observation.Values[84]; visible != 1 {
		t.Fatalf("frameVisible = %v, want 1", visible)
	}
	// frameDx/Dy/Dz at indices 81-83: frame ignition block is at (5,0,0), agent at (0,0,0).
	if dx := result.Observation.Values[81]; dx != 5 {
		t.Fatalf("frameDx = %v, want 5", dx)
	}
}

func TestStepLightPortalPropagatesLightPortalFrameError(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.frameFound = true
	agent.frame = models.PortalFrame{Axis: "x", InteriorMin: models.V3{X: 5, Y: 0, Z: 0}, InteriorMax: models.V3{X: 5, Y: 2, Z: 0}, IgnitionBlock: models.V3{X: 5, Y: 0, Z: 0}}
	agent.lightPortalFrameErr = errLightPortalFrameTest
	cfg := testConfig()
	cfg.LightPortalSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionLightPortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.lightPortalFrameCalls != 1 {
		t.Fatalf("LightPortalFrame calls = %d, want 1 (dispatch still happens; the error is the agent's own)", agent.lightPortalFrameCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (lightPortalRewardBonus is paid on an observed block-state change, not on dispatching the action - a failed attempt changes nothing)")
	}
}

// TestStepLightPortalIgnitesAndEndsEpisode mirrors
// TestStepMineActionMinesVisibleTargetAndEndsEpisode's own shape: dispatching
// ActionLightPortal against a visible, unlit frame should cause the fake's
// own LightPortalFrame to flip the frame's interior block name within the
// same Step call, which lightPortalRewardBonus's "block name changed" check
// (environment.go's litThisStep) picks up without needing a cross-step
// mutation (unlike portalRewardBonus/dimensionChangedThisStep, judged purely
// from world state that only a real server respawn event - not this
// dispatch - can change).
func TestStepLightPortalIgnitesAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.frameFound = true
	agent.frame = models.PortalFrame{Axis: "x", InteriorMin: models.V3{X: 5, Y: 0, Z: 0}, InteriorMax: models.V3{X: 5, Y: 2, Z: 0}, IgnitionBlock: models.V3{X: 5, Y: 0, Z: 0}}
	cfg := testConfig()
	cfg.LightPortalSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionLightPortal)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.lightPortalFrameCalls != 1 {
		t.Fatalf("LightPortalFrame calls = %d, want 1", agent.lightPortalFrameCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (frame was ignited)")
	}
	if result.Reward < lightPortalRewardBonusForTest-1 {
		t.Fatalf("reward = %v, want at least lightPortalRewardBonus (%v) minus a small time penalty", result.Reward, lightPortalRewardBonusForTest)
	}
	if got := agent.BlockNameAt(5, 0, 0); got != "minecraft:nether_portal" {
		t.Fatalf("block at frame interior after Step = %q, want minecraft:nether_portal", got)
	}
}

// lightPortalRewardBonusForTest mirrors reward.go's own unexported
// lightPortalRewardBonus constant (10.0) - duplicated here rather than
// exported, matching this package's existing convention (see
// portalRewardBonusForTest in portal_test.go).
const lightPortalRewardBonusForTest = 10.0
