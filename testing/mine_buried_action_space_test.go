package testing

import (
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

// MineBuriedActionSpaceFlatSuite is MineBuriedUnknownDirectionFlatSuite's
// successor: that suite proved the unknownDx/Dy/Dz signal itself points
// the right way, but drove movement by calling leader.Agent.MoveTo
// directly - a backdoor no real trained policy has. This suite drives the
// exact same scenario through nothing but the discrete action space
// (ActionGoToTarget/ActionMine/ActionWait) a policy actually chooses from,
// confirming the real fix this session made: before it, GoToTargetDisabled
// was unconditionally true for mine_buried with no TargetOffset of its
// own, so ActionWait was the *only* ever-legal action for the entire
// episode (confirmed live via rsi_action_mask_total, mc-rsi-trainer,
// 2026-10-26) - a trained policy had no way to move at all, regardless of
// what the observation told it.
type MineBuriedActionSpaceFlatSuite struct {
	VersionWorldSuite
}

func TestMineBuriedActionSpaceFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, []models.VersionTest{{Name: rlTrainTestVersion, MCVersion: rlTrainTestVersion}}, func() suite.TestingSuite {
		s := &MineBuriedActionSpaceFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// actionSpaceSearchRadius/actionSpaceMaxSteps mirror
// unknownDirectionSearchRadius/unknownDirectionMaxRounds's own role in
// MineBuriedUnknownDirectionFlatSuite, but actionSpaceMaxSteps bounds
// actual discrete Step calls here, not MoveTo legs - a real policy pays
// one Step per primitive action, not one per whole traversal, so this
// needs to be generous enough to cover the same real-world digging the
// other suite's 60-second MoveTo legs did, in many more, much smaller
// increments.
const (
	actionSpaceSearchRadius = 10
	actionSpaceMaxSteps     = 400
)

// TestGreedyPolicyFindsTheBuriedOreThroughRealActionsOnly buries an ore
// inside the search volume without ever telling the controller where, then
// repeatedly dispatches whichever discrete action is legal and relevant
// (ActionMine if ready, else ActionGoToTarget, else ActionWait) - exactly
// the choice rule a minimally-competent trained policy would have learned,
// with none of the backdoors the other two mine_buried suites used.
func (s *MineBuriedActionSpaceFlatSuite) TestGreedyPolicyFindsTheBuriedOreThroughRealActionsOnly() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ActionSpaceBot", "mine_buried_action_space")
	require.NoError(t, err, "spawn agent")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	minedCounter, ok := leader.Agent.(rlenv.BlocksMinedCounter)
	require.True(t, ok, "spawned test agent must satisfy rlenv.BlocksMinedCounter")
	startBlocksMined := minedCounter.BlocksMinedCount()

	startX := int(math.Floor(leader.Origin.X))
	startY := int(math.Floor(leader.Origin.Y))
	startZ := int(math.Floor(leader.Origin.Z))

	half := actionSpaceSearchRadius / 2
	oreX := startX + half
	oreY := startY - half
	oreZ := startZ - half
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:coal_ore", oreX, oreY, oreZ))
	require.NoError(t, err, "place buried ore")

	rlEnv, err := rlenv.New(liveAgent, actions.NewRegistry(), rlenv.Config{
		ArrivalThreshold:             1.5,
		StepTimeout:                  15 * time.Second,
		MineTargetBlock:              "minecraft:coal_ore",
		MineSearchRadius:             actionSpaceSearchRadius,
		MineRequiredTool:             "minecraft:netherite_pickaxe",
		MineRequiredToolEnchantments: map[string]int{"minecraft:efficiency": 5},
		MineTargetBuried:             true,
		Seeder:                       rlenv.DefaultEpisodeSeeder,
	})
	require.NoError(t, err, "construct rlenv.Environment")

	obs, err := rlEnv.Reset(s.Ctx)
	require.NoError(t, err, "Reset (includes RCON tool seeding)")
	require.Equal(t, float32(0), obs.Values[12], "mineVisible should be 0 right after Reset - the ore is buried")
	require.True(t, rlEnv.ActionMask()[rlenv.ActionGoToTarget], "ActionGoToTarget should be legal from the first observation - this is the whole fix")

	found := false
	for step := 0; step < actionSpaceMaxSteps; step++ {
		mask := rlEnv.ActionMask()
		action := rlenv.ActionWait
		switch {
		case mask[rlenv.ActionMine]:
			action = rlenv.ActionMine
		case mask[rlenv.ActionGoToTarget]:
			action = rlenv.ActionGoToTarget
		}
		result, stepErr := rlEnv.Step(s.Ctx, action)
		require.NoError(t, stepErr, "Step %d (action=%d)", step, action)
		obs = result.Observation
		if step%20 == 0 {
			t.Logf("step %d: action=%d mineVisible=%v unknownValid=%v reward=%.2f", step, action, obs.Values[12], obs.Values[24], result.Reward)
		}
		if result.Done {
			found = true
			t.Logf("step %d: episode done (reward=%.2f) via action=%d", step, result.Reward, action)
			break
		}
	}

	require.True(t, found, "a greedy policy choosing only legal discrete actions should have found and mined the buried ore within %d steps", actionSpaceMaxSteps)
	t.Log("✓ discrete action space alone (no hand-driven MoveTo) found and mined the buried ore")

	endBlocksMined := minedCounter.BlocksMinedCount()
	require.Greater(t, endBlocksMined, startBlocksMined, "BlocksMinedCount should have increased - digging through overburden plus the final ore should confirm at least one real break")
	t.Logf("✓ BlocksMinedCount %d -> %d (%d confirmed breaks) - digProgressBonus had real signal to reward", startBlocksMined, endBlocksMined, endBlocksMined-startBlocksMined)
}
