package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// MineBuriedOreFlatSuite is a version-parameterized suite for the
// rlenv.Environment/ActionMine completion half of the buried-mining
// scenario (docs/plans/14-buried-block-mining-and-memory-map.md, mc-rsi-
// trainer) - the half mine_buried_shaft_test.go's MineBuriedShaftFlatSuite
// deliberately leaves uncovered (see that file's own doc comment): not just
// "can MoveTo dig through solid ground," but "does a real mine_buried
// episode (RCON-seeded tool, buried-not-visible ore, overburden dug by the
// pathfinder, then Environment.Step(ActionMine) resolving and rewarding the
// completed dig) actually complete end to end against a live server."
//
// Deliberately run against a single version (rlTrainTestVersion, shared
// with rl_train_test.go), not models.StandardVersionTests - same reasoning
// as that file's own doc comment: nothing under test here is
// protocol-version-specific (rlenv/FindVisibleBlock/ActionMine dispatch all
// sit above the already-version-tested pathfinding/physics primitives
// mine_buried_shaft_test.go exercises directly), so looping across every
// version would just be repeated server-startup cost for no additional
// coverage.
type MineBuriedOreFlatSuite struct {
	VersionWorldSuite
}

func TestMineBuriedOreFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, []models.VersionTest{{Name: rlTrainTestVersion, MCVersion: rlTrainTestVersion}}, func() suite.TestingSuite {
		s := &MineBuriedOreFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// buriedOreApproachDepth is both the horizontal and vertical offset (from
// the agent's own working area) to the point just above the buried ore.
// Deep enough overall to land solidly within the flat-world preset's stone
// band (dirt is only 3 blocks deep - see buildServerConfig), matching
// docs/plans/14's own "Part 2: ore placement within the stone band" design.
// Unlike mine_buried_shaft_test.go's shaftDepth, equal horizontal/vertical
// span here doesn't guarantee an all-MineDownDiagonal path - A* found it
// cheaper to cover the horizontal distance with ordinary surface movement
// first, then descend with plain MineDown once directly above the shaft
// column (confirmed live) - this test isn't the headroom-cell regression
// check (that's mine_buried_shaft_test.go's job); it's confirming the
// seeding/burial/dig/ActionMine pipeline as a whole. The ore itself sits
// one block below the approach point, never mined by the MoveTo call
// itself - only the overburden above it.
const buriedOreApproachDepth = 6

// TestDigsBuriedOreThenCompletesMineEpisode is the integration-test port of
// this session's scratchpad smoke-test driver's *second* half (its first
// half - MoveTo digging through solid ground to an arbitrary point - is
// mine_buried_shaft_test.go's own test): place a target block buried under
// real overburden (not Config.Seeder's usual SeedNearbyBlock, which places
// a visible instance right next to the bot - see Config.MineTargetBuried's
// own doc comment for why that defeats burial entirely), dig down to it
// with the real pathfinder exactly like the other half, then hand off to
// the real rlenv.Environment.Step(ActionMine) once genuine line of sight
// exists, confirming the full pipeline - RCON tool seeding,
// pathfinding-driven overburden removal, FindVisibleBlock resolving the
// now-uncovered block, ActionMine's dispatch/completion/reward - works end
// to end, not just that MoveTo alone can reach a buried point.
func (s *MineBuriedOreFlatSuite) TestDigsBuriedOreThenCompletesMineEpisode() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("BuriedOreBot", "mine_buried_ore")
	require.NoError(t, err, "spawn agent")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = leader.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent (the test framework already wires RCON into agent.Config.RCON for every spawned agent)")

	startX := int(math.Floor(leader.Origin.X))
	startY := int(math.Floor(leader.Origin.Y))
	startZ := int(math.Floor(leader.Origin.Z))

	approachX := startX + buriedOreApproachDepth
	approachY := startY - buriedOreApproachDepth
	approachZ := startZ
	oreX, oreY, oreZ := approachX, approachY-1, approachZ

	// Place the ore before Reset, not via Config.Seeder's usual
	// SeedNearbyBlock path - see Config.MineTargetBuried's own doc comment
	// for why: SeedNearbyBlock always places a second, immediately-visible
	// instance beside the bot, which would defeat burial entirely.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:coal_ore", oreX, oreY, oreZ))
	require.NoError(t, err, "place buried ore")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		// ArrivalThreshold is required by Config.Validate even though this
		// test never dispatches ActionGoToTarget (it poses no TargetOffset
		// task - movement here is all direct MoveTo calls, outside the
		// action space) - matching the value every other test in this
		// package that constructs an rlenv.Config uses.
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:coal_ore",
		MineSearchRadius: 10,
		MineRequiredTool: "minecraft:wooden_pickaxe",
		MineTargetBuried: true,
		Seeder:           rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(s.Ctx)
	require.NoError(t, err, "Reset (includes RCON tool seeding)")
	require.Equal(t, float32(0), obs.Values[12], "mineVisible should be 0 right after Reset - the ore is buried, not seeded next to the bot")

	require.GreaterOrEqual(t, liveAgent.InventoryCount("minecraft:wooden_pickaxe"), 1, "DefaultEpisodeSeeder should have given a wooden pickaxe despite MineTargetBuried")

	// Dig to a point just above the ore via the real pathfinder - NOT a
	// manual per-layer loop, and NOT through the RL action space's
	// ActionGoToTarget (this Environment poses no TargetOffset task at
	// all) - exactly mirroring mine_buried_shaft_test.go's own approach,
	// playing the role of "a policy that already knows where the target
	// is" since there is no trained policy or exploration capability yet
	// (by design - deferred future work, same as the scratchpad driver
	// this ports).
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	err = leader.Agent.MoveTo(ctx, float64(approachX), float64(approachY), float64(approachZ), false)
	require.NoError(t, err, "MoveTo should dig through the overburden above the buried ore")

	require.NoError(t, waitForBotNear(leader.Agent, models.V3{X: float64(approachX), Y: float64(approachY), Z: float64(approachZ)}, 1.5, 10*time.Second),
		"bot should have settled just above the buried ore after MoveTo returned")

	// Hand off to the real Environment: the overburden is gone, so
	// FindVisibleBlock should resolve the ore for real now, exercising the
	// actual ActionMine dispatch/completion path - not asserting Done after
	// a single Step call, since the dig just finished and the mine-target
	// resolution state may need a step or two to catch up (see
	// stepUntilDone's own doc comment).
	result := stepUntilDone(t, rlEnv, s.Ctx, rlenv.ActionMine, rlTrainStepRetryAttempts)
	require.True(t, result.Done, "episode should end once the uncovered buried ore is mined (within %d attempts)", rlTrainStepRetryAttempts)
	require.Greater(t, result.Reward, float32(5), "reward should include mineRewardBonus")

	isAir, err := waitForBlockAir(s.Ctx, s.Inst.RCON, oreX, oreY, oreZ, 5*time.Second)
	require.NoError(t, err, "check ore block state via RCON")
	require.True(t, isAir, "ore block should be gone after the mine episode completed")

	t.Log("✓ buried-ore mine_buried episode (RCON tool seeding + pathfinding-driven overburden removal + ActionMine dispatch/completion/reward) confirmed against a live server")
}
