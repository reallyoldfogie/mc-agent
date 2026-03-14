package testing

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

func TestWaterFlow_LinearFlow(t *testing.T) {
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
			serverCfg.WorldGen = WorldGenFlat
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestWaterFlow_LinearFlow", tt.MCVersion)
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
			botName := "WaterFlowBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second), "agent never appeared in server player list")

			defer func() {
				if managedAgent == nil {
					return
				}
				// Stop agent with timeout in goroutine (non-blocking cleanup)
				stopCh := make(chan error, 1)
				go func() {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer stopCancel()
					stopCh <- managedAgent.Stop(stopCtx)
				}()

				select {
				case <-stopCh:
					// Agent stopped successfully
				case <-time.After(11 * time.Second):
					// Agent stop timed out - just continue, don't block
					t.Logf("WARNING: Agent %s stop timed out, continuing cleanup", managedAgent.Name)
				}
			}()

			// Get player starting position
			startPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player starting position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

			// Create a water stream flowing in +Z direction (south)
			// Source block at (startX, startY+1, startZ)
			// Flowing water blocks at (startX, startY+1, startZ+1) through (startZ+5)
			waterStartX := int(math.Floor(startPos.X))
			waterY := int(math.Floor(startPos.Y)) + 1
			waterStartZ := int(math.Floor(startPos.Z))

			t.Logf("creating water stream at X=%d Y=%d starting Z=%d", waterStartX, waterY, waterStartZ)

			// Place solid blocks to direct flow: place stone at -Z to prevent flow north
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterStartX, waterY, waterStartZ-1))
			require.NoError(t, err, "place stone to direct flow")

			// Place source block
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterStartX, waterY, waterStartZ))
			require.NoError(t, err, "place water source")

			// Place flowing water blocks in +Z direction
			// for z := waterStartZ + 1; z <= waterStartZ+5; z++ {
			// 	_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterStartX, waterY, z))
			// 	require.NoError(t, err, fmt.Sprintf("place water at z=%d", z))
			// }

			// Place stone blocks on sides to keep flow directional
			for z := waterStartZ; z <= waterStartZ+5; z++ {
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterStartX+1, waterY, z))
				require.NoError(t, err, fmt.Sprintf("place stone at x+1 z=%d", z))
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterStartX-1, waterY, z))
				require.NoError(t, err, fmt.Sprintf("place stone at x-1 z=%d", z))
			}

			time.Sleep(1 * time.Second) // Let server process blocks

			// Teleport agent into the flowing water
			teleportZ := waterStartZ + 2
			teleportCmd := fmt.Sprintf("tp %s %d.5 %d.5 %d.5", botName, waterStartX, waterY, teleportZ)
			t.Logf("teleporting agent: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(500 * time.Millisecond)

			// Record position before water movement
			beforePos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position before water movement")
			t.Logf("position before water movement: (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

			// Wait for agent to be pushed by water current
			time.Sleep(5 * time.Second)

			// Record position after water movement
			afterPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position after water movement")
			t.Logf("position after water movement: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

			// Check that agent moved in +Z direction (toward higher Z values, which is "south" in Minecraft)
			zDisplacement := afterPos.Z - beforePos.Z
			t.Logf("Z displacement: %.2f", zDisplacement)
			require.Greater(t, zDisplacement, 0.01, "agent should move south (positive Z) in water current")
		})
	}
}

func TestWaterFlow_SourceSurroundedByLevel1(t *testing.T) {
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
			serverCfg.WorldGen = WorldGenFlat
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestWaterFlow_SourceSurroundedByLevel1", tt.MCVersion)
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
			botName := "WaterSourceBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second), "agent never appeared in server player list")

			defer func() {
				if managedAgent == nil {
					return
				}
				stopCh := make(chan error, 1)
				go func() {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer stopCancel()
					stopCh <- managedAgent.Stop(stopCtx)
				}()
				select {
				case <-stopCh:
				case <-time.After(11 * time.Second):
					t.Logf("WARNING: Agent %s stop timed out, continuing cleanup", managedAgent.Name)
				}
			}()

			startPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player starting position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

			waterX := int(math.Floor(startPos.X))
			waterY := int(math.Floor(startPos.Y)) + 1
			waterZ := int(math.Floor(startPos.Z))

			t.Logf("creating source block surrounded by level 1 water at X=%d Y=%d Z=%d", waterX, waterY, waterZ)

			// Place source block in center
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterY, waterZ))
			require.NoError(t, err, "place water source")

			// Place flowing water blocks around it (they will be level 1)
			neighbors := [][3]int{
				{waterX + 1, waterY, waterZ},
				{waterX - 1, waterY, waterZ},
				{waterX, waterY, waterZ + 1},
				{waterX, waterY, waterZ - 1},
			}
			for _, n := range neighbors {
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", n[0], n[1], n[2]))
				require.NoError(t, err, fmt.Sprintf("place water at (%d,%d,%d)", n[0], n[1], n[2]))
			}
			time.Sleep(1 * time.Second)

			// Teleport agent to center (in source block)
			teleportCmd := fmt.Sprintf("tp %s %d.5 %d.5 %d.5", botName, waterX, waterY, waterZ)
			t.Logf("teleporting agent: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(500 * time.Millisecond)

			beforePos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position before water movement")
			t.Logf("position before: (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

			time.Sleep(5 * time.Second)

			afterPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position after water movement")
			t.Logf("position after: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

			// Should not move significantly (in source block, no net flow direction)
			totalDisplacement := math.Abs(afterPos.X-beforePos.X) + math.Abs(afterPos.Z-beforePos.Z)
			t.Logf("total horizontal displacement: %.2f", totalDisplacement)
			require.Less(t, totalDisplacement, 0.5, "agent in source block should not move significantly")
		})
	}
}

