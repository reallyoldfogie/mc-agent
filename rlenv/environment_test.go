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
	// 76 (31 before rl-action-space-expansion-ii's own block, 25 before
	// the Sleep block, 21 before the exploration-signal block, 17 before
	// the chain block, 14 before the goal block): indices 0-13 are the
	// original per-task numeric features; 14-16 are the goal-conditioning
	// block (goalGoToActive/goalMineActive/goalCraftActive) added by
	// ../mc-rsi-trainer/docs/plans/06-per-episode-task-selection-and-goal-conditioning.md;
	// 17-20 are the chain block; 21-24 are unknownDx/Dy/Dz/unknownValid,
	// added by
	// ../mc-rsi-trainer/docs/plans/14-buried-block-mining-and-memory-map.md's
	// option 2; 25-30 are isNight/timeUntilDawn/bedDx/Dy/Dz/bedVisible,
	// added by docs/plans/SLEEP_ACTION_PLAN.md (local/untracked); 31-75
	// are rl-action-space-expansion-ii's own equip-through-mace-smash
	// block (equip 31-32, pickup 33-37, fly 38-43, ride 44-49, combat
	// 50-55, standalone-place 56-57, follow 58-60, creeper-gunpowder
	// 61-65, spear-charge 66-70, mace-smash 71-75), shifted from that
	// branch's own 21-65 numbering when it merged — see observation.go's
	// own doc comment on observationSize for the full layout. This exact
	// value is also what makes cmd/rl-train's EnvironmentID
	// ("mc-agent-rlenv:actions=%d:obs=%d") automatically reject an old
	// checkpoint trained against a shorter observation.
	if got := env.ObservationSize(); got != 76 {
		t.Fatalf("ObservationSize() = %d, want 76", got)
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
	if len(obs.Values) != 76 {
		t.Fatalf("len(obs.Values) = %d, want 76 (see TestObservationSizeAndActionSpace's own comment)", len(obs.Values))
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

// TestResetClearsTheSpaceAboveTheGroundAroundTheOrigin: leftover seeded
// blocks and tables above the ground must be wiped each episode, from the
// origin's own Y up, leaving the ground alone.
func TestResetClearsTheSpaceAboveTheGroundAroundTheOrigin(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	origin := [3]float64{10.5, -60, 5.2}
	cfg := testConfig()
	cfg.ResetOrigin = &origin
	cfg.ClearAreaRadius = 3
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	want := clearedBox{7, -60, 2, 13, -56, 8} // radius 3 around (10,5), y from -60 up the default 4
	if len(agent.clearedBoxes) != 1 || agent.clearedBoxes[0] != want {
		t.Fatalf("cleared = %v, want exactly [%v]", agent.clearedBoxes, want)
	}
}

// TestResetClearOriginYRoundsRatherThanFloors guards the bug found live
// 2026-10-04: a resting feet-Y is always meant to be a clean integer
// (groundY+1), but a capture taken before physics fully settles it can
// land a hair below that integer. Flooring such a value rounded the clear
// box's Y down into the ground block itself, wiping it - repeated trainer
// restarts (each re-capturing ResetOrigin fresh) ate an entire 3-block
// soil layer down to bedrock on two of ten bots over several days.
func TestResetClearOriginYRoundsRatherThanFloors(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	origin := [3]float64{10.5, -60.00003, 5.2} // a hair below -60, not exactly -60
	cfg := testConfig()
	cfg.ResetOrigin = &origin
	cfg.ClearAreaRadius = 3
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	want := clearedBox{7, -60, 2, 13, -56, 8} // must round to -60, not floor to -61
	if len(agent.clearedBoxes) != 1 || agent.clearedBoxes[0] != want {
		t.Fatalf("cleared = %v, want exactly [%v] (a Floor regression would clear y=-61, the ground block itself)", agent.clearedBoxes, want)
	}
}

// TestResetClonesTheTemplateInsteadOfClearingWhenBothAreSet: CloneAreaFrom
// takes priority over ClearAreaRadius - a destination Y comes from the
// template, never from the live, possibly-imprecise ResetOrigin.
func TestResetClonesTheTemplateInsteadOfClearingWhenBothAreSet(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	origin := [3]float64{10.5, -60.00003, 5.2} // a hair below -60 - must not affect the clone's Y at all
	template := [3]float64{0, -61, 5000}
	cfg := testConfig()
	cfg.ResetOrigin = &origin
	cfg.ClearAreaRadius = 3 // set too, to prove it's ignored once CloneAreaFrom is set
	cfg.CloneAreaFrom = &template
	cfg.CloneAreaRadius = 3
	cfg.CloneAreaDepth = 3
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.clearedBoxes) != 0 {
		t.Fatalf("ClearAir must not be called once CloneAreaFrom is set: %v", agent.clearedBoxes)
	}
	want := clonedArea{-3, -64, 4997, 3, -57, 5003, 7, -64, 2}
	if len(agent.clonedAreas) != 1 || agent.clonedAreas[0] != want {
		t.Fatalf("cloned = %v, want exactly [%v]", agent.clonedAreas, want)
	}
}

// TestResetClonesInZBandsWhenTheFullBoxExceedsVanillasCloneLimit confirms
// cloneArea splits into multiple /clone calls rather than issuing one
// doomed-to-fail oversized command, or silently cloning nothing - found
// live, 2026-10-29, widening -clone-area-radius to 32 so it covers
// mine_buried's own, larger search radius (the two had silently drifted
// apart: a radius-16 template left a third of mine_buried's 24-radius
// search volume with no ore ever possible). Radius 32/height 4(default)/
// depth 8 is 65*65*13=54,925 blocks, over vanilla's 32,768-block /clone
// cap in one call.
func TestResetClonesInZBandsWhenTheFullBoxExceedsVanillasCloneLimit(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	origin := [3]float64{10, -60, 5}
	template := [3]float64{0, -60, 5000}
	cfg := testConfig()
	cfg.ResetOrigin = &origin
	cfg.CloneAreaFrom = &template
	cfg.CloneAreaRadius = 32
	cfg.CloneAreaDepth = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.clonedAreas) < 2 {
		t.Fatalf("clonedAreas = %v, want at least 2 calls (one would exceed vanilla's 32,768-block /clone limit)", agent.clonedAreas)
	}

	const r = 32
	const wantXSpan = 2*r + 1 // 65
	const wantYSpan = 4 + 8 + 1
	wantZRows := 0
	for i, c := range agent.clonedAreas {
		if c.srcX1 != -r || c.srcX2 != r {
			t.Fatalf("call %d: X span = [%d,%d], want [%d,%d] unchanged by Z-banding", i, c.srcX1, c.srcX2, -r, r)
		}
		rows := c.srcZ2 - c.srcZ1 + 1
		if blocks := wantXSpan * rows * wantYSpan; blocks > 32768 {
			t.Fatalf("call %d covers %d blocks (%d x %d x %d), over vanilla's 32,768 limit", i, blocks, wantXSpan, rows, wantYSpan)
		}
		if got, want := c.dstX1-10, c.srcX1; got != want {
			t.Fatalf("call %d: dst X offset from src = %d, want %d (same relative offset every band)", i, got, want)
		}
		if c.dstZ1-c.srcZ1 != 5-5000 {
			t.Fatalf("call %d: dst/src Z offset = %d, want %d (every band must carry the same relative Z shift)", i, c.dstZ1-c.srcZ1, 5-5000)
		}
		wantZRows += rows
	}
	if wantZRows != wantXSpan {
		t.Fatalf("total Z rows across all calls = %d, want %d (the whole span, no gaps, no overlaps)", wantZRows, wantXSpan)
	}
	// No gaps/overlaps: each call's own srcZ1 must pick up exactly where
	// the previous one's srcZ2 left off.
	for i := 1; i < len(agent.clonedAreas); i++ {
		if agent.clonedAreas[i].srcZ1 != agent.clonedAreas[i-1].srcZ2+1 {
			t.Fatalf("call %d starts at Z=%d, want %d (right after call %d's own Z=%d)", i, agent.clonedAreas[i].srcZ1, agent.clonedAreas[i-1].srcZ2+1, i-1, agent.clonedAreas[i-1].srcZ2)
		}
	}
}

func TestResetDoesNotClearWhenTheFeatureIsOff(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	origin := [3]float64{10, -60, 5}
	cfg := testConfig()
	cfg.ResetOrigin = &origin
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.clearedBoxes) != 0 {
		t.Fatalf("ClearAreaRadius is 0 but the area was cleared: %v", agent.clearedBoxes)
	}
}

// TestMineOnlyEpisodeIgnoresThePhantomGotoTarget: with the goto task
// disabled, the jittered "target" left over from Reset is not a goal. Moving
// onto it used to pay the +10 arrival bonus and end the episode without the
// block ever being mined, and moving toward it earned distance shaping.
func TestMineOnlyEpisodeIgnoresThePhantomGotoTarget(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 40, 0, 0) // far: not the thing under test
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.TargetOffset = [3]float64{3, 0, 0} // the phantom target
	cfg.ArrivalThreshold = 1.5
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.setPosition(3, 0, 0) // walk right onto the phantom target
	result, err := env.Step(ctx, rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatal("Done = true: arriving at a phantom target must not end a mine episode")
	}
	if result.Reward > 0 || result.Reward < -0.5 {
		t.Fatalf("Reward = %v, want only the tiny time penalty: no shaping, no arrival bonus", result.Reward)
	}
}

