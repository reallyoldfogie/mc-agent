//go:build integration

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

// TestFlatMovementSingleAgent tests that a single agent can navigate using basic movement commands on flat terrain.
// Uses flat-world generation to ensure predictable, non-pathfinding movement.
func TestFlatMovementSingleAgent(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Create framework
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	// Use flat-world configuration for predictable terrain
	serverCfg := FlatWorldServerConfig()
	serverCfg.Version = "1.21.5"
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
	agentCfg.EnablePathfinding = false // IMPORTANT: Test movement WITHOUT pathfinding
	agentCfg.EnableFollowing = false
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_movement_single_%s.mcpr", time.Now().Format("20060102_150405"))

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned on flat world (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	// Wait for agent to join
	time.Sleep(5 * time.Second)

	// Get agent's starting position
	startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get starting position")

	startPos := Position{X: startX, Y: startY, Z: startZ}
	logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startX, startY, startZ)

	// Define destination on the same Y level (10 blocks east)
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

	// Send lineTo command (straight-line for flat world)
	navCmd := fmt.Sprintf("lineTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
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

	// Wait for agent to reach destination
	err = tracker.WaitForPosition(ctx, agent.Name, destination, 1.0, timeout)
	if err != nil {
		finalPos, ok := tracker.GetPosition(agent.Name)
		if ok {
			finalDist := finalPos.Distance(destination)
			logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from target: %.2f)",
				finalPos.X, finalPos.Y, finalPos.Z, finalDist)
		}
		require.NoError(t, err, "agent should reach destination on flat terrain without pathfinding")
	}

	// Verify final position
	finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get final position")

	finalPos := Position{X: finalX, Y: finalY, Z: finalZ}
	finalDistance := finalPos.Distance(destination)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from target: %.2f blocks", finalDistance)
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	assert.LessOrEqual(t, finalDistance, 1.0, "agent should be within 1 block of destination on flat terrain")
}

// TestFlatMovementMultipleDestinations tests navigation to multiple waypoints on flat terrain.
func TestFlatMovementMultipleDestinations(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	serverCfg := FlatWorldServerConfig()
	serverCfg.Version = "1.21.5"

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
	agentCfg.EnablePathfinding = false // Test pure movement without pathfinding
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_waypoints_%s.mcpr", time.Now().Format("20060102_150405"))

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
	waypoints := []Position{
		{X: startX + 10, Y: startY, Z: startZ},      // East
		{X: startX + 10, Y: startY, Z: startZ + 10}, // Southeast
		{X: startX, Y: startY, Z: startZ + 10},      // South
		{X: startX, Y: startY, Z: startZ},           // Back to start
	}

	for i, waypoint := range waypoints {
		logger.Logf("Waypoint %d: %.2f, %.2f, %.2f", i+1, waypoint.X, waypoint.Y, waypoint.Z)

		currentX, currentY, currentZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
		require.NoError(t, err, "get current position")
		current := Position{X: currentX, Y: currentY, Z: currentZ}

		navCmd := fmt.Sprintf("lineTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
		_, err = sayCmd.Exec(ctx)
		require.NoError(t, err, "send navigation command")

		distance := current.Distance(waypoint)
		timeout := CalculateMovementTimeout(distance)
		if timeout < 30*time.Second {
			timeout = 30 * time.Second
		}

		err = tracker.WaitForPosition(ctx, agent.Name, waypoint, 1.0, timeout)
		require.NoError(t, err, "agent should reach waypoint %d on flat terrain", i+1)

		logger.Logf("Reached waypoint %d", i+1)
	}

	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)
}

// TestFlatMovementVertical tests vertical movement on flat terrain.
func TestFlatMovementVertical(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	serverCfg := FlatWorldServerConfig()
	serverCfg.Version = "1.21.5"

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
	agentCfg.EnablePathfinding = false // Test vertical movement without pathfinding
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_vertical_%s.mcpr", time.Now().Format("20060102_150405"))

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

	// Test moveUp command
	upDistance := 3.0
	moveUpCmd := fmt.Sprintf("moveUp %.2f", upDistance)
	sayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, moveUpCmd))
	_, err = sayCmd.Exec(ctx)
	require.NoError(t, err, "send moveUp command")

	logger.Logf("Sent moveUp %.2f command", upDistance)

	// Wait for movement to complete
	time.Sleep(5 * time.Second)

	// Verify vertical movement occurred
	finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get final position")

	yChange := finalY - startY
	logger.Logf("Y position change: %.2f (expected ~%.2f)", yChange, upDistance)

	assert.Greater(t, yChange, upDistance*0.5, "agent should move up at least 50% of requested distance")
	assert.Equal(t, startX, finalX, "X position should remain the same")
	assert.Equal(t, startZ, finalZ, "Z position should remain the same")
}

// TestFlatMovementForwardCommand tests the moveForward command on flat terrain.
func TestFlatMovementForwardCommand(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	serverCfg := FlatWorldServerConfig()
	serverCfg.Version = "1.21.5"

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
	agentCfg.EnablePathfinding = false
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = fmt.Sprintf("./replays/flat_forward_%s.mcpr", time.Now().Format("20060102_150405"))

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	time.Sleep(5 * time.Second)

	// Get starting position
	startX, startY, startZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get starting position")

	startPos := Position{X: startX, Y: startY, Z: startZ}

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

	finalPos := Position{X: finalX, Y: finalY, Z: finalZ}
	moved := startPos.Distance(finalPos)

	logger.Logf("Distance moved: %.2f blocks (requested %.2f)", moved, distance)
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	// Agent should move forward
	assert.Greater(t, moved, distance*0.5, "agent should move forward at least 50% of requested distance")
}
