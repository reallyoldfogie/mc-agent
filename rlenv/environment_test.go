package rlenv_test

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
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
	// 17, not 14: indices 0-13 are the original per-task numeric features;
	// 14-16 are the goal-conditioning block (goalGoToActive/goalMineActive/
	// goalCraftActive) added by
	// ../mc-rsi-trainer/docs/plans/06-per-episode-task-selection-and-goal-conditioning.md
	// — see observation.go's own doc comment on observationSize for the
	// full layout. This exact value is also what makes cmd/rl-train's
	// EnvironmentID ("mc-agent-rlenv:actions=%d:obs=%d") automatically
	// reject an old checkpoint trained against the pre-goal-block size.
	if got := env.ObservationSize(); got != 17 {
		t.Fatalf("ObservationSize() = %d, want 17", got)
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
	if len(obs.Values) != 17 {
		t.Fatalf("len(obs.Values) = %d, want 17 (see TestObservationSizeAndActionSpace's own comment)", len(obs.Values))
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

// TestResetCancelsAndWaitsForAnActionLeftRunningByATimedOutStep: an action
// that outlives its step must not survive into the next episode, where it
// would act on the new episode's freshly seeded state (found live: a
// leftover bowl craft spent a chest episode's planks).
func TestResetCancelsAndWaitsForAnActionLeftRunningByATimedOutStep(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.moveToWithChatDelay = 5 * time.Second
	cfg := testConfig()
	cfg.StepTimeout = 20 * time.Millisecond
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(ctx, rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}

	start := time.Now()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("second Reset: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Reset took %v: it should cancel the running action, not wait out its 5s delay", elapsed)
	}
	agent.mu.Lock()
	canceled := agent.moveToCanceled
	agent.mu.Unlock()
	if !canceled {
		t.Fatal("the in-flight action never saw its context canceled by Reset")
	}
}

// TestMaxConsecutiveStepTimeoutsEndsAWedgedEpisode: an episode whose
// dispatched action never resolves within StepTimeout, step after step, must
// end after the configured count instead of running its whole step budget.
func TestMaxConsecutiveStepTimeoutsEndsAWedgedEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.moveToWithChatDelay = 5 * time.Second
	cfg := testConfig()
	cfg.StepTimeout = 10 * time.Millisecond
	cfg.MaxConsecutiveStepTimeouts = 3
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	for step := 1; step <= 3; step++ {
		result, err := env.Step(ctx, rlenv.ActionGoToTarget)
		if err != nil {
			t.Fatalf("Step %d: %v", step, err)
		}
		if want := step == 3; result.Done != want {
			t.Fatalf("Step %d: Done = %v, want %v", step, result.Done, want)
		}
	}
}

