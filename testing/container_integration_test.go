package testing

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
	"github.com/stretchr/testify/require"
)

func TestChestInteraction(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			cwd, err := os.Getwd()
			require.NoError(t, err, "get current working directory")

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")
			t.Log("framework initialized")

			serverCfg := DefaultServerConfig()
			serverCfg.Memory = "512M"
			serverCfg.Version = tt.MCVersion
			serverCfg.GameMode = GameModeSurvival
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestChestInteraction", tt.MCVersion)
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			t.Logf("server started: %s:%d", inst.Server.Host, inst.Server.HostServerPort)
			require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
			defer framework.CloseAgentLog()
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)
			botName := "ChestBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			// Version handler is auto-detected by the framework

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			scr := managedAgent.ScreenManager()
			botClient := managedAgent.BotClient()
			require.NotNil(t, botClient, "bot client should be available")

			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second), "agent never appeared in server player list")

			// Clear agent inventory
			t.Log("clearing agent inventory")
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("data merge entity %s {Inventory:[]}", botName))
			require.NoError(t, err, "clear inventory")
			time.Sleep(500 * time.Millisecond) // Give server time to process

			// Verify inventory is empty
			t.Log("verifying inventory is empty")
			invItems, err := GetInventoryItems(ctx, inst.RCON, botName)
			require.NoError(t, err, "get inventory")
			require.Empty(t, invItems, "inventory should be empty after clear")

			// Get player position to place chest nearby
			playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player position: %+v", playerPos)

			// Place a chest in front of the player
			chestX := int(math.Floor(playerPos.X)) + 2
			chestY := int(math.Floor(playerPos.Y))
			chestZ := int(math.Floor(playerPos.Z))
			chestPos := models.V3{
				X: float64(chestX),
				Y: float64(chestY),
				Z: float64(chestZ),
			}
			t.Logf("placing chest at %+v", chestPos)
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, chestPos, "minecraft:chest", "minecraft:chest", 10*time.Second)
			require.NoError(t, err, "place chest")

			// Verify chest is empty initially
			chestItems, err := GetChestContents(ctx, inst.RCON, chestPos)
			require.NoError(t, err, "get chest contents")
			require.Empty(t, chestItems, "chest should be empty initially")
			t.Log("chest verified to be empty")

			// Check player's game mode
			gamemodeResp, err := inst.RCON.Exec(ctx, fmt.Sprintf("data get entity %s playerGameType", botName))
			require.NoError(t, err, "get player gamemode")
			t.Logf("player game mode: %s", gamemodeResp)

			// Teleport player right next to the chest to be sure they're in range
			teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", botName, chestPos.X-0.6, chestPos.Y, chestPos.Z)
			t.Logf("teleporting player: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(500 * time.Millisecond)

			// Check if client has the chest block loaded
			worldMgr := managedAgent.Agent.GetWorld()
			blockStateID, _ := worldMgr.GetBlockAt(chestPos.X, chestPos.Y, chestPos.Z)
			t.Logf("client world shows blockStateID at chest position: %d", blockStateID)
			if blockStateID != 0 {
				blockMgr := managedAgent.Config.BlockMgr
				if blockID, ok := blockMgr.BlockIDByStateID(blockStateID); ok {
					if block, ok := blockMgr.GetByID(blockID); ok {
						t.Logf("client sees block: %s", block.Name)
					}
				}
			} else {
				t.Log("WARNING: client sees air/unloaded chunk at chest position")
			}

			// Create helper objects for container operations
			itemUsage := items.NewItemUsage(botClient.Conn(), managedAgent.Config.PacketMgr)
			// Set version-specific container handler
			if managedAgent.Config.VersionHandler != nil {
				itemUsage.SetContainerHandler(managedAgent.Config.VersionHandler.Play().Containers())
			}
			invMgr := items.NewInventoryManager(scr)
			invMgr.SetWaitForUpdates(false) // Use workaround for ServerUpdateVersion issue
			containerHelper := items.NewContainerHelper(itemUsage, invMgr, scr, botClient, managedAgent.Config.PacketMgr)
			managedAgent.Agent.SetContainerHelper(containerHelper)

			// Open the chest (player is west of chest, so click on the east face which faces the player)
			t.Log("opening chest")
			windowID, err := OpenContainerWithLOS(ctx, managedAgent.Agent, chestPos, models.FaceEast, 5*time.Second)
			require.NoError(t, err, "open chest")
			t.Logf("chest opened with window ID: %d", windowID)

			// Verify the chest window is open
			rows := containerHelper.GetChestRows(windowID)
			require.Greater(t, rows, 0, "chest should have rows")
			t.Logf("chest has %d rows", rows)

			// Close the chest
			t.Log("closing chest")
			_ = containerHelper.CloseContainer()
			t.Log("chest closed successfully")

			// Verify inventory is still empty (no items were taken from the empty chest)
			t.Log("verifying player inventory is still empty")
			invItems, err = GetInventoryItems(ctx, inst.RCON, botName)
			require.NoError(t, err, "get inventory")
			require.Empty(t, invItems, "player inventory should still be empty")
			t.Log("confirmed: player inventory empty")

			// Verify chest is still empty
			t.Log("verifying chest is still empty")
			chestItems, err = GetChestContents(ctx, inst.RCON, chestPos)
			require.NoError(t, err, "get chest contents")
			require.Empty(t, chestItems, "chest should still be empty")
			t.Log("confirmed: chest is empty")

			_ = botClient.Close()
		})
	}
}

