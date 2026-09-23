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

// CraftTableFlatSuite is a
// version-parameterized suite for TestTableRequiredWoodenPickaxe below: one server per version
// instead of the previous per-test-function setupStandaloneTest server-per-test pattern.
// WorldGen = WorldGenFlat + Difficulty = DifficultyEasy, matching setupStandaloneTest's own
// default exactly.
//
// This file was left on the per-test-server pattern for a while: its only container use is a
// real crafting table's 3x3 window, opened via CraftItem's table-crafting path, which was
// initially assumed to be constrained by a container window-ID limit per connection. That limit
// was re-measured and found stale (see container_window_id_probe_test.go's
// TestWindowIDLimitProbe, which opens/closes the same chest 25 times cleanly on one connection),
// and craft_pipeline_test.go already proved a real crafting-table 3x3 open works fine inside a
// shared-server suite; this file is the same category of open, just isolated to its own smaller
// test rather than chained after several other crafts.
type CraftTableFlatSuite struct {
	VersionWorldSuite
}

func TestCraftTableFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &CraftTableFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestTableRequiredWoodenPickaxe tests CraftItem end to end against a real
// recipe that does not fit the player's 2x2 inventory grid: "wooden_pickaxe"
// (3 planks across the top row, 2 sticks down the center column - a real
// 3x3-only shaped recipe, mirroring agent/craft_test.go's
// TestRecipeGrid_ShapedFitsIn3x3Only). Exercises the whole
// docs/plans/CRAFTING_TABLE_3X3_PLAN.md path: finding a visible crafting
// table, walking to it, opening its window, placing ingredients into its
// 3x3 grid, and collecting the result - none of which
// CraftItemFlatSuite.TestStickFromPlanks/TestShapelessChestBoat (both
// window-0-only) exercise. Equivalent to the original
// TestCraftItem_TableRequired_WoodenPickaxe.
func (s *CraftTableFlatSuite) TestTableRequiredWoodenPickaxe() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("CraftTableBot", "craft_table_required")
	require.NoError(t, err, "spawn agent")

	tablePos := models.V3{X: math.Floor(leader.Origin.X) + 5, Y: math.Floor(leader.Origin.Y), Z: math.Floor(leader.Origin.Z)}
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, tablePos, "minecraft:crafting_table", "crafting_table", 30*time.Second)
	require.NoError(t, err, "place crafting table")

	// 15 minutes, not 60s: CraftItem's table path calls MoveToWithChat,
	// which pathfinds via HPA* - this codebase's own navigation live tests
	// budget 15-20 minutes for a single MoveTo, since cold-start HPA*
	// graph-building on a freshly-connected agent is genuinely slow, even
	// for a target only a few blocks away. Found live: a 60s budget let
	// FindVisibleBlock's fix work (found the table almost instantly) but
	// then starved MoveToWithChat's own pathfinding of the time it
	// actually needs.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:oak_planks 3", leader.Name))
	require.NoError(t, err, "give oak planks")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:stick 2", leader.Name))
	require.NoError(t, err, "give sticks")

	hasPlanks, err := waitForAgentHasItem(ctx, leader.Agent, "minecraft:oak_planks", 5*time.Second)
	require.NoError(t, err, "wait for planks to sync to agent")
	require.True(t, hasPlanks, "agent should see the given oak planks")
	hasSticks, err := waitForAgentHasItem(ctx, leader.Agent, "minecraft:stick", 5*time.Second)
	require.NoError(t, err, "wait for sticks to sync to agent")
	require.True(t, hasSticks, "agent should see the given sticks")

	err = leader.Agent.CraftItem(ctx, "minecraft:wooden_pickaxe")
	require.NoError(t, err, "craft wooden pickaxe")

	hasPickaxe, err := waitForInventoryItem(s.Ctx, s.Inst.RCON, leader.Name, "minecraft:wooden_pickaxe", 5*time.Second)
	require.NoError(t, err, "check inventory for crafted wooden pickaxe")
	require.True(t, hasPickaxe, "should have crafted a wooden pickaxe")

	t.Log("✓ Craft wooden pickaxe from crafting table test passed")
}