func TestWaterFlow_MixedLevels(t *testing.T) {
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
			serverCfg.WorldGen = WorldGenFlat
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestWaterFlow_MixedLevels", tt.MCVersion)
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
			botName := "WaterMixedBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second), "agent never appeared in server player list")

			defer func() {
				if managedAgent == nil {
					return
				}
				// Stop agent with timeout in goroutine (non-blocking cleanup)
				stopCh := make(chan error, 1)
				go func() {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer stopCancel()
					stopCh <- managedAgent.Stop(stopCtx)
				}()

				select {
				case <-stopCh:
					// Agent stopped successfully
				case <-time.After(11 * time.Second):
					// Agent stop timed out - just continue, don't block
					t.Logf("WARNING: Agent %s stop timed out, continuing cleanup", managedAgent.Name)
				}
			}()

			startPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player starting position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

			// Create water at different heights so agent straddles multiple levels
			// Place source at Y+1, flowing water continuing at Y+2 (higher up)
			waterX := int(math.Floor(startPos.X))
			waterLowY := int(math.Floor(startPos.Y)) + 1  // Agent's feet will be at Y+1.5 (in this level)
			waterHighY := int(math.Floor(startPos.Y)) + 2 // Agent's head will be at Y+1.5-2.5 (in this level)
			waterZ := int(math.Floor(startPos.Z))

			t.Logf("creating mixed-level water: low at Y=%d, high at Y=%d", waterLowY, waterHighY)

			// Place source block at low Y
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterHighY, waterZ))
			require.NoError(t, err, "place water source at low Y")

			// Place stone barriers to direct lateral flow
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX, waterHighY, waterZ-1))
			require.NoError(t, err, fmt.Sprintf("place barrier at x+1 low Y z=%d", waterZ-1))
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX, waterLowY, waterZ-1))
			require.NoError(t, err, fmt.Sprintf("place barrier at x+1 low Y z=%d", waterZ-1))

			for z := waterZ; z <= waterZ+5; z++ {
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX+1, waterLowY, z))
				require.NoError(t, err, fmt.Sprintf("place barrier at x+1 low Y z=%d", z))
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterLowY, z))
				require.NoError(t, err, fmt.Sprintf("place barrier at x-1 low Y z=%d", z))
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX+1, waterHighY, z))
				require.NoError(t, err, fmt.Sprintf("place barrier at x+1 high Y z=%d", z))
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterHighY, z))
				require.NoError(t, err, fmt.Sprintf("place barrier at x-1 high Y z=%d", z))
			}

			time.Sleep(1 * time.Second)

			LogBlocksInArea(context.Background(), t, inst.RCON, managedAgent, models.V3{X: float64(waterX), Y: float64(waterLowY), Z: float64(waterZ)}, 8)

			// Teleport agent to straddle the water levels (feet in low water, head in high water)
			teleportCmd := fmt.Sprintf("tp %s %.1f %.1f %.1f", botName, float64(waterX)+0.5, float64(waterLowY)+0.6, float64(waterZ)+1.5)
			t.Logf("teleporting agent to straddle water levels: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(500 * time.Millisecond)

			beforePos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position before water movement")
			t.Logf("position before (straddling): (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

			time.Sleep(5 * time.Second)

			afterPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position after water movement")
			t.Logf("position after: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

			// Agent should move in +Z direction from combined water flow (both levels flow in same direction)
			zDisplacement := afterPos.Z - beforePos.Z
			t.Logf("Z displacement: %.2f", zDisplacement)
			require.Greater(t, zDisplacement, 0.01, "agent straddling multiple water levels should move in flow direction")
		})
	}
}

