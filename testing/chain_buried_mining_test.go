package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ChainBuriedMiningFlatSuite is a version-parameterized suite for
// chain_buried_mining (docs/plans/14-buried-block-mining-and-memory-map.md,
// mc-rsi-trainer, "Part 4"): the full multi-step crafting chain - gather
// logs, craft planks and a table, place it, craft a wooden_pickaxe from
// scratch (no tool is ever given, unlike mine_buried) - then dig to and
// mine a genuinely buried block, which is what actually ends the episode.
// rlenv/chain_env_test.go's TestChainMineEpisodeRunsTheWholeChain already
// proves this logic exhaustively against a fake agent; this is that same
// sequence driven against a real live server for the first time - every
// earlier chain stage (ChainGather/ChainTable/ChainPlace/ChainUse) has
// never had live coverage either, so this doubles as their first live
// exercise too, not just ChainMine's own buried-block tail.
//
// Deliberately run against a single version (rlTrainTestVersion, shared
// with rl_train_test.go) - nothing under test here is
// protocol-version-specific, same reasoning that file's own doc comment
// gives for the same choice.
type ChainBuriedMiningFlatSuite struct {
	VersionWorldSuite
}

func TestChainBuriedMiningFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, []models.VersionTest{{Name: rlTrainTestVersion, MCVersion: rlTrainTestVersion}}, func() suite.TestingSuite {
		s := &ChainBuriedMiningFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// chainStepRetryAttempts is generous on purpose: unlike the fake agent in
// rlenv/chain_env_test.go, a real dispatch's completion can lag a step or
// two behind the actual server-side change (see stepUntilDone's own doc
// comment in rl_train_test.go for the same reasoning, confirmed live more
// than once this session).
const chainStepRetryAttempts = 15

// driveActionUntilCount steps action against the live environment until
// itemName's held count reaches want, retrying up to chainStepRetryAttempts
// times - the chain-episode analogue of stepUntilDone, for a milestone
// that isn't "episode done" but "inventory count reached," which every
// intermediate chain stage needs and ActionMine's eventual completion at
// the buried block (handled separately, via stepUntilDone itself) doesn't.
func driveActionUntilCount(t *testing.T, env *rlenv.Environment, ctx context.Context, agent rlenv.LiveAgent, action rl.Action, itemName string, want int) {
	t.Helper()
	for attempt := 1; attempt <= chainStepRetryAttempts; attempt++ {
		r, err := env.Step(ctx, action)
		require.NoError(t, err, "Step(%v) attempt %d", action, attempt)
		require.False(t, r.Done, "episode ended unexpectedly mid-chain (attempt %d, reward=%.2f)", attempt, r.Reward)
		if agent.InventoryCount(itemName) >= want {
			return
		}
		t.Logf("attempt %d: %s count = %d, want %d - retrying", attempt, itemName, agent.InventoryCount(itemName), want)
	}
	t.Fatalf("%s count never reached %d after %d attempts (have %d)", itemName, want, chainStepRetryAttempts, agent.InventoryCount(itemName))
}

// driveActionUntilIllegal is driveActionUntilCount's counterpart for a
// milestone with no inventory count to check - placing the table removes
// it from the inventory rather than adding anything countable, so its own
// completion signal is ActionPlace itself becoming illegal afterward
// (mirrors chain_env_test.go's own "place still legal after placing"
// check, via the same public env.ActionMask() this live test can call
// directly too).
func driveActionUntilIllegal(t *testing.T, env *rlenv.Environment, ctx context.Context, action rl.Action) {
	t.Helper()
	for attempt := 1; attempt <= chainStepRetryAttempts; attempt++ {
		r, err := env.Step(ctx, action)
		require.NoError(t, err, "Step(%v) attempt %d", action, attempt)
		require.False(t, r.Done, "episode ended unexpectedly mid-chain (attempt %d, reward=%.2f)", attempt, r.Reward)
		if !env.ActionMask()[action] {
			return
		}
		t.Logf("attempt %d: action %v still legal - retrying", attempt, action)
	}
	t.Fatalf("action %v still legal after %d attempts", action, chainStepRetryAttempts)
}

// TestRunsTheWholeChainToABuriedBlock is the live port of
// rlenv/chain_env_test.go's TestChainMineEpisodeRunsTheWholeChain: gather
// logs from a real seeded tree, craft and place a table, craft a
// wooden_pickaxe from scratch, then dig to and mine a block buried under
// real overburden (placed directly via RCON - not through
// Config.Seeder, same reasoning as mine_buried_ore_action_test.go's
// Config.MineTargetBuried: nothing seeds the buried block per-episode in
// production either, see that block's own ChainMineTargetBlock doc
// comment - it comes from the shared clone-template's underground stone
// band, which this self-contained test stands in for with one direct
// RCON placement).
//
// Unlike the fake-agent version, the mine target switching from the log to
// the buried block does NOT make ActionMine immediately legal: the fake
// agent's FindVisibleBlock has no real occlusion model, but this block is
// genuinely buried under real overburden, so it isn't visible (or legal
// to mine) until something actually digs down to it - exactly
// mine_buried_ore_action_test.go's own MoveTo-then-ActionMine handoff,
// reused here for this episode's tail instead of a bare mine_buried one.
func (s *ChainBuriedMiningFlatSuite) TestRunsTheWholeChainToABuriedBlock() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ChainBuriedBot", "chain_buried_mining")
	require.NoError(t, err, "spawn agent")

	liveAgent, ok := leader.Agent.(rlenv.LiveAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.LiveAgent")
	_, ok = leader.Agent.(rlenv.SeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.SeedAgent")
	_, ok = leader.Agent.(rlenv.ChainSeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.ChainSeedAgent")
	_, ok = leader.Agent.(rlenv.FarSeedAgent)
	require.True(t, ok, "spawned test agent must satisfy rlenv.FarSeedAgent")

	startX := int(math.Floor(leader.Origin.X))
	startY := int(math.Floor(leader.Origin.Y))
	startZ := int(math.Floor(leader.Origin.Z))

	// The buried block: placed once, directly, well clear of where the
	// tree/table work happens (TargetOffset below puts the tree around
	// startX+13) - two diggable overburden levels, then solid ground.
	oreX, oreZ := startX+19, startZ
	approachY := startY - 2
	oreY := approachY - 1
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:dirt", oreX, startY-1, oreZ))
	require.NoError(t, err, "place overburden level 1")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:dirt", oreX, approachY, oreZ))
	require.NoError(t, err, "place overburden level 2")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:coal_ore", oreX, oreY, oreZ))
	require.NoError(t, err, "place the buried ore")

	cfg := rlenv.Config{
		TargetOffset:         [3]float64{12, 0, 0},
		ArrivalThreshold:     1.5,
		StepTimeout:          15 * time.Second,
		MineTargetBlock:      "minecraft:oak_log",
		MineSearchRadius:     8,
		CraftTargetItem:      "minecraft:wooden_pickaxe",
		ChainStage:           rlenv.ChainMine,
		ChainMineTargetBlock: "minecraft:coal_ore",
		CollectDrops:         true,
		SeedAtGoal:           true,
		Seeder:               rlenv.DefaultEpisodeSeeder,
	}
	env, err := rlenv.New(liveAgent, actions.NewRegistry(), cfg)
	require.NoError(t, err, "construct rlenv.Environment")

	_, err = env.Reset(s.Ctx)
	require.NoError(t, err, "Reset (seeds the tree, clears inventory/drops)")

	// Gather: a real tree of 4 logs (3 needed for a pickaxe's planks +
	// sticks, +1 spare - see ChainTreeHeight) was just seeded by Reset
	// itself, out of sight of the start.
	for attempt := 1; attempt <= 3; attempt++ {
		r, stepErr := env.Step(s.Ctx, rlenv.ActionGoToTarget)
		require.NoError(t, stepErr, "Step(ActionGoToTarget) attempt %d", attempt)
		require.False(t, r.Done, "arriving at the tree ended the episode")
		if env.ActionMask()[rlenv.ActionMine] {
			break
		}
		t.Logf("attempt %d: not yet arrived at the tree (ActionMine not legal) - retrying", attempt)
	}
	require.True(t, env.ActionMask()[rlenv.ActionMine], "ActionMine should be legal once arrived at the seeded tree")

	for i := 1; i <= 4; i++ {
		driveActionUntilCount(t, env, s.Ctx, liveAgent, rlenv.ActionMine, "minecraft:oak_log", i)
	}
	t.Logf("gathered %d oak_log", liveAgent.InventoryCount("minecraft:oak_log"))

	// Table: planks, then the table itself.
	driveActionUntilCount(t, env, s.Ctx, liveAgent, rlenv.ActionCraft, "minecraft:oak_planks", 4)
	driveActionUntilCount(t, env, s.Ctx, liveAgent, rlenv.ActionCraft, "minecraft:crafting_table", 1)

	r, err := env.Step(s.Ctx, rlenv.ActionPlace)
	require.NoError(t, err, "Step(ActionPlace)")
	require.False(t, r.Done, "placing the table ended a ChainMine episode")
	if env.ActionMask()[rlenv.ActionPlace] {
		driveActionUntilIllegal(t, env, s.Ctx, rlenv.ActionPlace)
	}

	// Pickaxe: planks for sticks, the sticks, more planks for the pickaxe
	// itself, then the pickaxe - the resolver's own choices (nextChainCraft),
	// same as the fake-agent version.
	driveActionUntilCount(t, env, s.Ctx, liveAgent, rlenv.ActionCraft, "minecraft:wooden_pickaxe", 1)
	t.Log("crafted a wooden_pickaxe from scratch")

	// The mine target has switched from the (now-irrelevant) log to the
	// buried block, but it genuinely isn't visible yet - dig to it first,
	// exactly like mine_buried_ore_action_test.go's own handoff.
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	err = leader.Agent.MoveTo(ctx, float64(oreX), float64(approachY), float64(oreZ), false)
	require.NoError(t, err, "MoveTo should dig through the overburden above the buried ore")

	require.NoError(t, waitForBotNear(leader.Agent, models.V3{X: float64(oreX), Y: float64(approachY), Z: float64(oreZ)}, 1.5, 10*time.Second),
		"bot should have settled just above the buried ore after MoveTo returned")

	result := stepUntilDone(t, env, s.Ctx, rlenv.ActionMine, rlTrainStepRetryAttempts)
	require.True(t, result.Done, "chain_buried_mining episode should end once the uncovered buried block is mined (within %d attempts)", rlTrainStepRetryAttempts)
	require.Greater(t, result.Reward, float32(9), "reward should include the chain's completion bonus")

	isAir, err := waitForBlockAir(s.Ctx, s.Inst.RCON, oreX, oreY, oreZ, 5*time.Second)
	require.NoError(t, err, "check ore block state via RCON")
	require.True(t, isAir, "ore block should be gone after the chain episode completed")

	t.Log("✓ chain_buried_mining ran end to end against a live server: gather → craft planks/table → place → craft pickaxe → dig to and mine the buried block")
}
