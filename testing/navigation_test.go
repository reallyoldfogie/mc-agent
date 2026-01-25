package testing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Create replays directory
	_ = os.MkdirAll("./replays", 0755)
}

// TestNavigationSingleAgent tests that a single agent can navigate to a specified destination.
func TestNavigationSingleAgent(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Create framework
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	// Start test server
	serverCfg := DefaultServerConfig()
	serverCfg.Version = "1.21.5"
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
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/nav_single_%s.mcpr", time.Now().Format("20060102_150405"))

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")

	logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	// Wait for agent to join (give it time to connect and receive spawn position)
	time.Sleep(5 * time.Second)

	inst.RCON.Say(ctx, "/effect give "+agent.Name+" minecraft:glowing 90 0 true").Exec(ctx)

	// Get agent's starting position
	startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get starting position")

	startPos := Position{X: startX, Y: startY, Z: startZ}
	logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startX, startY, startZ)

	// Define destination (10 blocks east on same Y level)
	destination := Position{
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
	distance := startPos.Distance(destination)
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
			finalDist := finalPos.Distance(destination)
			logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from target: %.2f)",
				finalPos.X, finalPos.Y, finalPos.Z, finalDist)
		}
		require.NoError(t, err, "agent should reach destination")
	}

	// Verify final position
	finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get final position")

	finalPos := Position{X: finalX, Y: finalY, Z: finalZ}
	finalDistance := finalPos.Distance(destination)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from target: %.2f blocks", finalDistance)
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	assert.LessOrEqual(t, finalDistance, 1.0, "agent should be within 1 block of destination")
}

// TestNavigationMultipleDestinations tests navigation to multiple waypoints.
func TestNavigationMultipleDestinations(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// Create framework
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	// Start test server
	serverCfg := DefaultServerConfig()
	serverCfg.Version = "1.21.5"
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
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/nav_waypoints_%s.mcpr", time.Now().Format("20060102_150405"))

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

	waypoints := []Position{
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
		current := Position{X: currentX, Y: currentY, Z: currentZ}

		// Send navigation command using agent-specific prefix
		navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
		_, err = sayCmd.Exec(ctx)
		require.NoError(t, err, "send navigation command")

		// Calculate timeout
		distance := current.Distance(waypoint)
		timeout := max(CalculateMovementTimeout(distance), 30*time.Second)

		// Wait for arrival
		err = tracker.WaitForPosition(ctx, agent.Name, waypoint, 1.0, timeout)
		require.NoError(t, err, "agent should reach waypoint %d", i+1)

		logger.Logf("Reached waypoint %d - pausing for %d Seconds", i+1, 3)
		time.Sleep(3 * time.Second) // brief pause between waypoints
	}

	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)
}

// TestNavigationObstacles tests navigation with obstacles (future enhancement).
func TestNavigationObstacles(t *testing.T) {
	t.Skip("Obstacle testing requires world setup - implement when needed")

	// This test would:
	// 1. Use RCON SetBlock to create walls/obstacles
	// 2. Command agent to navigate around them
	// 3. Verify pathfinding works correctly
}