func TestWaterFlow_DiagonalFlow(t *testing.T) {
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
			serverCfg.WorldGen = WorldGenFlat
			serverCfg.ExtraEnv = map[string]string{
				"FORCE_GAMEMODE": "true",
			}
			serverCfg.PullImage = false
			serverCfg.CacheDir = filepath.Join(cwd, ".server_cache", "TestWaterFlow_DiagonalFlow", tt.MCVersion)
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
			botName := "WaterDiagonalBot"
			agentCfg := AgentConfig{
				Name:          botName,
				ServerAddress: addr,
				Version:       serverCfg.Version,
			}

			managedAgent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			require.True(t, waitForPlayerOnline(ctx, inst.RCON, botName, 30*time.Second), "agent never appeared in server player list")

			defer func() {
				if managedAgent == nil {
					return
				}
				stopCh := make(chan error, 1)
				go func() {
					stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer stopCancel()
					stopCh <- managedAgent.Stop(stopCtx)
				}()
				select {
				case <-stopCh:
				case <-time.After(11 * time.Second):
					t.Logf("WARNING: Agent %s stop timed out, continuing cleanup", managedAgent.Name)
				}
			}()

			startPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get player position")
			t.Logf("player starting position: (%.2f, %.2f, %.2f)", startPos.X, startPos.Y, startPos.Z)

			// Create water flowing diagonally (+X and +Z)
			waterX := int(math.Floor(startPos.X))
			waterY := int(math.Floor(startPos.Y)) + 1
			waterZ := int(math.Floor(startPos.Z))

			t.Logf("creating diagonal water flow at X=%d Y=%d Z=%d", waterX, waterY, waterZ)

			// Place water source
			// _, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterY, waterZ))
			// require.NoError(t, err, "place water source")

			// // Create diagonal flow: place water blocks going both +X and +Z
			// // This creates a gradient that should push the agent diagonally
			// for i := 1; i <= 4; i++ {
			// 	_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterX+i, waterY, waterZ))
			// 	require.NoError(t, err, fmt.Sprintf("place water at +X z=%d", i))
			// 	_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterY, waterZ+i))
			// 	require.NoError(t, err, fmt.Sprintf("place water at +Z z=%d", i))
			// 	// Also place at diagonal
			// 	_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterX+i, waterY, waterZ+i))
			// 	require.NoError(t, err, fmt.Sprintf("place water at diagonal z=%d", i))
			// }

			_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d water", waterX, waterY+1, waterZ))
			require.NoError(t, err, "place water source")

			// Place barriers on -X and -Z sides to keep flow directional
			for z := waterZ; z <= waterZ+9; z++ {
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterY, z))
				require.NoError(t, err, fmt.Sprintf("place barrier at x-1 z=%d", z))
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", waterX-1, waterY+1, z))
				require.NoError(t, err, fmt.Sprintf("place barrier at x-1 z=%d", z))
			}

			for x := waterX; x <= waterX+9; x++ {
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", x, waterY, waterZ-1))
				require.NoError(t, err, fmt.Sprintf("place barrier at x=%d z-1", x))
				_, err = inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", x, waterY+1, waterZ-1))
				require.NoError(t, err, fmt.Sprintf("place barrier at x=%d z-1", x))
			}

			time.Sleep(1 * time.Second)

			// Teleport agent into the diagonal flow
			teleportCmd := fmt.Sprintf("tp %s %d.5 %d.5 %d.5", botName, waterX+1, waterY, waterZ+1)
			t.Logf("teleporting agent to diagonal flow: %s", teleportCmd)
			_, err = inst.RCON.Exec(ctx, teleportCmd)
			require.NoError(t, err, "teleport player")
			time.Sleep(500 * time.Millisecond)

			beforePos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position before water movement")
			t.Logf("position before: (%.2f, %.2f, %.2f)", beforePos.X, beforePos.Y, beforePos.Z)

			time.Sleep(5 * time.Second)

			afterPos, err := GetPlayerPosition(ctx, inst.RCON, botName)
			require.NoError(t, err, "get position after water movement")
			t.Logf("position after: (%.2f, %.2f, %.2f)", afterPos.X, afterPos.Y, afterPos.Z)

			// Agent should move diagonally (both +X and +Z)
			xDisplacement := afterPos.X - beforePos.X
			zDisplacement := afterPos.Z - beforePos.Z
			t.Logf("X displacement: %.2f, Z displacement: %.2f", xDisplacement, zDisplacement)

			// Both should be positive (moving in +X and +Z direction)
			require.Greater(t, xDisplacement, 0.01, "agent should move east (+X) in diagonal flow")
			require.Greater(t, zDisplacement, 0.01, "agent should move south (+Z) in diagonal flow")
		})
	}
}
