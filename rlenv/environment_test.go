package rlenv_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

func testConfig() rlenv.Config {
	return rlenv.Config{
		TargetOffset:     [3]float64{5, 0, 0},
		ArrivalThreshold: 0.5,
		StepTimeout:      200 * time.Millisecond,
	}
}

func newTestEnvironment(t *testing.T, agent *fakeAgent, cfg rlenv.Config) *rlenv.Environment {
	t.Helper()
	registry := actions.NewRegistry()
	env, err := rlenv.New(agent, registry, cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	return env
}

func TestNewRejectsInvalidArguments(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	registry := actions.NewRegistry()
	valid := testConfig()

	if _, err := rlenv.New(nil, registry, valid); err == nil {
		t.Fatalf("nil agent: want error, got nil")
	}
	if _, err := rlenv.New(agent, nil, valid); err == nil {
		t.Fatalf("nil registry: want error, got nil")
	}
	invalid := valid
	invalid.ArrivalThreshold = 0
	if _, err := rlenv.New(agent, registry, invalid); err == nil {
		t.Fatalf("invalid config: want error, got nil")
	}
}

func TestObservationSizeAndActionSpace(t *testing.T) {
	env := newTestEnvironment(t, newFakeAgent(0, 0, 0), testConfig())
	if got := env.ObservationSize(); got != 13 {
		t.Fatalf("ObservationSize() = %d, want 13", got)
	}
	if got := env.ActionSpace(); got != rlenv.NumActions {
		t.Fatalf("ActionSpace() = %d, want %d", got, rlenv.NumActions)
	}
}

func TestResetCapturesOriginAndPosesTarget(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(obs.Values) != 13 {
		t.Fatalf("len(obs.Values) = %d, want 13", len(obs.Values))
	}
	if dx := obs.Values[0]; dx != 5 {
		t.Fatalf("dx = %v, want 5 (target offset)", dx)
	}
	if known := obs.Values[8]; known != 0 {
		t.Fatalf("healthKnown = %v, want 0 (no HealthChange observed yet)", known)
	}
}

func TestResetErrorsWhenPositionUnknown(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.posKnown = false
	env := newTestEnvironment(t, agent, testConfig())

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatalf("Reset with unknown position: want error, got nil")
	}
}

func TestStepBeforeResetErrors(t *testing.T) {
	env := newTestEnvironment(t, newFakeAgent(0, 0, 0), testConfig())
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err == nil {
		t.Fatalf("Step before Reset: want error, got nil")
	}
}

func TestStepWaitDoesNotDispatchMovement(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveToWithChat calls = %d, want 0 for ActionWait", agent.moveToWithChatCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (bot never moved toward the target)")
	}
	if result.Reward >= 0 {
		t.Fatalf("Reward = %v, want negative (time penalty only, no progress)", result.Reward)
	}
}

func TestStepGoToTargetReachesAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionGoToTarget)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 1 {
		t.Fatalf("MoveToWithChat calls = %d, want 1", agent.moveToWithChatCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (fakeAgent simulates instant arrival)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (progress + arrival bonus)", result.Reward)
	}
	if dx := result.Observation.Values[0]; dx > 0.01 || dx < -0.01 {
		t.Fatalf("post-arrival dx = %v, want ~0", dx)
	}
}

func TestStepReturnHomeDispatchesToOriginNotTarget(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step(GoToTarget): %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionReturnHome)
	if err != nil {
		t.Fatalf("Step(ReturnHome): %v", err)
	}
	pos, _, _, _ := agent.GetPosition()
	if pos.X != 0 || pos.Y != 0 || pos.Z != 0 {
		t.Fatalf("position after ReturnHome = %v, want (0,0,0) (origin)", pos)
	}
	if result.Reward >= 0 {
		t.Fatalf("Reward = %v, want negative (moved away from the GoToTarget objective)", result.Reward)
	}
}

func TestStepAppliesDamagePenaltyBetweenSteps(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setHealth(20, 20, 5)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step 1: %v", err)
	}

	agent.setHealth(12, 20, 5)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step 2: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (health > 0)")
	}
	if result.Reward >= -1 {
		t.Fatalf("Reward = %v, want a clear damage penalty (< -1)", result.Reward)
	}
}

func TestStepReflectsExternalPositionChange(t *testing.T) {
	// Position can change for reasons unrelated to the dispatched action
	// (knockback, water current, another system moving the bot) — reward
	// is computed from the bot's actual position each step, not just from
	// what the action itself did, so this must show up the same way
	// self-directed movement does.
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.setPosition(4, 0, 0) // pushed 4 blocks toward the target externally
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (external push reduced distance to target)", result.Reward)
	}
}

func TestStepDeathEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setHealth(20, 20, 5)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step 1: %v", err)
	}

	agent.setHealth(0, 20, 5)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step 2: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (health reached 0)")
	}
}