// TestMaxConsecutiveStepTimeoutsResetsWhenAnActionResolves: only consecutive
// timeouts count, and a new episode starts from zero.
func TestMaxConsecutiveStepTimeoutsResetsWhenAnActionResolves(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.StepTimeout = 200 * time.Millisecond
	cfg.MaxConsecutiveStepTimeouts = 2
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.moveToWithChatDelay = time.Second // times out
	if r, _ := env.Step(ctx, rlenv.ActionGoToTarget); r.Done {
		t.Fatal("one timeout must not end the episode")
	}
	// Resolves promptly (with an error, so the bot does not reach the target
	// and end the episode by arrival), resetting the count.
	agent.moveToWithChatDelay = 0
	agent.moveToWithChatErr = context.DeadlineExceeded
	if r, _ := env.Step(ctx, rlenv.ActionGoToTarget); r.Done {
		t.Fatal("a resolving step must not end the episode")
	}
	agent.moveToWithChatErr = nil
	agent.moveToWithChatDelay = time.Second // one timeout again, still below 2
	if r, _ := env.Step(ctx, rlenv.ActionGoToTarget); r.Done {
		t.Fatal("the count must have reset when the action resolved")
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

// TestResetRestoresBlocksMinedDuringTheEpisode: a mine episode whose target
// is part of the terrain (dirt/grass in a superflat world) must not leave a
// hole behind - found live as craters that made teleports land in pits.
func TestResetRestoresBlocksMinedDuringTheEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:dirt", 5, -1, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:dirt"
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.restoredBlocks) != 0 {
		t.Fatalf("nothing mined yet, but blocks were restored: %v", agent.restoredBlocks)
	}
	if _, err := env.Step(ctx, rlenv.ActionMine); err != nil {
		t.Fatalf("Step: %v", err)
	}

	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("second Reset: %v", err)
	}
	want := restoredBlock{5, -1, 0, "minecraft:dirt"}
	if len(agent.restoredBlocks) != 1 || agent.restoredBlocks[0] != want {
		t.Fatalf("restored = %v, want exactly [%v]", agent.restoredBlocks, want)
	}

	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("third Reset: %v", err)
	}
	if len(agent.restoredBlocks) != 1 {
		t.Fatalf("a block must be restored once, not on every later Reset: %v", agent.restoredBlocks)
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

// --- ActionCraft (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1) ---

func TestStepCraftWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCraftTarget("minecraft:stick", true, 0) // ready in the world, but Config never asks for it
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionCraft)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.craftItemCalls != 0 {
		t.Fatalf("CraftItem calls = %d, want 0 (Config.CraftTargetItem unset, safe no-op like ActionWait)", agent.craftItemCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if ready := result.Observation.Values[13]; ready != 0 {
		t.Fatalf("craftReady = %v, want 0 (no task configured)", ready)
	}
}

// TestStepCraftWithIngredientsNotReadyDoesNotDispatch mirrors
// TestStepMineWithNoVisibleTargetDoesNotDispatch — until 2026-09-12 this
// test (then named TestStepCraftWithIngredientsNotReadyStillDispatches)
// asserted the opposite: that ActionCraft dispatched to the real CraftItem
// call even with no ingredients present, on the theory that
// agent.Craftable's "approximate" caveat meant Environment shouldn't gate
// on it at all. Rereading that caveat: Craftable can only be
// over-optimistic (reports true for a multi-cell recipe needing two units
// of one scarce item when only one is held), never
// under-pessimistic — it never reports false for a genuinely craftable
// state. So gating on it, like ActionMine already gates on mineVisible,
// can't block a real opportunity; it can only stop the specific case this
// test now covers (zero ingredients at all), which used to reach the real
// CraftItem call and fail with a hard error instead of a graceful no-op —
// found live via testing/rl_train_test.go's
// TestRLTrainingLoop_LongRunLearnsToConditionOnTaskAvailabilityLive. See
// rlenv/action.go's actionLegal for the actual fix (added "&& e.craftReady").
func TestStepCraftWithIngredientsNotReadyDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCraftTarget("minecraft:stick", false, 0)
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionCraft)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.craftItemCalls != 0 {
		t.Fatalf("CraftItem calls = %d, want 0 (nothing to craft with)", agent.craftItemCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if ready := result.Observation.Values[13]; ready != 0 {
		t.Fatalf("craftReady = %v, want 0 (ingredients not ready)", ready)
	}
}

func TestStepCraftActionCraftsTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCraftTarget("minecraft:stick", true, 0)
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionCraft)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.craftItemCalls != 1 {
		t.Fatalf("CraftItem calls = %d, want 1", agent.craftItemCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target item was crafted)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (craftRewardBonus)", result.Reward)
	}
	if got := agent.InventoryCount("minecraft:stick"); got != 1 {
		t.Fatalf("held count after Step = %d, want 1", got)
	}
}

