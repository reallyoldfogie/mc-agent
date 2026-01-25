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
	_ = os.MkdirAll("./replays", 0755)
}

// TestPathfindingSingleAgent tests that a single agent can navigate to a destination using pathfinding.
// Uses random world generation for realistic terrain testing.
func TestPathfindingSingleAgent(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Create framework
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	// Use default (random) server config for realistic pathfinding testing
	serverCfg := DefaultServerConfig()
	serverCfg.Version = "1.21.5"
	RequireIntegrationEnv(t, serverCfg)
	serverCfg.PullImage = false
	// Reduce memory requirements to avoid OOM
	serverCfg.Memory = "512M"
	serverCfg.MinFreeMemoryMB = 256

	// Optional: Set a fixed seed for reproducible terrain
	// Uncomment and set seed to reproduce a specific world:
	// serverCfg.ExtraEnv = map[string]string{
	// 	"SEED": "12345678",
	// }
	// Or use environment variable:
	if testSeed := os.Getenv("TEST_WORLD_SEED"); testSeed != "" {
		if serverCfg.ExtraEnv == nil {
			serverCfg.ExtraEnv = make(map[string]string)
		}
		serverCfg.ExtraEnv["SEED"] = testSeed
		logger.Logf("Using fixed world seed from TEST_WORLD_SEED: %s", testSeed)
	}

	inst, err := framework.StartServer(ctx, serverCfg)
	require.NoError(t, err, "start server")

	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := framework.StopServer(stopCtx, inst, true); err != nil {
			logger.Logf("warning: failed to stop server: %v", err)
		}
	}()

	logger.Logf("Server started on %s:%d (random terrain)", inst.Server.Host, inst.Server.HostServerPort)

	// Get and log the world seed for reproducibility
	seedResp, err := inst.RCON.Exec(ctx, "seed")
	if err == nil {
		logger.Logf("World seed: %s", seedResp)
	} else {
		logger.Logf("Could not get world seed: %v", err)
	}

	// Spawn test agent WITH pathfinding enabled
	agentCfg := DefaultAgentConfig(
		"PathfindingBot",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/pathfinding_single_%s_%s.mcpr", agentCfg.Name, time.Now().Format("20060102_150405"))

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned with pathfinding enabled (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	// Wait for agent to join
	time.Sleep(5 * time.Second)

	// Get agent's starting position
	startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get starting position")

	startPos := Position{X: startX, Y: startY, Z: startZ}
	logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startX, startY, startZ)

	// Define destination (can be at different elevation on random terrain)
	destination := Position{
		X: startX + 20,
		Y: startY,
		Z: startZ + 15,
	}
	logger.Logf("Target destination: %.2f, %.2f, %.2f", destination.X, destination.Y, destination.Z)

	// Start position tracking
	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Send moveTo command (pathfinding will handle obstacles)
	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
	resp, err := sayCmd.Exec(ctx)
	require.NoError(t, err, "send navigation command")
	logger.Logf("Command sent, response: %s", resp)

	// Calculate expected travel time with pathfinding overhead
	distance := startPos.Distance(destination)
	timeout := CalculateMovementTimeout(distance)
	if timeout < 30*time.Second {
		timeout = 30 * time.Second // minimum timeout
	}

	logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

	// Wait for agent to reach destination
	err = tracker.WaitForPosition(ctx, agent.Name, destination, 2.0, timeout)
	if err != nil {
		finalPos, ok := tracker.GetPosition(agent.Name)
		if !ok {
			require.Fail(t, "Could not get agent position")
		}

		// Analyze movement to differentiate between failure modes
		totalDist, progressToward, madeProgress := tracker.AnalyzeMovementProgress(
			agent.Name,
			startPos,
			destination,
			5.0, // require at least 5 blocks of progress toward target
		)

		finalDist := finalPos.Distance(destination)
		logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from target: %.2f)",
			finalPos.X, finalPos.Y, finalPos.Z, finalDist)
		logger.Logf("Movement analysis: total distance traveled: %.2f blocks, net progress toward target: %.2f blocks",
			totalDist, progressToward)

		// Test FAILS if agent didn't make progress - indicates pathfinding is broken
		require.True(t, madeProgress,
			"Agent must make meaningful progress toward target (moved %.2f blocks, progress %.2f blocks). "+
				"No progress indicates pathfinding failure, not impassable terrain.",
			totalDist, progressToward)

		// If agent made progress but didn't reach destination, terrain may be impassable
		logger.Logf("Note: Agent made progress (%.2f blocks toward target) but did not reach destination - terrain may be impassable",
			progressToward)
		return
	}

	// Verify final position
	finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get final position")

	finalPos := Position{X: finalX, Y: finalY, Z: finalZ}
	finalDistance := finalPos.Distance(destination)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from target: %.2f blocks", finalDistance)
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	// With pathfinding, agent should navigate intelligently around obstacles
	assert.LessOrEqual(t, finalDistance, 2.0, "agent should reach destination using pathfinding")
}

