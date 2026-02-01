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
	_ = os.MkdirAll("./replays", 0755)
}

// TestFlatMovementSingleAgent tests that a single agent can navigate using basic movement commands on flat terrain.
// Uses flat-world generation to ensure predictable, non-pathfinding movement.
func TestFlatMovementSingleAgent(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewTestLogger(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			// Create framework
			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			// Use flat-world configuration for predictable terrain
			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.mcVersion
			RequireIntegrationEnv(t, serverCfg)
			serverCfg.PullImage = false

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")

			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				if err := framework.StopServer(stopCtx, inst, true); err != nil {
					logger.Logf("warning: failed to stop server: %v", err)
				}
			}()

			logger.Logf("Server started on %s:%d (flat world)", inst.Server.Host, inst.Server.HostServerPort)

			// Spawn test agent without pathfinding (testing basic movement commands)
			agentCfg := DefaultAgentConfig(
				"FlatMovementBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_movement_single_%s_%s.mcpr", tt.name, time.Now().Format("20060102_150405"))

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			logger.Logf("Agent %s spawned on flat world (replay: %s)", agent.Name, agentCfg.ReplayOutput)

			// Wait for agent to join
			time.Sleep(5 * time.Second)

			// Get agent's starting position
			startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get starting position")

			startPos := models.V3{X: startX, Y: startY, Z: startZ}
			logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startX, startY, startZ)

			// Define destination on the same Y level (10 blocks east)
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

			// Send lineTo command (straight-line for flat world)
			navCmd := fmt.Sprintf("lineTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
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

			// Wait for agent to reach destination
			err = tracker.WaitForPosition(ctx, agent.Name, destination, 1.0, timeout)
			if err != nil {
				finalPos, ok := tracker.GetPosition(agent.Name)
				if ok {
					finalDist := finalPos.DistanceTo(destination)
					logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from target: %.2f)",
						finalPos.X, finalPos.Y, finalPos.Z, finalDist)
				}
				require.NoError(t, err, "agent should reach destination on flat terrain without pathfinding")
			}

			// Verify final position
			finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get final position")

			finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}
			finalDistance := finalPos.DistanceTo(destination)

			logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
			logger.Logf("Distance from target: %.2f blocks", finalDistance)
			logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

			assert.LessOrEqual(t, finalDistance, 1.0, "agent should be within 1 block of destination on flat terrain")
		})
	}
}

// TestFlatMovementMultipleDestinations tests navigation to multiple waypoints on flat terrain.
func TestFlatMovementMultipleDestinations(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewTestLogger(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.mcVersion
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")

			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			agentCfg := DefaultAgentConfig(
				"WaypointBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_waypoints_%s_%s.mcpr", tt.name, time.Now().Format("20060102_150405"))

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			logger.Logf("Agent %s spawned on flat world (replay: %s)", agent.Name, agentCfg.ReplayOutput)

			time.Sleep(5 * time.Second)

			tracker := NewPositionTracker(inst, 500*time.Millisecond)
			tracker.Start(ctx)
			defer tracker.Stop()

			// Get starting position
			startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get starting position")

			// Define waypoints (square pattern on flat terrain at same Y level)
			waypoints := []models.V3{
				{X: startX + 10, Y: startY, Z: startZ},      // East
				{X: startX + 10, Y: startY, Z: startZ + 10}, // Southeast
				{X: startX, Y: startY, Z: startZ + 10},      // South
				{X: startX, Y: startY, Z: startZ},           // Back to start
			}

			for i, waypoint := range waypoints {
				logger.Logf("Waypoint %d: %.2f, %.2f, %.2f", i+1, waypoint.X, waypoint.Y, waypoint.Z)

				currentX, currentY, currentZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
				require.NoError(t, err, "get current position")
				current := models.V3{X: currentX, Y: currentY, Z: currentZ}

				navCmd := fmt.Sprintf("lineTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
				sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
				_, err = sayCmd.Exec(ctx)
				require.NoError(t, err, "send navigation command")

				distance := current.DistanceTo(waypoint)
				timeout := CalculateMovementTimeout(distance)
				if timeout < 30*time.Second {
					timeout = 30 * time.Second
				}

				err = tracker.WaitForPosition(ctx, agent.Name, waypoint, 1.0, timeout)
				require.NoError(t, err, "agent should reach waypoint %d on flat terrain", i+1)

				logger.Logf("Reached waypoint %d", i+1)
			}

			logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)
		})
	}
}