func TestStepObservationReflectsCraftReadyRegardlessOfAction(t *testing.T) {
	// craftReady is populated every step once configured, not only on steps
	// that dispatch ActionCraft — mirrors mineVisible's equivalent test.
	agent := newFakeAgent(0, 0, 0)
	agent.setCraftTarget("minecraft:stick", true, 0)
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if ready := obs.Values[13]; ready != 1 {
		t.Fatalf("craftReady at Reset = %v, want 1", ready)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.craftItemCalls != 0 {
		t.Fatalf("CraftItem calls = %d, want 0 (ActionWait doesn't dispatch craft)", agent.craftItemCalls)
	}
	if ready := result.Observation.Values[13]; ready != 1 {
		t.Fatalf("craftReady after ActionWait = %v, want 1 (still ready, nothing consumed)", ready)
	}
}

func TestStepCraftDoesNotAwardBonusWhenActionCraftFailsToProduceIt(t *testing.T) {
	// Judged from an actual inventory-count increase, not from the
	// dispatched action: if CraftItem itself errors (e.g. a real missing
	// ingredient), the held count never changes and no bonus/Done should be
	// granted — mirrors TestStepMineDoesNotAwardBonusWhenActionMineFailsToBreakIt.
	agent := newFakeAgent(0, 0, 0)
	agent.setCraftTarget("minecraft:stick", true, 0)
	agent.craftItemErr = context.DeadlineExceeded
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionCraft)
	if err != nil {
		t.Fatalf("Step: %v, want nil (a failed dispatched action is not an environment error)", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (nothing was actually crafted)")
	}
}

// --- Config.Seeder (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4) ---

func TestResetWithNoSeederConfiguredDoesNotSeed(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedNearbyBlockCalls) != 0 {
		t.Fatalf("SeedNearbyBlock calls = %d, want 0 (Config.Seeder unset)", len(agent.seedNearbyBlockCalls))
	}
}

func TestResetCallsSeederForConfiguredTasks(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.MineSearchRadius = 20
	cfg.CraftTargetItem = "minecraft:stick"
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedNearbyBlockCalls) != 1 {
		t.Fatalf("SeedNearbyBlock calls = %d, want 1", len(agent.seedNearbyBlockCalls))
	}
	if got := agent.seedNearbyBlockCalls[0]; got.blockName != "minecraft:stone" || got.radius != 20 {
		t.Fatalf("SeedNearbyBlock call = %+v, want {minecraft:stone 20}", got)
	}
	if len(agent.seedCraftIngredientsCalls) != 1 || agent.seedCraftIngredientsCalls[0] != "minecraft:stick" {
		t.Fatalf("SeedCraftIngredients calls = %v, want [minecraft:stick]", agent.seedCraftIngredientsCalls)
	}
}

func TestResetSkipsSeedingUnconfiguredTasks(t *testing.T) {
	// DefaultEpisodeSeeder should only seed the tasks this instance
	// actually poses — mirrors how ActionMine/ActionCraft themselves stay
	// no-ops when unconfigured.
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedCraftIngredientsCalls) != 0 {
		t.Fatalf("SeedCraftIngredients calls = %d, want 0 (Config.CraftTargetItem unset)", len(agent.seedCraftIngredientsCalls))
	}
}

func TestResetPropagatesSeederError(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.seedErr = context.DeadlineExceeded
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatalf("Reset with failing Seeder: want error, got nil")
	}
}

// --- Config.ResetOrigin ---

func TestResetWithoutResetOriginDoesNotTeleport(t *testing.T) {
	agent := newFakeAgent(10, 0, 10)
	cfg := testConfig()
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.teleportCalls) != 0 {
		t.Fatalf("teleportCalls = %d, want 0 (Config.ResetOrigin unset)", len(agent.teleportCalls))
	}
}

func TestResetTeleportsToConfiguredOrigin(t *testing.T) {
	agent := newFakeAgent(10, 0, 10)
	cfg := testConfig()
	origin := [3]float64{0, 4, 0}
	cfg.ResetOrigin = &origin
	env := newTestEnvironment(t, agent, cfg)

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.teleportCalls) != 1 || agent.teleportCalls[0] != (teleportCall{x: 0, y: 4, z: 0}) {
		t.Fatalf("teleportCalls = %v, want [{0 4 0}]", agent.teleportCalls)
	}
	// TargetOffset is {5,0,0}: the target should be relative to the
	// post-teleport origin (0,4,0), not the pre-teleport position (10,0,10)
	// — dx should read 5, not -5.
	if got := obs.Values[0]; got != 5 {
		t.Fatalf("post-teleport dx = %v, want 5 (target computed from post-teleport origin)", got)
	}
}

