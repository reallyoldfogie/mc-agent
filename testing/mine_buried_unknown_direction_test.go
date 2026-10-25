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

// MineBuriedUnknownDirectionFlatSuite validates that unknownDx/Dy/Dz
// (rlenv/observation.go indices 21-24, docs/plans/14-buried-block-mining-
// and-memory-map.md's option 2 in mc-rsi-trainer) is actually an
// informative signal for finding a buried target - not just that the
// mechanics work once something already knows where the ore is (that's
// MineBuriedOreFlatSuite's job, and this suite deliberately reuses its
// setup shape). Drives a trivial scripted controller - "walk toward
// whatever direction unknownDx/Dy/Dz currently points, repeat" - using
// only the Environment's own observation and nothing else about the ore's
// placement, the same way an RL policy eventually would once trained.
//
// This is explicitly NOT a PPO run: it is the minimum check that a policy
// with nothing but this signal and good-faith exploration *could* ever
// reach the target, before paying for a multi-day retrain finds out the
// hard way. mine_buried/chain_mine currently complete 0 of several
// thousand live training episodes (see mc-rsi-trainer's own live
// diagnosis) - skipping this check would mean re-running that same
// multi-day experiment on faith that the new observation feature helps.
type MineBuriedUnknownDirectionFlatSuite struct {
	VersionWorldSuite
}

func TestMineBuriedUnknownDirectionFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, []models.VersionTest{{Name: rlTrainTestVersion, MCVersion: rlTrainTestVersion}}, func() suite.TestingSuite {
		s := &MineBuriedUnknownDirectionFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// unknownDirectionSearchRadius matches Config.MineSearchRadius for this
// suite - half of it (observationUnknownDirection's own convention) bounds
// how far any single octant-walk leg travels. unknownDirectionMaxRounds
// generously bounds the walk (up to 8 octants, plus slack for a MoveTo leg
// that stalls on one octant and has to let the next round's observation
// redirect it) before the test declares the signal failed to lead anywhere.
const (
	unknownDirectionSearchRadius = 10
	unknownDirectionMaxRounds    = 24
)

// TestGreedyOctantWalkFindsTheBuriedOre buries an ore inside the search
// volume without ever telling the controller where, then repeatedly: reads
// unknownDx/Dy/Dz off the environment's own observation, walks toward that
// point with the real pathfinder (MoveTo, same stand-in role
// MineBuriedOreFlatSuite's own doc comment describes - digging through
// overburden is pathfinding's job, already proven by
// mine_buried_shaft_test.go, not this test's), and re-observes. Success is
// mineVisible (observation index 12) flipping to 1 - FindVisibleBlock
// genuinely resolving the ore - not Done, since completing the actual mine
// is MineBuriedOreFlatSuite's scope, not this signal's.
func (s *MineBuriedUnknownDirectionFlatSuite) TestGreedyOctantWalkFindsTheBuriedOre() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("UnknownDirBot", "mine_buried_unknown_direction")
	require.NoError(t, err, "spawn agent")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")

	startX := int(math.Floor(leader.Origin.X))
	startY := int(math.Floor(leader.Origin.Y))
	startZ := int(math.Floor(leader.Origin.Z))

	// Buried off in one arbitrary octant (+X, -Y, -Z) - the controller
	// below never reads these coordinates, only Reset/Step's observation.
	half := unknownDirectionSearchRadius / 2
	oreX := startX + half
	oreY := startY - half
	oreZ := startZ - half
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:coal_ore", oreX, oreY, oreZ))
	require.NoError(t, err, "place buried ore")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		ArrivalThreshold: 1.5,
		StepTimeout:      15 * time.Second,
		MineTargetBlock:  "minecraft:coal_ore",
		MineSearchRadius: unknownDirectionSearchRadius,
		MineRequiredTool: "minecraft:netherite_pickaxe",
		// Efficiency V: break time isn't the dominant per-block cost here
		// (movement/pathing overhead is — see this suite's own findings),
		// but it's free and safe, so there's no reason to leave it at
		// wooden-pickaxe speed while testing the real bottleneck.
		MineRequiredToolEnchantments: map[string]int{"minecraft:efficiency": 5},
		MineTargetBuried:             true,
		Seeder:                       rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(s.Ctx)
	require.NoError(t, err, "Reset (includes RCON tool seeding)")
	require.Equal(t, float32(0), obs.Values[12], "mineVisible should be 0 right after Reset - the ore is buried")
	require.Equal(t, float32(1), obs.Values[24], "unknownValid should be 1 right after Reset - nothing explored yet, target unseen")

	found := false
	for round := 0; round < unknownDirectionMaxRounds; round++ {
		// mineVisible can flip to 1 without the ore being destroyed yet
		// (found live building this suite: the pathfinder can clear
		// enough overburden to resolve line of sight without the final
		// block happening to be in its path) - once that's true, a real
		// policy would dispatch ActionMine, not keep walking toward the
		// exploration signal, so the controller does the same here before
		// falling through to the walk below.
		if obs.Values[12] == 1 {
			t.Logf("round %d: mineVisible=1 - dispatching ActionMine", round)
			result := stepUntilDone(t, rlEnv, s.Ctx, rlenv.ActionMine, rlTrainStepRetryAttempts)
			obs = result.Observation
			if result.Done {
				found = true
				t.Logf("round %d: episode done (reward=%.2f) via ActionMine", round, result.Reward)
				break
			}
			t.Logf("round %d: mineVisible=1 but ActionMine didn't finish within %d attempts - resuming the exploration walk", round, rlTrainStepRetryAttempts)
		}
		if obs.Values[24] == 0 {
			t.Logf("round %d: unknownValid=0 (every octant explored) before the ore was ever seen/mined", round)
			break
		}
		unknownDx, unknownDy, unknownDz := obs.Values[21], obs.Values[22], obs.Values[23]

		pos, _, _, posOK := liveAgent.GetPosition()
		require.True(t, posOK, "agent position should be known mid-episode")
		targetX := pos.X + float64(unknownDx)
		targetY := pos.Y + float64(unknownDy)
		targetZ := pos.Z + float64(unknownDz)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		moveErr := leader.Agent.MoveTo(ctx, targetX, targetY, targetZ, false)
		cancel()
		// A MoveTo that times out or can't fully path isn't fatal here —
		// markExploredOctant fires off wherever the bot actually ended up,
		// not off MoveTo succeeding, so a partial attempt still moves the
		// walk forward; only running out of rounds fails the test.
		t.Logf("round %d: walked toward (%.1f,%.1f,%.1f) [unknownD=(%.1f,%.1f,%.1f)], moveErr=%v", round, targetX, targetY, targetZ, unknownDx, unknownDy, unknownDz, moveErr)

		result, stepErr := rlEnv.Step(s.Ctx, rlenv.ActionWait)
		require.NoError(t, stepErr, "Step")
		obs = result.Observation
		// Checking Done, not just mineVisible: the pathfinder digging
		// toward the signal's target routinely destroys the buried ore
		// as ordinary overburden before FindVisibleBlock ever resolves
		// it (found live building this suite) - mineVisible alone missed
		// that outcome entirely. Done reflects BuriedTargetAgent's
		// LOS-free destruction check too (see rlenv.BuriedTargetAgent).
		if obs.Values[12] == 1 {
			t.Logf("round %d: mineVisible flipped to 1 - ore exposed", round)
		}
		if result.Done {
			found = true
			t.Logf("round %d: episode done (reward=%.2f) - ore mined, seen or not", round, result.Reward)
			break
		}
	}

	require.True(t, found, "greedy walk driven purely by unknownDx/Dy/Dz should have exposed or destroyed the buried ore within %d rounds", unknownDirectionMaxRounds)
	t.Log("✓ unknownDx/Dy/Dz alone, with no other knowledge of the ore's location, led a greedy walk to it")
}
