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

// TestSleepSucceedsWithAnotherAwakeBotOnSharedServer is the plan's own
// Phase 8 requirement: confirm playersSleepingPercentage=0 under something
// resembling the real cmd/rsi-train -shared-server -parallel-envs setup,
// not just a single disposable bot alone on the server. Vanilla's sleep
// skip is computed across every player connected to the dimension, not
// just ones nearby - a second bot elsewhere on this same server, left
// connected and awake (never dispatching Sleep, never touching a bed), is
// exactly the condition that would silently defeat the skip if the
// percentage=0 mitigation didn't actually work the way it's assumed to:
// idle-but-connected is enough to exercise the real mechanic, since what
// counts is the other player's sleep state, not what else it's doing.
func (s *SleepActionFlatSuite) TestSleepSucceedsWithAnotherAwakeBotOnSharedServer() {
	t := s.T()

	// Its own working area (256+ blocks from the sleeping bot's - see
	// SpawnWorkingAreaAgent's doc comment), matching how two independent
	// -parallel-envs training bots actually share one dimension in
	// production: never interacting, but both counted by
	// playersSleepingPercentage. Never referenced again below - it only
	// needs to exist and stay connected for the rest of this test.
	_, err := s.SpawnWorkingAreaAgent("SleepSharedOtherBot", "sleep_action_shared_other")
	require.NoError(t, err, "spawn other, awake bot")

	leader, err := s.SpawnWorkingAreaAgent("SleepSharedBot", "sleep_action_shared")
	require.NoError(t, err, "spawn sleeping bot")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	_, err = s.Inst.RCON.Exec(s.Ctx, "gamerule playersSleepingPercentage 0")
	require.NoError(t, err, "set playersSleepingPercentage")
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
	require.Equal(t, float32(1), obs.Values[25], "isNight should be 1 right after Reset")
	require.Equal(t, float32(1), obs.Values[30], "bedVisible should be 1 right after Reset")
	require.True(t, rlEnv.ActionMask()[rlenv.ActionSleep], "ActionSleep should be legal: night + visible bed")

	done := false
	for step := 0; step < 10; step++ {
		result, stepErr := rlEnv.Step(s.Ctx, rlenv.ActionSleep)
		require.NoError(t, stepErr, "Step %d", step)
		t.Logf("step %d: reward=%.2f done=%v isNight=%v timeUntilDawn=%v", step, result.Reward, result.Done, result.Observation.Values[25], result.Observation.Values[26])
		if result.Done {
			done = true
			t.Logf("step %d: episode done (reward=%.2f) - slept through the night despite a second, awake player sharing the server", step, result.Reward)
			break
		}
	}
	require.True(t, done, "a single sleeping bot should still skip the night with playersSleepingPercentage=0, even while another connected player elsewhere on the same server never sleeps")
}

// TestSleepSucceedsWithoutNightSkipWhenPercentageNotMet covers the other
// half of the shared-server risk: playersSleepingPercentage=0 is a
// mitigation this repo's own training setup chooses to apply, but a real
// server the agent might run against could leave it at vanilla's own
// default (100 - every player must be in bed before the night skips).
// Sleeping still matters even when that aggregate skip never happens -
// vanilla resets a player's own minecraft:custom:time_since_rest
// statistic (what actually prevents phantom spawns) as soon as THAT
// player's own bed-entry succeeds, independent of whether anyone else is
// asleep or the night ever advances. The real risk this test is for: with
// a second, connected bot that never sleeps and percentage left at 100,
// confirm the night genuinely never skips (so this test is actually
// exercising that path, not accidentally skipping anyway), and - the part
// nothing else in this suite checks - that the sleeping bot isn't left
// permanently stuck lying in the bed once the episode's step budget runs
// out: WakeUp (dispatched directly, the same way a real caller would once
// it decided an episode without a night skip had gone on long enough)
// still recovers it to normal movement afterward.
func (s *SleepActionFlatSuite) TestSleepSucceedsWithoutNightSkipWhenPercentageNotMet() {
	t := s.T()

	_, err := s.SpawnWorkingAreaAgent("SleepNoSkipOther", "sleep_action_noskip_other")
	require.NoError(t, err, "spawn other, never-sleeping bot")

	leader, err := s.SpawnWorkingAreaAgent("SleepNoSkipBot", "sleep_action_noskip")
	require.NoError(t, err, "spawn sleeping bot")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	_, err = s.Inst.RCON.Exec(s.Ctx, "gamerule playersSleepingPercentage 100") // vanilla's own default
	require.NoError(t, err, "set playersSleepingPercentage to vanilla default")
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
	require.Equal(t, float32(1), obs.Values[25], "isNight should be 1 right after Reset")
	require.True(t, rlEnv.ActionMask()[rlenv.ActionSleep], "ActionSleep should be legal: night + visible bed")

	for step := 0; step < 5; step++ {
		result, stepErr := rlEnv.Step(s.Ctx, rlenv.ActionSleep)
		require.NoError(t, stepErr, "Step %d", step)
		t.Logf("step %d: reward=%.2f done=%v isNight=%v", step, result.Reward, result.Done, result.Observation.Values[25])
		require.False(t, result.Done, "the night should never skip here - the other bot never sleeps and percentage=100 requires every connected player to")
		require.Equal(t, float32(1), result.Observation.Values[25], "isNight should still be 1 - no skip should have happened")
	}

	// The actual risk: a training loop that let an episode like this run
	// out its step budget (rather than this test's deliberately short 5
	// steps) needs the bot to still be recoverable afterward, not
	// permanently wedged in the sleeping pose for the rest of the run.
	err = liveAgent.WakeUp(s.Ctx)
	require.NoError(t, err, "WakeUp should succeed even though the night never skipped")
	time.Sleep(1 * time.Second) // let the server process leaving the bed before trusting movement again

	pos, _, _, ok := liveAgent.GetPosition()
	require.True(t, ok, "position should be known after WakeUp")
	err = liveAgent.MoveTo(s.Ctx, pos.X+1, pos.Y, pos.Z, false)
	require.NoError(t, err, "bot should move normally after WakeUp - not left stuck in the sleeping pose when the night never skips")
}
