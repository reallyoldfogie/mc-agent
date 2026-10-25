package testing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/agent"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// inventorySnapshotRunner is the narrow slice of the live agent this file's
// test needs - docs/plans/COMBAT_COMPETITION_PLAN.md Phase 0 item 5's
// SaveInventorySnapshot/RestoreInventorySnapshot.
type inventorySnapshotRunner interface {
	SaveInventorySnapshot(ctx context.Context, playerName string, center models.V3, radius float64) (agent.InventorySnapshot, error)
	RestoreInventorySnapshot(ctx context.Context, playerName string, snapshot agent.InventorySnapshot, center models.V3, radius float64) error
}

// InventorySnapshotFlatSuite live-validates agent/rl_seed_inventory.go's
// save/restore round trip against a real server - specifically the two
// parts unit tests against synthetic RCON responses couldn't confirm on
// their own: (1) the components-NBT-to-bracket-syntax translation actually
// matches vanilla's /item replace entity ... with <item>[<components>]
// grammar, and (2) worn armor/offhand genuinely live in a separate
// "equipment" NBT compound on this server version rather than inline in
// "Inventory" - discovered live while building this test, not assumed.
type InventorySnapshotFlatSuite struct {
	VersionWorldSuite
}

func TestInventorySnapshotFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &InventorySnapshotFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestSaveAndRestoreRoundTrip gives a live player a plain item (no
// components, in the hotbar/"Inventory" location) and an enchanted item
// (with components, worn as armor - the separate "equipment" location),
// saves a snapshot, clears the inventory, restores the snapshot, and
// confirms both items - including the enchantment - came back exactly.
func (s *InventorySnapshotFlatSuite) TestSaveAndRestoreRoundTrip() {
	t := s.T()

	player, err := s.SpawnWorkingAreaAgentNoCam("InventorySnapshotBot", "inventory_snapshot")
	require.NoError(t, err, "spawn player")

	runner, ok := player.Agent.(inventorySnapshotRunner)
	require.True(t, ok, "spawned test agent must satisfy inventorySnapshotRunner")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("item replace entity %s hotbar.0 with minecraft:iron_sword 1", player.Name))
	require.NoError(t, err, "give plain iron sword")
	helmetGiveResp, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(
		`item replace entity %s armor.head with minecraft:diamond_helmet[minecraft:enchantments={"minecraft:protection":2}] 1`, player.Name))
	require.NoError(t, err, "give enchanted diamond helmet")
	require.Contains(t, helmetGiveResp, "Replaced", "helmet give command should have succeeded: %s", helmetGiveResp)
	time.Sleep(500 * time.Millisecond)

	center := player.Origin
	snapshot, err := runner.SaveInventorySnapshot(s.Ctx, player.Name, center, 20)
	require.NoError(t, err, "SaveInventorySnapshot")
	require.Len(t, snapshot, 2, "expected exactly the two seeded slots (hotbar sword + equipment helmet): %+v", snapshot)

	var sword, helmet *agent.InventorySnapshotSlot
	for i := range snapshot {
		switch snapshot[i].ID {
		case "minecraft:iron_sword":
			sword = &snapshot[i]
		case "minecraft:diamond_helmet":
			helmet = &snapshot[i]
		}
	}
	require.NotNil(t, sword, "snapshot should include the iron sword")
	require.Equal(t, 0, sword.Slot)
	require.Equal(t, 1, sword.Count)
	require.Empty(t, sword.Components, "plain sword should have no captured components")

	require.NotNil(t, helmet, "snapshot should include the diamond helmet")
	require.Equal(t, 103, helmet.Slot)
	require.Contains(t, helmet.Components, "minecraft:protection", "captured components should include the enchantment")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("clear %s", player.Name))
	require.NoError(t, err, "clear inventory before restore")
	time.Sleep(500 * time.Millisecond)

	cleared, err := GetInventoryItems(s.Ctx, s.Inst.RCON, player.Name)
	require.NoError(t, err, "read inventory after clear")
	require.Empty(t, cleared, "hotbar/main inventory should be empty before restore")

	clearedEquipResp, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("data get entity %s equipment", player.Name))
	require.NoError(t, err, "read equipment after clear")
	require.NotContains(t, clearedEquipResp, "diamond_helmet", "worn helmet should also be cleared by /clear: %s", clearedEquipResp)

	require.NoError(t, runner.RestoreInventorySnapshot(s.Ctx, player.Name, snapshot, center, 20), "RestoreInventorySnapshot")
	time.Sleep(500 * time.Millisecond)

	// The helmet may legitimately come back either inline in "Inventory"
	// (slot 103, older server versions) or under the separate "equipment"
	// compound (newer versions) - SaveInventorySnapshot/RestoreInventorySnapshot
	// are deliberately version-agnostic about which, so this check accepts
	// either rather than assuming one, unlike an earlier version of this
	// test that assumed "equipment"-only and failed live against 1.21.1.
	restored, err := GetInventoryItems(s.Ctx, s.Inst.RCON, player.Name)
	require.NoError(t, err, "read inventory after restore")
	var restoredSword, restoredHelmetInInventory *InventoryItem
	for i := range restored {
		switch restored[i].ID {
		case "minecraft:iron_sword":
			restoredSword = &restored[i]
		case "minecraft:diamond_helmet":
			restoredHelmetInInventory = &restored[i]
		}
	}
	require.NotNil(t, restoredSword, "restored inventory should include the iron sword: %+v", restored)
	require.Equal(t, 0, restoredSword.Slot)
	require.Equal(t, 1, restoredSword.Count)

	restoredEquipResp, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("data get entity %s equipment", player.Name))
	require.NoError(t, err, "read equipment after restore")
	helmetInEquipment := strings.Contains(restoredEquipResp, "minecraft:diamond_helmet")

	require.True(t, restoredHelmetInInventory != nil || helmetInEquipment,
		"restored helmet should appear somewhere (Inventory or equipment): inventory=%+v equipment=%s", restored, restoredEquipResp)
	if helmetInEquipment {
		require.Contains(t, restoredEquipResp, "minecraft:protection", "restored helmet should carry its enchantment through the components bracket syntax: %s", restoredEquipResp)
	} else {
		fullResp, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("data get entity %s Inventory", player.Name))
		require.NoError(t, err, "read full Inventory NBT after restore")
		require.Contains(t, fullResp, "minecraft:protection", "restored helmet should carry its enchantment through the components bracket syntax: %s", fullResp)
	}
}
