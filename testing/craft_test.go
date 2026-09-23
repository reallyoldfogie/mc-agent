package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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

// CraftItemFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the CraftItem tests below: one server per version, shared by
// both methods, instead of the previous per-test-function setupStandaloneTestForEntity
// server-per-test pattern. WorldGen = WorldGenFlat + Difficulty = DifficultyEasy, matching
// setupStandaloneTestForEntity's own default exactly.
//
// This file was listed in the checklist as "genuinely window-ID-constrained (`minecraft:chest`)"
// on the strength of a "chest" substring match - but that match was
// `minecraft:acacia_chest_boat`/`minecraft:chest` as *crafting ingredients*, not an opened
// container: both recipes here (a shaped stick, a shapeless chest boat) fit the player's own 2x2
// crafting grid, so CraftItem never opens a real container window at all (no
// OpenContainerWithLOS/ScreenMgr call appears anywhere in this file). The same
// substring-vs-actual-usage gap `29-phase1-cam-follow-spectator-noclip-conversion.md` already
// found and corrected for `elytra_unequip_test.go`'s `ScreenMgr.` hit. Confirmed container-free,
// not just presumed, before converting.
type CraftItemFlatSuite struct {
	VersionWorldSuite
}

func TestCraftItemFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &CraftItemFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestStickFromPlanks tests CraftItem end to end against a real shaped,
// tag-gated recipe: "stick" is a 1-wide, 2-tall pattern within the 2x2 grid
// (window-0 slots 1 and 3 only — see agent/craft.go's window-0 layout
// comment), gated by the #minecraft:planks tag rather than a concrete item
// id. Exercises both the narrower-than-grid placement path and
// tag-to-inventory-item resolution, the two things
// TestLoadCraftingRecipes_AgainstRealCache (agent/craft_test.go) already
// verifies for parsing but can't verify for the actual inventory-click
// sequence, which needs a live server. Equivalent to the pre-Phase-1
// TestCraftItem_StickFromPlanks.
func (s *CraftItemFlatSuite) TestStickFromPlanks() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("CraftStickBot", "craft_stick")
	require.NoError(t, err, "spawn agent")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:oak_planks 2", leader.Name))
	require.NoError(t, err, "give oak planks")

	hasPlanks, err := waitForAgentHasItem(ctx, leader.Agent, "minecraft:oak_planks", 5*time.Second)
	require.NoError(t, err, "wait for planks to sync to agent")
	require.True(t, hasPlanks, "agent should see the given oak planks")

	err = leader.Agent.CraftItem(ctx, "minecraft:stick")
	require.NoError(t, err, "craft stick")

	hasStick, err := waitForInventoryItem(s.Ctx, s.Inst.RCON, leader.Name, "minecraft:stick", 5*time.Second)
	require.NoError(t, err, "check inventory for crafted stick")
	require.True(t, hasStick, "should have crafted a stick")

	t.Log("✓ Craft stick from planks test passed")
}

// TestShapelessChestBoat tests CraftItem against a shapeless recipe with
// concrete (non-tag) ingredients — acacia_chest_boat (chest + acacia_boat),
// confirmed present with this exact ingredient list across every version in
// models.StandardVersionTests. Covers the shapeless path (order-independent
// grid placement) that TestStickFromPlanks's shaped recipe doesn't
// exercise. Equivalent to the pre-Phase-1 TestCraftItem_ShapelessChestBoat.
func (s *CraftItemFlatSuite) TestShapelessChestBoat() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("CraftChestBoatBot", "craft_chest_boat")
	require.NoError(t, err, "spawn agent")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:chest 1", leader.Name))
	require.NoError(t, err, "give chest")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("give %s minecraft:acacia_boat 1", leader.Name))
	require.NoError(t, err, "give acacia boat")

	hasChest, err := waitForAgentHasItem(ctx, leader.Agent, "minecraft:chest", 5*time.Second)
	require.NoError(t, err, "wait for chest to sync to agent")
	require.True(t, hasChest, "agent should see the given chest")
	hasBoat, err := waitForAgentHasItem(ctx, leader.Agent, "minecraft:acacia_boat", 5*time.Second)
	require.NoError(t, err, "wait for boat to sync to agent")
	require.True(t, hasBoat, "agent should see the given acacia boat")

	err = leader.Agent.CraftItem(ctx, "minecraft:acacia_chest_boat")
	require.NoError(t, err, "craft acacia chest boat")

	hasChestBoat, err := waitForInventoryItem(s.Ctx, s.Inst.RCON, leader.Name, "minecraft:acacia_chest_boat", 5*time.Second)
	require.NoError(t, err, "check inventory for crafted chest boat")
	require.True(t, hasChestBoat, "should have crafted an acacia chest boat")

	t.Log("✓ Craft shapeless chest boat test passed")
}
