package testing

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ContainerIntegrationRandomSuite is a
// version-parameterized suite for the chest-interaction tests below: one server per version,
// shared by both methods, instead of the previous per-test-function StartServer/StopServer
// pattern. WorldGen = WorldGenRandom + GameMode = survival (with FORCE_GAMEMODE via ExtraEnv),
// matching both pre-conversion functions' own identical inline DefaultServerConfig() override,
// the same pattern as container_finder_test.go/container_types_test.go (35/37).
type ContainerIntegrationRandomSuite struct {
	VersionWorldSuite
}

func TestContainerIntegrationRandomSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ContainerIntegrationRandomSuite{}
		s.WorldGen = WorldGenRandom
		s.GameMode = GameModeSurvival
		s.ExtraEnv = map[string]string{"FORCE_GAMEMODE": "true"}
		return s
	})
}

// TestChestInteraction verifies opening and closing an empty chest leaves
// both the chest and the player's inventory untouched. Equivalent to the
// original TestChestInteraction.
func (s *ContainerIntegrationRandomSuite) TestChestInteraction() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ChestInteractBot", "chest_interaction")
	require.NoError(t, err, "spawn agent")

	t.Log("clearing agent inventory")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("data merge entity %s {Inventory:[]}", leader.Name))
	require.NoError(t, err, "clear inventory")
	time.Sleep(500 * time.Millisecond)

	t.Log("verifying inventory is empty")
	invItems, err := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get inventory")
	require.Empty(t, invItems, "inventory should be empty after clear")

	chestX := int(math.Floor(leader.Origin.X)) + 2
	chestY := int(math.Floor(leader.Origin.Y))
	chestZ := int(math.Floor(leader.Origin.Z))
	chestPos := models.V3{
		X: float64(chestX),
		Y: float64(chestY),
		Z: float64(chestZ),
	}
	t.Logf("placing chest at %+v", chestPos)
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, chestPos, "minecraft:chest", "minecraft:chest", 10*time.Second)
	require.NoError(t, err, "place chest")

	chestItems, err := GetChestContents(s.Ctx, s.Inst.RCON, chestPos)
	require.NoError(t, err, "get chest contents")
	require.Empty(t, chestItems, "chest should be empty initially")
	t.Log("chest verified to be empty")

	gamemodeResp, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("data get entity %s playerGameType", leader.Name))
	require.NoError(t, err, "get player gamemode")
	t.Logf("player game mode: %s", gamemodeResp)

	teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, chestPos.X-0.6, chestPos.Y, chestPos.Z)
	t.Logf("teleporting player: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(500 * time.Millisecond)

	worldMgr := leader.Agent.GetWorld()
	blockStateID, _ := worldMgr.GetBlockAt(chestPos.X, chestPos.Y, chestPos.Z)
	t.Logf("client world shows blockStateID at chest position: %d", blockStateID)
	if blockStateID != 0 {
		blockMgr := leader.Config.BlockMgr
		if blockID, ok := blockMgr.BlockIDByStateID(blockStateID); ok {
			if block, ok := blockMgr.GetByID(blockID); ok {
				t.Logf("client sees block: %s", block.Name)
			}
		}
	} else {
		t.Log("WARNING: client sees air/unloaded chunk at chest position")
	}

	itemUsage := items.NewItemUsage(leader.BotClient().Conn(), leader.Config.PacketMgr, nil)
	if leader.Config.VersionHandler != nil {
		itemUsage.SetContainerHandler(leader.Config.VersionHandler.Play().Containers())
	}

	t.Log("opening chest")
	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, chestPos, models.FaceEast, 5*time.Second)
	require.NoError(t, err, "open chest")
	t.Logf("chest opened with window ID: %d", windowID)

	rows := leader.Agent.GetChestRows(windowID)
	require.Greater(t, rows, 0, "chest should have rows")
	t.Logf("chest has %d rows", rows)

	t.Log("closing chest")
	_ = leader.Agent.CloseContainer()
	t.Log("chest closed successfully")

	t.Log("verifying player inventory is still empty")
	invItems, err = GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get inventory")
	require.Empty(t, invItems, "player inventory should still be empty")
	t.Log("confirmed: player inventory empty")

	t.Log("verifying chest is still empty")
	chestItems, err = GetChestContents(s.Ctx, s.Inst.RCON, chestPos)
	require.NoError(t, err, "get chest contents")
	require.Empty(t, chestItems, "chest should still be empty")
	t.Log("confirmed: chest is empty")
}