// TestCompositeGotoThenMinePaysArrivalOnceAndEndsOnTheMine covers "go there,
// then mine": reaching the goto target pays once without ending the episode;
// only the mine does.
func TestCompositeGotoThenMinePaysArrivalOnceAndEndsOnTheMine(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 6, 0, 0)
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{5, 0, 0}
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.ArrivalThreshold = 1.5
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	arrive, err := env.Step(ctx, rlenv.ActionGoToTarget)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if arrive.Done {
		t.Fatal("arriving must not end a go-then-mine episode")
	}
	if arrive.Reward < 10 {
		t.Fatalf("arrival reward = %v, want the arrival bonus (>= 10) plus shaping", arrive.Reward)
	}

	again, _ := env.Step(ctx, rlenv.ActionWait)
	if again.Done || again.Reward > 0 {
		t.Fatalf("staying at the target: Done=%v Reward=%v, want no second arrival bonus", again.Done, again.Reward)
	}

	mine, err := env.Step(ctx, rlenv.ActionMine)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !mine.Done || mine.Reward < 9 {
		t.Fatalf("mining: Done=%v Reward=%v, want the episode to end on the mine bonus", mine.Done, mine.Reward)
	}
}

// TestSeedAtGoalPlacesTargetsNextToTheGotoTargetAndRemovesThemNextReset:
// a composite episode's block must be near the goto target (out of sight of
// the start), not beside the bot, and must not pile up across episodes.
func TestSeedAtGoalPlacesTargetsNextToTheGotoTargetAndRemovesThemNextReset(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{30, 0, 0}
	cfg.MineTargetBlock = "minecraft:sand"
	cfg.SeedAtGoal = true
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedNearbyBlockCalls) != 0 {
		t.Fatalf("SeedNearbyBlock was called %d times: a far episode must not seed beside the bot", len(agent.seedNearbyBlockCalls))
	}
	if len(agent.seedBlockAtCalls) != 1 || agent.seedBlockAtCalls[0] != (farSeedCall{"minecraft:sand", 32, 0, 0}) {
		t.Fatalf("SeedBlockAt calls = %v, want sand two blocks +X of the goto target (30,0,0)", agent.seedBlockAtCalls)
	}

	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("second Reset: %v", err)
	}
	removed := false
	for _, r := range agent.restoredBlocks {
		if r == (restoredBlock{32, 0, 0, "minecraft:air"}) {
			removed = true
		}
	}
	if !removed {
		t.Fatalf("restored = %v: the previous episode's far-seeded block must be removed at the next Reset", agent.restoredBlocks)
	}
}

func TestSeedAtGoalIsIgnoredWithoutACompositeTask(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:sand"
	cfg.SeedAtGoal = true
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedBlockAtCalls) != 0 || len(agent.seedNearbyBlockCalls) != 1 {
		t.Fatalf("a plain mine episode must seed beside the bot as before: far=%v near=%v", agent.seedBlockAtCalls, agent.seedNearbyBlockCalls)
	}
}

func TestStepChargesTheConfiguredTimePenaltyPerStep(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.TimePenalty = 0.2
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	for step := 1; step <= 3; step++ {
		result, err := env.Step(ctx, rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step %d: %v", step, err)
		}
		if result.Reward != -0.2 {
			t.Fatalf("Step %d reward = %v, want -0.2: a wasted step must cost the configured penalty", step, result.Reward)
		}
	}
}

func TestNewRejectsANegativeTimePenalty(t *testing.T) {
	cfg := testConfig()
	cfg.TimePenalty = -0.1
	if _, err := rlenv.New(newFakeAgent(0, 0, 0), actions.NewRegistry(), cfg); err == nil {
		t.Fatal("a negative TimePenalty would reward wasted steps and must be rejected")
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

// TestStepObservationPersistsMineTargetAfterLineOfSightIsLost is the core
// new behavior docs/plans/14-buried-block-mining-and-memory-map.md's
// "minimum viable" memory-map option adds: once Config.MineTargetBlock has
// been visible at least once this episode, the observation keeps reporting
// its last-known position even on a step where FindVisibleBlock currently
// finds nothing (LOS lost, not the block itself gone) — a target glimpsed
// once stays a navigable goal instead of vanishing from the policy's input
// the instant current-step LOS is lost.
func TestStepObservationPersistsMineTargetAfterLineOfSightIsLost(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:coal_ore", 3, 0, 4)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:coal_ore"
	cfg.MineTargetBuried = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Lose line-of-sight (the block itself is unchanged - BlockNameAt at
	// (3,0,4) still reports minecraft:coal_ore, so this isn't "mined").
	agent.setMineBlockHidden(true)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[12]; visible != 1 {
		t.Fatalf("mineVisible after losing LOS = %v, want 1 (remembered from this episode)", visible)
	}
	if dx, dz := result.Observation.Values[9], result.Observation.Values[11]; dx != 3 || dz != 4 {
		t.Fatalf("mineDx,mineDz after losing LOS = %v,%v, want 3,4 (remembered position)", dx, dz)
	}

	// The memory is observation-only: actually dispatching a mine action
	// stays gated on genuinely-current visibility, unchanged by this
	// feature - ActionMine must not be in the mask while LOS is lost, even
	// though the observation itself still shows mineVisible=1.
	mask := env.ActionMask()
	if mask[rlenv.ActionMine] {
		t.Fatalf("ActionMine legal while LOS is lost; want illegal (remembered position is observation-only)")
	}
}

// TestResetClearsTheMineTargetMemoryAcrossEpisodes verifies the memory
// doesn't leak into a later episode whose world state has since been
// restored/reseeded - docs/plans/14's explicit "not X-ray, but a real
// episode-boundary information leak" caution.
func TestResetClearsTheMineTargetMemoryAcrossEpisodes(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:coal_ore", 3, 0, 4)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:coal_ore"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset (episode 1): %v", err)
	}

	// Episode 2: the target was never (re-)placed/found, so the memory
	// from episode 1 must not carry over.
	agent.setMineBlock("", 0, 0, 0)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset (episode 2): %v", err)
	}
	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible at the start of episode 2 = %v, want 0 (episode 1's memory must not leak)", visible)
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

// --- ActionEquip / ActionUseItem (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 1) ---

func TestStepEquipWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionEquip)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.heldItem != "" {
		t.Fatalf("heldItem = %q, want empty (Config.EquipTargetItem unset, safe no-op like ActionWait)", agent.heldItem)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if ready := result.Observation.Values[31]; ready != 0 {
		t.Fatalf("equipReady = %v, want 0 (no task configured)", ready)
	}
	if active := result.Observation.Values[32]; active != 0 {
		t.Fatalf("goalEquipActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepUseItemWithNoConfiguredEquipTargetIsNoOp(t *testing.T) {
	// ActionUseItem is gated on Config.EquipTargetItem, not an independent
	// flag — see Config.EquipTargetItem's own doc comment.
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionUseItem); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.useItemCalls != 0 {
		t.Fatalf("UseItem calls = %d, want 0 (Config.EquipTargetItem unset)", agent.useItemCalls)
	}
}

func TestStepEquipActionEquipsTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.EquipTargetItem = "minecraft:elytra"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionEquip)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.heldItem != "minecraft:elytra" {
		t.Fatalf("heldItem = %q, want minecraft:elytra", agent.heldItem)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target item was equipped)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (equipRewardBonus)", result.Reward)
	}
	if ready := result.Observation.Values[31]; ready != 1 {
		t.Fatalf("equipReady = %v, want 1", ready)
	}
}

func TestStepObservationReflectsEquipReadyRegardlessOfAction(t *testing.T) {
	// equipReady is populated every step once configured, not only on steps
	// that dispatch ActionEquip — mirrors craftReady's equivalent test.
	agent := newFakeAgent(0, 0, 0)
	agent.heldItem = "minecraft:elytra" // already equipped before the episode starts
	cfg := testConfig()
	cfg.EquipTargetItem = "minecraft:elytra"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if ready := obs.Values[31]; ready != 1 {
		t.Fatalf("equipReady at Reset = %v, want 1", ready)
	}
	if active := obs.Values[32]; active != 1 {
		t.Fatalf("goalEquipActive at Reset = %v, want 1", active)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.useItemCalls != 0 {
		t.Fatalf("UseItem calls = %d, want 0 (ActionWait doesn't dispatch useitem)", agent.useItemCalls)
	}
	if ready := result.Observation.Values[31]; ready != 1 {
		t.Fatalf("equipReady after ActionWait = %v, want 1 (still equipped)", ready)
	}
	// Reset already satisfied the equip task (heldItem was pre-set), so
	// Reset's own buildObservation call already saw equipReady=1 — the
	// transition reward only fires on a false->true change, which happened
	// before Step ever ran. No further bonus/Done from a step that changed
	// nothing.
	if result.Done {
		t.Fatalf("Done = true, want false (already equipped before this episode's first step)")
	}
}

func TestStepUseItemDispatchesWithConfiguredHand(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.EquipTargetItem = "minecraft:elytra"
	cfg.UseItemOffhand = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionUseItem); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.useItemCalls != 1 {
		t.Fatalf("UseItem calls = %d, want 1", agent.useItemCalls)
	}
	if len(agent.useItemHands) != 1 || agent.useItemHands[0] != models.OffHand {
		t.Fatalf("useItemHands = %v, want [OffHand]", agent.useItemHands)
	}
}

