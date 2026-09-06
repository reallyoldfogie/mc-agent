package testing

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// craftSettleDelay is a short pause between back-to-back CraftItem calls in
// TestCraftItem_FullPipeline_LogsToWoodenPickaxe. Unlike every other
// CraftItem live test (agent/craft_test.go, testing/craft_test.go,
// testing/craft_table_test.go - one craft per test), this one chains
// several calls in a row (planks, planks, planks, sticks, crafting table),
// so the next call's ingredient search needs the previous craft's
// shift-click to have actually settled into the client's tracked inventory
// first, not just returned without error.
const craftSettleDelay = 300 * time.Millisecond

// waitForVisibleCraftingTable polls FindVisibleBlock for a
// "minecraft:crafting_table" within maxDistance blocks until found or
// timeout elapses - mirrors waitForVisibleItem's poll pattern
// (testing/lookaround_test.go). Needed here because the client needs a
// moment to receive the block-change packet after the agent's own
// UseItemOnBlock places the table - unlike every other live test's
// crafting table, which is placed via RCON/PlaceBlockAndWait before the
// agent even connects.
func waitForVisibleCraftingTable(ctx context.Context, agent models.Agent, maxDistance int, timeout time.Duration) (x, y, z float64, found bool, err error) {
	deadline := time.Now().Add(timeout)
	for {
		x, y, z, found, err = agent.FindVisibleBlock(ctx, "minecraft:crafting_table", maxDistance)
		if err != nil || found {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// TestCraftItem_FullPipeline_LogsToWoodenPickaxe exercises the entire
// gather-craft-place-craft pipeline a real player would go through, driven
// entirely by the agent's own actions (CraftItem/Equip/UseItemOnBlock) - no
// RCON shortcuts for any of the crafting or the block placement: given raw
// oak logs, the agent turns them into planks (2x2 grid), planks into
// sticks (2x2 grid), planks into a crafting table (2x2 grid), places that
// table in the world itself, then crafts a wooden pickaxe using the placed
// table's 3x3 grid - the scenario
// docs/plans/CRAFTING_TABLE_3X3_PLAN.md was written for.
//
// Unlike TestCraftItem_TableRequired_WoodenPickaxe (testing/craft_table_test.go),
// which pre-places the table via RCON/setupStandaloneTest to isolate the
// table-crafting path in a smaller test, this test's table only exists
// because the agent placed it - the first live test of agent-driven block
// placement in this codebase (UseItemOnBlock's only other real caller is
// agent/clutch.go's real-time obstacle placement while falling, which has
// no live test of its own to compare against, so this is also the first
// live confirmation that UseItemOnBlock actually places a block server-side
// without agent/clutch.go's split-second time pressure).
func TestCraftItem_FullPipeline_LogsToWoodenPickaxe(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "craft_pipeline", tt.MCVersion)
			defer env.Cancel()

			// 15 minutes, not 90s - see TestCraftItem_TableRequired_WoodenPickaxe's
			// matching comment: the wooden_pickaxe craft at the end of this
			// pipeline also goes through CraftItem's table path, which
			// pathfinds via HPA* and needs the same generous budget this
			// codebase's own navigation live tests use.
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			// 3 logs -> 12 planks (4 per craft): 4 consumed by the table, 2
			// by one stick craft (yields 4 sticks, only 2 needed for the
			// pickaxe), 3 by the pickaxe's own top row - 9 consumed total,
			// 3 planks and 2 sticks left over.
			_, err := env.Inst.RCON.Exec(env.Ctx, fmt.Sprintf("give %s minecraft:oak_log 3", env.BotName))
			require.NoError(t, err, "give oak logs")
			hasLogs, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:oak_log", 5*time.Second)
			require.NoError(t, err, "wait for logs to sync to agent")
			require.True(t, hasLogs, "agent should see the given oak logs")

			for i := 0; i < 3; i++ {
				require.NoErrorf(t, env.Agent.Agent.CraftItem(ctx, "minecraft:oak_planks"), "craft oak planks (%d/3)", i+1)
				time.Sleep(craftSettleDelay)
			}
			hasPlanks, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:oak_planks", 5*time.Second)
			require.NoError(t, err, "wait for planks to sync to agent")
			require.True(t, hasPlanks, "agent should have crafted oak planks")

			require.NoError(t, env.Agent.Agent.CraftItem(ctx, "minecraft:stick"), "craft sticks from planks")
			time.Sleep(craftSettleDelay)
			hasSticks, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:stick", 5*time.Second)
			require.NoError(t, err, "wait for sticks to sync to agent")
			require.True(t, hasSticks, "agent should have crafted sticks")

			require.NoError(t, env.Agent.Agent.CraftItem(ctx, "minecraft:crafting_table"), "craft a crafting table from planks")
			time.Sleep(craftSettleDelay)
			hasTable, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:crafting_table", 5*time.Second)
			require.NoError(t, err, "wait for crafting table to sync to agent")
			require.True(t, hasTable, "agent should have crafted a crafting table")

			// Place the table: equip it, then right-click the top face of
			// a ground block a couple of blocks away from where the agent
			// is standing - not directly underfoot, which the server
			// rejects as colliding with the agent's own hitbox.
			require.NoError(t, env.Agent.Agent.Equip(ctx, "minecraft:crafting_table"), "equip crafting table")

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")
			groundX := math.Floor(botPos.X) + 2
			groundY := math.Floor(botPos.Y) - 1
			groundZ := math.Floor(botPos.Z)

			require.NoError(t, env.Agent.Agent.UseItemOnBlock(ctx, groundX, groundY, groundZ, models.FaceUp, models.MainHand), "place crafting table")

			_, _, _, placed, err := waitForVisibleCraftingTable(ctx, env.Agent.Agent, 8, 5*time.Second)
			require.NoError(t, err, "look for the placed crafting table")
			require.True(t, placed, "agent should see the crafting table it just placed")

			// wooden_pickaxe (3 planks across the top row, 2 sticks down
			// the center column) doesn't fit the 2x2 grid, so this
			// exercises CraftItem's table path end to end: it has to
			// re-find the table the agent just placed, walk to it, open
			// it, and craft against its 3x3 grid.
			require.NoError(t, env.Agent.Agent.CraftItem(ctx, "minecraft:wooden_pickaxe"), "craft wooden pickaxe using the placed table")

			hasPickaxe, err := waitForInventoryItem(env, "minecraft:wooden_pickaxe", 5*time.Second)
			require.NoError(t, err, "check inventory for crafted wooden pickaxe")
			require.True(t, hasPickaxe, "should have crafted a wooden pickaxe")

			t.Log("✓ Full logs-to-pickaxe crafting pipeline test passed")
		})
	}
}
