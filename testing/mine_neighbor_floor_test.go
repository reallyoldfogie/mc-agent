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

// MineNeighborFloorFlatSuite is a version-parameterized suite for the
// MineNeighborFloor movement type (docs/plans/14-buried-block-mining-and-
// memory-map.md, mc-rsi-trainer - a live design discussion settled on this
// as a simpler, position-changing alternative to an earlier straddle-based
// attempt at the same "safe 2-wide shaft" idea): mine an adjacent column's
// floor - one or two levels - while standing safely in that column, having
// already walked there via an ordinary MineThrough step, rather than ever
// mining the exact block the bot is standing on with nothing else holding
// it up.
//
// Deliberately run against a single version (rlTrainTestVersion, shared
// with rl_train_test.go) - nothing under test here is
// protocol-version-specific, same reasoning that file's own doc comment
// gives for the same choice.
type MineNeighborFloorFlatSuite struct {
	VersionWorldSuite
}

func TestMineNeighborFloorFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, []models.VersionTest{{Name: rlTrainTestVersion, MCVersion: rlTrainTestVersion}}, func() suite.TestingSuite {
		s := &MineNeighborFloorFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// TestChoosesMineNeighborFloorOverMineDownSurcharge is a cost-ordering
// regression test, not just a reachability one. MineNeighborFloor's whole
// point is ending up BACK in the column the bot started in, one or two
// levels lower - it mines "the floor it was just standing on," from the
// safety of a neighbor it detoured into via an ordinary MineThrough step,
// never the neighbor's own floor. So the goal here is two levels straight
// down in the *starting* column, not the neighbor - reaching it via two
// plain MineDown calls would pay MineDown's own risk surcharge twice;
// detouring through the neighbor and mining both starting-column levels
// at once via MineNeighborFloor pays it zero times, at the cost of one
// cheap entry dig through a soft (dirt) wall. Soft wall deliberately
// chosen over stone: the margin by which the detour route must win is
// (one entry dig) vs (two risk surcharges) - dirt's lower hardness widens
// that margin well clear of rounding noise, rather than leaving it to
// chance on which exact block hardness was used.
//
// If MineNeighborFloor's cost model or preconditions ever regress such
// that the pathfinder stops preferring this route, this test catches that
// directly from the real path chosen, not just from unit-level
// GetPossibleMoves checks (see pathfinding/mine_movement_test.go for
// those, which exercise MineNeighborFloor's own preconditions in
// isolation rather than this ordering).
func (s *MineNeighborFloorFlatSuite) TestChoosesMineNeighborFloorOverMineDownSurcharge() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("MineNeighborFloorBot", "mine_neighbor_floor")
	require.NoError(t, err, "spawn agent")

	startX := int(math.Floor(leader.Origin.X))
	startY := int(math.Floor(leader.Origin.Y))
	startZ := int(math.Floor(leader.Origin.Z))

	// Seal every other direction with undiggable bedrock at the bot's own
	// height - found necessary live: with every side left at its natural,
	// already-open flat-world default, the pathfinder found an even
	// cheaper free Traverse through one of the *other* sides before
	// circling back to use MineNeighborFloor anyway, which still proved
	// the move works but never actually exercised digging through the
	// deliberately-placed wall below. Forcing every other direction
	// closed makes the east route (through that wall) the only one
	// available at all, not just the cheapest.
	for _, d := range [][2]int{{-1, 0}, {0, 1}, {0, -1}} {
		_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:bedrock", startX+d[0], startY, startZ+d[1]))
		require.NoError(t, err, "seal direction (%d,%d)", d[0], d[1])
	}

	// Starting column: two diggable (dirt) floor levels, then solid
	// ground - this is what MineNeighborFloor mines from the neighbor's
	// safety, and what two plain MineDowns would otherwise dig through
	// directly, paying the risk surcharge each time.
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:dirt", startX, startY-1, startZ))
	require.NoError(t, err, "place starting-column floor level 1")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:dirt", startX, startY-2, startZ))
	require.NoError(t, err, "place starting-column floor level 2")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", startX, startY-3, startZ))
	require.NoError(t, err, "place starting-column ground support")

	// Adjacent column (east): a one-cell-high dirt wall at the bot's own
	// height (feet level only - the cell above is left open, matching
	// ordinary flat-world sky) forces MineThrough rather than an
	// immediate ordinary Traverse; solid ground right at that same
	// height so the bot has somewhere real to stand once through it.
	eastX := startX + 1
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:dirt", eastX, startY, startZ))
	require.NoError(t, err, "place the wall")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", eastX, startY-1, startZ))
	require.NoError(t, err, "place solid ground to stand on just past the wall")

	// Give a pickaxe so every dig has a realistic, fast break time.
	giveCmd := fmt.Sprintf("item replace entity %s hotbar.1 with wooden_pickaxe 1", leader.Name)
	_, err = s.Inst.RCON.Exec(s.Ctx, giveCmd)
	require.NoError(t, err, "give wooden pickaxe")
	time.Sleep(500 * time.Millisecond)

	// Back in the starting column, two levels down - not the neighbor.
	target := models.V3{X: float64(startX), Y: float64(startY - 2), Z: float64(startZ)}
	t.Logf("digging from (%d, %d, %d) to (%.0f, %.0f, %.0f)", startX, startY, startZ, target.X, target.Y, target.Z)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	err = leader.Agent.MoveTo(ctx, target.X, target.Y, target.Z, false)
	require.NoError(t, err, "MoveTo should detour through the neighbor and dig both starting-column floor levels")

	require.NoError(t, waitForBotNear(leader.Agent, target, 1.5, 10*time.Second),
		"bot should have settled back in the starting column, two levels down, after MoveTo returned")

	isAir, err := waitForBlockAir(s.Ctx, s.Inst.RCON, startX, startY-1, startZ, 5*time.Second)
	require.NoError(t, err, "check starting-column floor level 1 via RCON")
	require.True(t, isAir, "starting-column floor level 1 should be mined")

	isAir, err = waitForBlockAir(s.Ctx, s.Inst.RCON, startX, startY-2, startZ, 5*time.Second)
	require.NoError(t, err, "check starting-column floor level 2 via RCON")
	require.True(t, isAir, "starting-column floor level 2 should be mined")

	t.Log("✓ bot detoured through the neighbor and dug both starting-column floor levels at once, landing back where it started")
}
