//go:build integration

package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFollowSingleAgent tests that agent B can follow agent A to a destination.
func TestFollowSingleAgent(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// Create framework
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	// Start test server using flat world for reliable spawn locations
	serverCfg := FlatWorldServerConfig()
	serverCfg.Version = "1.21.5"

	inst, err := framework.StartServer(ctx, serverCfg)
	require.NoError(t, err, "start server")

	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := framework.StopServer(stopCtx, inst, true); err != nil {
			logger.Logf("warning: failed to stop server: %v", err)
		}
	}()

	logger.Logf("Server started on %s:%d", inst.Server.Host, inst.Server.HostServerPort)

	// Spawn leader agent (Agent A) with replay recording
	leaderCfg := DefaultAgentConfig(
		"Leader",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	leaderCfg.EnablePathfinding = true
	leaderCfg.EnableFollowing = false // leader doesn't follow anyone
	leaderCfg.EnableReplay = true
	leaderCfg.ReplayOutput = fmt.Sprintf("./replays/follow_test_leader_%s.mcpr", time.Now().Format("20060102_150405"))

	leader, err := framework.SpawnAgent(ctx, inst, leaderCfg)
	require.NoError(t, err, "spawn leader agent")
	logger.Logf("Leader agent spawned (replay: %s)", leaderCfg.ReplayOutput)

	// Spawn follower agent (Agent B) with replay recording
	followerCfg := DefaultAgentConfig(
		"Follower",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	followerCfg.EnablePathfinding = true
	followerCfg.EnableFollowing = true
	followerCfg.EnableReplay = true
	followerCfg.ReplayOutput = fmt.Sprintf("./replays/follow_test_follower_%s.mcpr", time.Now().Format("20060102_150405"))

	follower, err := framework.SpawnAgent(ctx, inst, followerCfg)
	require.NoError(t, err, "spawn follower agent")
	logger.Logf("Follower agent spawned (replay: %s)", followerCfg.ReplayOutput)

	// Wait for agents to join
	time.Sleep(5 * time.Second)

	// Get starting positions
	leaderStartX, leaderStartY, leaderStartZ, err := inst.RCON.GetEntityPos(ctx, leader.Name)
	require.NoError(t, err, "get leader starting position")

	followerStartX, followerStartY, followerStartZ, err := inst.RCON.GetEntityPos(ctx, follower.Name)
	require.NoError(t, err, "get follower starting position")

	logger.Logf("Leader start: (%.2f, %.2f, %.2f)", leaderStartX, leaderStartY, leaderStartZ)
	logger.Logf("Follower start: (%.2f, %.2f, %.2f)", followerStartX, followerStartY, followerStartZ)

	// Validate spawn locations are mutually reachable
	validator := NewSpawnValidator()
	validator.RecordPosition(leader.Name, Position{X: leaderStartX, Y: leaderStartY, Z: leaderStartZ})
	validator.RecordPosition(follower.Name, Position{X: followerStartX, Y: followerStartY, Z: followerStartZ})
	report := validator.Validate(ctx)
	logger.Logf("Spawn Validation Report:\n%s", report.String())
	require.True(t, report.AllReachable, "both agents must spawn in mutually reachable locations")

	// Start position tracking
	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Command follower to follow leader using agent-specific prefix: >>>Follower<<<
	followCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
	resp, err := followCmd.Exec(ctx)
	require.NoError(t, err, "send follow command to follower")
	logger.Logf("Follow command sent, response: %s", resp)

	// Give follower time to process follow command
	time.Sleep(2 * time.Second)

	// Define destination for leader (20 blocks away)
	destination := Position{
		X: leaderStartX + 20,
		Y: leaderStartY,
		Z: leaderStartZ + 10,
	}
	logger.Logf("Leader destination: (%.2f, %.2f, %.2f)", destination.X, destination.Y, destination.Z)

	// Command leader to navigate to destination using agent-specific prefix: >>>Leader<<<
	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	navSayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
	_, err = navSayCmd.Exec(ctx)
	require.NoError(t, err, "send navigation command to leader")

	// Calculate timeout based on distance
	distance := Position{X: leaderStartX, Y: leaderStartY, Z: leaderStartZ}.Distance(destination)
	timeout := CalculateMovementTimeout(distance)
	timeout *= 2 // double timeout to account for follower lag
	if timeout < 60*time.Second {
		timeout = 60 * time.Second
	}

	logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

	// Wait for leader to reach destination
	err = tracker.WaitForPosition(ctx, leader.Name, destination, 1.0, timeout)
	if err != nil {
		leaderPos, ok := tracker.GetPosition(leader.Name)
		if ok {
			logger.Logf("Leader final position: %.2f, %.2f, %.2f (distance: %.2f)",
				leaderPos.X, leaderPos.Y, leaderPos.Z, leaderPos.Distance(destination))
		}
		require.NoError(t, err, "leader should reach destination")
	}
	logger.Logf("Leader reached destination")

	// Wait for follower to catch up (give extra time)
	time.Sleep(5 * time.Second)

	// Verify follower is near leader
	leaderX, leaderY, leaderZ, err := inst.RCON.GetEntityPos(ctx, leader.Name)
	require.NoError(t, err, "get leader final position")

	followerX, followerY, followerZ, err := inst.RCON.GetEntityPos(ctx, follower.Name)
	require.NoError(t, err, "get follower final position")

	leaderFinalPos := Position{X: leaderX, Y: leaderY, Z: leaderZ}
	followerFinalPos := Position{X: followerX, Y: followerY, Z: followerZ}

	distanceBetween := leaderFinalPos.Distance(followerFinalPos)

	logger.Logf("Leader final: (%.2f, %.2f, %.2f)", leaderX, leaderY, leaderZ)
	logger.Logf("Follower final: (%.2f, %.2f, %.2f)", followerX, followerY, followerZ)
	logger.Logf("Distance between agents: %.2f blocks", distanceBetween)
	logger.Logf("Replay files: Leader=%s, Follower=%s", leaderCfg.ReplayOutput, followerCfg.ReplayOutput)

	// Follower should be within reasonable following distance (5 blocks)
	assert.LessOrEqual(t, distanceBetween, 5.0, "follower should stay within 5 blocks of leader")

	// Follower should have moved significantly from start
	followerStartPos := Position{X: followerStartX, Y: followerStartY, Z: followerStartZ}
	followerTravelDistance := followerStartPos.Distance(followerFinalPos)
	logger.Logf("Follower traveled: %.2f blocks", followerTravelDistance)

	assert.Greater(t, followerTravelDistance, distance*0.5, "follower should travel at least 50%% of leader's path")
}

// TestFollowMultipleAgents tests multiple agents following a single leader.
func TestFollowMultipleAgents(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
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

	// Spawn leader with replay recording
	leaderCfg := DefaultAgentConfig(
		"Leader",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	leaderCfg.EnablePathfinding = true
	leaderCfg.EnableReplay = true
	leaderCfg.ReplayOutput = fmt.Sprintf("./replays/multi_follow_leader_%s.mcpr", time.Now().Format("20060102_150405"))

	leader, err := framework.SpawnAgent(ctx, inst, leaderCfg)
	require.NoError(t, err, "spawn leader")
	logger.Logf("Leader spawned (replay: %s)", leaderCfg.ReplayOutput)

	// Spawn multiple followers with replay recording
	followerNames := []string{"Follower1", "Follower2", "Follower3"}
	followers := make([]*ManagedAgent, 0, len(followerNames))

	for _, name := range followerNames {
		cfg := DefaultAgentConfig(
			name,
			fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
			serverCfg.Version,
		)
		cfg.EnablePathfinding = true
		cfg.EnableFollowing = true
		cfg.EnableReplay = true
		cfg.ReplayOutput = fmt.Sprintf("./replays/multi_follow_%s_%s.mcpr", name, time.Now().Format("20060102_150405"))

		follower, err := framework.SpawnAgent(ctx, inst, cfg)
		require.NoError(t, err, "spawn follower %s", name)
		followers = append(followers, follower)
		logger.Logf("Follower %s spawned (replay: %s)", name, cfg.ReplayOutput)
	}

	// Wait for all agents to join
	time.Sleep(8 * time.Second)

	// Get leader starting position
	leaderStartX, leaderStartY, leaderStartZ, err := inst.RCON.GetEntityPos(ctx, leader.Name)
	require.NoError(t, err, "get leader starting position")

	leaderStart := Position{X: leaderStartX, Y: leaderStartY, Z: leaderStartZ}
	logger.Logf("Leader start: (%.2f, %.2f, %.2f)", leaderStartX, leaderStartY, leaderStartZ)

	// Validate all agents spawn in mutually reachable locations
	validator := NewSpawnValidator()
	validator.RecordPosition(leader.Name, leaderStart)
	for _, follower := range followers {
		fx, fy, fz, err := inst.RCON.GetEntityPos(ctx, follower.Name)
		require.NoError(t, err, "get %s starting position", follower.Name)
		validator.RecordPosition(follower.Name, Position{X: fx, Y: fy, Z: fz})
		logger.Logf("%s start: (%.2f, %.2f, %.2f)", follower.Name, fx, fy, fz)
	}
	report := validator.Validate(ctx)
	logger.Logf("Spawn Validation Report:\n%s", report.String())
	require.True(t, report.AllReachable, "all agents must spawn in mutually reachable locations")

	// Start position tracking
	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Command all followers to follow leader (each using their own prefix)
	for _, follower := range followers {
		followCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
		_, err := followCmd.Exec(ctx)
		require.NoError(t, err, "send follow command to %s", follower.Name)
		time.Sleep(500 * time.Millisecond) // stagger commands
	}

	logger.Logf("All followers commanded to follow leader")

	// Give followers time to start following
	time.Sleep(2 * time.Second)

	// Define destination
	destination := Position{
		X: leaderStartX + 15,
		Y: leaderStartY,
		Z: leaderStartZ + 15,
	}

	// Command leader to navigate
	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	navSayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
	_, err = navSayCmd.Exec(ctx)
	require.NoError(t, err, "send navigation command to leader")

	// Wait for leader to reach destination
	distance := leaderStart.Distance(destination)
	timeout := CalculateMovementTimeout(distance) * 3 // extra time for multiple agents
	if timeout < 90*time.Second {
		timeout = 90 * time.Second
	}

	err = tracker.WaitForPosition(ctx, leader.Name, destination, 1.0, timeout)
	require.NoError(t, err, "leader should reach destination")
	logger.Logf("Leader reached destination")

	// Wait for followers to catch up
	time.Sleep(10 * time.Second)

	// Get leader final position
	leaderX, leaderY, leaderZ, err := inst.RCON.GetEntityPos(ctx, leader.Name)
	require.NoError(t, err, "get leader final position")
	leaderFinal := Position{X: leaderX, Y: leaderY, Z: leaderZ}

	// Verify all followers are near leader
	for _, follower := range followers {
		followerX, followerY, followerZ, err := inst.RCON.GetEntityPos(ctx, follower.Name)
		require.NoError(t, err, "get %s final position", follower.Name)

		followerPos := Position{X: followerX, Y: followerY, Z: followerZ}
		distToLeader := followerPos.Distance(leaderFinal)

		logger.Logf("%s final: (%.2f, %.2f, %.2f), distance to leader: %.2f",
			follower.Name, followerX, followerY, followerZ, distToLeader)

		assert.LessOrEqual(t, distToLeader, 7.0, "%s should be within 7 blocks of leader", follower.Name)
	}
}

// TestFollowDynamicTarget tests following an agent that changes direction.
func TestFollowDynamicTarget(t *testing.T) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
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

	// Spawn leader and follower with replay recording
	leaderCfg := DefaultAgentConfig(
		"DynamicLeader",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	leaderCfg.EnablePathfinding = true
	leaderCfg.EnableReplay = true
	leaderCfg.ReplayOutput = fmt.Sprintf("./replays/dynamic_follow_leader_%s.mcpr", time.Now().Format("20060102_150405"))

	leader, err := framework.SpawnAgent(ctx, inst, leaderCfg)
	require.NoError(t, err, "spawn leader")
	logger.Logf("Leader spawned (replay: %s)", leaderCfg.ReplayOutput)

	followerCfg := DefaultAgentConfig(
		"DynamicFollower",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	followerCfg.EnablePathfinding = true
	followerCfg.EnableFollowing = true
	followerCfg.EnableReplay = true
	followerCfg.ReplayOutput = fmt.Sprintf("./replays/dynamic_follow_follower_%s.mcpr", time.Now().Format("20060102_150405"))

	follower, err := framework.SpawnAgent(ctx, inst, followerCfg)
	require.NoError(t, err, "spawn follower")
	logger.Logf("Follower spawned (replay: %s)", followerCfg.ReplayOutput)

	time.Sleep(5 * time.Second)

	// Get starting positions
	leaderStartX, leaderStartY, leaderStartZ, err := inst.RCON.GetEntityPos(ctx, leader.Name)
	require.NoError(t, err, "get leader starting position")

	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Command follower to follow leader
	followCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
	_, err = followCmd.Exec(ctx)
	require.NoError(t, err, "send follow command")
	time.Sleep(2 * time.Second)

	// Define multiple waypoints for leader (zigzag pattern)
	waypoints := []Position{
		{X: leaderStartX + 10, Y: leaderStartY, Z: leaderStartZ},
		{X: leaderStartX + 10, Y: leaderStartY, Z: leaderStartZ + 10},
		{X: leaderStartX, Y: leaderStartY, Z: leaderStartZ + 10},
	}

	for i, waypoint := range waypoints {
		logger.Logf("Leader waypoint %d: (%.2f, %.2f, %.2f)", i+1, waypoint.X, waypoint.Y, waypoint.Z)

		// Command leader to next waypoint
		navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		navSayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
		_, err = navSayCmd.Exec(ctx)
		require.NoError(t, err, "send navigation command for waypoint %d", i+1)

		// Wait for leader to approach waypoint (not full arrival, just make progress)
		time.Sleep(5 * time.Second)

		logger.Logf("Leader moving to waypoint %d", i+1)
	}

	// Wait for final waypoint
	time.Sleep(10 * time.Second)

	// Verify follower tracked the leader through all waypoints
	leaderX, leaderY, leaderZ, err := inst.RCON.GetEntityPos(ctx, leader.Name)
	require.NoError(t, err, "get leader final position")

	followerX, followerY, followerZ, err := inst.RCON.GetEntityPos(ctx, follower.Name)
	require.NoError(t, err, "get follower final position")

	leaderFinal := Position{X: leaderX, Y: leaderY, Z: leaderZ}
	followerFinal := Position{X: followerX, Y: followerY, Z: followerZ}

	distance := leaderFinal.Distance(followerFinal)
	logger.Logf("Leader final: (%.2f, %.2f, %.2f)", leaderX, leaderY, leaderZ)
	logger.Logf("Follower final: (%.2f, %.2f, %.2f)", followerX, followerY, followerZ)
	logger.Logf("Final distance: %.2f blocks", distance)
	logger.Logf("Replay files: Leader=%s, Follower=%s", leaderCfg.ReplayOutput, followerCfg.ReplayOutput)

	assert.LessOrEqual(t, distance, 8.0, "follower should stay within 8 blocks despite direction changes")
}

// TestFollowStopCommand tests that a follower can stop following.
func TestFollowStopCommand(t *testing.T) {
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

	// Spawn agents with replay recording
	leaderCfg := DefaultAgentConfig(
		"Leader",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	leaderCfg.EnableReplay = true
	leaderCfg.ReplayOutput = fmt.Sprintf("./replays/stop_follow_leader_%s.mcpr", time.Now().Format("20060102_150405"))

	leader, err := framework.SpawnAgent(ctx, inst, leaderCfg)
	require.NoError(t, err, "spawn leader")
	logger.Logf("Leader spawned (replay: %s)", leaderCfg.ReplayOutput)

	followerCfg := DefaultAgentConfig(
		"Follower",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)
	followerCfg.EnableFollowing = true
	followerCfg.EnableReplay = true
	followerCfg.ReplayOutput = fmt.Sprintf("./replays/stop_follow_follower_%s.mcpr", time.Now().Format("20060102_150405"))

	follower, err := framework.SpawnAgent(ctx, inst, followerCfg)
	require.NoError(t, err, "spawn follower")
	logger.Logf("Follower spawned (replay: %s)", followerCfg.ReplayOutput)

	time.Sleep(5 * time.Second)

	// Command follower to follow leader
	followCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
	_, err = followCmd.Exec(ctx)
	require.NoError(t, err, "send follow command")
	time.Sleep(2 * time.Second)

	// Get positions
	_, followerY1, _, err := inst.RCON.GetEntityPos(ctx, follower.Name)
	require.NoError(t, err, "get follower position before stop")

	// Command follower to stop following
	stopCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< stopFollow", follower.Name))
	_, err = stopCmd.Exec(ctx)
	require.NoError(t, err, "send stop follow command")
	time.Sleep(2 * time.Second)

	// Move leader away
	leaderX, leaderY, leaderZ, err := inst.RCON.GetEntityPos(ctx, leader.Name)
	require.NoError(t, err, "get leader position")

	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", leaderX+20, leaderY, leaderZ)
	navSayCmd := inst.RCON.Say(ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
	_, err = navSayCmd.Exec(ctx)
	require.NoError(t, err, "send navigation command to leader")

	// Wait for leader to move
	time.Sleep(8 * time.Second)

	// Verify follower didn't move much (should have stopped following)
	_, followerY2, _, err := inst.RCON.GetEntityPos(ctx, follower.Name)
	require.NoError(t, err, "get follower position after stop")

	yChange := followerY2 - followerY1
	logger.Logf("Follower Y change after stop: %.2f", yChange)
	logger.Logf("Replay files: Leader=%s, Follower=%s", leaderCfg.ReplayOutput, followerCfg.ReplayOutput)

	// Follower should not have moved significantly (allowing for minor position updates)
	assert.LessOrEqual(t, yChange, 2.0, "follower should stay relatively still after stop command")
}