// TestFlatMovementVertical tests vertical movement on flat terrain.
func TestFlatMovementVertical(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewTestLogger(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.mcVersion
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")

			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			agentCfg := DefaultAgentConfig(
				"VerticalBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_vertical_%s_%s.mcpr", tt.name, time.Now().Format("20060102_150405"))

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			logger.Logf("Agent %s spawned on flat world (replay: %s)", agent.Name, agentCfg.ReplayOutput)

			time.Sleep(5 * time.Second)

			tracker := NewPositionTracker(inst, 500*time.Millisecond)
			tracker.Start(ctx)
			defer tracker.Stop()

			// Get starting position
			startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get starting position")

			// Forceload chunks before building to ensure chunks are loaded
			chunkX := int(startX) >> 4
			chunkZ := int(startZ) >> 4
			forceloadCmd := fmt.Sprintf("forceload add %d %d %d %d", (chunkX-1)<<4, (chunkZ-1)<<4, (chunkX+1)<<4, (chunkZ+1)<<4)
			_, err = inst.RCON.Exec(ctx, forceloadCmd)
			if err != nil {
				logger.Logf("Warning: Failed to forceload chunks: %v", err)
			}
			time.Sleep(1 * time.Second) // Wait for chunks to load

			// BUILD LADDER for vertical movement (fix for flat terrain)
			logger.Logf("Building ladder at agent position for vertical movement test")
			ladderHeight := 5
			// Build backing wall for ladder
			for y := int(startY); y <= int(startY)+ladderHeight; y++ {
				_, err := inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", int(startX)+1, y, int(startZ)))
				if err != nil {
					logger.Logf("Warning: Failed to place backing wall at (%d, %d, %d): %v", int(startX)+1, y, int(startZ), err)
				}
			}
			// Place ladders on the wall
			for y := int(startY); y <= int(startY)+ladderHeight; y++ {
				_, err := inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:ladder[facing=west]", int(startX), y, int(startZ)))
				if err != nil {
					logger.Logf("Warning: Failed to place ladder at (%d, %d, %d): %v", int(startX), y, int(startZ), err)
				}
			}
			logger.Logf("Ladder built, waiting for chunks to sync")
			time.Sleep(2 * time.Second)

			// Teleport agent to the center of the ladder block to ensure proper positioning
			ladderCenterX := float64(int(startX)) + 0.5
			ladderCenterZ := float64(int(startZ)) + 0.5
			tpCmd := fmt.Sprintf("tp %s %.2f %.2f %.2f", agent.Name, ladderCenterX, startY, ladderCenterZ)
			_, err = inst.RCON.Exec(ctx, tpCmd)
			if err != nil {
				logger.Logf("Warning: Failed to teleport agent to ladder center: %v", err)
			}
			logger.Logf("Teleported agent to ladder center (%.2f, %.2f, %.2f)", ladderCenterX, startY, ladderCenterZ)
			time.Sleep(500 * time.Millisecond)

			// Test moveToAndSneak command - uses pathfinding to climb ladder and holds position with sneak
			upDistance := 3.0
			targetY := startY + upDistance
			moveCmd := fmt.Sprintf("moveToAndSneak %.2f %.2f %.2f", ladderCenterX, targetY, ladderCenterZ)
			sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, moveCmd))
			_, err = sayCmd.Exec(ctx)
			require.NoError(t, err, "send moveToAndSneak command")

			logger.Logf("Sent moveToAndSneak to (%.2f, %.2f, %.2f)", ladderCenterX, targetY, ladderCenterZ)

			// Wait for movement to complete
			time.Sleep(5 * time.Second)

			// Verify vertical movement occurred
			finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get final position")

			yChange := finalY - startY
			logger.Logf("Y position change: %.2f (expected ~%.2f)", yChange, upDistance)

			assert.Greater(t, yChange, upDistance*0.5, "agent should move up at least 50% of requested distance")
			// Allow some X/Z drift during ladder climbing - this happens in vanilla Minecraft
			// when the player isn't perfectly perpendicular to the ladder
			assert.InDelta(t, ladderCenterX, finalX, 0.25, "X position should remain near ladder center")
			assert.InDelta(t, ladderCenterZ, finalZ, 0.25, "Z position should remain near ladder center")
		})
	}
}