func TestResetPropagatesTeleportError(t *testing.T) {
	agent := newFakeAgent(10, 0, 10)
	agent.teleportErr = context.DeadlineExceeded
	cfg := testConfig()
	origin := [3]float64{0, 4, 0}
	cfg.ResetOrigin = &origin
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatalf("Reset with failing TeleportTo: want error, got nil")
	}
}

// --- Config.Jitter ---

func TestResetJitterVariesTargetAcrossEpisodes(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.Jitter = [3]float64{2, 0, 2}
	cfg.JitterSeed = 42
	env := newTestEnvironment(t, agent, cfg)

	obs1, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset 1: %v", err)
	}
	obs2, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset 2: %v", err)
	}
	if obs1.Values[0] == obs2.Values[0] && obs1.Values[2] == obs2.Values[2] {
		t.Fatalf("dx/dz identical across two Resets with Jitter set: %v vs %v — want variation", obs1.Values, obs2.Values)
	}
}

func TestResetJitterIsDeterministicForFixedSeed(t *testing.T) {
	cfg := testConfig()
	cfg.Jitter = [3]float64{2, 0, 2}
	cfg.JitterSeed = 42

	agent1 := newFakeAgent(0, 0, 0)
	env1 := newTestEnvironment(t, agent1, cfg)
	obs1, err := env1.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset (env1): %v", err)
	}

	agent2 := newFakeAgent(0, 0, 0)
	env2 := newTestEnvironment(t, agent2, cfg)
	obs2, err := env2.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset (env2): %v", err)
	}

	if obs1.Values[0] != obs2.Values[0] || obs1.Values[2] != obs2.Values[2] {
		t.Fatalf("jittered dx/dz differ across two Environments built with the same JitterSeed: %v vs %v — want identical", obs1.Values, obs2.Values)
	}
}

func TestResetJitterAppliesToResetOrigin(t *testing.T) {
	agent := newFakeAgent(10, 0, 10)
	cfg := testConfig()
	origin := [3]float64{0, 0, 0}
	cfg.ResetOrigin = &origin
	cfg.Jitter = [3]float64{5, 0, 5}
	cfg.JitterSeed = 42
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.teleportCalls) != 1 {
		t.Fatalf("teleportCalls = %d, want 1", len(agent.teleportCalls))
	}
	got := agent.teleportCalls[0]
	if got.x == 0 && got.z == 0 {
		t.Fatalf("teleport target = %+v, want jittered away from exact origin (0,_,0)", got)
	}
	if got.y != 0 {
		t.Fatalf("teleport Y = %v, want unchanged (Jitter[1]=0)", got.y)
	}
}

func TestResetJitterStaysWithinConfiguredMagnitude(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig() // TargetOffset = {5, 0, 0}
	cfg.Jitter = [3]float64{3, 0, 0}
	cfg.JitterSeed = 1
	env := newTestEnvironment(t, agent, cfg)

	for i := 0; i < 200; i++ {
		obs, err := env.Reset(context.Background())
		if err != nil {
			t.Fatalf("Reset %d: %v", i, err)
		}
		dx := obs.Values[0]
		if dx < 2 || dx > 8 {
			t.Fatalf("Reset %d: dx = %v, want within [2, 8] (TargetOffset.X=5 +/- Jitter.X=3)", i, dx)
		}
	}
}