// TestChestWithItems verifies a chest placed with NBT-seeded items shows
// those items to the client on open, and that they survive a close
// untouched. Equivalent to the original TestChestWithItems.
func (s *ContainerIntegrationRandomSuite) TestChestWithItems() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ChestItemsBot", "chest_with_items")
	require.NoError(t, err, "spawn agent")

	t.Log("clearing agent inventory")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("data merge entity %s {Inventory:[]}", leader.Name))
	require.NoError(t, err, "clear inventory")
	time.Sleep(500 * time.Millisecond)

	t.Log("verifying inventory is empty")
	invItems, err := GetInventoryItems(s.Ctx, s.Inst.RCON, leader.Name)
	require.NoError(t, err, "get inventory")
	require.Empty(t, invItems, "inventory should be empty after clear")

	// Place it 1 block above spawn to ensure it's in air.
	chestX := int(math.Floor(leader.Origin.X)) + 2
	chestY := int(math.Floor(leader.Origin.Y)) + 1
	chestZ := int(math.Floor(leader.Origin.Z))
	chestPos := models.V3{
		X: float64(chestX),
		Y: float64(chestY),
		Z: float64(chestZ),
	}
	t.Logf("placing chest with items at %+v", chestPos)

	// Use setblock with direct NBT (no BlockEntityTag wrapper in 1.20.5+).
	// In Minecraft 1.20.5+, item IDs don't use "minecraft:" prefix in NBT.
	// Items in random slots to test finding specific items.
	blockSpec := `minecraft:chest{Items:[{Slot:5b,id:"iron_ingot",Count:10b},{Slot:2b,id:"gold_ingot",Count:8b},{Slot:12b,id:"diamond",Count:3b},{Slot:1b,id:"cobblestone",Count:32b}]}`
	_, err = PlaceBlockAndWait(s.Ctx, s.Inst.RCON, leader.ManagedAgent, chestPos, blockSpec, "minecraft:chest", 10*time.Second)
	require.NoError(t, err, "place chest with items")

	fullDataCmd := fmt.Sprintf("data get block %d %d %d", chestX, chestY, chestZ)
	fullResp, err := s.Inst.RCON.Exec(s.Ctx, fullDataCmd)
	require.NoError(t, err, "get full block data")
	t.Logf("full block data: %s", fullResp)

	t.Log("verifying chest has items via RCON")
	chestItems, err := GetChestContents(s.Ctx, s.Inst.RCON, chestPos)
	require.NoError(t, err, "get chest contents")
	require.Len(t, chestItems, 4, "chest should have 4 items")
	t.Logf("chest contents via RCON: %+v", chestItems)

	teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", leader.Name, chestPos.X-0.6, chestPos.Y, chestPos.Z)
	t.Logf("teleporting player: %s", teleportCmd)
	_, err = s.Inst.RCON.Exec(s.Ctx, teleportCmd)
	require.NoError(t, err, "teleport player")
	time.Sleep(500 * time.Millisecond)

	worldMgr := leader.Agent.GetWorld()
	blockStateID, _ := worldMgr.GetBlockAt(chestPos.X, chestPos.Y, chestPos.Z)
	t.Logf("client world shows blockStateID at chest position: %d", blockStateID)
	if blockStateID != 0 {
		blockMgr := leader.Config.BlockMgr
		if blockID, ok := blockMgr.BlockIDByStateID(blockStateID); ok {
			if block, ok := blockMgr.GetByID(blockID); ok {
				t.Logf("client sees block: %s", block.Name)
			}
		}
	} else {
		t.Log("WARNING: client sees air/unloaded chunk at chest position")
	}

	itemUsage := items.NewItemUsage(leader.BotClient().Conn(), leader.Config.PacketMgr, nil)
	if leader.Config.VersionHandler != nil {
		itemUsage.SetContainerHandler(leader.Config.VersionHandler.Play().Containers())
	}

	t.Log("opening chest")
	windowID, err := OpenContainerWithLOS(s.Ctx, leader.Agent, chestPos, models.FaceEast, 5*time.Second)
	require.NoError(t, err, "open chest")
	t.Logf("chest opened with window ID: %d", windowID)

	rows := leader.Agent.GetChestRows(windowID)
	require.Greater(t, rows, 0, "chest should have rows")
	t.Logf("chest has %d rows", rows)

	screen, ok := leader.ScreenManager().Screens()[int(windowID)]
	require.True(t, ok, "chest window should exist")
	chest, ok := screen.(*mcscreen.Chest)
	require.True(t, ok, "screen should be a chest")
	t.Logf("client chest slots count: %d (expected 63 = 27 chest + 36 player)", len(chest.Slots))

	itemsFound := 0
	for i, slot := range chest.Slots[:27] { // Only check chest slots
		if slot.ID != 0 {
			t.Logf("  chest slot %d: ID=%d Count=%d", i, slot.ID, slot.Count)
			itemsFound++
		}
	}
	require.Equal(t, 4, itemsFound, "client should see 4 items in chest")
	t.Log("SUCCESS: Client can see items in chest that were placed via setblock with NBT!")

	// Note: There's a known issue where ClientboundWindowItems only sends
	// chest slots (27) instead of the full window (63 slots including
	// player inventory). This causes "slot index out of bounds" errors when
	// trying to take items. For now, we'll just verify the chest can be
	// opened and items are visible.

	t.Log("closing chest")
	_ = leader.Agent.CloseContainer()
	time.Sleep(500 * time.Millisecond)
	t.Log("chest closed successfully")

	t.Log("verifying chest still has all items")
	chestItems, err = GetChestContents(s.Ctx, s.Inst.RCON, chestPos)
	require.NoError(t, err, "get chest contents")
	require.Len(t, chestItems, 4, "chest should still have 4 items")
	t.Logf("chest contents after close: %+v", chestItems)

	t.Log("Test complete - chest with items works via setblock NBT!")
}