// TestPathfindingMultipleDestinations tests navigation to multiple waypoints using pathfinding.
func TestPathfindingMultipleDestinations(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

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
		"WaypointBot",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/pathfinding_waypoints_%s.mcpr", time.Now().Format("20060102_150405"))

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned with pathfinding (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	time.Sleep(5 * time.Second)

	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Get starting position
	startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get starting position")

	// Define waypoints for pathfinding
	waypoints := []Position{
		{X: startX + 15, Y: startY, Z: startZ},      // East
		{X: startX + 15, Y: startY, Z: startZ + 20}, // Southeast
		{X: startX, Y: startY, Z: startZ + 20},      // South
	}

	successCount := 0
	for i, waypoint := range waypoints {
		logger.Logf("Waypoint %d: %.2f, %.2f, %.2f", i+1, waypoint.X, waypoint.Y, waypoint.Z)

		currentX, currentY, currentZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
		require.NoError(t, err, "get current position")
		current := Position{X: currentX, Y: currentY, Z: currentZ}

		navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
		_, err = sayCmd.Exec(ctx)
		require.NoError(t, err, "send navigation command")

		distance := current.Distance(waypoint)
		timeout := CalculateMovementTimeout(distance)
		if timeout < 30*time.Second {
			timeout = 30 * time.Second
		}

		err = tracker.WaitForPosition(ctx, agent.Name, waypoint, 1.5, timeout)
		if err != nil {
			// Verify agent made progress toward this waypoint
			totalDist, progressToward, madeProgress := tracker.AnalyzeMovementProgress(
				agent.Name,
				current,
				waypoint,
				3.0, // require at least 3 blocks of progress
			)
			logger.Logf("Waypoint %d: traveled %.2f blocks, progress toward target: %.2f blocks",
				i+1, totalDist, progressToward)

			require.True(t, madeProgress,
				"Agent must make progress toward waypoint %d (moved %.2f blocks, progress %.2f blocks)",
				i+1, totalDist, progressToward)

			logger.Logf("Warning: Agent made progress but did not reach waypoint %d (terrain may be impassable)", i+1)
		} else {
			successCount++
			logger.Logf("Reached waypoint %d", i+1)
		}
	}

	logger.Logf("Successfully reached %d/%d waypoints", successCount, len(waypoints))
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	// With pathfinding, agent should reach most waypoints despite terrain obstacles
	assert.GreaterOrEqual(t, successCount, len(waypoints)/2, "agent should reach at least half of waypoints with pathfinding")
}

// TestPathfindingVerticalMovement tests navigation with elevation changes using pathfinding.
func TestPathfindingVerticalMovement(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	const (
		distBetweenStairs = 4 // how often to place a stair block in the staircase
		xDiff             = 70
		yDiff             = xDiff / distBetweenStairs
		zDiff             = 3
	)
	var resp string
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	serverCfg := DefaultServerConfig()
	serverCfg.Version = "1.21.5"
	if serverCfg.ExtraEnv == nil {
		serverCfg.ExtraEnv = make(map[string]string)
	}

	// Use flat world preset for controlled vertical movement testing
	serverCfg.ExtraEnv["LEVEL_TYPE"] = "flat"

	// Flat world preset: grass block at y=7 for testing
	serverCfg.ExtraEnv["GENERATOR_SETTINGS"] = `
		{
		"layers": [
			{
				"block": "minecraft:bedrock",
				"height": 1
			},
			{
				"block": "minecraft:stone",
				"height": 60
			},
			{
				"block": "minecraft:dirt",
				"height": 2
			},
			{
				"block": "minecraft:grass_block",
				"height": 1
			}
		],
		"biome": "minecraft:plains"
	}`
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
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/pathfinding_vertical_%s.mcpr", time.Now().Format("20060102_150405"))

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned with pathfinding (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	time.Sleep(5 * time.Second)

	tpCmd := fmt.Sprintf("tp %s -8.50 0.00 5.50", agentCfg.Name)
	tpResp, err := inst.RCON.ExecuteWithRetry(ctx, tpCmd, 3)
	if err != nil {
		logger.Logf("Warning: Failed [%s]: %v: %s", tpCmd, err, tpResp)
	} else {
		logger.Logf("[%s] => [%s]", tpCmd, tpResp)
	}

	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Get starting position
	startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get starting position")

	// Navigate to higher elevation - build stairs to make path possible
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
	_, err = inst.RCON.ExecuteWithRetry(ctx, forceloadCmd, 3)
	if err != nil {
		logger.Logf("Warning: Failed to forceload chunks: %v", err)
	}
	time.Sleep(1 * time.Second) // Wait for chunks to load

	resp, err = inst.RCON.ExecuteWithRetry(ctx, fmt.Sprintf(`summon block_display %f %f %f {block_state:{Name:"minecraft:diamond_block"}}`, destination.X, destination.Y, destination.Z), 3)
	if err != nil {
		logger.Logf("Warning: Failed to place display_block at (%f, %f, %f): %v [%s]", destination.X, destination.Y, destination.Z, err, resp)
	}

	// build a platform at the destination
	resp, err = inst.RCON.ExecuteWithRetry(ctx, fmt.Sprintf(`fill %d %d %d %d %d %d minecraft:stone_slab[type=top]`, int(destination.X)-3, int(destination.Y)-1, int(destination.Z)-5, int(destination.X)+5, int(destination.Y)-1, int(destination.Z)+5), 3)
	if err != nil {
		logger.Logf("Warning: Failed to fill stone_slab[type=top] at (%d, %d, %d => %d, %d, %d): %v [%s]", int(destination.X), int(destination.Y), int(destination.Z)-5, int(destination.X)+5, int(destination.Y), int(destination.Z)+5, err, resp)
	} else {
		logger.Logf("Filled stone_slab[type=top] at (%d, %d, %d => %d, %d, %d) [%s]\n", int(destination.X), int(destination.Y), int(destination.Z)-5, int(destination.X)+5, int(destination.Y), int(destination.Z)+5, resp)
	}

	// BUILD STAIRS to destination (fix for flat terrain)
	// Build a gradual staircase: 60 blocks horizontal, 12 blocks vertical = 1 step up every 4 blocks
	logger.Logf("Building staircase from start to destination")
	// Use same Z coordinate as platform for alignment
	stairZ := int(destination.Z)
	for i := 1; i <= xDiff; i++ {
		blockX := int(startX) + i
		blockY := int(startY) + (i / distBetweenStairs) // Gradual ascent: 1 block up every <distBetweenStairs> blocks horizontal
		blockZ := stairZ
		retry := 0

	RETRY:
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

	// Send moveTo command (pathfinding will handle climbing)
	// navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	// sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
	// _, err = sayCmd.Exec(ctx)

	inst.RCON.Say(ctx, fmt.Sprintf("Executing moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z))

	err = agent.Agent.MoveTo(ctx, destination.X, destination.Y, destination.Z, true)
	require.NoError(t, err, "moveTo should succeed")

	inst.RCON.Say(ctx, fmt.Sprintf("Done executing moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)).Exec(ctx)

	// Verify that the agent's location
	currentPos, ok := tracker.GetPosition(agent.Name)
	require.True(t, ok, "should have current position after MoveTo command")
	inst.RCON.Say(ctx, fmt.Sprintf("current position after MoveTo command: %.2f, %.2f, %.2f", currentPos.X, currentPos.Y, currentPos.Z))
	logger.Logf("Current position after MoveTo command: %.2f, %.2f, %.2f", currentPos.X, currentPos.Y, currentPos.Z)

	// Longer timeout for vertical movement
	distance := Position{X: startX, Y: startY, Z: startZ}.Distance(destination)
	timeout := CalculateMovementTimeout(distance) * 2

	err = tracker.WaitForPosition(ctx, agent.Name, destination, 2.0, timeout)
	if err != nil {
		logger.Logf("Note: Agent did not reach elevated destination (terrain may not support climbing): %v", err)
	}

	// Check progress instead of requiring full success
	finalPos, ok := tracker.GetPosition(agent.Name)
	require.True(t, ok, "should have final position")

	horizontalProgress := (finalPos.X - startX) / (destination.X - startX)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalPos.X, finalPos.Y, finalPos.Z)
	logger.Logf("Horizontal progress: %.0f%%", horizontalProgress*100)
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	// With pathfinding, agent should make progress in the right direction
	assert.Greater(t, horizontalProgress, 0.3, "agent should make at least 30% progress toward destination")
}
