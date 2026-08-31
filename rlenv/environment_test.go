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
		PollInterval:     time.Millisecond,
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
	if got := env.ObservationSize(); got != 9 {
		t.Fatalf("ObservationSize() = %d, want 9", got)
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
	if len(obs.Values) != 9 {
		t.Fatalf("len(obs.Values) = %d, want 9", len(obs.Values))
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

func TestStepTimesOutWithoutErrorWhenTargetUnreachable(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.moveToWithChatErr = context.DeadlineExceeded // MoveToWithChat "fails" every call: position never changes
	cfg := testConfig()
	cfg.StepTimeout = 20 * time.Millisecond
	cfg.PollInterval = time.Millisecond
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionGoToTarget)
	if err != nil {
		t.Fatalf("Step: %v, want nil (a step timeout is not an environment error)", err)
	}
	if result.Done {
		t.Fatalf("Done = true, want false (target never reached)")
	}
}

func TestStepPropagatesContextCancellation(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.moveToWithChatErr = context.DeadlineExceeded // position never changes; forces the poll loop to run
	cfg := testConfig()
	cfg.StepTimeout = time.Second
	cfg.PollInterval = time.Millisecond
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

// compile-time check that fakeAgent satisfies both interfaces rlenv.LiveAgent
// requires.
var (
	_ models.CommandAgent   = (*fakeAgent)(nil)
	_ models.HealthProvider = (*fakeAgent)(nil)
	_ rlenv.LiveAgent       = (*fakeAgent)(nil)
)