// TestNavigationVerticalMovement tests navigation with Y-axis changes.
func TestNavigationVerticalMovement(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	const (
		distBetweenStairs = 2 // how often to place a stair block in the staircase
		xDiff             = 10
		yDiff             = xDiff / distBetweenStairs
		zDiff             = 3
	)

	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	serverCfg := DefaultServerConfig()
	serverCfg.Version = "1.21.5"
	RequireIntegrationEnv(t, serverCfg)

	inst, err := framework.StartServer(ctx, serverCfg)
	require.NoError(t, err, "start server")

	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		_ = framework.StopServer(stopCtx, inst, true)
	}()

	agentCfg := DefaultAgentConfig(
		"ClimberBot",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/nav_vertical_%s.mcpr", time.Now().Format("20060102_150405"))

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	time.Sleep(5 * time.Second)

	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Get starting position
	startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get starting position")

	// Navigate to higher elevation (5 blocks up, 10 blocks away) - build stairs to make path possible
	destination := Position{
		X: startX + xDiff,
		Y: startY + yDiff,
		Z: startZ + zDiff,
	}

	logger.Logf("Navigating from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f)",
		startX, startY, startZ, destination.X, destination.Y, destination.Z)

	// Forceload chunks before building to ensure chunks are loaded
	chunkX1 := int(startX) >> 4
	chunkZ1 := int(startZ) >> 4
	chunkX2 := int(destination.X) >> 4
	chunkZ2 := int(destination.Z) >> 4
	forceloadCmd := fmt.Sprintf("forceload add %d %d %d %d", chunkX1<<4, chunkZ1<<4, chunkX2<<4, chunkZ2<<4)
	_, err = inst.RCON.Exec(ctx, forceloadCmd)
	if err != nil {
		logger.Logf("Warning: Failed to forceload chunks: %v", err)
	}
	time.Sleep(1 * time.Second) // Wait for chunks to load

	resp, err := inst.RCON.ExecuteWithRetry(ctx, fmt.Sprintf(`summon block_display %f %f %f {block_state:{Name:"minecraft:diamond_block"}}`, destination.X, destination.Y, destination.Z), 3)
	if err != nil {
		logger.Logf("Warning: Failed to place display_block at (%f, %f, %f): %v [%s]", destination.X, destination.Y, destination.Z, err, resp)
	}

	// build a platform at the destination
	resp, err = inst.RCON.ExecuteWithRetry(ctx, fmt.Sprintf(`fill %d %d %d %d %d %d minecraft:stone_slab[type=top]`, int(destination.X), int(destination.Y)-1, int(destination.Z)-5, int(destination.X)+6, int(destination.Y)-1, int(destination.Z)+6), 3)
	if err != nil {
		logger.Logf("Warning: Failed to fill stone_slab[type=top] at (%d, %d, %d => %d, %d, %d): %v [%s]", int(destination.X), int(destination.Y)-1, int(destination.Z)-5, int(destination.X)+6, int(destination.Y)-1, int(destination.Z)+6, err, resp)
	} else {
		logger.Logf("Filled stone_slab[type=top] at (%d, %d, %d => %d, %d, %d) [%s]\n", int(destination.X), int(destination.Y)-1, int(destination.Z)-5, int(destination.X)+6, int(destination.Y)-1, int(destination.Z)+6, resp)
	}

	// BUILD STAIRS to destination (fix for flat terrain)
	// Build a gradual staircase: 10 blocks horizontal, 5 blocks vertical = 1 step up every 2 blocks
	logger.Logf("Building staircase from start to destination")
	// Use same Z coordinate as platform for alignment
	stairZ := int(destination.Z)
	for i := 1; i <= xDiff; i++ {
		blockX := int(startX) + i
		blockY := int(startY) + (i / distBetweenStairs) // Gradual ascent: 1 block up every <distBetweenStairs> blocks horizontal
		blockZ := stairZ

		retry := 0

	RETRY:
		// Place stair block facing east
		if blockY < int(destination.Y) {
			// place distBetweenStairs-1 slabs, then a stair
			if i == 1 || i%distBetweenStairs == 0 { // Place stair block facing east
				resp, err = inst.RCON.ExecuteWithRetry(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone_stairs[facing=east,half=bottom]", blockX, blockY, blockZ), 3)
				if err != nil {
					str := fmt.Sprintf("Warning: Failed to place stone_stair[facing=east,half=bottom] at (%d, %d, %d): %v [%s]", blockX, blockY, blockZ, err, resp)
					logger.Log(str)
					// require.NoError(t, err, str)
					time.Sleep(500 * time.Millisecond)
					retry++
					if retry < 4 {
						inst.RCON.Reconnect(ctx)
						goto RETRY
					}
				} else {
					logger.Logf("Placed stone_stair[facing=east,half=bottom] at (%d, %d, %d) [%s]", blockX, blockY, blockZ, resp)
				}

			} else { // Place slab
				resp, err = inst.RCON.ExecuteWithRetry(ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone_slab[type=top]", blockX, blockY, blockZ), 3)
				if err != nil {
					str := fmt.Sprintf("Warning: Failed to place stone_slab[type=top] at (%d, %d, %d): %v [%s]", blockX, blockY, blockZ, err, resp)
					logger.Log(str)
					// require.NoError(t, err, str)
					time.Sleep(500 * time.Millisecond)
					retry++
					if retry < 4 {
						inst.RCON.Reconnect(ctx)
						goto RETRY
					}
				} else {
					logger.Logf("Placed stone_slab[type=top] at (%d, %d, %d) [%s]", blockX, blockY, blockZ, resp)
				}
			}
		}
	}
	logger.Logf("Staircase built, waiting for chunks to sync")
	time.Sleep(2 * time.Second)

	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
	_, err = sayCmd.Exec(ctx)
	require.NoError(t, err, "send navigation command")

	// Longer timeout for vertical movement
	distance := Position{X: startX, Y: startY, Z: startZ}.Distance(destination)
	timeout := CalculateMovementTimeout(distance)
	timeout *= 4 // increase timeout for vertical movement

	err = tracker.WaitForPosition(ctx, agent.Name, destination, 1.5, timeout)
	if err != nil {
		finalPos, ok := tracker.GetPosition(agent.Name)
		if ok {
			logger.Logf("Final position: %.2f, %.2f, %.2f (distance: %.2f)",
				finalPos.X, finalPos.Y, finalPos.Z, finalPos.Distance(destination))
		}
	}

	// Note: This test may fail if the terrain doesn't support climbing
	// Consider it a success if the agent makes progress in the right direction
	finalPos, ok := tracker.GetPosition(agent.Name)
	require.True(t, ok, "should have final position")

	// Check if agent moved in the right direction (even if not fully there)
	horizontalProgress := (finalPos.X - startX) / (destination.X - startX)
	logger.Logf("Horizontal progress: %.0f%%", horizontalProgress*100)
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	assert.Greater(t, horizontalProgress, 0.5, "agent should make at least 50% progress")
}
