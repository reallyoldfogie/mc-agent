package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// --- ActionSleep (docs/plans/SLEEP_ACTION_PLAN.md, local/untracked) ---

func TestStepSleepWithRadiusZeroIsNoOp(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.bedFound, agent.bedX, agent.bedY, agent.bedZ = true, 5, 0, 0
	agent.timeOfDayTicks, agent.timeOfDayKnown = 13000, true // within the sleep window
	env := newTestEnvironment(t, agent, testConfig())        // SleepSearchRadius unset (0)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionSleep)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.sleepCalls != 0 {
		t.Fatalf("SleepInBed calls = %d, want 0 (Config.SleepSearchRadius unset, safe no-op like ActionWait)", agent.sleepCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
	if visible := result.Observation.Values[30]; visible != 0 {
		t.Fatalf("bedVisible = %v, want 0 (no task configured)", visible)
	}
}

func TestStepSleepWithNoVisibleBedDoesNotDispatch(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.timeOfDayTicks, agent.timeOfDayKnown = 13000, true
	cfg := testConfig()
	cfg.SleepSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	result, err := env.Step(context.Background(), rlenv.ActionSleep)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.sleepCalls != 0 {
		t.Fatalf("SleepInBed calls = %d, want 0 (nothing visible to sleep in)", agent.sleepCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepSleepIllegalDuringDaytime(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.bedFound, agent.bedX, agent.bedY, agent.bedZ = true, 5, 0, 0
	agent.timeOfDayTicks, agent.timeOfDayKnown = 6000, true // midday, outside the sleep window
	cfg := testConfig()
	cfg.SleepSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if legal(env, rlenv.ActionSleep) {
		t.Fatalf("ActionSleep legal at daytime (tick 6000), want illegal (mask=%v)", env.ActionMask())
	}

	result, err := env.Step(context.Background(), rlenv.ActionSleep)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.sleepCalls != 0 {
		t.Fatalf("SleepInBed calls = %d, want 0 (not night)", agent.sleepCalls)
	}
	if result.Done {
		t.Fatalf("Done = true, want false")
	}
}

func TestStepSleepIllegalWhenTimeOfDayUnknown(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.bedFound, agent.bedX, agent.bedY, agent.bedZ = true, 5, 0, 0
	agent.timeOfDayKnown = false // no Update Time packet received yet
	cfg := testConfig()
	cfg.SleepSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if legal(env, rlenv.ActionSleep) {
		t.Fatalf("ActionSleep legal with unknown time of day, want illegal - must not be misread as \"definitely not night\" OR \"definitely night\" (mask=%v)", env.ActionMask())
	}
}

func TestStepSleepDispatchesAtNightWithVisibleBed(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.bedFound, agent.bedX, agent.bedY, agent.bedZ = true, 5, 0, 0
	agent.timeOfDayTicks, agent.timeOfDayKnown = 13000, true
	cfg := testConfig()
	cfg.SleepSearchRadius = 8
	env := newTestEnvironment(t, agent, cfg)
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if !legal(env, rlenv.ActionSleep) {
		t.Fatalf("ActionSleep illegal at night with a visible bed, want legal (mask=%v)", env.ActionMask())
	}

	if _, err := env.Step(context.Background(), rlenv.ActionSleep); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.sleepCalls != 1 {
		t.Fatalf("SleepInBed calls = %d, want 1", agent.sleepCalls)
	}
}

func TestStepSleptThroughNightPaysBonusAndEndsEpisode(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.timeOfDayTicks, agent.timeOfDayKnown = 13000, true // within the sleep window
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// A step where nothing changes: not a backward jump, no bonus.
	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true on an ordinary step, want false")
	}

	// Simulate the server advancing to morning via a successful sleep
	// attempt elsewhere this step (e.g. another managed participant, or
	// this same one via ActionSleep on a prior step) - refreshSleepTarget
	// picks this up regardless of which action was dispatched, same
	// "judge from world-state deltas" philosophy as mined/craftedThisStep.
	agent.timeOfDayTicks = 500

	result, err = env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if !result.Done {
		t.Fatalf("Done = false after a night-to-morning time jump, want true")
	}
	// Allow for the per-step time penalty (Config.TimePenalty, default
	// 0.01) subtracted from every step regardless of outcome - this just
	// needs to clearly be "the bonus fired," not match it to the exact cent.
	if result.Reward < sleptThroughNightBonusForTest-1 {
		t.Fatalf("reward = %v, want at least sleptThroughNightBonus (%v) minus a small time penalty", result.Reward, sleptThroughNightBonusForTest)
	}
}

// sleptThroughNightBonusForTest mirrors reward.go's own unexported
// sleptThroughNightBonus constant (10.0) - duplicated here rather than
// exported, matching this package's existing convention of keeping
// reward magnitudes internal (see e.g. arrivalBonus/mineRewardBonus,
// neither exported either).
const sleptThroughNightBonusForTest = 10.0

func TestStepNaturalDayRolloverDoesNotFalselyTriggerSleptBonus(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.timeOfDayTicks, agent.timeOfDayKnown = 23999, true // just before the sleep window closes
	env := newTestEnvironment(t, agent, testConfig())
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// A real day rollover (23999 -> 0) also looks like "ticks went
	// backward," but only matters as a false sleep signal while still
	// inside the sleep window - 23999 is just past vanillaSleepWindowEnd
	// (23459), so prevSleepWindowActive is already false here and this
	// must not pay the bonus.
	agent.timeOfDayTicks = 0

	result, err := env.Step(context.Background(), rlenv.ActionWait)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	if result.Done {
		t.Fatalf("Done = true on an ordinary post-window day rollover, want false (prevSleepWindowActive was false)")
	}
}

// fakeSeedBedAgent is a separate wrapper type implementing
// rlenv.SeedBedAgent, mirroring fakeBuriedTargetAgent/fakeBlocksMinedAgent's
// own pattern (fake_agent_test.go) - not a method directly on the base
// fakeAgent, so existing DefaultEpisodeSeeder tests against a plain
// fakeAgent keep exercising the "agent doesn't support this" skip path.
type fakeSeedBedAgent struct {
	*fakeAgent
	seedBedCalls  int
	seedBedRadius int
	seedBedErr    error
}

func (f *fakeSeedBedAgent) SeedBed(_ context.Context, radius int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seedBedCalls++
	f.seedBedRadius = radius
	return f.seedBedErr
}

func TestResetCallsSeedBedWhenSleepSearchRadiusConfigured(t *testing.T) {
	agent := &fakeSeedBedAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	cfg := testConfig()
	cfg.SleepSearchRadius = 12
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if agent.seedBedCalls != 1 {
		t.Fatalf("SeedBed calls = %d, want 1", agent.seedBedCalls)
	}
	if agent.seedBedRadius != 12 {
		t.Fatalf("SeedBed radius = %d, want 12", agent.seedBedRadius)
	}
}

func TestResetSkipsSeedBedWithoutSleepSearchRadius(t *testing.T) {
	agent := &fakeSeedBedAgent{fakeAgent: newFakeAgent(0, 0, 0)}
	cfg := testConfig()
	cfg.Seeder = rlenv.DefaultEpisodeSeeder // SleepSearchRadius left at 0
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if agent.seedBedCalls != 0 {
		t.Fatalf("SeedBed calls = %d, want 0 (Config.SleepSearchRadius unset)", agent.seedBedCalls)
	}
}

func TestResetSucceedsWithoutSeedBedAgentCapability(t *testing.T) {
	// A plain fakeAgent (no SeedBedAgent) with SleepSearchRadius
	// configured must not fail Reset - matches ToolSeedAgent's own
	// "unhonored, not an error" convention for an agent that doesn't
	// implement the optional seeding capability.
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig()
	cfg.SleepSearchRadius = 12
	cfg.Seeder = rlenv.DefaultEpisodeSeeder
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v (SeedBedAgent not implemented should be silently skipped, not an error)", err)
	}
}