// TestStepSwallowsActionFailureWithoutError covers the dispatched action's
// own models.Completion resolving with a real (non-timeout) error — e.g.
// MoveToWithChat's pathfinding genuinely failing. See awaitStep's doc
// comment: this is treated the same as a step timeout, not propagated as
// a Step error, since reward is computed from wherever the bot actually
// ended up, not from the completion's outcome.
func TestStepSwallowsActionFailureWithoutError(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.moveToWithChatErr = context.DeadlineExceeded // MoveToWithChat "fails" every call: position never changes
	cfg := testConfig()
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionGoToTarget)
	if err != nil {
		t.Fatalf("Step: %v, want nil (a failed dispatched action is not an environment error)", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (target never reached)")
	}
}

// TestStepTimesOutWhileActionStillRunning covers Config.StepTimeout
// actually elapsing before the dispatched action's models.Completion
// resolves — Step must return promptly around StepTimeout rather than
// blocking for however long the (still in-flight) action takes.
func TestStepTimesOutWhileActionStillRunning(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.moveToWithChatDelay = 200 * time.Millisecond
	cfg := testConfig()
	cfg.StepTimeout = 20 * time.Millisecond
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	start := time.Now()
	result, err := env.Step(context.Background(), rlenv.ActionGoToTarget)
	if err != nil {
		t.Fatalf("Step: %v, want nil (a step timeout is not an environment error)", err)
	}
	if elapsed := time.Since(start); elapsed >= agent.moveToWithChatDelay {
		t.Fatalf("Step took %v, want it to return around StepTimeout (%v) instead of blocking for the full in-flight action duration", elapsed, cfg.StepTimeout)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (target not actually reached yet)")
	}
}

func TestStepPropagatesContextCancellation(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.moveToWithChatErr = context.DeadlineExceeded // position never changes
	cfg := testConfig()
	cfg.StepTimeout = time.Second
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := env.Step(ctx, rlenv.ActionGoToTarget); err == nil {
		t.Fatalf("Step with canceled ctx: want error, got nil")
	}
}

// --- ActionMine (docs/plans/RL_ACTION_SPACE_EXPANSION.md Phase 2) ---

func TestStepMineWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 5, 0, 0) // exists in the world, but Config never asks for it
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMine)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.mineBlockAtCalls != 0 {
		t.Fatalf("MineBlockAt calls = %d, want 0 (Config.MineTargetBlock unset, safe no-op like ActionWait)", agent.mineBlockAtCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[12]; visible != 0 {
		t.Fatalf("mineVisible = %v, want 0 (no task configured)", visible)
	}
}

func TestStepMineWithNoVisibleTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMine)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.mineBlockAtCalls != 0 {
		t.Fatalf("MineBlockAt calls = %d, want 0 (nothing visible to mine)", agent.mineBlockAtCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepMineActionMinesVisibleTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 5, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMine)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.mineBlockAtCalls != 1 {
		t.Fatalf("MineBlockAt calls = %d, want 1", agent.mineBlockAtCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target block was mined)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (mineRewardBonus)", result.Reward)
	}
	if got := agent.BlockNameAt(5, 0, 0); got != "minecraft:air" {
		t.Fatalf("block at mine target after Step = %q, want minecraft:air", got)
	}
}

func TestStepObservationReflectsMineTargetDeltaRegardlessOfAction(t *testing.T) {
	// mineDx/Dy/Dz/mineVisible are populated every step once configured,
	// not only on steps that dispatch ActionMine — mirrors how
	// GoToTarget's dx/dy/dz is always present regardless of chosen action.
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 3, 0, 4)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[12]; visible != 1 {
		t.Fatalf("mineVisible = %v, want 1", visible)
	}
	if dx, dz := obs.Values[9], obs.Values[11]; dx != 3 || dz != 4 {
		t.Fatalf("mineDx,mineDz = %v,%v, want 3,4", dx, dz)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.mineBlockAtCalls != 0 {
		t.Fatalf("MineBlockAt calls = %d, want 0 (ActionWait doesn't dispatch mine)", agent.mineBlockAtCalls)
	}
	if visible := result.Observation.Values[12]; visible != 1 {
		t.Fatalf("mineVisible after ActionWait = %v, want 1 (target still there, unmined)", visible)
	}
}

func TestStepMineDoesNotAwardBonusWhenActionMineFailsToBreakIt(t *testing.T) {
	// Judged from actual world state, not from the dispatched action: if
	// MineBlockAt itself errors (e.g. the real implementation's server
	// rejected the dig, out of reach), the block never changes and no
	// bonus/Done should be granted — mirrors
	// TestStepSwallowsActionFailureWithoutError's movement-side precedent.
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 5, 0, 0)
	agent.mineBlockAtErr = context.DeadlineExceeded
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMine)
	if err != nil {
		t.Fatalf("Step: %v, want nil (a failed dispatched action is not an environment error)", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (block never actually broke)")
	}
}

// compile-time check that fakeAgent satisfies both interfaces rlenv.LiveAgent
// requires.
var (
	_ models.CommandAgent   = (*fakeAgent)(nil)
	_ models.HealthProvider = (*fakeAgent)(nil)
	_ rlenv.LiveAgent       = (*fakeAgent)(nil)
)