func TestActionMaskEquipAndUseItemLegalOnlyWhenConfigured(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionEquip]; legal {
		t.Fatalf("ActionEquip legal = true, want false (Config.EquipTargetItem unset)")
	}
	if legal := env.ActionMask()[rlenv.ActionUseItem]; legal {
		t.Fatalf("ActionUseItem legal = true, want false (Config.EquipTargetItem unset)")
	}

	cfg := testConfig()
	cfg.EquipTargetItem = "minecraft:elytra"
	configured := newTestEnvironment(t, newFakeAgent(0, 0, 0), cfg)
	if _, err := configured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := configured.ActionMask()[rlenv.ActionEquip]; !legal {
		t.Fatalf("ActionEquip legal = false, want true (Config.EquipTargetItem set)")
	}
	if legal := configured.ActionMask()[rlenv.ActionUseItem]; !legal {
		t.Fatalf("ActionUseItem legal = false, want true (Config.EquipTargetItem set)")
	}
}

// --- ActionPickUpItem (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 2) ---

func TestStepPickUpWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setPickupItem("minecraft:stick", 3, 0, 0) // present in the world, but Config never asks for it
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPickUpItem)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveToWithChat calls = %d, want 0 (Config.PickupTargetItem unset, safe no-op like ActionWait)", agent.moveToWithChatCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[36]; visible != 0 {
		t.Fatalf("pickupVisible = %v, want 0 (no task configured)", visible)
	}
}

func TestStepPickUpWithNoVisibleTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.PickupTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPickUpItem)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveToWithChat calls = %d, want 0 (nothing visible to pick up)", agent.moveToWithChatCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[36]; visible != 0 {
		t.Fatalf("pickupVisible = %v, want 0", visible)
	}
}

func TestStepPickUpActionCollectsTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setPickupItem("minecraft:stick", 3, 0, 0)
	cfg := testConfig()
	cfg.PickupTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPickUpItem)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 1 {
		t.Fatalf("MoveToWithChat calls = %d, want 1", agent.moveToWithChatCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target item was collected)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (pickupRewardBonus)", result.Reward)
	}
	if got := agent.InventoryCount("minecraft:stick"); got != 1 {
		t.Fatalf("held count after Step = %d, want 1", got)
	}
}

func TestStepObservationReflectsPickupTargetDeltaRegardlessOfAction(t *testing.T) {
	// pickupVisible is populated every step once configured, not only on
	// steps that dispatch ActionPickUpItem — mirrors mineVisible's
	// equivalent test.
	agent := newFakeAgent(0, 0, 0)
	agent.setPickupItem("minecraft:stick", 3, 0, 0)
	cfg := testConfig()
	cfg.PickupTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[36]; visible != 1 {
		t.Fatalf("pickupVisible at Reset = %v, want 1", visible)
	}
	if active := obs.Values[37]; active != 1 {
		t.Fatalf("goalPickupActive at Reset = %v, want 1", active)
	}
	if dx := obs.Values[33]; dx != 3 {
		t.Fatalf("pickupDx at Reset = %v, want 3", dx)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveToWithChat calls = %d, want 0 (ActionWait doesn't dispatch pickupitem)", agent.moveToWithChatCalls)
	}
	if visible := result.Observation.Values[36]; visible != 1 {
		t.Fatalf("pickupVisible after ActionWait = %v, want 1 (still visible, nothing consumed)", visible)
	}
}

func TestActionMaskPickUpItemLegalOnlyWhenConfiguredAndVisible(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.PickupTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionPickUpItem]; legal {
		t.Fatalf("ActionPickUpItem legal = true, want false (configured but nothing visible yet)")
	}
	// Cross-check the mask against what Step actually does with it.
	if _, err := env.Step(context.Background(), rlenv.ActionPickUpItem); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveToWithChat calls = %d, want 0 (mask reported ActionPickUpItem illegal)", agent.moveToWithChatCalls)
	}

	agent.setPickupItem("minecraft:stick", 3, 0, 0)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[36]; visible != 1 {
		t.Fatalf("pickupVisible = %v, want 1 (sanity check target is now visible)", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionPickUpItem]; !legal {
		t.Fatalf("ActionPickUpItem legal = false, want true (target now visible)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionPickUpItem]; legal {
		t.Fatalf("ActionPickUpItem legal = true, want false (Config.PickupTargetItem unset, even though visible)")
	}
}

// --- ActionFlyTo (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3) ---

func TestStepFlyWithTargetInactiveIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.heldItem = "minecraft:elytra" // equipped, but Config never asks for flight
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionFlyTo)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if pos, _, _, _ := agent.GetPosition(); pos.X != 0 {
		t.Fatalf("position moved to x=%v, want 0 (Config.FlyTargetActive is false, safe no-op like ActionWait)", pos.X)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[43]; active != 0 {
		t.Fatalf("goalFlyActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepFlyWithoutElytraDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.FlyTargetActive = true
	cfg.FlyTargetOffset = [3]float64{10, 0, 0}
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionFlyTo)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if pos, _, _, _ := agent.GetPosition(); pos.X != 0 {
		t.Fatalf("position moved to x=%v, want 0 (no elytra equipped, must not dispatch)", pos.X)
	}
	if hasElytra := result.Observation.Values[42]; hasElytra != 0 {
		t.Fatalf("hasElytra = %v, want 0 (sanity check)", hasElytra)
	}
}

func TestStepFlyActionReachesTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.heldItem = "minecraft:elytra"
	cfg := testConfig()
	cfg.GoToTargetDisabled = true // isolate the fly reward from the unrelated phantom goto target
	cfg.FlyTargetActive = true
	cfg.FlyTargetOffset = [3]float64{20, 0, 0}
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionFlyTo)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (flight reached the target)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (flyRewardBonus)", result.Reward)
	}
	if pos, _, _, _ := agent.GetPosition(); pos.X != 20 {
		t.Fatalf("position after Step x=%v, want 20 (FlyTo's own simulated arrival)", pos.X)
	}
	if flying := result.Observation.Values[41]; flying != 1 {
		t.Fatalf("isFlying = %v, want 1 (FlyTo marks the bot gliding)", flying)
	}
}

func TestStepObservationReflectsFlyStateRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.heldItem = "minecraft:elytra"
	cfg := testConfig()
	cfg.FlyTargetActive = true
	cfg.FlyTargetOffset = [3]float64{20, 5, 0}
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if dx := obs.Values[38]; dx != 20 {
		t.Fatalf("flyDx at Reset = %v, want 20", dx)
	}
	if dy := obs.Values[39]; dy != 5 {
		t.Fatalf("flyDy at Reset = %v, want 5", dy)
	}
	if hasElytra := obs.Values[42]; hasElytra != 1 {
		t.Fatalf("hasElytra at Reset = %v, want 1", hasElytra)
	}
	if active := obs.Values[43]; active != 1 {
		t.Fatalf("goalFlyActive at Reset = %v, want 1", active)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if pos, _, _, _ := agent.GetPosition(); pos.X != 0 {
		t.Fatalf("position moved to x=%v, want 0 (ActionWait doesn't dispatch flyto)", pos.X)
	}
	if dx := result.Observation.Values[38]; dx != 20 {
		t.Fatalf("flyDx after ActionWait = %v, want 20 (still unreached)", dx)
	}
}

func TestActionMaskFlyToLegalOnlyWhenActiveAndElytraEquipped(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.FlyTargetActive = true
	cfg.FlyTargetOffset = [3]float64{20, 0, 0}
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionFlyTo]; legal {
		t.Fatalf("ActionFlyTo legal = true, want false (no elytra equipped)")
	}

	agent.heldItem = "minecraft:elytra"
	if legal := env.ActionMask()[rlenv.ActionFlyTo]; !legal {
		t.Fatalf("ActionFlyTo legal = false, want true (elytra now equipped)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionFlyTo]; legal {
		t.Fatalf("ActionFlyTo legal = true, want false (Config.FlyTargetActive is false, even with an elytra equipped)")
	}
}

// --- ActionMount / ActionDismount / ActionVehicleJump (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 4) ---

func TestStepMountWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setRideVehicle("minecraft:horse", 7, 3, 0, 0) // present in the world, but Config never asks for it
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMount)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.IsMounted() {
		t.Fatalf("IsMounted() = true, want false (Config.RideVehicleType unset, safe no-op like ActionWait)")
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[49]; active != 0 {
		t.Fatalf("goalRideActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepMountWithNoVisibleTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.RideVehicleType = "minecraft:horse"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMount)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.IsMounted() {
		t.Fatalf("IsMounted() = true, want false (nothing visible to mount)")
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[47]; visible != 0 {
		t.Fatalf("rideVisible = %v, want 0", visible)
	}
}

func TestStepMountActionMountsTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setRideVehicle("minecraft:horse", 7, 3, 0, 0)
	cfg := testConfig()
	cfg.RideVehicleType = "minecraft:horse"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMount)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !agent.IsMounted() {
		t.Fatalf("IsMounted() = false, want true")
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target vehicle was mounted)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (rideRewardBonus)", result.Reward)
	}
	if mounted := result.Observation.Values[48]; mounted != 1 {
		t.Fatalf("isMounted = %v, want 1", mounted)
	}
}

func TestStepObservationReflectsRideStateRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setRideVehicle("minecraft:horse", 7, 3, 0, 0)
	cfg := testConfig()
	cfg.RideVehicleType = "minecraft:horse"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[47]; visible != 1 {
		t.Fatalf("rideVisible at Reset = %v, want 1", visible)
	}
	if active := obs.Values[49]; active != 1 {
		t.Fatalf("goalRideActive at Reset = %v, want 1", active)
	}
	if dx := obs.Values[44]; dx != 3 {
		t.Fatalf("rideDx at Reset = %v, want 3", dx)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.IsMounted() {
		t.Fatalf("IsMounted() = true, want false (ActionWait doesn't dispatch mount)")
	}
	if visible := result.Observation.Values[47]; visible != 1 {
		t.Fatalf("rideVisible after ActionWait = %v, want 1 (still visible)", visible)
	}
}

