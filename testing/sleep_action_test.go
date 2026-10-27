package testing

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SleepActionFlatSuite is the Sleep action's own live validation suite
// (docs/plans/SLEEP_ACTION_PLAN.md, local/untracked), mirroring
// mine_buried_action_space_test.go's own shape: drive the real discrete
// action space (ActionSleep), through the real registry/rlenv pipeline
// against a real server, not a hand-driven backdoor. This is the one
// piece of the plan explicitly flagged as needing real protocol
// verification rather than guessing: whether SleepInBed's position-snap
// confirmation signal actually fires, and whether
// playersSleepingPercentage actually lets a single bot skip the night.
type SleepActionFlatSuite struct {
	VersionWorldSuite
}

func TestSleepActionFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, []models.VersionTest{{Name: rlTrainTestVersion, MCVersion: rlTrainTestVersion}}, func() suite.TestingSuite {
		s := &SleepActionFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

const sleepActionSearchRadius = 10

// TestSleepThroughTheNightViaDiscreteActionSpace seeds a bed, sets the
// world clock to night, and dispatches nothing but ActionSleep (the real
// discrete action a trained policy would choose) until the episode
// reports done - confirming SleepInBed's own position-snap-based
// confirmation actually fires against a real server, not just that the
// RCON calls and packet sends don't error.
func (s *SleepActionFlatSuite) TestSleepThroughTheNightViaDiscreteActionSpace() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("SleepBot", "sleep_action")
	require.NoError(t, err, "spawn agent")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	// playersSleepingPercentage=0 lets this single bot skip the night on
	// its own, regardless of how many other players/bots might share the
	// dimension in a real shared-server training run - the architectural
	// risk docs/plans/SLEEP_ACTION_PLAN.md flags as needing exactly this
	// live confirmation before relying on it.
	_, err = s.Inst.RCON.Exec(s.Ctx, "gamerule playersSleepingPercentage 0")
	require.NoError(t, err, "set playersSleepingPercentage")
	// testing/framework.go's standard server setup disables doDaylightCycle
	// (so unrelated tests get a stable, manually-controlled clock) - but
	// vanilla gates the sleep-triggered night skip on that same gamerule,
	// so this test needs it back on to exercise the real mechanic at all.
	_, err = s.Inst.RCON.Exec(s.Ctx, "gamerule doDaylightCycle true")
	require.NoError(t, err, "re-enable doDaylightCycle for this test")
	_, err = s.Inst.RCON.Exec(s.Ctx, "time set 13000") // within the vanilla sleep window
	require.NoError(t, err, "set time to night")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		ArrivalThreshold:   1.5,
		StepTimeout:        15 * time.Second,
		SleepSearchRadius:  sleepActionSearchRadius,
		GoToTargetDisabled: true,
		Seeder:             rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(s.Ctx)
	require.NoError(t, err, "Reset (includes RCON bed seeding)")
	require.Equal(t, float32(1), obs.Values[25], "isNight should be 1 right after Reset - time was set to 13000")
	require.Equal(t, float32(1), obs.Values[30], "bedVisible should be 1 right after Reset - DefaultEpisodeSeeder should have placed one")
	require.True(t, rlEnv.ActionMask()[rlenv.ActionSleep], "ActionSleep should be legal: night + visible bed")

	done := false
	for step := 0; step < 10; step++ {
		result, stepErr := rlEnv.Step(s.Ctx, rlenv.ActionSleep)
		require.NoError(t, stepErr, "Step %d", step)
		t.Logf("step %d: reward=%.2f done=%v isNight=%v timeUntilDawn=%v", step, result.Reward, result.Done, result.Observation.Values[25], result.Observation.Values[26])
		if result.Done {
			done = true
			t.Logf("step %d: episode done (reward=%.2f) - slept through the night", step, result.Reward)
			break
		}
	}
	require.True(t, done, "dispatching ActionSleep repeatedly should confirm sleep and skip the night within 10 steps")

	timeAfter, err := s.Inst.RCON.Exec(s.Ctx, "time query daytime")
	require.NoError(t, err)
	t.Logf("✓ slept through the night - server time query after: %s", timeAfter)
}

// TestSleepIllegalDuringDaytimeLive confirms the legality gate's vanilla
// sleep-window boundaries against a real server's own time-of-day
// reporting, not just the fake's controllable one (rlenv/sleep_test.go).
func (s *SleepActionFlatSuite) TestSleepIllegalDuringDaytimeLive() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("SleepDayBot", "sleep_action_day")
	require.NoError(t, err, "spawn agent")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	_, err = s.Inst.RCON.Exec(s.Ctx, "time set 6000") // midday
	require.NoError(t, err, "set time to midday")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		ArrivalThreshold:   1.5,
		StepTimeout:        15 * time.Second,
		SleepSearchRadius:  sleepActionSearchRadius,
		GoToTargetDisabled: true,
		Seeder:             rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(s.Ctx)
	require.NoError(t, err, "Reset")
	require.Equal(t, float32(0), obs.Values[25], "isNight should be 0 at midday")
	require.Equal(t, float32(1), obs.Values[30], "bedVisible should still be 1 - the bed itself doesn't care what time it is")
	require.False(t, rlEnv.ActionMask()[rlenv.ActionSleep], "ActionSleep should be illegal at midday even with a visible bed")

	result, err := rlEnv.Step(s.Ctx, rlenv.ActionSleep)
	require.NoError(t, err, "Step")
	require.False(t, result.Done, "an illegal ActionSleep dispatch should be a safe no-op, not end the episode")
}