// TestLongLadderClimbAndHold tests climbing a long ladder (25 blocks) and holding position
// near the top. This validates that:
// 1. The agent can climb long distances without falling off due to drift
// 2. The sneak-to-hold-position logic works correctly for extended periods
// 3. The agent doesn't fall after reaching the goal on the ladder
func TestLongLadderClimbAndHold(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewTestLogger(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.mcVersion
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")

			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			agentCfg := DefaultAgentConfig(
				"LongClimbBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = fmt.Sprintf("./replays/long_ladder_climb_%s_%s.mcpr", tt.name, time.Now().Format("20060102_150405"))

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

			time.Sleep(5 * time.Second)

			// Get starting position
			startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get starting position")
			logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startX, startY, startZ)

			// Forceload chunks before building
			chunkX := int(startX) >> 4
			chunkZ := int(startZ) >> 4
			forceloadCmd := fmt.Sprintf("forceload add %d %d %d %d", (chunkX-1)<<4, (chunkZ-1)<<4, (chunkX+1)<<4, (chunkZ+1)<<4)
			_, err = inst.RCON.Exec(ctx, forceloadCmd)
			if err != nil {
				logger.Logf("Warning: Failed to forceload chunks: %v", err)
			}
			time.Sleep(1 * time.Second)

			// Build a tall ladder (25 blocks)
			ladderHeight := 25
			logger.Logf("Building %d-block tall ladder", ladderHeight)

			// Build backing wall for ladder
			for y := int(startY); y <= int(startY)+ladderHeight; y++ {
				_, err := inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", int(startX)+1, y, int(startZ)))
				if err != nil {
					logger.Logf("Warning: Failed to place backing wall at Y=%d: %v", y, err)
				}
			}

			// Place ladders on the wall
			for y := int(startY); y <= int(startY)+ladderHeight; y++ {
				_, err := inst.RCON.Exec(ctx, fmt.Sprintf("setblock %d %d %d minecraft:ladder[facing=west]", int(startX), y, int(startZ)))
				if err != nil {
					logger.Logf("Warning: Failed to place ladder at Y=%d: %v", y, err)
				}
			}
			logger.Logf("Ladder built, waiting for chunks to sync")
			time.Sleep(2 * time.Second)

			// Teleport agent to the center of the ladder block
			ladderCenterX := float64(int(startX)) + 0.5
			ladderCenterZ := float64(int(startZ)) + 0.5
			tpCmd := fmt.Sprintf("tp %s %.2f %.2f %.2f", agent.Name, ladderCenterX, startY, ladderCenterZ)
			_, err = inst.RCON.Exec(ctx, tpCmd)
			require.NoError(t, err, "teleport agent to ladder")
			logger.Logf("Teleported agent to ladder center")
			time.Sleep(500 * time.Millisecond)

			// Target is near the top of the ladder (leaving 2 blocks headroom)
			targetY := startY + float64(ladderHeight) - 2.0
			logger.Logf("Target Y: %.2f (ladder top minus 2 blocks)", targetY)

			// Send moveToAndSneak command - goal is ON the ladder
			moveCmd := fmt.Sprintf("moveToAndSneak %.2f %.2f %.2f", ladderCenterX, targetY, ladderCenterZ)
			sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, moveCmd))
			_, err = sayCmd.Exec(ctx)
			require.NoError(t, err, "send moveToAndSneak command")
			logger.Logf("Sent moveToAndSneak to (%.2f, %.2f, %.2f)", ladderCenterX, targetY, ladderCenterZ)

			// Wait for movement to complete (longer for tall ladder)
			// At ~0.15 blocks/tick effective climb speed, 23 blocks takes ~150 ticks = 7.5 seconds
			// Add buffer for pathfinding, physics, and any drift corrections
			time.Sleep(30 * time.Second)

			// Get position after climbing
			climbX, climbY, climbZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get position after climb")
			logger.Logf("models.V3 after climb: %.2f, %.2f, %.2f", climbX, climbY, climbZ)

			climbHeight := climbY - startY
			logger.Logf("Climb height achieved: %.2f blocks (target: %.2f)", climbHeight, targetY-startY)

			// Verify agent climbed most of the way
			expectedClimb := targetY - startY
			assert.Greater(t, climbHeight, expectedClimb*0.8, "agent should climb at least 80%% of target height")

			// Now wait additional time to verify agent HOLDS position (doesn't fall)
			logger.Logf("Waiting 10 seconds to verify agent holds position...")
			time.Sleep(10 * time.Second)

			// Get final position
			finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get final position")
			logger.Logf("Final position after hold: %.2f, %.2f, %.2f", finalX, finalY, finalZ)

			// Agent should not have fallen significantly (allow 0.5 block tolerance for minor physics adjustments)
			yDrop := climbY - finalY
			logger.Logf("Y drop during hold period: %.2f blocks", yDrop)
			assert.LessOrEqual(t, yDrop, 0.5, "agent should hold position on ladder (not fall more than 0.5 blocks)")

			// Final height should still be significant
			finalHeight := finalY - startY
			logger.Logf("Final height from start: %.2f blocks", finalHeight)
			assert.Greater(t, finalHeight, expectedClimb*0.7, "agent should maintain at least 70%% of target height after hold period")

			// X/Z drift should be reasonable (within ladder block)
			assert.InDelta(t, ladderCenterX, finalX, 0.5, "X position should remain within ladder area")
			assert.InDelta(t, ladderCenterZ, finalZ, 0.5, "Z position should remain within ladder area")

			logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)
		})
	}
}