func TestStepDismountAndVehicleJumpDispatchWhileMounted(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setRideVehicle("minecraft:horse", 7, 3, 0, 0)
	cfg := testConfig()
	cfg.RideVehicleType = "minecraft:horse"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(context.Background(), rlenv.ActionMount); err != nil {
		t.Fatalf("Step (mount): %v", err)
	}
	if !agent.IsMounted() {
		t.Fatalf("IsMounted() = false, want true (setup failed)")
	}

	if _, err := env.Step(context.Background(), rlenv.ActionVehicleJump); err != nil {
		t.Fatalf("Step (vehicle jump): %v", err)
	}
	if agent.vehicleJumpCalls != 1 {
		t.Fatalf("JumpVehicle calls = %d, want 1", agent.vehicleJumpCalls)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionDismount); err != nil {
		t.Fatalf("Step (dismount): %v", err)
	}
	if agent.IsMounted() {
		t.Fatalf("IsMounted() = true, want false (dismounted)")
	}
}

func TestActionMaskMountDismountVehicleJumpGating(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.RideVehicleType = "minecraft:horse"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMount]; legal {
		t.Fatalf("ActionMount legal = true, want false (configured but nothing visible yet)")
	}
	if legal := env.ActionMask()[rlenv.ActionDismount]; legal {
		t.Fatalf("ActionDismount legal = true, want false (not mounted)")
	}
	if legal := env.ActionMask()[rlenv.ActionVehicleJump]; legal {
		t.Fatalf("ActionVehicleJump legal = true, want false (not mounted)")
	}

	agent.setRideVehicle("minecraft:horse", 7, 3, 0, 0)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[47]; visible != 1 {
		t.Fatalf("rideVisible = %v, want 1 (sanity check target is now visible)", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionMount]; !legal {
		t.Fatalf("ActionMount legal = false, want true (target now visible)")
	}

	if _, err := env.Step(context.Background(), rlenv.ActionMount); err != nil {
		t.Fatalf("Step (mount): %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMount]; legal {
		t.Fatalf("ActionMount legal = true, want false (already mounted)")
	}
	if legal := env.ActionMask()[rlenv.ActionDismount]; !legal {
		t.Fatalf("ActionDismount legal = false, want true (now mounted)")
	}
	if legal := env.ActionMask()[rlenv.ActionVehicleJump]; !legal {
		t.Fatalf("ActionVehicleJump legal = false, want true (now mounted)")
	}

	unconfigured := newTestEnvironment(t, newFakeAgent(0, 0, 0), testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionMount]; legal {
		t.Fatalf("ActionMount legal = true, want false (Config.RideVehicleType unset)")
	}
}

// --- ActionCombat (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 5) ---

func TestStepCombatWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCombatTarget("minecraft:zombie", 11, 5, 0, 0, 20) // present in the world, but Config never asks for it
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionCombat)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.runCombatCalls) != 0 {
		t.Fatalf("RunCombat calls = %v, want none (Config.CombatTargetMobType unset, safe no-op like ActionWait)", agent.runCombatCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[55]; active != 0 {
		t.Fatalf("goalCombatActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepCombatWithNoVisibleTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.CombatTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionCombat)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.runCombatCalls) != 0 {
		t.Fatalf("RunCombat calls = %v, want none (nothing visible to fight)", agent.runCombatCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[54]; visible != 0 {
		t.Fatalf("combatVisible = %v, want 0", visible)
	}
}

func TestStepCombatActionKillsTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCombatTarget("minecraft:zombie", 11, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.CombatTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionCombat)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.runCombatCalls) != 1 {
		t.Fatalf("RunCombat calls = %v, want exactly 1", agent.runCombatCalls)
	}
	if want := (runCombatCall{radius: 16, includeNeutral: false}); agent.runCombatCalls[0] != want {
		t.Fatalf("RunCombat call = %+v, want %+v", agent.runCombatCalls[0], want)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target was killed)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (combatRewardBonus)", result.Reward)
	}
}

// TestStepCombatDefaultsToPlayerAndNeutralSafePolicy confirms
// Config.CombatIncludePlayers/CombatIncludeNeutral default to false
// (confirmed via testConfig(), which never sets them) even when a combat
// task is otherwise fully configured — a trained policy must never be
// able to attack real players unless an episode explicitly opts in
// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md's "Deferred" section).
func TestStepCombatDefaultsToPlayerAndNeutralSafePolicy(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCombatTarget("minecraft:zombie", 11, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.CombatTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionCombat); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.runCombatCalls) != 1 {
		t.Fatalf("RunCombatWithPolicy calls = %v, want exactly 1", agent.runCombatCalls)
	}
	if call := agent.runCombatCalls[0]; call.includePlayers || call.includeNeutral {
		t.Fatalf("RunCombatWithPolicy call = %+v, want includePlayers=false, includeNeutral=false by default", call)
	}
}

// TestStepCombatDispatchesConfiguredPolicyFlags confirms
// Config.CombatIncludePlayers/CombatIncludeNeutral, once explicitly set,
// actually reach the dispatched runcombatwithpolicy call — this is the
// one piece of behavior "runcombatwithpolicy" adds over the already-wired
// ActionCombat (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md's own framing
// of this item).
func TestStepCombatDispatchesConfiguredPolicyFlags(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCombatTarget("minecraft:zombie", 11, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.CombatTargetMobType = "minecraft:zombie"
	cfg.CombatIncludePlayers = true
	cfg.CombatIncludeNeutral = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionCombat); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.runCombatCalls) != 1 {
		t.Fatalf("RunCombatWithPolicy calls = %v, want exactly 1", agent.runCombatCalls)
	}
	if want := (runCombatCall{radius: 16, includePlayers: true, includeNeutral: true}); agent.runCombatCalls[0] != want {
		t.Fatalf("RunCombatWithPolicy call = %+v, want %+v", agent.runCombatCalls[0], want)
	}
}

func TestStepObservationReflectsCombatTargetHealthRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCombatTarget("minecraft:zombie", 11, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.CombatTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[54]; visible != 1 {
		t.Fatalf("combatVisible at Reset = %v, want 1", visible)
	}
	if active := obs.Values[55]; active != 1 {
		t.Fatalf("goalCombatActive at Reset = %v, want 1", active)
	}
	if dx := obs.Values[50]; dx != 5 {
		t.Fatalf("combatDx at Reset = %v, want 5", dx)
	}
	if health := obs.Values[53]; health != 20 {
		t.Fatalf("combatTargetHealth at Reset = %v, want 20", health)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.runCombatCalls) != 0 {
		t.Fatalf("RunCombat calls = %v, want none (ActionWait doesn't dispatch runcombat)", agent.runCombatCalls)
	}
	if health := result.Observation.Values[53]; health != 20 {
		t.Fatalf("combatTargetHealth after ActionWait = %v, want 20 (unchanged)", health)
	}
}

// TestStepCombatRewardJudgedByWorldStateNotDispatchedAction mirrors
// TestStepMineDoesNotAwardBonusWhenActionMineFailsToBreakIt's inverse:
// the target dying from something other than the dispatched action
// (environmental damage, another cause entirely) still counts, since the
// reward is judged from the resolved entity's own health, not from
// whether ActionCombat was chosen this step.
func TestStepCombatRewardJudgedByWorldStateNotDispatchedAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCombatTarget("minecraft:zombie", 11, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.CombatTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.killCombatTarget() // died from something other than ActionCombat
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.runCombatCalls) != 0 {
		t.Fatalf("RunCombat calls = %v, want none (ActionWait doesn't dispatch runcombat)", agent.runCombatCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (the resolved target died, regardless of cause)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (combatRewardBonus)", result.Reward)
	}
}

func TestActionMaskCombatLegalOnlyWhenConfiguredAndVisible(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.CombatTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionCombat]; legal {
		t.Fatalf("ActionCombat legal = true, want false (configured but nothing visible yet)")
	}

	agent.setCombatTarget("minecraft:zombie", 11, 5, 0, 0, 20)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[54]; visible != 1 {
		t.Fatalf("combatVisible = %v, want 1 (sanity check target is now visible)", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionCombat]; !legal {
		t.Fatalf("ActionCombat legal = false, want true (target now visible)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionCombat]; legal {
		t.Fatalf("ActionCombat legal = true, want false (Config.CombatTargetMobType unset, even though visible)")
	}
}

// --- Config.PickupAnyItem (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 1) ---

func TestStepPickupAnyItemCollectsWhateverIsNearestAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setPickupItem("minecraft:rotten_flesh", 3, 0, 0) // content is irrelevant to this mode
	cfg := testConfig()
	cfg.PickupAnyItem = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPickUpItem)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 1 {
		t.Fatalf("MoveToWithChat calls = %d, want 1", agent.moveToWithChatCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (something was collected)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (pickupRewardBonus)", result.Reward)
	}
}

func TestStepPickupAnyItemWithNoneVisibleDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.PickupAnyItem = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPickUpItem)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveToWithChat calls = %d, want 0 (nothing visible)", agent.moveToWithChatCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

