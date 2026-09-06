package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestCraftItem_TableRequired_WoodenPickaxe tests CraftItem end to end
// against a real recipe that does not fit the player's 2x2 inventory grid:
// "wooden_pickaxe" (3 planks across the top row, 2 sticks down the center
// column - a real 3x3-only shaped recipe, mirroring
// agent/craft_test.go's TestRecipeGrid_ShapedFitsIn3x3Only). Exercises the
// whole docs/plans/CRAFTING_TABLE_3X3_PLAN.md path: finding a visible
// crafting table, walking to it, opening its window, placing ingredients
// into its 3x3 grid, and collecting the result - none of which
// TestCraftItem_StickFromPlanks/TestCraftItem_ShapelessChestBoat
// (both window-0-only) exercise.
//
// Uses setupStandaloneTest (not setupStandaloneTestForEntity) because a
// real crafting_table block needs to exist in the world this time - see
// getContainerBlockType's "crafting_table" mapping
// (testing/container_standalone_test.go).
func TestCraftItem_TableRequired_WoodenPickaxe(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTest(t, "crafting_table", tt.MCVersion)
			defer env.Cancel()

			// 15 minutes, not 60s: CraftItem's table path calls MoveToWithChat,
			// which pathfinds via HPA* - this codebase's own navigation
			// live tests (testing/navigation_pathfinding_test.go) budget
			// 15-20 minutes for a single MoveTo, since cold-start
			// HPA* graph-building on a freshly-connected agent is genuinely
			// slow, even for a target only a few blocks away. Found live:
			// a 60s budget let FindVisibleBlock's fix work (found the table
			// almost instantly) but then starved MoveToWithChat's own
			// pathfinding of the time it actually needs.
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			_, err := env.Inst.RCON.Exec(env.Ctx, fmt.Sprintf("give %s minecraft:oak_planks 3", env.BotName))
			require.NoError(t, err, "give oak planks")
			_, err = env.Inst.RCON.Exec(env.Ctx, fmt.Sprintf("give %s minecraft:stick 2", env.BotName))
			require.NoError(t, err, "give sticks")

			hasPlanks, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:oak_planks", 5*time.Second)
			require.NoError(t, err, "wait for planks to sync to agent")
			require.True(t, hasPlanks, "agent should see the given oak planks")
			hasSticks, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:stick", 5*time.Second)
			require.NoError(t, err, "wait for sticks to sync to agent")
			require.True(t, hasSticks, "agent should see the given sticks")

			err = env.Agent.Agent.CraftItem(ctx, "minecraft:wooden_pickaxe")
			require.NoError(t, err, "craft wooden pickaxe")

			hasPickaxe, err := waitForInventoryItem(env, "minecraft:wooden_pickaxe", 5*time.Second)
			require.NoError(t, err, "check inventory for crafted wooden pickaxe")
			require.True(t, hasPickaxe, "should have crafted a wooden pickaxe")

			t.Log("✓ Craft wooden pickaxe from crafting table test passed")
		})
	}
}
