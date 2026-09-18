package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// waitForAgentHasItem polls the agent's own client-tracked inventory (via
// FindSlotWith) until itemName appears anywhere in it, or timeout elapses.
// An RCON `give` needs a moment to reach the client as a
// ContainerSetSlot/SetContainerContent packet — CraftItem reads only the
// client's own tracked inventory state (not RCON), so it's this, not
// GetInventoryItems (server-truth via RCON), that needs to be true before
// calling CraftItem.
func waitForAgentHasItem(ctx context.Context, agent models.Agent, itemName string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		_, found, err := agent.FindSlotWith(ctx, itemName, -2)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// TestCraftItem_StickFromPlanks tests CraftItem end to end against a real
// shaped, tag-gated recipe: "stick" is a 1-wide, 2-tall pattern within the
// 2x2 grid (window-0 slots 1 and 3 only — see agent/craft.go's window-0
// layout comment), gated by the #minecraft:planks tag rather than a
// concrete item id. Exercises both the narrower-than-grid placement path
// and tag-to-inventory-item resolution, the two things
// TestLoadCraftingRecipes_AgainstRealCache (agent/craft_test.go) already
// verifies for parsing but can't verify for the actual inventory-click
// sequence, which needs a live server.
func TestCraftItem_StickFromPlanks(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "craft_stick", tt.MCVersion)
			defer env.Cancel()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err := env.Inst.RCON.Exec(env.Ctx, fmt.Sprintf("give %s minecraft:oak_planks 2", env.BotName))
			require.NoError(t, err, "give oak planks")

			hasPlanks, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:oak_planks", 5*time.Second)
			require.NoError(t, err, "wait for planks to sync to agent")
			require.True(t, hasPlanks, "agent should see the given oak planks")

			err = env.Agent.Agent.CraftItem(ctx, "minecraft:stick")
			require.NoError(t, err, "craft stick")

			hasStick, err := waitForInventoryItem(env.Ctx, env.Inst.RCON, env.BotName, "minecraft:stick", 5*time.Second)
			require.NoError(t, err, "check inventory for crafted stick")
			require.True(t, hasStick, "should have crafted a stick")

			t.Log("✓ Craft stick from planks test passed")
		})
	}
}

// TestCraftItem_ShapelessChestBoat tests CraftItem against a shapeless
// recipe with concrete (non-tag) ingredients — acacia_chest_boat (chest +
// acacia_boat), confirmed present with this exact ingredient list across
// every version in models.StandardVersionTests. Covers the shapeless path
// (order-independent grid placement) that TestCraftItem_StickFromPlanks's
// shaped recipe doesn't exercise.
func TestCraftItem_ShapelessChestBoat(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestForEntity(t, "craft_chest_boat", tt.MCVersion)
			defer env.Cancel()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			_, err := env.Inst.RCON.Exec(env.Ctx, fmt.Sprintf("give %s minecraft:chest 1", env.BotName))
			require.NoError(t, err, "give chest")
			_, err = env.Inst.RCON.Exec(env.Ctx, fmt.Sprintf("give %s minecraft:acacia_boat 1", env.BotName))
			require.NoError(t, err, "give acacia boat")

			hasChest, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:chest", 5*time.Second)
			require.NoError(t, err, "wait for chest to sync to agent")
			require.True(t, hasChest, "agent should see the given chest")
			hasBoat, err := waitForAgentHasItem(ctx, env.Agent.Agent, "minecraft:acacia_boat", 5*time.Second)
			require.NoError(t, err, "wait for boat to sync to agent")
			require.True(t, hasBoat, "agent should see the given acacia boat")

			err = env.Agent.Agent.CraftItem(ctx, "minecraft:acacia_chest_boat")
			require.NoError(t, err, "craft acacia chest boat")

			hasChestBoat, err := waitForInventoryItem(env.Ctx, env.Inst.RCON, env.BotName, "minecraft:acacia_chest_boat", 5*time.Second)
			require.NoError(t, err, "check inventory for crafted chest boat")
			require.True(t, hasChestBoat, "should have crafted an acacia chest boat")

			t.Log("✓ Craft shapeless chest boat test passed")
		})
	}
}
