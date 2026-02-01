package testing

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/items"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

func TestFindContainersNearby(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			cwd, err := os.Getwd()
			require.NoError(t, err)

			framework, err := NewFramework()
			require.NoError(t, err)

			serverCfg := DefaultServerConfig()
			serverCfg.Memory = "512M"
			serverCfg.Version = tt.mcVersion
			serverCfg.GameMode = "survival"
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestFindContainersNearby", tt.mcVersion)
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)
			botName := "FinderBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			// Version handler is auto-detected by the framework

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err)
			botClient := managedAgent.BotClient()
			require.NotNil(t, botClient)

			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second))

			// Get player position
			playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err)
			t.Logf("player position: %+v", playerPos)

			// Place multiple chests at different positions
			baseX := int(math.Floor(playerPos.X))
			baseY := int(math.Floor(playerPos.Y))
			baseZ := int(math.Floor(playerPos.Z))
			chest1Pos := models.V3{X: float64(baseX + 2), Y: float64(baseY), Z: float64(baseZ)}
			chest2Pos := models.V3{X: float64(baseX - 3), Y: float64(baseY), Z: float64(baseZ)}
			chest3Pos := models.V3{X: float64(baseX), Y: float64(baseY), Z: float64(baseZ + 4)}
			barrelPos := models.V3{X: float64(baseX + 1), Y: float64(baseY + 1), Z: float64(baseZ)}

			t.Log("placing containers around player")
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, chest1Pos, "minecraft:chest", "minecraft:chest", 10*time.Second)
			require.NoError(t, err, "place chest1")
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, chest2Pos, "minecraft:chest", "minecraft:chest", 10*time.Second)
			require.NoError(t, err, "place chest2")
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, chest3Pos, "minecraft:trapped_chest", "minecraft:trapped_chest", 10*time.Second)
			require.NoError(t, err, "place trapped chest")
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, barrelPos, "minecraft:barrel", "minecraft:barrel", 10*time.Second)
			require.NoError(t, err, "place barrel")

			// Get world and block manager from agent
			worldMgr := managedAgent.Agent.GetWorld()
			require.NotNil(t, worldMgr, "world manager should be available")

			blockMgr := managedAgent.Config.BlockMgr
			require.NotNil(t, blockMgr, "block manager should be available")

			// Create container finder
			containerFinder := items.NewContainerFinder(worldMgr, blockMgr)

			// Find all containers within radius 5
			t.Log("searching for containers within radius 5")
			searchPos := models.V3{X: playerPos.X, Y: playerPos.Y, Z: playerPos.Z}
			containers := containerFinder.FindContainersNearby(searchPos, 5)

			t.Logf("found %d containers", len(containers))
			for i, container := range containers {
				t.Logf("  [%d] %s at (%.0f, %.0f, %.0f)", i, container.Name, container.Position.X, container.Position.Y, container.Position.Z)
			}

			// Should find all 4 containers
			require.GreaterOrEqual(t, len(containers), 4, "should find at least 4 containers")

			// Find nearest container
			t.Log("finding nearest container")
			nearest, found := containerFinder.FindNearestContainer(searchPos, 5)
			require.True(t, found, "should find at least one container")
			t.Logf("nearest container: %s at (%.0f, %.0f, %.0f)", nearest.Name, nearest.Position.X, nearest.Position.Y, nearest.Position.Z)

			// Distance from player to nearest should be <= distance to any other container
			nearestDist := searchPos.DistanceTo(nearest.Position)
			for _, container := range containers {
				dist := searchPos.DistanceTo(container.Position)
				require.LessOrEqual(t, nearestDist, dist, "nearest container should be closest")
			}

			// Find only chests (should find 3: 2 regular + 1 trapped)
			t.Log("finding only chests")
			chests := containerFinder.FindContainersByType(searchPos, 5, "chest")
			t.Logf("found %d chests", len(chests))
			require.GreaterOrEqual(t, len(chests), 3, "should find at least 3 chests")

			// Verify all found containers are chests
			for _, chest := range chests {
				require.True(t, items.IsChest(chest.Name), "container should be a chest: %s", chest.Name)
			}

			// Find only barrels (should find 1)
			t.Log("finding only barrels")
			barrels := containerFinder.FindContainersByType(searchPos, 5, "barrel")
			t.Logf("found %d barrels", len(barrels))
			require.GreaterOrEqual(t, len(barrels), 1, "should find at least 1 barrel")

			_ = botClient.Close()
		})
	}
}
func TestFindAndOpenContainer(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			cwd, err := os.Getwd()
			require.NoError(t, err)

			framework, err := NewFramework()
			require.NoError(t, err)

			serverCfg := DefaultServerConfig()
			serverCfg.Memory = "512M"
			serverCfg.Version = tt.mcVersion
			serverCfg.GameMode = "survival"
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestFindAndOpenContainer", tt.mcVersion)

			if os.Getenv("TEST_INTEGRATION_KEEP_SERVER_CACHE") == "" {
				os.RemoveAll(serverCfg.CacheDir) // Ensure clean state
			}

			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)
			botName := "FindOpenBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
				EnableReplay:  true,
				ReplayOutput:  fmt.Sprintf("./replays/find_open_test_%s_%s.mcpr", tt.name, time.Now().Format("20060102_150405")),
			}

			// Version handler is auto-detected by the framework

			defer t.Logf("Replay files: FindOpenBot=%s", agentCfg.ReplayOutput)

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err)
			scr := managedAgent.ScreenManager()
			botClient := managedAgent.BotClient()
			require.NotNil(t, botClient)

			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second))

			// Get player position
			playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err)

			inst.RCON.Exec(ctx, "fill %d %d %d %d %d %d minecraft air")

			// Place chest with item
			baseX := int(math.Floor(playerPos.X))
			baseY := int(math.Floor(playerPos.Y))
			baseZ := int(math.Floor(playerPos.Z))
			chestPos := models.V3{X: float64(baseX + 2), Y: float64(baseY), Z: float64(baseZ)}
			inst.RCON.Exec(ctx, fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air",
				int(math.Floor(chestPos.X))-3, int(math.Floor(chestPos.Y))-1, int(math.Floor(chestPos.Z))-3,
				int(math.Floor(chestPos.X+3)), int(math.Floor(chestPos.Y+3)), int(math.Floor(chestPos.Z+3))),
			)

			t.Log("placing chest with diamond")
			_, err = PlaceBlockAndWait(ctx, inst.RCON, managedAgent, chestPos, "minecraft:chest", "minecraft:chest", 10*time.Second)
			require.NoError(t, err, "place chest")
			slotID := rand.Intn(26) + 1 // Chest has 27 slots
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("data merge block %d %d %d {Items:[{Slot:0b,id:\"minecraft:diamond\",Count:%db}]}", baseX+2, baseY, baseZ, slotID))
			require.NoError(t, err)

			// Find the chest
			worldMgr := managedAgent.Agent.GetWorld()
			blockMgr := managedAgent.Config.BlockMgr
			containerFinder := items.NewContainerFinder(worldMgr, blockMgr)

			t.Log("searching for chest")
			searchPos := models.V3{X: playerPos.X, Y: playerPos.Y, Z: playerPos.Z}
			nearest, found := containerFinder.FindNearestContainer(searchPos, 5)
			require.True(t, found, "should find the chest")
			t.Logf("found chest at (%.0f, %.0f, %.0f)", nearest.Position.X, nearest.Position.Y, nearest.Position.Z)

			// Verify it's the chest we placed
			require.Equal(t, math.Floor(chestPos.X), nearest.Position.X)
			require.Equal(t, math.Floor(chestPos.Y), nearest.Position.Y)
			require.Equal(t, math.Floor(chestPos.Z), nearest.Position.Z)

			// Open and interact with the chest
			itemUsage := items.NewItemUsage(botClient.Conn(), managedAgent.Config.PacketMgr)
			// Set version-specific container handler
			if managedAgent.Config.VersionHandler != nil {
				itemUsage.SetContainerHandler(managedAgent.Config.VersionHandler.Play().Containers())
			}
			invMgr := items.NewInventoryManager(scr)
			invMgr.SetWaitForUpdates(false)
			containerHelper := items.NewContainerHelper(itemUsage, invMgr, scr, botClient, managedAgent.Config.PacketMgr)
			managedAgent.Agent.SetContainerHelper(containerHelper)

			t.Log("opening found chest")
			windowID, err := OpenContainerWithLOS(ctx, managedAgent.Agent, nearest.Position, items.FaceNorth, 5*time.Second)
			require.NoError(t, err)
			t.Logf("chest opened with window ID: %d", windowID)

			// Take the diamond
			t.Log("taking diamond from chest")
			foundSlotID, slotIDFound, err := managedAgent.Agent.FindSlotWith(ctx, "minecraft:diamond", int(windowID))
			require.True(t, slotIDFound, "diamond should be in chest")
			require.NoError(t, err, "failed to find diamond in chest")

			err = containerHelper.TakeItemFromChest(windowID, int16(foundSlotID))
			require.NoError(t, err)

			_ = containerHelper.CloseContainer()
			time.Sleep(500 * time.Millisecond)

			// Verify diamond is in player inventory
			invItems, err := GetInventoryItems(ctx, inst.RCON, botName)
			require.NoError(t, err)
			hasDiamond := false
			for _, item := range invItems {
				if item.ID == "minecraft:diamond" {
					hasDiamond = true
					break
				}
			}
			require.True(t, hasDiamond, "player should have diamond after taking from chest")
			t.Log("confirmed: diamond successfully taken from found chest")

			_ = botClient.Close()
		})
	}
}
