package testing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Create replays directory
	_ = os.MkdirAll("./replays", 0755)
}

// TestNavigationSingleAgent tests that a single agent can navigate to a specified destination.
func TestNavigationSingleAgent(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			logger := NewTestLogger(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			// Create framework
			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			// Start test server
			serverCfg := DefaultServerConfig()
			serverCfg.Version = tt.MCVersion
			serverCfg.PullImage = false // set to true to pull latest image
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")

			// Always cleanup server
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				if err := framework.StopServer(stopCtx, inst, true); err != nil {
					logger.Logf("warning: failed to stop server: %v", err)
				}
			}()

			logger.Logf("Server started on %s:%d", inst.Server.Host, inst.Server.HostServerPort)

			// Spawn test agent with replay recording
			agentCfg := DefaultAgentConfig(
				"TestBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version, fmt.Sprintf("nav_single_%s_%s.mcpr", tt.Name, time.Now().Format("20060102_150405")), agentCfg.Name)

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")

			logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

			// Wait for agent to join (give it time to connect and receive spawn position)
			time.Sleep(5 * time.Second)

			inst.RCON.Say(ctx, "/effect give "+agent.Name+" minecraft:glowing 90 0 true").Exec(ctx)

			// Get agent's starting position
			startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get starting position")

			startPos := models.V3{X: startX, Y: startY, Z: startZ}
			logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startX, startY, startZ)

			// Define destination (10 blocks east on same Y level)
			destination := models.V3{
				X: startX + 10,
				Y: startY,
				Z: startZ,
			}
			logger.Logf("Target destination: %.2f, %.2f, %.2f", destination.X, destination.Y, destination.Z)

			// Start position tracking
			tracker := NewPositionTracker(inst, 500*time.Millisecond)
			tracker.Start(ctx)
			defer tracker.Stop()

			// Command agent to navigate using agent-specific prefix: >>>TestBot<<<
			navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
			sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
			resp, err := sayCmd.Exec(ctx)
			require.NoError(t, err, "send navigation command")
			logger.Logf("Command sent, response: %s", resp)

			// Calculate expected travel time
			distance := startPos.DistanceTo(destination)
			timeout := CalculateMovementTimeout(distance)
			if timeout < 30*time.Second {
				timeout = 30 * time.Second // minimum timeout
			}

			logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

			// Wait for agent to reach destination (tolerance: 1.0 blocks)
			err = tracker.WaitForPosition(ctx, agent.Name, destination, 1.0, timeout)
			if err != nil {
				// Get final position for debugging
				finalPos, ok := tracker.GetPosition(agent.Name)
				if ok {
					finalDist := finalPos.DistanceTo(destination)
					logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from target: %.2f)",
						finalPos.X, finalPos.Y, finalPos.Z, finalDist)
				}
				require.NoError(t, err, "agent should reach destination")
			}

			// Verify final position
			finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get final position")

			finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}
			finalDistance := finalPos.DistanceTo(destination)

			logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
			logger.Logf("Distance from target: %.2f blocks", finalDistance)
			logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

			assert.LessOrEqual(t, finalDistance, 1.0, "agent should be within 1 block of destination")
		})
	}
}

// TestNavigationMultipleDestinations tests navigation to multiple waypoints.
func TestNavigationMultipleDestinations(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			logger := NewTestLogger(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			// Create framework
			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			// Start test server
			serverCfg := DefaultServerConfig()
			serverCfg.Version = tt.MCVersion
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")

			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				if err := framework.StopServer(stopCtx, inst, true); err != nil {
					logger.Logf("warning: failed to stop server: %v", err)
				}
			}()

			// Spawn test agent with replay recording
			agentCfg := DefaultAgentConfig(
				"WaypointBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version, fmt.Sprintf("nav_waypoints_%s_%s.mcpr", tt.Name, time.Now().Format("20060102_150405")), agentCfg.Name)

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

			// Wait for agent to join
			time.Sleep(5 * time.Second)

			// Start position tracking
			tracker := NewPositionTracker(inst, 500*time.Millisecond)
			tracker.Start(ctx)
			defer tracker.Stop()

			// Define waypoints (square pattern)
			startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get starting position")

			waypoints := []models.V3{
				{X: startX + 30, Y: startY, Z: startZ},      // East
				{X: startX + 30, Y: startY, Z: startZ + 30}, // Southeast
				{X: startX, Y: startY, Z: startZ + 30},      // South
				{X: startX, Y: startY, Z: startZ},           // Back to start
			}

			for i, waypoint := range waypoints {
				logger.Logf("Waypoint %d: %.2f, %.2f, %.2f", i+1, waypoint.X, waypoint.Y, waypoint.Z)

				// Get current position
				currentX, currentY, currentZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
				require.NoError(t, err, "get current position")
				current := models.V3{X: currentX, Y: currentY, Z: currentZ}

				// Send navigation command using agent-specific prefix
				navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
				sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
				_, err = sayCmd.Exec(ctx)
				require.NoError(t, err, "send navigation command")

				// Calculate timeout
				distance := current.DistanceTo(waypoint)
				timeout := max(CalculateMovementTimeout(distance), 30*time.Second)

				// Wait for arrival
				err = tracker.WaitForPosition(ctx, agent.Name, waypoint, 1.0, timeout)
				require.NoError(t, err, "agent should reach waypoint %d", i+1)

				logger.Logf("Reached waypoint %d - pausing for %d Seconds", i+1, 3)
				time.Sleep(3 * time.Second) // brief pause between waypoints
			}

			logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)
		})
	}
}

// TestNavigationObstacles tests navigation with obstacles.
func TestNavigationObstacles(t *testing.T) {
	t.Skip("Obstacle testing requires world setup - implement when needed")

	// TODO: Implement

	// This test would:
	// 1. Use RCON SetBlock to create walls/obstacles
	// 2. Command agent to navigate around them
	// 3. Verify pathfinding works correctly
}