// TestStepPickupAnyItemRewardJudgedByWorldStateNotDispatchedAction mirrors
// TestStepCombatRewardJudgedByWorldStateNotDispatchedAction: PickupAnyItem
// has no single item name to run an InventoryCount delta against, so it's
// judged from the resolved entity no longer being tracked — correct
// regardless of what caused that (picked up by vanilla auto-collect while
// walking somewhere else entirely, expired, anything), not just a
// dispatched ActionPickUpItem.
func TestStepPickupAnyItemRewardJudgedByWorldStateNotDispatchedAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setPickupItem("minecraft:rotten_flesh", 3, 0, 0)
	cfg := testConfig()
	cfg.PickupAnyItem = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.mu.Lock()
	agent.pickupItemPresent = false // collected by something other than ActionPickUpItem
	agent.mu.Unlock()
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveToWithChat calls = %d, want 0 (ActionWait doesn't dispatch pickupitem)", agent.moveToWithChatCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (the resolved item is gone, regardless of cause)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (pickupRewardBonus)", result.Reward)
	}
}

func TestActionMaskPickUpItemLegalForAnyItemModeToo(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.PickupAnyItem = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionPickUpItem]; legal {
		t.Fatalf("ActionPickUpItem legal = true, want false (nothing visible yet)")
	}

	agent.setPickupItem("minecraft:rotten_flesh", 3, 0, 0)
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionPickUpItem]; !legal {
		t.Fatalf("ActionPickUpItem legal = false, want true (something now visible, PickupAnyItem mode)")
	}
}

// --- Config.ArriveSneaking (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 3) ---

func TestStepGoToTargetArriveSneakingDispatchesSneakVariant(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.ArriveSneaking = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToAndSneakCalls != 1 {
		t.Fatalf("MoveToAndSneak calls = %d, want 1", agent.moveToAndSneakCalls)
	}
	if agent.moveToWithChatCalls != 0 {
		t.Fatalf("MoveTo calls = %d, want 0 (should dispatch the sneak variant instead)", agent.moveToWithChatCalls)
	}
	if !agent.sneaking {
		t.Fatalf("sneaking = false, want true (arrived via the sneak variant)")
	}
}

func TestStepGoToTargetWithoutArriveSneakingDispatchesNormalVariant(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig()) // ArriveSneaking defaults false
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.moveToAndSneakCalls != 0 {
		t.Fatalf("MoveToAndSneak calls = %d, want 0 (ArriveSneaking is false, unchanged behavior)", agent.moveToAndSneakCalls)
	}
	if agent.moveToWithChatCalls != 1 {
		t.Fatalf("MoveTo calls = %d, want 1", agent.moveToWithChatCalls)
	}
}

// TestResetStopsSneakingLeftOverFromThePreviousEpisode mirrors
// dismountForReset's own test precedent: a persistent toggle from a
// previous episode (here, Config.ArriveSneaking having left the bot
// sneaking) must never survive into the next episode, regardless of the
// next episode's own Config.
func TestResetStopsSneakingLeftOverFromThePreviousEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.mu.Lock()
	agent.sneaking = true // leftover from a previous episode
	agent.mu.Unlock()
	env := newTestEnvironment(t, agent, testConfig())

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if agent.stopSneakingCalls < 1 {
		t.Fatalf("StopSneaking calls = %d, want at least 1", agent.stopSneakingCalls)
	}
	if agent.sneaking {
		t.Fatalf("sneaking = true, want false after Reset")
	}
}

// --- Config.PlaceTargetItem, standalone (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 2) ---

func TestStepPlaceStandaloneWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setPlaceTarget("minecraft:dirt", 1) // held, but Config never asks for it
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPlace)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if got := agent.InventoryCount("minecraft:dirt"); got != 1 {
		t.Fatalf("held count = %d, want 1 (Config.PlaceTargetItem unset, safe no-op like ActionWait)", got)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[57]; active != 0 {
		t.Fatalf("goalPlaceStandaloneActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepPlaceStandaloneWithNothingHeldDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.PlaceTargetItem = "minecraft:dirt"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPlace)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (nothing held to place)")
	}
	if ready := result.Observation.Values[56]; ready != 0 {
		t.Fatalf("placeReady = %v, want 0 (nothing held)", ready)
	}
}

func TestStepPlaceStandaloneActionPlacesTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setPlaceTarget("minecraft:dirt", 1)
	cfg := testConfig()
	cfg.PlaceTargetItem = "minecraft:dirt"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionPlace)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target item was placed)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (placeRewardBonus)", result.Reward)
	}
	if got := agent.InventoryCount("minecraft:dirt"); got != 0 {
		t.Fatalf("held count after Step = %d, want 0", got)
	}
}

func TestStepObservationReflectsPlaceReadyRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setPlaceTarget("minecraft:dirt", 1)
	cfg := testConfig()
	cfg.PlaceTargetItem = "minecraft:dirt"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if ready := obs.Values[56]; ready != 1 {
		t.Fatalf("placeReady at Reset = %v, want 1", ready)
	}
	if active := obs.Values[57]; active != 1 {
		t.Fatalf("goalPlaceStandaloneActive at Reset = %v, want 1", active)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if got := agent.InventoryCount("minecraft:dirt"); got != 1 {
		t.Fatalf("held count after ActionWait = %d, want 1 (nothing placed)", got)
	}
	if ready := result.Observation.Values[56]; ready != 1 {
		t.Fatalf("placeReady after ActionWait = %v, want 1 (still held)", ready)
	}
}

func TestActionMaskPlaceStandaloneLegalOnlyWhenConfiguredAndHeld(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.PlaceTargetItem = "minecraft:dirt"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionPlace]; legal {
		t.Fatalf("ActionPlace legal = true, want false (configured but nothing held yet)")
	}

	agent.setPlaceTarget("minecraft:dirt", 1)
	if legal := env.ActionMask()[rlenv.ActionPlace]; !legal {
		t.Fatalf("ActionPlace legal = false, want true (target now held)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionPlace]; legal {
		t.Fatalf("ActionPlace legal = true, want false (Config.PlaceTargetItem unset, even though held)")
	}
}

// --- Config.FollowTargetPlayerName (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 4) ---

func TestStepFollowWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setFollowablePlayer("Other") // resolvable, but Config never asks for it
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionFollow)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.IsFollowing() {
		t.Fatalf("IsFollowing() = true, want false (Config.FollowTargetPlayerName unset, safe no-op like ActionWait)")
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[60]; active != 0 {
		t.Fatalf("goalFollowActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepFollowWithUnresolvableTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.FollowTargetPlayerName = "Other"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionFollow)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.IsFollowing() {
		t.Fatalf("IsFollowing() = true, want false (named player never resolvable)")
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[58]; visible != 0 {
		t.Fatalf("followTargetVisible = %v, want 0", visible)
	}
}

func TestStepFollowActionStartsFollowingAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setFollowablePlayer("Other")
	cfg := testConfig()
	cfg.FollowTargetPlayerName = "Other"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionFollow)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !agent.IsFollowing() {
		t.Fatalf("IsFollowing() = false, want true")
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (follow started)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (followRewardBonus)", result.Reward)
	}
	if following := result.Observation.Values[59]; following != 1 {
		t.Fatalf("isFollowing = %v, want 1", following)
	}
}

func TestStepObservationReflectsFollowStateRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setFollowablePlayer("Other")
	cfg := testConfig()
	cfg.FollowTargetPlayerName = "Other"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[58]; visible != 1 {
		t.Fatalf("followTargetVisible at Reset = %v, want 1", visible)
	}
	if active := obs.Values[60]; active != 1 {
		t.Fatalf("goalFollowActive at Reset = %v, want 1", active)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.IsFollowing() {
		t.Fatalf("IsFollowing() = true, want false (ActionWait doesn't dispatch follow)")
	}
	if visible := result.Observation.Values[58]; visible != 1 {
		t.Fatalf("followTargetVisible after ActionWait = %v, want 1 (still resolvable)", visible)
	}
}

func TestStepStopFollowDispatchesWhileFollowing(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setFollowablePlayer("Other")
	cfg := testConfig()
	cfg.FollowTargetPlayerName = "Other"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(context.Background(), rlenv.ActionFollow); err != nil {
		t.Fatalf("Step (follow): %v", err)
	}
	if !agent.IsFollowing() {
		t.Fatalf("IsFollowing() = false, want true (setup failed)")
	}

	if _, err := env.Step(context.Background(), rlenv.ActionStopFollow); err != nil {
		t.Fatalf("Step (stopfollow): %v", err)
	}
	if agent.IsFollowing() {
		t.Fatalf("IsFollowing() = true, want false (stopped)")
	}
}