func TestResetNeverPosesAnAlreadyArrivedTarget(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig() // ArrivalThreshold = 0.5
	// TargetOffset.X=1 with Jitter.X=3 means a naive single draw lands
	// within ArrivalThreshold (0.5) a large fraction of the time (X drawn
	// uniformly from [-2,4], and any |X|<0.5 with Z=0 qualifies) — this
	// configuration only passes if Reset's retry guard is actually doing
	// its job on the draws that would otherwise be degenerate.
	cfg.TargetOffset = [3]float64{1, 0, 0}
	cfg.Jitter = [3]float64{3, 0, 0}
	cfg.JitterSeed = 7
	env := newTestEnvironment(t, agent, cfg)

	for i := 0; i < 200; i++ {
		obs, err := env.Reset(context.Background())
		if err != nil {
			t.Fatalf("Reset %d: %v", i, err)
		}
		if dx := float64(obs.Values[0]); dx > -cfg.ArrivalThreshold && dx < cfg.ArrivalThreshold {
			t.Fatalf("Reset %d: dx = %v, posed an already-arrived target (within ArrivalThreshold %v)", i, dx, cfg.ArrivalThreshold)
		}
	}
}

func TestResetErrorsWhenJitterCannotAvoidArrival(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.ArrivalThreshold = 10 // no draw below can ever exceed this
	cfg.TargetOffset = [3]float64{0, 0, 0}
	cfg.Jitter = [3]float64{0.1, 0, 0.1}
	cfg.JitterSeed = 1
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err == nil {
		t.Fatalf("Reset with a TargetOffset/Jitter/ArrivalThreshold combination that can never avoid arrival: want error, got nil")
	}
}

// --- Config.StuckTimeout ---

func TestStepEndsEpisodeAfterStuckTimeout(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	// Large offset so the bot never arrives via ActionWait's zero movement,
	// isolating the effect under test.
	cfg.TargetOffset = [3]float64{100, 0, 0}
	cfg.StuckTimeout = 3
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	for i := 1; i <= 3; i++ {
		result, err := env.Step(context.Background(), rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		wantDone := i == 3
		if result.Done != wantDone {
			t.Fatalf("Step %d: Done = %v, want %v", i, result.Done, wantDone)
		}
	}
}

func TestStepDoesNotEndEpisodeBeforeStuckTimeout(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{100, 0, 0}
	cfg.StuckTimeout = 5
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	for i := 1; i <= 4; i++ {
		result, err := env.Step(context.Background(), rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if result.Done {
			t.Fatalf("Step %d: Done = true, want false (below StuckTimeout)", i)
		}
	}
}

func TestStepDisablesStuckTimeoutWhenZero(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{100, 0, 0}
	// StuckTimeout left at its zero value (disabled).
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	for i := 1; i <= 20; i++ {
		result, err := env.Step(context.Background(), rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step %d: %v", i, err)
		}
		if result.Done {
			t.Fatalf("Step %d: Done = true, want false (StuckTimeout disabled)", i)
		}
	}
}

func TestResetClearsStuckCounterAcrossEpisodes(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{100, 0, 0}
	cfg.StuckTimeout = 2
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if result, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step 1: %v", err)
	} else if result.Done {
		t.Fatalf("Step 1: Done = true, want false")
	}
	if result, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step 2: %v", err)
	} else if !result.Done {
		t.Fatalf("Step 2: Done = false, want true (StuckTimeout=2 reached)")
	}

	// A fresh episode should start with a clean stuck counter, not
	// immediately re-trigger from leftover state.
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if result, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step after Reset: %v", err)
	} else if result.Done {
		t.Fatalf("Step after Reset: Done = true, want false (stuck counter should have cleared)")
	}
}

// --- ActionMask (cRL-go rl.ActionMasker, docs/plans/19-training-time-action-masking.md) ---

func TestActionMaskAllLegalWhenNoTaskConfigured(t *testing.T) {
	// Wait/GoToTarget are always legal; Mine/Craft are illegal with no
	// task configured — mirrors resolveDispatch's own no-op conditions
	// (see actionLegal's doc comment for why the two must never drift).
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	mask := env.ActionMask()
	if len(mask) != rlenv.NumActions {
		t.Fatalf("len(ActionMask()) = %d, want %d", len(mask), rlenv.NumActions)
	}
	want := map[rl.Action]bool{
		rlenv.ActionWait:       true,
		rlenv.ActionGoToTarget: true,
		rlenv.ActionMine:       false,
		rlenv.ActionCraft:      false,
	}
	for action, wantLegal := range want {
		if got := mask[action]; got != wantLegal {
			t.Fatalf("mask[%d] = %v, want %v", action, got, wantLegal)
		}
	}
}