// TestFlatMovementForwardCommand tests the moveForward command on flat terrain.
func TestFlatMovementForwardCommand(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewTestLogger(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			framework, err := NewFramework()
			require.NoError(t, err, "create framework")

			serverCfg := FlatWorldServerConfig()
			serverCfg.Version = tt.mcVersion
			RequireIntegrationEnv(t, serverCfg)

			inst, err := framework.StartServer(ctx, serverCfg)
			require.NoError(t, err, "start server")

			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = framework.StopServer(stopCtx, inst, true)
			}()

			agentCfg := DefaultAgentConfig(
				"ForwardBot",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				serverCfg.Version,
			)
			agentCfg.EnableReplay = true
			agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_forward_%s_%s.mcpr", tt.name, time.Now().Format("20060102_150405"))

			// Version handler is auto-detected by the framework

			agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
			require.NoError(t, err, "spawn agent")
			logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

			time.Sleep(5 * time.Second)

			// Get starting position
			startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get starting position")

			startPos := models.V3{X: startX, Y: startY, Z: startZ}

			// Send moveForward command
			distance := 5.0
			moveCmd := fmt.Sprintf("moveForward %.2f", distance)
			sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, moveCmd))
			_, err = sayCmd.Exec(ctx)
			require.NoError(t, err, "send moveForward command")

			logger.Logf("Sent moveForward %.2f command", distance)

			// Wait for movement
			time.Sleep(5 * time.Second)

			// Get final position
			finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
			require.NoError(t, err, "get final position")

			finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}
			moved := startPos.DistanceTo(finalPos)

			logger.Logf("Distance moved: %.2f blocks (requested %.2f)", moved, distance)
			logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

			// Agent should move forward
			assert.Greater(t, moved, distance*0.5, "agent should move forward at least 50% of requested distance")
		})
	}
}