func TestActionMaskFollowStopFollowGating(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.FollowTargetPlayerName = "Other"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionFollow]; legal {
		t.Fatalf("ActionFollow legal = true, want false (configured but not resolvable yet)")
	}
	if legal := env.ActionMask()[rlenv.ActionStopFollow]; legal {
		t.Fatalf("ActionStopFollow legal = true, want false (not following)")
	}

	agent.setFollowablePlayer("Other")
	if _, err := env.Step(context.Background(), rlenv.ActionWait); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionFollow]; !legal {
		t.Fatalf("ActionFollow legal = false, want true (target now resolvable)")
	}

	if _, err := env.Step(context.Background(), rlenv.ActionFollow); err != nil {
		t.Fatalf("Step (follow): %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionFollow]; legal {
		t.Fatalf("ActionFollow legal = true, want false (already following)")
	}
	if legal := env.ActionMask()[rlenv.ActionStopFollow]; !legal {
		t.Fatalf("ActionStopFollow legal = false, want true (now following)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionStopFollow]; legal {
		t.Fatalf("ActionStopFollow legal = true, want false (Config.FollowTargetPlayerName unset)")
	}
}

// --- Config.CreeperGunpowderActive (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5) ---

const creeperGunpowderWeaponItemName = "minecraft:iron_sword"

func TestStepKillCreeperForGunpowderWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCreeperTarget(21, 5, 0, 0, true) // present in the world, but Config never asks for it
	if err := agent.Equip(context.Background(), creeperGunpowderWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionKillCreeperForGunpowder)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !agent.creeperVisible {
		t.Fatalf("creeperVisible = false, want true (Config.CreeperGunpowderActive unset, safe no-op like ActionWait)")
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[65]; active != 0 {
		t.Fatalf("goalCreeperGunpowderActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepKillCreeperForGunpowderWithNoVisibleTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	if err := agent.Equip(context.Background(), creeperGunpowderWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.CreeperGunpowderActive = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionKillCreeperForGunpowder)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (nothing visible to fight)")
	}
	if visible := result.Observation.Values[64]; visible != 0 {
		t.Fatalf("creeperVisible = %v, want 0", visible)
	}
}

func TestStepKillCreeperForGunpowderWithoutWeaponDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCreeperTarget(21, 5, 0, 0, true)
	cfg := testConfig()
	cfg.CreeperGunpowderActive = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionKillCreeperForGunpowder)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !agent.creeperVisible {
		t.Fatalf("creeperVisible = false, want true (no weapon equipped, dispatch must not have reached KillCreeperForGunpowder)")
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepKillCreeperForGunpowderActionCollectsGunpowderAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCreeperTarget(21, 5, 0, 0, true)
	if err := agent.Equip(context.Background(), creeperGunpowderWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.CreeperGunpowderActive = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionKillCreeperForGunpowder)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.creeperVisible {
		t.Fatalf("creeperVisible = true, want false (creeper was fought)")
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (gunpowder was collected)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (creeperGunpowderRewardBonus)", result.Reward)
	}
	if got := agent.InventoryCount("minecraft:gunpowder"); got != 1 {
		t.Fatalf("held gunpowder count after Step = %d, want 1", got)
	}
}

// TestStepKillCreeperForGunpowderNoDropEncounterDoesNotEndEpisode exercises
// agent.KillCreeperForGunpowder's own documented "no-drop encounter" outcome
// (a valid result, not an error): the reward/done check is judged from an
// actual InventoryCount increase, not from whether the dispatch succeeded,
// mirroring craftRewardBonus/pickupRewardBonus's own philosophy — a kill
// with no drop earns nothing (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
// item 5).
func TestStepKillCreeperForGunpowderNoDropEncounterDoesNotEndEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCreeperTarget(21, 5, 0, 0, false)
	if err := agent.Equip(context.Background(), creeperGunpowderWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.CreeperGunpowderActive = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionKillCreeperForGunpowder)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.creeperVisible {
		t.Fatalf("creeperVisible = true, want false (creeper was fought)")
	}
	if result.Done {
		t.Fatalf("Done = true, want false (no-drop encounter, a valid outcome)")
	}
	if got := agent.InventoryCount("minecraft:gunpowder"); got != 0 {
		t.Fatalf("held gunpowder count after Step = %d, want 0", got)
	}
}

func TestStepObservationReflectsCreeperStateRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCreeperTarget(21, 5, 0, 0, true)
	cfg := testConfig()
	cfg.CreeperGunpowderActive = true
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[64]; visible != 1 {
		t.Fatalf("creeperVisible at Reset = %v, want 1", visible)
	}
	if active := obs.Values[65]; active != 1 {
		t.Fatalf("goalCreeperGunpowderActive at Reset = %v, want 1", active)
	}
	if dx := obs.Values[61]; dx != 5 {
		t.Fatalf("creeperDx at Reset = %v, want 5", dx)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !agent.creeperVisible {
		t.Fatalf("creeperVisible = false, want true (ActionWait doesn't dispatch killcreeperforgunpowder)")
	}
	if visible := result.Observation.Values[64]; visible != 1 {
		t.Fatalf("creeperVisible after ActionWait = %v, want 1 (still visible)", visible)
	}
}

