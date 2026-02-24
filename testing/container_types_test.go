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

// TestFurnaceInteraction tests opening a furnace container
func TestFurnaceInteraction(t *testing.T) {
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
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestFurnaceInteraction", tt.MCVersion)
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
			botName := "FurnaceBot"
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

			// Get player position
			playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player position: %+v", playerPos)

			// Place a furnace (match chest test pattern exactly)
			baseX := int(math.Floor(playerPos.X))
			baseY := int(math.Floor(playerPos.Y))
			baseZ := int(math.Floor(playerPos.Z))
			furnacePos := models.V3{
				X: float64(baseX + 2),
				Y: float64(baseY + 1),
				Z: float64(baseZ),
			}
			t.Logf("placing furnace at %+v", furnacePos)
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, furnacePos, "minecraft:furnace", "minecraft:furnace", 10*time.Second)
			require.NoError(t, err, "place furnace")

			// Teleport player right next to furnace
			teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", botName, furnacePos.X-0.6, furnacePos.Y, furnacePos.Z)
			t.Logf("teleporting player: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(500 * time.Millisecond)

			// Check if client has the furnace block loaded (match chest test)
			worldMgr := managedAgent.Agent.GetWorld()
			blockStateID, _ := worldMgr.GetBlockAt(furnacePos.X, furnacePos.Y, furnacePos.Z)
			t.Logf("client world shows blockStateID at furnace position: %d", blockStateID)
			if blockStateID != 0 {
				blockMgr := managedAgent.Config.BlockMgr
				if blockID, ok := blockMgr.BlockIDByStateID(blockStateID); ok {
					if block, ok := blockMgr.GetByID(blockID); ok {
						t.Logf("client sees block: %s", block.Name)
					}
				}
			} else {
				t.Log("WARNING: client sees air/unloaded chunk at furnace position - attempting to open anyway")
			}

			// Create helper objects
			itemUsage := items.NewItemUsage(botClient.Conn(), managedAgent.Config.PacketMgr)
			// Set version-specific container handler
			if managedAgent.Config.VersionHandler != nil {
				itemUsage.SetContainerHandler(managedAgent.Config.VersionHandler.Play().Containers())
			}
			invMgr := items.NewInventoryManager(scr)
			invMgr.SetWaitForUpdates(false)
			containerHelper := items.NewContainerHelper(itemUsage, invMgr, scr, botClient, managedAgent.Config.PacketMgr)
			managedAgent.Agent.SetContainerHelper(containerHelper)

			// Open the furnace
			t.Log("opening furnace")
			windowID, err := OpenContainerWithLOS(ctx, managedAgent.Agent, furnacePos, items.FaceEast, 5*time.Second)
			require.NoError(t, err, "open furnace")
			t.Logf("furnace opened with window ID: %d", windowID)

			// Verify the furnace window is open
			screen, ok := scr.Screens()[int(windowID)]
			require.True(t, ok, "furnace window should exist")

			// Check if it's a GenericContainer (type 14 = furnace)
			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "screen should be a GenericContainer for furnace")
			require.Equal(t, int32(14), genericContainer.Type, "container type should be 14 (furnace)")
			t.Logf("furnace container has %d total slots (%d container + %d player)",
				len(genericContainer.Slots), genericContainer.ContainerSlots, len(genericContainer.Slots)-genericContainer.ContainerSlots)

			// Furnace should have 3 container slots + 36 player slots = 39 total
			require.Equal(t, 39, len(genericContainer.Slots), "furnace should have 39 total slots")
			require.Equal(t, 3, genericContainer.ContainerSlots, "furnace should have 3 container slots")

			// Close the furnace
			t.Log("closing furnace")
			_ = containerHelper.CloseContainer()
			t.Log("furnace closed successfully")

			_ = botClient.Close()
		})
	}
}

// TestHopperInteraction tests opening a hopper container
func TestHopperInteraction(t *testing.T) {
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
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestHopperInteraction", tt.MCVersion)
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
			botName := "HopperBot"
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

			// Get player position
			playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player position: %+v", playerPos)

			// Define hopper position
			baseX := int(math.Floor(playerPos.X))
			baseY := int(math.Floor(playerPos.Y))
			baseZ := int(math.Floor(playerPos.Z))
			hopperPos := models.V3{
				X: float64(baseX + 2),
				Y: float64(baseY),
				Z: float64(baseZ),
			}

			// Teleport player near hopper location FIRST to ensure chunk is loaded
			teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", botName, hopperPos.X-0.6, hopperPos.Y, hopperPos.Z)
			t.Logf("teleporting player near hopper location: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(1 * time.Second) // Give chunks time to load

			// Now place the hopper
			t.Logf("placing hopper at %+v", hopperPos)
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, hopperPos, "minecraft:hopper", "minecraft:hopper", 10*time.Second)
			require.NoError(t, err, "place hopper")

			// Create helper objects
			itemUsage := items.NewItemUsage(botClient.Conn(), managedAgent.Config.PacketMgr)
			// Set version-specific container handler
			if managedAgent.Config.VersionHandler != nil {
				itemUsage.SetContainerHandler(managedAgent.Config.VersionHandler.Play().Containers())
			}
			invMgr := items.NewInventoryManager(scr)
			invMgr.SetWaitForUpdates(false)
			containerHelper := items.NewContainerHelper(itemUsage, invMgr, scr, botClient, managedAgent.Config.PacketMgr)
			managedAgent.Agent.SetContainerHelper(containerHelper)

			// Open the hopper
			t.Log("opening hopper")
			windowID, err := OpenContainerWithLOS(ctx, managedAgent.Agent, hopperPos, items.FaceEast, 5*time.Second)
			require.NoError(t, err, "open hopper")
			t.Logf("hopper opened with window ID: %d", windowID)

			// Verify the hopper window is open
			screen, ok := scr.Screens()[int(windowID)]
			require.True(t, ok, "hopper window should exist")

			// Check if it's a GenericContainer (type 16 = hopper)
			genericContainer, ok := screen.(*mcscreen.GenericContainer)
			require.True(t, ok, "screen should be a GenericContainer for hopper")
			require.Equal(t, int32(16), genericContainer.Type, "container type should be 16 (hopper)")
			t.Logf("hopper container has %d total slots (%d container + %d player)",
				len(genericContainer.Slots), genericContainer.ContainerSlots, len(genericContainer.Slots)-genericContainer.ContainerSlots)

			// Hopper should have 5 container slots + 36 player slots = 41 total
			require.Equal(t, 41, len(genericContainer.Slots), "hopper should have 41 total slots")
			require.Equal(t, 5, genericContainer.ContainerSlots, "hopper should have 5 container slots")

			// Close the hopper
			t.Log("closing hopper")
			_ = containerHelper.CloseContainer()
			t.Log("hopper closed successfully")

			_ = botClient.Close()
		})
	}
}
