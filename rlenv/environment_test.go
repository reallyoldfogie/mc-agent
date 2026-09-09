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
	if got := env.ObservationSize(); got != 14 {
		t.Fatalf("ObservationSize() = %d, want 14", got)
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
	if len(obs.Values) != 14 {
		t.Fatalf("len(obs.Values) = %d, want 14", len(obs.Values))
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

func TestStepCraftWithIngredientsNotReadyStillDispatches(t *testing.T) {
	// Unlike ActionMine (which never dispatches without a visible target),
	// ActionCraft always dispatches once Config.CraftTargetItem is set —
	// CraftItem itself is the source of truth for whether ingredients are
	// actually available (see agent.Craftable's doc comment on why the
	// craftReady signal is approximate), so Environment doesn't withhold
	// dispatch based on its own coarser check.
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
	if agent.craftItemCalls != 1 {
		t.Fatalf("CraftItem calls = %d, want 1", agent.craftItemCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (not ready, nothing crafted)")
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

// compile-time check that fakeAgent satisfies both interfaces rlenv.LiveAgent
// requires.
var (
	_ models.CommandAgent   = (*fakeAgent)(nil)
	_ models.HealthProvider = (*fakeAgent)(nil)
	_ rlenv.LiveAgent       = (*fakeAgent)(nil)
	_ rlenv.SeedAgent       = (*fakeAgent)(nil)
	_ rlenv.ResetAgent      = (*fakeAgent)(nil)
)