func TestActionMaskKillCreeperForGunpowderGatedOnConfiguredVisibleAndWeapon(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.CreeperGunpowderActive = true
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionKillCreeperForGunpowder]; legal {
		t.Fatalf("ActionKillCreeperForGunpowder legal = true, want false (configured but nothing visible, no weapon)")
	}

	agent.setCreeperTarget(21, 5, 0, 0, true)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[64]; visible != 1 {
		t.Fatalf("creeperVisible = %v, want 1 (sanity check target is now visible)", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionKillCreeperForGunpowder]; legal {
		t.Fatalf("ActionKillCreeperForGunpowder legal = true, want false (visible but no weapon equipped yet)")
	}

	if err := agent.Equip(context.Background(), creeperGunpowderWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionKillCreeperForGunpowder]; !legal {
		t.Fatalf("ActionKillCreeperForGunpowder legal = false, want true (now visible and weapon equipped)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionKillCreeperForGunpowder]; legal {
		t.Fatalf("ActionKillCreeperForGunpowder legal = true, want false (Config.CreeperGunpowderActive unset, even though visible and equipped)")
	}
}

// --- Config.SpearChargeTargetMobType (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 6) ---

const spearChargeWeaponItemName = "minecraft:iron_spear"

func TestStepSpearChargeWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setSpearTarget("minecraft:zombie", 31, 5, 0, 0, 20) // present in the world, but Config never asks for it
	if err := agent.Equip(context.Background(), spearChargeWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionSpearChargeWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.spearChargeCalls) != 0 {
		t.Fatalf("SpearChargeAtWithApproach calls = %v, want none (Config.SpearChargeTargetMobType unset, safe no-op like ActionWait)", agent.spearChargeCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[70]; active != 0 {
		t.Fatalf("goalSpearChargeActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepSpearChargeWithNoVisibleTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	if err := agent.Equip(context.Background(), spearChargeWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.SpearChargeTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionSpearChargeWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.spearChargeCalls) != 0 {
		t.Fatalf("SpearChargeAtWithApproach calls = %v, want none (nothing visible to charge)", agent.spearChargeCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[69]; visible != 0 {
		t.Fatalf("spearVisible = %v, want 0", visible)
	}
}

func TestStepSpearChargeWithoutWeaponDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setSpearTarget("minecraft:zombie", 31, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.SpearChargeTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionSpearChargeWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.spearChargeCalls) != 0 {
		t.Fatalf("SpearChargeAtWithApproach calls = %v, want none (no spear equipped)", agent.spearChargeCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepSpearChargeDispatchesWithResolvedEntityAndFixedArgs(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setSpearTarget("minecraft:zombie", 31, 5, 0, 0, 20)
	if err := agent.Equip(context.Background(), spearChargeWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.SpearChargeTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionSpearChargeWithApproach); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.spearChargeCalls) != 1 {
		t.Fatalf("SpearChargeAtWithApproach calls = %v, want exactly 1", agent.spearChargeCalls)
	}
	call := agent.spearChargeCalls[0]
	if call.targetID != 31 {
		t.Fatalf("targetID = %d, want 31 (Environment's own resolved entity, not a by-name search)", call.targetID)
	}
	if call.itemName != spearChargeWeaponItemName {
		t.Fatalf("itemName = %q, want %q", call.itemName, spearChargeWeaponItemName)
	}
}

func TestStepSpearChargeActionKillsTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setSpearTarget("minecraft:zombie", 31, 5, 0, 0, 20)
	if err := agent.Equip(context.Background(), spearChargeWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.SpearChargeTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionSpearChargeWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target was killed)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (spearChargeRewardBonus)", result.Reward)
	}
}

// TestStepSpearChargeRewardJudgedByWorldStateNotDispatchedAction mirrors
// TestStepCombatRewardJudgedByWorldStateNotDispatchedAction's own reasoning:
// the target dying from something other than the dispatched action still
// counts, since the reward is judged from the resolved entity's own health,
// not from whether ActionSpearChargeWithApproach was chosen this step.
func TestStepSpearChargeRewardJudgedByWorldStateNotDispatchedAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setSpearTarget("minecraft:zombie", 31, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.SpearChargeTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.killSpearTarget() // died from something other than the spear charge
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.spearChargeCalls) != 0 {
		t.Fatalf("SpearChargeAtWithApproach calls = %v, want none (ActionWait doesn't dispatch the charge)", agent.spearChargeCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (the resolved target died, regardless of cause)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (spearChargeRewardBonus)", result.Reward)
	}
}

func TestStepObservationReflectsSpearTargetVisibilityRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setSpearTarget("minecraft:zombie", 31, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.SpearChargeTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[69]; visible != 1 {
		t.Fatalf("spearVisible at Reset = %v, want 1", visible)
	}
	if active := obs.Values[70]; active != 1 {
		t.Fatalf("goalSpearChargeActive at Reset = %v, want 1", active)
	}
	if dx := obs.Values[66]; dx != 5 {
		t.Fatalf("spearDx at Reset = %v, want 5", dx)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.spearChargeCalls) != 0 {
		t.Fatalf("SpearChargeAtWithApproach calls = %v, want none (ActionWait doesn't dispatch the charge)", agent.spearChargeCalls)
	}
	if visible := result.Observation.Values[69]; visible != 1 {
		t.Fatalf("spearVisible after ActionWait = %v, want 1 (still visible)", visible)
	}
}

func TestActionMaskSpearChargeGatedOnConfiguredVisibleAndWeapon(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.SpearChargeTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionSpearChargeWithApproach]; legal {
		t.Fatalf("ActionSpearChargeWithApproach legal = true, want false (configured but nothing visible, no weapon)")
	}

	agent.setSpearTarget("minecraft:zombie", 31, 5, 0, 0, 20)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[69]; visible != 1 {
		t.Fatalf("spearVisible = %v, want 1 (sanity check target is now visible)", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionSpearChargeWithApproach]; legal {
		t.Fatalf("ActionSpearChargeWithApproach legal = true, want false (visible but no weapon equipped yet)")
	}

	if err := agent.Equip(context.Background(), spearChargeWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionSpearChargeWithApproach]; !legal {
		t.Fatalf("ActionSpearChargeWithApproach legal = false, want true (now visible and weapon equipped)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionSpearChargeWithApproach]; legal {
		t.Fatalf("ActionSpearChargeWithApproach legal = true, want false (Config.SpearChargeTargetMobType unset, even though visible and equipped)")
	}
}

// --- Config.MaceSmashTargetMobType (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 7) ---

const maceSmashWeaponItemName = "minecraft:mace"

func TestStepMaceSmashWithNoConfiguredTargetIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMaceTarget("minecraft:zombie", 41, 5, 0, 0, 20) // present in the world, but Config never asks for it
	if err := agent.Equip(context.Background(), maceSmashWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMaceSmashWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.maceSmashCalls) != 0 {
		t.Fatalf("MaceSmashAtWithApproach calls = %v, want none (Config.MaceSmashTargetMobType unset, safe no-op like ActionWait)", agent.maceSmashCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if active := result.Observation.Values[75]; active != 0 {
		t.Fatalf("goalMaceSmashActive = %v, want 0 (no task configured)", active)
	}
}

func TestStepMaceSmashWithNoVisibleTargetDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	if err := agent.Equip(context.Background(), maceSmashWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.MaceSmashTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMaceSmashWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.maceSmashCalls) != 0 {
		t.Fatalf("MaceSmashAtWithApproach calls = %v, want none (nothing visible to smash)", agent.maceSmashCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[74]; visible != 0 {
		t.Fatalf("maceVisible = %v, want 0", visible)
	}
}

func TestStepMaceSmashWithoutWeaponDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMaceTarget("minecraft:zombie", 41, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.MaceSmashTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMaceSmashWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.maceSmashCalls) != 0 {
		t.Fatalf("MaceSmashAtWithApproach calls = %v, want none (no mace equipped)", agent.maceSmashCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepMaceSmashDispatchesWithResolvedEntityAndFixedWeapon(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMaceTarget("minecraft:zombie", 41, 5, 0, 0, 20)
	if err := agent.Equip(context.Background(), maceSmashWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.MaceSmashTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionMaceSmashWithApproach); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.maceSmashCalls) != 1 {
		t.Fatalf("MaceSmashAtWithApproach calls = %v, want exactly 1", agent.maceSmashCalls)
	}
	call := agent.maceSmashCalls[0]
	if call.targetID != 41 {
		t.Fatalf("targetID = %d, want 41 (Environment's own resolved entity, not a by-name search)", call.targetID)
	}
	if call.itemName != maceSmashWeaponItemName {
		t.Fatalf("itemName = %q, want %q", call.itemName, maceSmashWeaponItemName)
	}
}

func TestStepMaceSmashActionKillsTargetAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMaceTarget("minecraft:zombie", 41, 5, 0, 0, 20)
	if err := agent.Equip(context.Background(), maceSmashWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	cfg := testConfig()
	cfg.MaceSmashTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionMaceSmashWithApproach)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (target was killed)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (maceSmashRewardBonus)", result.Reward)
	}
}

// TestStepMaceSmashRewardJudgedByWorldStateNotDispatchedAction mirrors
// TestStepSpearChargeRewardJudgedByWorldStateNotDispatchedAction's own
// reasoning: the target dying from something other than the dispatched
// action still counts, since the reward is judged from the resolved
// entity's own health, not from whether ActionMaceSmashWithApproach was
// chosen this step.
func TestStepMaceSmashRewardJudgedByWorldStateNotDispatchedAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMaceTarget("minecraft:zombie", 41, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.MaceSmashTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	agent.killMaceTarget() // died from something other than the mace smash
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.maceSmashCalls) != 0 {
		t.Fatalf("MaceSmashAtWithApproach calls = %v, want none (ActionWait doesn't dispatch the smash)", agent.maceSmashCalls)
	}
	if !result.Done {
		t.Fatalf("Done = false, want true (the resolved target died, regardless of cause)")
	}
	if result.Reward <= 0 {
		t.Fatalf("Reward = %v, want positive (maceSmashRewardBonus)", result.Reward)
	}
}

func TestStepObservationReflectsMaceTargetVisibilityRegardlessOfAction(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMaceTarget("minecraft:zombie", 41, 5, 0, 0, 20)
	cfg := testConfig()
	cfg.MaceSmashTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if visible := obs.Values[74]; visible != 1 {
		t.Fatalf("maceVisible at Reset = %v, want 1", visible)
	}
	if active := obs.Values[75]; active != 1 {
		t.Fatalf("goalMaceSmashActive at Reset = %v, want 1", active)
	}
	if dx := obs.Values[71]; dx != 5 {
		t.Fatalf("maceDx at Reset = %v, want 5", dx)
	}

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if len(agent.maceSmashCalls) != 0 {
		t.Fatalf("MaceSmashAtWithApproach calls = %v, want none (ActionWait doesn't dispatch the smash)", agent.maceSmashCalls)
	}
	if visible := result.Observation.Values[74]; visible != 1 {
		t.Fatalf("maceVisible after ActionWait = %v, want 1 (still visible)", visible)
	}
}

func TestActionMaskMaceSmashGatedOnConfiguredVisibleAndWeapon(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MaceSmashTargetMobType = "minecraft:zombie"
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMaceSmashWithApproach]; legal {
		t.Fatalf("ActionMaceSmashWithApproach legal = true, want false (configured but nothing visible, no weapon)")
	}

	agent.setMaceTarget("minecraft:zombie", 41, 5, 0, 0, 20)
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if visible := result.Observation.Values[74]; visible != 1 {
		t.Fatalf("maceVisible = %v, want 1 (sanity check target is now visible)", visible)
	}
	if legal := env.ActionMask()[rlenv.ActionMaceSmashWithApproach]; legal {
		t.Fatalf("ActionMaceSmashWithApproach legal = true, want false (visible but no weapon equipped yet)")
	}

	if err := agent.Equip(context.Background(), maceSmashWeaponItemName); err != nil {
		t.Fatalf("Equip: %v", err)
	}
	if legal := env.ActionMask()[rlenv.ActionMaceSmashWithApproach]; !legal {
		t.Fatalf("ActionMaceSmashWithApproach legal = false, want true (now visible and weapon equipped)")
	}

	unconfigured := newTestEnvironment(t, agent, testConfig())
	if _, err := unconfigured.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if legal := unconfigured.ActionMask()[rlenv.ActionMaceSmashWithApproach]; legal {
		t.Fatalf("ActionMaceSmashWithApproach legal = true, want false (Config.MaceSmashTargetMobType unset, even though visible and equipped)")
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

// TestResetSeedsMineRequiredToolBeforeTheMineTarget mirrors
// TestResetCallsSeederForConfiguredTasks for Config.MineRequiredTool
// (docs/plans/14): DefaultEpisodeSeeder must give the tool via
// SeedGiveItem whenever it's set alongside MineTargetBlock.
func TestResetSeedsMineRequiredToolBeforeTheMineTarget(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:coal_ore"
	cfg.MineSearchRadius = 10
	cfg.MineRequiredTool = "minecraft:wooden_pickaxe"
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedGiveItemCalls) != 1 {
		t.Fatalf("SeedGiveItem calls = %d, want 1", len(agent.seedGiveItemCalls))
	}
	if got := agent.seedGiveItemCalls[0]; got.itemName != "minecraft:wooden_pickaxe" || got.count != 1 {
		t.Fatalf("SeedGiveItem call = %+v, want {minecraft:wooden_pickaxe 1}", got)
	}
}

// TestResetGivesEnchantedToolWhenAgentSupportsIt confirms
// Config.MineRequiredToolEnchantments routes to SeedGiveEnchantedItem
// (rlenv.EnchantedToolSeedAgent) instead of the plain SeedGiveItem path
// when the agent implements it.
func TestResetGivesEnchantedToolWhenAgentSupportsIt(t *testing.T) {
	agent := &fakeEnchantedToolAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:coal_ore"
	cfg.MineSearchRadius = 10
	cfg.MineRequiredTool = "minecraft:netherite_pickaxe"
	cfg.MineRequiredToolEnchantments = map[string]int{"minecraft:efficiency": 5}
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	// newTestEnvironment's *fakeAgent parameter type would drop the
	// wrapper's extra EnchantedToolSeedAgent method, so construct
	// directly against the wrapper instead — rlenv.New resolves
	// SeedAgent/ToolSeedAgent/EnchantedToolSeedAgent from whatever this
	// value actually implements.
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.fakeAgent.seedGiveItemCalls) != 0 {
		t.Fatalf("plain SeedGiveItem calls = %d, want 0 (enchanted path should have been taken instead)", len(agent.fakeAgent.seedGiveItemCalls))
	}
	if len(agent.seedGiveEnchantedItemCalls) != 1 {
		t.Fatalf("SeedGiveEnchantedItem calls = %d, want 1", len(agent.seedGiveEnchantedItemCalls))
	}
	got := agent.seedGiveEnchantedItemCalls[0]
	if got.itemName != "minecraft:netherite_pickaxe" || got.count != 1 {
		t.Fatalf("SeedGiveEnchantedItem call = %+v, want itemName=minecraft:netherite_pickaxe count=1", got)
	}
	if got.enchantments["minecraft:efficiency"] != 5 {
		t.Fatalf("SeedGiveEnchantedItem enchantments = %+v, want minecraft:efficiency=5", got.enchantments)
	}
}

// TestResetFallsBackToPlainToolWhenAgentLacksEnchantedSupport confirms
// Config.MineRequiredToolEnchantments never fails an episode just because
// the agent in use doesn't implement rlenv.EnchantedToolSeedAgent — see
// that field's own doc comment.
func TestResetFallsBackToPlainToolWhenAgentLacksEnchantedSupport(t *testing.T) {
	agent := newFakeAgent(0, 0, 0) // plain fakeAgent: no SeedGiveEnchantedItem
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:coal_ore"
	cfg.MineSearchRadius = 10
	cfg.MineRequiredTool = "minecraft:netherite_pickaxe"
	cfg.MineRequiredToolEnchantments = map[string]int{"minecraft:efficiency": 5}
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedGiveItemCalls) != 1 {
		t.Fatalf("SeedGiveItem calls = %d, want 1 (fallback path)", len(agent.seedGiveItemCalls))
	}
	if got := agent.seedGiveItemCalls[0]; got.itemName != "minecraft:netherite_pickaxe" || got.count != 1 {
		t.Fatalf("SeedGiveItem call = %+v, want {minecraft:netherite_pickaxe 1}", got)
	}
}

// TestResetSkipsSeedingMineRequiredToolWhenUnset mirrors
// TestResetSkipsSeedingUnconfiguredTasks for MineRequiredTool: a plain mine
// task (today's surface variety, never needing a tool) must not trigger
// SeedGiveItem just because MineTargetBlock is set.
func TestResetSkipsSeedingMineRequiredToolWhenUnset(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedGiveItemCalls) != 0 {
		t.Fatalf("SeedGiveItem calls = %d, want 0 (Config.MineRequiredTool unset)", len(agent.seedGiveItemCalls))
	}
}

// TestResetSkipsSeedNearbyBlockWhenMineTargetIsBuried covers
// Config.MineTargetBuried (docs/plans/14-buried-block-mining-and-memory-map.md,
// mc-rsi-trainer) — caught live, against a real server: without this,
// DefaultEpisodeSeeder's SeedNearbyBlock placed a second, immediately-
// visible instance right next to the bot every episode, since the real
// buried instance is never visible to FindVisibleBlock by design,
// defeating burial entirely.
func TestResetSkipsSeedNearbyBlockWhenMineTargetIsBuried(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:coal_ore"
	cfg.MineRequiredTool = "minecraft:wooden_pickaxe"
	cfg.MineTargetBuried = true
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedNearbyBlockCalls) != 0 {
		t.Fatalf("SeedNearbyBlock calls = %d, want 0 (Config.MineTargetBuried: the target must already be in the world)", len(agent.seedNearbyBlockCalls))
	}
	// MineRequiredTool's own seeding is unaffected by MineTargetBuried.
	if len(agent.seedGiveItemCalls) != 1 {
		t.Fatalf("SeedGiveItem calls = %d, want 1 (MineTargetBuried must not also suppress tool seeding)", len(agent.seedGiveItemCalls))
	}
}

// TestResetSeedsNearbyBlockWhenMineTargetIsNotBuried confirms the default
// (MineTargetBuried false) still places a fresh instance as before —
// MineTargetBuried changes behavior only when explicitly set.
func TestResetSeedsNearbyBlockWhenMineTargetIsNotBuried(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:stone"
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(agent.seedNearbyBlockCalls) != 1 {
		t.Fatalf("SeedNearbyBlock calls = %d, want 1 (Config.MineTargetBuried is false)", len(agent.seedNearbyBlockCalls))
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

// A table gate (CraftSearchRadius) makes craft legal only while a table is
// visible that close, so the policy has to travel to it - see
// Config.CraftSearchRadius.
func TestCraftSearchRadiusGatesCraftOnAVisibleTable(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCraftTarget("minecraft:bowl", true, 0) // ingredients held
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:bowl"
	cfg.CraftSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if obs.Values[13] != 0 || env.ActionMask()[rlenv.ActionCraft] {
		t.Fatalf("craft ready/legal with no table in range: obs=%v mask=%v", obs.Values[13], env.ActionMask()[rlenv.ActionCraft])
	}
	if _, err := env.Step(context.Background(), rlenv.ActionCraft); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.craftItemCalls != 0 {
		t.Fatalf("CraftItem calls = %d, want 0 (no table within the radius)", agent.craftItemCalls)
	}

	agent.tableVisible = true
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Observation.Values[13] != 1 || !env.ActionMask()[rlenv.ActionCraft] {
		t.Fatalf("craft not ready/legal with a table in range: obs=%v mask=%v", result.Observation.Values[13], env.ActionMask()[rlenv.ActionCraft])
	}
	for _, r := range agent.tableSearchRadii {
		if r != 8 {
			t.Fatalf("table searched with radius %d, want 8", r)
		}
	}
	if _, err := env.Step(context.Background(), rlenv.ActionCraft); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.craftItemCalls != 1 {
		t.Fatalf("CraftItem calls = %d, want 1", agent.craftItemCalls)
	}
}

// Without CraftSearchRadius no table search happens at all, so ordinary craft
// tasks (table beside the bot, or none needed) are unchanged.
func TestCraftWithoutSearchRadiusNeverSearchesForATable(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setCraftTarget("minecraft:stick", true, 0)
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:stick"
	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if obs.Values[13] != 1 {
		t.Fatalf("craftReady = %v, want 1", obs.Values[13])
	}
	if len(agent.tableSearchRadii) != 0 {
		t.Fatalf("searched for a table %d times, want 0", len(agent.tableSearchRadii))
	}
}

// With Config.NormalizeObservation every feature is scaled to about [-1, 1];
// without it the raw layout is unchanged.
func TestNormalizeObservationScalesFeaturesAndIsOffByDefault(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.yaw, agent.pitch = 90, -45
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{24, 0, 0}
	cfg.Jitter = [3]float64{}

	raw := newTestEnvironment(t, agent, cfg)
	rawObs, err := raw.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if rawObs.Values[0] != 24 || rawObs.Values[3] != 90 || rawObs.Values[4] != -45 {
		t.Fatalf("raw obs dx/yaw/pitch = %v/%v/%v, want 24/90/-45", rawObs.Values[0], rawObs.Values[3], rawObs.Values[4])
	}

	cfg.NormalizeObservation = true
	norm := newTestEnvironment(t, agent, cfg)
	obs, err := norm.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	want := map[int]float32{0: 24.0 / 16, 3: 0.5, 4: -0.5}
	for i, w := range want {
		if got := obs.Values[i]; got < w-1e-6 || got > w+1e-6 {
			t.Errorf("normalized Values[%d] = %v, want %v", i, got, w)
		}
	}
	for i, v := range obs.Values {
		if v > 2 || v < -2 {
			t.Errorf("normalized Values[%d] = %v, want within about [-2, 2]", i, v)
		}
	}
}

// Once a go-there-then-mine episode has arrived and the block is in reach,
// going anywhere only wastes steps, so goto is masked (and a masked goto is
// the usual safe no-op).
func TestCompositeMasksGotoOnceArrivedAndTheTaskActionIsReady(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:stone", 6, 0, 0)
	cfg := testConfig()
	cfg.TargetOffset = [3]float64{5, 0, 0}
	cfg.MineTargetBlock = "minecraft:stone"
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !env.ActionMask()[rlenv.ActionGoToTarget] {
		t.Fatal("goto masked before arriving")
	}

	if _, err := env.Step(ctx, rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}
	mask := env.ActionMask()
	if mask[rlenv.ActionGoToTarget] {
		t.Fatal("goto still legal after arriving with the block in reach")
	}
	if !mask[rlenv.ActionMine] || !mask[rlenv.ActionWait] {
		t.Fatalf("mask = %v, want mine and wait legal", mask)
	}

	// The mine action isn't possible (block out of sight): re-approaching is
	// the only way forward, so goto must come back.
	agent.setMineBlock("", 0, 0, 0)
	if _, err := env.Step(ctx, rlenv.ActionWait); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !env.ActionMask()[rlenv.ActionGoToTarget] {
		t.Fatal("goto masked although nothing can be mined from here")
	}
}

// A plain goto episode ends on arrival, so its mask never changes.
func TestPlainGotoIsNeverMaskedByArrival(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if !env.ActionMask()[rlenv.ActionGoToTarget] {
		t.Fatal("goto masked in a plain goto episode")
	}
}

// With Config.CollectDrops a mine step collects what it dropped; other
// steps, and a Config without it, never do.
func TestCollectDropsRunsAfterAMineStepOnly(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:oak_log", 1, 0, 0)
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:oak_log"
	cfg.CollectDrops = true
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(ctx, rlenv.ActionWait); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.collectDropsCalls != 0 {
		t.Fatalf("collect calls after a Wait = %d, want 0", agent.collectDropsCalls)
	}
	if _, err := env.Step(ctx, rlenv.ActionMine); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.collectDropsCalls != 1 {
		t.Fatalf("collect calls after a Mine = %d, want 1", agent.collectDropsCalls)
	}
}

func TestCollectDropsIsOffByDefault(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:oak_log", 1, 0, 0)
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:oak_log"
	env := newTestEnvironment(t, agent, cfg)
	ctx := context.Background()
	if _, err := env.Reset(ctx); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if _, err := env.Step(ctx, rlenv.ActionMine); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.collectDropsCalls != 0 {
		t.Fatalf("collect calls = %d, want 0 without Config.CollectDrops", agent.collectDropsCalls)
	}
}
