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

// TestRepeatedContainerOpen tests opening the same container multiple times
// This isolates whether the issue is with:
// - Opening different container types
// - Moving between positions
// - Server-side window ID exhaustion
func TestRepeatedContainerOpen(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			// Get working directory
			cwd, err := os.Getwd()
			require.NoError(t, err, "get current working directory")

			// Create framework
			framework, err := NewFramework()
			require.NoError(t, err, "create framework")
			t.Log("framework initialized")

			// Configure server
			serverCfg := DefaultServerConfig()
			serverCfg.Memory = "1024M"
			serverCfg.MinFreeMemoryMB = 512
			serverCfg.Version = tt.MCVersion
			serverCfg.GameMode = "creative"
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestRepeatedContainerOpen", tt.MCVersion)
			RequireIntegrationEnv(t, serverCfg)

			// Start server
			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()
			t.Logf("server started: %s:%d", inst.Server.Host, inst.Server.HostServerPort)

			// Setup agent logging
			require.NoError(t, framework.setupAgentLogging(), "setup agent logging")
			defer framework.CloseAgentLog()

			// Spawn agent
			addr := fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort)
			botName := "RepeatBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			defer func() {
				if agent != nil && agent.BotClient() != nil {
					_ = agent.BotClient().Close()
				}
			}()

			// Get components
			screenMgr := agent.ScreenManager()

			botClient := agent.BotClient()
			require.NotNil(t, botClient, "bot client should be available")

			// Wait for player to be online
			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second),
				"agent never appeared in server player list")

			// Get spawn point
			playerPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			spawnPoint := models.V3{
				X: playerPos.X,
				Y: playerPos.Y,
				Z: playerPos.Z,
			}
			t.Logf("spawn point: %+v", spawnPoint)

			// Create container helpers
			itemUsage := items.NewItemUsage(botClient.Conn(), agent.Config.PacketMgr, nil)
			// Set version-specific container handler
			if agent.Config.VersionHandler != nil {
				itemUsage.SetContainerHandler(agent.Config.VersionHandler.Play().Containers())
			}
			// Container helper is auto-initialized during agent.Start()
			// No manual setup needed

			// Place a single chest at fixed integer coordinates
			chestX := int(math.Floor(spawnPoint.X)) + 5
			chestY := int(math.Floor(spawnPoint.Y))
			chestZ := int(math.Floor(spawnPoint.Z))

			_, err = PlaceBlockAndWait(ctx, inst.RCON, agent, models.V3{X: float64(chestX), Y: float64(chestY), Z: float64(chestZ)}, "minecraft:chest", "minecraft:chest", 10*time.Second)
			require.NoError(t, err, "place chest")
			t.Logf("placed chest at (%d, %d, %d)", chestX, chestY, chestZ)

			// Wait for chunk to load on client
			t.Log("waiting for chunks to load...")
			time.Sleep(3 * time.Second)

			// Use center of chest block for position
			chestPos := models.V3{
				X: float64(chestX) + 0.5,
				Y: float64(chestY),
				Z: float64(chestZ) + 0.5,
			}

			// Teleport next to chest
			teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", botName, chestPos.X-2, chestPos.Y, chestPos.Z)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport to chest")
			t.Logf("teleported to (%.1f, %.1f, %.1f)", chestPos.X-2, chestPos.Y, chestPos.Z)
			time.Sleep(1 * time.Second)

			// Attempt to open and close the SAME chest 15 times
			const numAttempts = 15
			successCount := 0

			for i := 1; i <= numAttempts; i++ {
				t.Logf("\n=== Attempt %d/%d ===", i, numAttempts)

				// Check screens before open
				screensBefore := len(screenMgr.Screens())
				t.Logf("Screens before open: %d %v", screensBefore, getScreenIDs(screenMgr.Screens()))

				// Open chest
				windowID, err := OpenContainerWithLOS(ctx, agent.Agent, chestPos, models.FaceEast, 5*time.Second)
				if err != nil {
					t.Logf("❌ Attempt %d FAILED to open: %v", i, err)
					t.Logf("   Server stopped responding after %d successful opens", successCount)
					break
				}

				successCount++
				t.Logf("✅ Attempt %d: Opened chest with window ID %d", i, windowID)

				// Verify it's a chest
				screen, ok := screenMgr.Screens()[int(windowID)]
				require.True(t, ok, "chest window should exist")

				chest, ok := screen.(*mcscreen.Chest)
				require.True(t, ok, "screen should be a chest")
				require.Equal(t, 3, chest.Rows, "should be single chest (3 rows)")

				// Close chest
				err = agent.Agent.CloseContainer()
				require.NoError(t, err, "close chest")
				t.Logf("   Closed window ID %d", windowID)

				// Check screens after close
				screensAfter := len(screenMgr.Screens())
				t.Logf("   Screens after close: %d %v", screensAfter, getScreenIDs(screenMgr.Screens()))

				// Pause between attempts (configurable)
				pauseDuration := 500 * time.Millisecond
				if i < numAttempts {
					t.Logf("   Pausing %v before next attempt...", pauseDuration)
					time.Sleep(pauseDuration)
				}
			}

			// Summary
			t.Logf("\n=== SUMMARY ===")
			t.Logf("Attempted: %d", numAttempts)
			t.Logf("Succeeded: %d", successCount)
			t.Logf("Failed:    %d", numAttempts-successCount)

			if successCount < numAttempts {
				t.Logf("\n⚠️  Server stopped accepting container opens after %d attempts", successCount)
			} else {
				t.Logf("\n✅ All %d attempts succeeded!", numAttempts)
			}
		})
	}
}