func TestChestWithItems(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			cwd, err := os.Getwd()
			require.NoError(t, err, "get current working directory")

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")
			t.Log("framework initialized")

			serverCfg := DefaultServerConfig()
			serverCfg.Memory = "512M"
			serverCfg.Version = tt.MCVersion
			serverCfg.GameMode = "survival"
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestChestWithItems", tt.MCVersion)
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			t.Logf("server started: %s:%d", inst.Server.Host, inst.Server.HostServerPort)
			require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
			defer framework.CloseAgentLog()
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)
			botName := "ItemChestBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			// Version handler is auto-detected by the framework

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			scr := managedAgent.ScreenManager()
			botClient := managedAgent.BotClient()
			require.NotNil(t, botClient, "bot client should be available")

			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second), "agent never appeared in server player list")

			// Clear agent inventory
			t.Log("clearing agent inventory")
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("data merge entity %s {Inventory:[]}", botName))
			require.NoError(t, err, "clear inventory")
			time.Sleep(500 * time.Millisecond)

			// Verify inventory is empty
			t.Log("verifying inventory is empty")
			invItems, err := GetInventoryItems(ctx, inst.RCON, botName)
			require.NoError(t, err, "get inventory")
			require.Empty(t, invItems, "inventory should be empty after clear")

			// Get player position
			playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player position: %+v", playerPos)

			// Place a chest with items using setblock with NBT data
			// Place it 1 block above player to ensure it's in air
			chestX := int(math.Floor(playerPos.X)) + 2
			chestY := int(math.Floor(playerPos.Y)) + 1
			chestZ := int(math.Floor(playerPos.Z))
			chestPos := models.V3{
				X: float64(chestX),
				Y: float64(chestY),
				Z: float64(chestZ),
			}
			t.Logf("placing chest with items at %+v", chestPos)

			// Use setblock with direct NBT (no BlockEntityTag wrapper in 1.20.5+)
			// In Minecraft 1.20.5+, item IDs don't use "minecraft:" prefix in NBT
			// Items in random slots to test finding specific items
			blockSpec := `minecraft:chest{Items:[{Slot:5b,id:"iron_ingot",Count:10b},{Slot:2b,id:"gold_ingot",Count:8b},{Slot:12b,id:"diamond",Count:3b},{Slot:1b,id:"cobblestone",Count:32b}]}`
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, chestPos, blockSpec, "minecraft:chest", 10*time.Second)
			require.NoError(t, err, "place chest with items")

			// Debug: Check full block data
			fullDataCmd := fmt.Sprintf("data get block %d %d %d", chestX, chestY, chestZ)
			fullResp, err := inst.RCON.Exec(ctx, fullDataCmd)
			require.NoError(t, err, "get full block data")
			t.Logf("full block data: %s", fullResp)

			// Verify items are in the chest via RCON
			t.Log("verifying chest has items via RCON")
			chestItems, err := GetChestContents(ctx, inst.RCON, chestPos)
			require.NoError(t, err, "get chest contents")
			require.Len(t, chestItems, 4, "chest should have 4 items")
			t.Logf("chest contents via RCON: %+v", chestItems)

			// Teleport player right next to the chest to be sure they're in range
			teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", botName, chestPos.X-0.6, chestPos.Y, chestPos.Z)
			t.Logf("teleporting player: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(500 * time.Millisecond)

			// Check if client has the chest block loaded
			worldMgr := managedAgent.Agent.GetWorld()
			blockStateID, _ := worldMgr.GetBlockAt(chestPos.X, chestPos.Y, chestPos.Z)
			t.Logf("client world shows blockStateID at chest position: %d", blockStateID)
			if blockStateID != 0 {
				blockMgr := managedAgent.Config.BlockMgr
				if blockID, ok := blockMgr.BlockIDByStateID(blockStateID); ok {
					if block, ok := blockMgr.GetByID(blockID); ok {
						t.Logf("client sees block: %s", block.Name)
					}
				}
			} else {
				t.Log("WARNING: client sees air/unloaded chunk at chest position")
			}

			// Create helper objects for container operations
			itemUsage := items.NewItemUsage(botClient.Conn(), managedAgent.Config.PacketMgr)
			// Set version-specific container handler
			if managedAgent.Config.VersionHandler != nil {
				itemUsage.SetContainerHandler(managedAgent.Config.VersionHandler.Play().Containers())
			}
			invMgr := items.NewInventoryManager(scr)
			invMgr.SetWaitForUpdates(false) // Use workaround for ServerUpdateVersion issue
			containerHelper := items.NewContainerHelper(itemUsage, invMgr, scr, botClient, managedAgent.Config.PacketMgr)
			managedAgent.Agent.SetContainerHelper(containerHelper)

			// Open the chest (player is west of chest, so click on the east face which faces the player)
			t.Log("opening chest")
			windowID, err := OpenContainerWithLOS(ctx, managedAgent.Agent, chestPos, models.FaceEast, 5*time.Second)
			require.NoError(t, err, "open chest")
			t.Logf("chest opened with window ID: %d", windowID)

			// Verify the chest window is open
			rows := containerHelper.GetChestRows(windowID)
			require.Greater(t, rows, 0, "chest should have rows")
			t.Logf("chest has %d rows", rows)

			// Debug: Check what the client sees in the chest
			screen, ok := scr.Screens()[int(windowID)]
			require.True(t, ok, "chest window should exist")
			chest, ok := screen.(*mcscreen.Chest)
			require.True(t, ok, "screen should be a chest")
			t.Logf("client chest slots count: %d (expected 63 = 27 chest + 36 player)", len(chest.Slots))

			// Verify client can see the items in the chest
			itemsFound := 0
			for i, slot := range chest.Slots[:27] { // Only check chest slots
				if slot.ID != 0 {
					t.Logf("  chest slot %d: ID=%d Count=%d", i, slot.ID, slot.Count)
					itemsFound++
				}
			}
			require.Equal(t, 4, itemsFound, "client should see 4 items in chest")
			t.Log("SUCCESS: Client can see items in chest that were placed via setblock with NBT!")

			// Note: There's a known issue where ClientboundWindowItems only sends chest slots (27)
			// instead of the full window (63 slots including player inventory).
			// This causes "slot index out of bounds" errors when trying to take items.
			// For now, we'll just verify the chest can be opened and items are visible.

			// Close the chest
			t.Log("closing chest")
			_ = containerHelper.CloseContainer()
			time.Sleep(500 * time.Millisecond)
			t.Log("chest closed successfully")

			// Verify chest still has all 4 items (nothing was taken)
			t.Log("verifying chest still has all items")
			chestItems, err = GetChestContents(ctx, inst.RCON, chestPos)
			require.NoError(t, err, "get chest contents")
			require.Len(t, chestItems, 4, "chest should still have 4 items")
			t.Logf("chest contents after close: %+v", chestItems)

			// Test complete - we've successfully demonstrated:
			// 1. Creating a chest with items using setblock with NBT (without BlockEntityTag wrapper)
			// 2. Opening the chest and seeing the items on the client side
			// 3. Closing the chest
			t.Log("Test complete - chest with items works via setblock NBT!")

			_ = botClient.Close()
		})
	}
}