func TestActionMaskMineLegalOnlyWhenConfiguredAndVisible(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMine]; legal {
		t.Fatalf("ActionMine legal = true, want false (configured but nothing currently visible)")
	}
	// Cross-check the mask against what Step actually does with it, not
	// just the mask's own internal logic: a masked-illegal action must
	// also be the safe no-op resolveDispatch already makes it.
	if _, err := env.Step(context.Background(), rlenv.ActionMine); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.mineBlockAtCalls != 0 {
		t.Fatalf("MineBlockAt calls = %d, want 0 (mask reported ActionMine illegal)", agent.mineBlockAtCalls)
	}

	agent.setMineBlock("minecraft:stone", 5, 0, 0)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[12]; visible != 1 {
		t.Fatalf("mineVisible = %v, want 1 (sanity check target is now visible)", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionMine]; !legal {
		t.Fatalf("ActionMine legal = false, want true (target now visible)")
	}
}

// TestActionMaskCraftLegalOnlyWhenConfiguredAndReady mirrors
// TestActionMaskMineLegalOnlyWhenConfiguredAndVisible's structure exactly
// — until 2026-09-12 this test (then named
// TestActionMaskCraftLegalIffConfigured) asserted the opposite of what it
// checks now: that ActionCraft's legality tracked only Config.CraftTargetItem,
// not craftReady, "mirroring resolveDispatch's own lack of a craftReady
// gate." That asymmetry with ActionMine (which already correctly gates on
// mineVisible) turned out to be a real bug, not a deliberate design choice
// anyone had verified: testing/rl_train_test.go's
// TestRLTrainingLoop_LongRunLearnsToConditionOnTaskAvailabilityLive — the
// first live test to ever configure CraftTargetItem while craftReady could
// actually be false — caught ActionCraft being dispatched with no
// ingredients present, which reached the real CraftItem call and failed
// with a hard error instead of the graceful no-op ActionMine already got
// in the equivalent situation. Fixed in rlenv/action.go's actionLegal to
// add "&& e.craftReady", and this test rewritten to actually cover that
// gate instead of asserting its absence.
func TestActionMaskCraftLegalOnlyWhenConfiguredAndReady(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionCraft]; legal {
		t.Fatalf("ActionCraft legal = true, want false (configured but not currently craftable)")
	}
	// Cross-check the mask against what Step actually does with it, not
	// just the mask's own internal logic: a masked-illegal action must
	// also be the safe no-op resolveDispatch already makes it.
	if _, err := env.Step(context.Background(), rlenv.ActionCraft); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.craftItemCalls != 0 {
		t.Fatalf("CraftItem calls = %d, want 0 (mask reported ActionCraft illegal)", agent.craftItemCalls)
	}

	agent.setCraftTarget("minecraft:stick", true, 0)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if ready := result.Observation.Values[13]; ready != 1 {
		t.Fatalf("craftReady = %v, want 1 (sanity check target is now craftable)", ready)
	}
	if legal := env.ActionMask()[rlenv.ActionCraft]; !legal {
		t.Fatalf("ActionCraft legal = false, want true (target now craftable)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionCraft]; legal {
		t.Fatalf("ActionCraft legal = true, want false (Config.CraftTargetItem unset, even though craftable)")
	}
}

// compile-time check that fakeAgent satisfies both interfaces rlenv.LiveAgent
// requires, and that Environment satisfies cRL-go's rl.ActionMasker.
var (
	_ models.CommandAgent   = (*fakeAgent)(nil)
	_ models.HealthProvider = (*fakeAgent)(nil)
	_ rlenv.LiveAgent       = (*fakeAgent)(nil)
	_ rlenv.SeedAgent       = (*fakeAgent)(nil)
	_ rlenv.ResetAgent      = (*fakeAgent)(nil)
	_ rl.ActionMasker       = (*rlenv.Environment)(nil)
)
