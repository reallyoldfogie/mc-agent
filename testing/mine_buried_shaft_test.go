package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// MineBuriedShaftFlatSuite is a version-parameterized suite for the
// narrow-shaft-navigability fix (docs/plans/14-buried-block-mining-and-
// memory-map.md in mc-rsi-trainer): a bot commanded to MoveTo a point
// several blocks underground, with no existing tunnel, should offer
// MineDown/MineDownDiagonal moves and actually dig its way there - not
// reject the goal outright, not walk blind into solid rock, and not get
// permanently stuck once some of the overburden is already cleared.
//
// WorldGenFlat is deliberate, not just convenient: the standard flat-world
// preset (bedrock, 59 stone, 3 dirt, 1 grass - see buildServerConfig) is
// already solid, undug ground in every direction once you go underground,
// so no custom terrain needs to be built via RCON fill the way
// navigation_pathfinding_vertical_test.go's staircase does for its own
// (above-ground) scenario - the natural flat-world terrain itself reproduces
// the buried-shaft shape this fix targets.
type MineBuriedShaftFlatSuite struct {
	VersionWorldSuite
}

func TestMineBuriedShaftFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &MineBuriedShaftFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// shaftDepth is both the horizontal and vertical span of the diagonal shaft
// this test digs. Equal depth/span means every single move down to the
// target can be MineDownDiagonal (A*'s own cost model already prefers it
// over plain MineDown - see
// TestGetPossibleMoves_OffersMineDownDiagonalCheaperThanMineDown in
// pathfinding/mine_movement_test.go), with no slack needing a separate
// Traverse or plain MineDown to cover.
//
// 5 is deep enough that several of those diagonal steps happen while the
// bot is already standing inside its own freshly-dug tunnel, below the
// original ground surface - the specific condition
// tryMineDownDiagonal's missing third mined cell (the headroom cell one
// above the bot's current standing height in the new column,
// `from.Y+1` - see that function's own doc comment) needed to ever
// manifest. The first diagonal step never exercises it (open sky above
// ground level regardless of the bug), but by the third step the cell in
// question is ordinary undug dirt/grass, same as it was live.
const shaftDepth = 5

// TestDigsDiagonalShaftThroughBuriedTerrain is the integration-test port of
// this session's scratchpad smoke-test driver (used to find and confirm the
// fix for mc-agent commit 1a6871f, "Fix MineDownDiagonal's missing headroom
// cell and the Descend-family edge-walk stall"): command the real MoveTo/
// pathfinder at a point buried in solid, never-before-dug ground, and
// confirm the bot actually arrives, not just that mining happened somewhere
// along the way.
//
// Before that fix, this exact scenario (a multi-step diagonal descent into
// solid flat-world terrain) hung forever: the bot would mine real blocks
// but never physically move into the opened space, with MoveTo's own
// stuck-recovery logic re-pathfinding to the identical spot every
// ~stuckThreshold indefinitely. This test's bounded context timeout is the
// regression guard for that - a reintroduced version of the bug fails this
// test via timeout, not a silent hang.
func (s *MineBuriedShaftFlatSuite) TestDigsDiagonalShaftThroughBuriedTerrain() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("BuriedShaftBot", "mine_buried_shaft")
	require.NoError(t, err, "spawn agent")

	// Give the bot a wooden pickaxe (anywhere in inventory - both
	// MineBlockAt's own direct-call auto-tool-selection, exercised in
	// mine_test.go, and the physics executor's SetMineCallback path this
	// test actually drives both go through the same selectBestToolForBlock
	// logic) so every dig along the shaft has a realistic, fast break time
	// instead of the bare-handed fallback's slower one.
	giveCmd := fmt.Sprintf("item replace entity %s hotbar.1 with wooden_pickaxe 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give wooden pickaxe")
	time.Sleep(500 * time.Millisecond)

	startX := int(math.Floor(leader.Origin.X))
	startY := int(math.Floor(leader.Origin.Y))
	startZ := int(math.Floor(leader.Origin.Z))

	target := models.V3{
		X: float64(startX + shaftDepth),
		Y: float64(startY - shaftDepth),
		Z: float64(startZ),
	}

	t.Logf("digging from (%d, %d, %d) to buried target (%.0f, %.0f, %.0f)",
		startX, startY, startZ, target.X, target.Y, target.Z)

	// Generous but bounded: ~shaftDepth landing cells plus up to two extra
	// mined cells (headroom/headroomAbove) per diagonal step, each up to a
	// few real seconds to break even with a pickaxe, plus pathfinding and
	// physics-tick overhead. A reintroduced stuck-recovery loop fails this
	// via timeout well before it would exhaust the suite's own 45-minute
	// SetupSuite context.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	err = leader.Agent.MoveTo(ctx, target.X, target.Y, target.Z, false)
	require.NoError(t, err, "MoveTo should dig through the buried shaft and succeed")

	require.NoError(t, waitForBotNear(leader.Agent, target, 1.5, 10*time.Second),
		"bot should have settled near the buried target after MoveTo returned")

	// The bot's two-block-tall hitbox must have both its feet-level and
	// head-level cells clear to be legitimately standing at the target -
	// this holds regardless of the exact path shape A* chose to get there,
	// unlike asserting specific intermediate cells along one assumed route.
	// This is the structural invariant the live bug violated: real blocks
	// got mined, but the bot's own standing column was never fully clear.
	targetX, targetY, targetZ := int(target.X), int(target.Y), int(target.Z)
	isAir, err := waitForBlockAir(s.Ctx, s.Inst.RCON, targetX, targetY, targetZ, 5*time.Second)
	require.NoError(t, err, "check target feet-level block via RCON")
	require.True(t, isAir, "target feet-level cell should be air after arrival")

	isAir, err = waitForBlockAir(s.Ctx, s.Inst.RCON, targetX, targetY+1, targetZ, 5*time.Second)
	require.NoError(t, err, "check target head-level block via RCON")
	require.True(t, isAir, "target head-level cell should be air after arrival")
}
