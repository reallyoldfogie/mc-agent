package testing

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func init() {
	// Create replays directory
	cacheDir, err := utils.FindOrCreateCacheDir()
	if err != nil {
		panic(fmt.Sprintf("find cache directory: %v", err))
	}
	_ = os.MkdirAll(filepath.Join(cacheDir, "replays"), 0755)
}

// NavigationRandomSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for random-terrain navigation tests: one
// server per version, shared by every test method below, instead of the
// previous per-test-function StartServer/StopServer pattern. World gen is
// WorldGenRandom (the package default) because these tests specifically
// exercise pathfinding over generated terrain - see NavigationFlatSuite in
// navigation_flat_test.go for the flat-world counterpart that shares a
// *different* server (world gen is fixed per suite instance, see
// VersionWorldSuite's own doc comment on why flat and random tests can't
// share one server).
type NavigationRandomSuite struct {
	VersionWorldSuite
}

func TestNavigationRandomSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &NavigationRandomSuite{}
		s.WorldGen = WorldGenRandom
		return s
	})
}

// TestSingleAgent tests that a single agent can navigate to a specified
// destination. Equivalent to the pre-Phase-1 TestNavigationSingleAgent.
func (s *NavigationRandomSuite) TestSingleAgent() {
	t := s.T()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("TestBot", "nav_single")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	s.Inst.RCON.Say(s.Ctx, "/effect give "+agent.Name+" minecraft:glowing 90 0 true").Exec(s.Ctx)

	startPos := agent.Origin
	logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startPos.X, startPos.Y, startPos.Z)

	// Define destination (10 blocks east on same Y level)
	destination := models.V3{
		X: startPos.X + 10,
		Y: startPos.Y,
		Z: startPos.Z,
	}
	logger.Logf("Target destination: %.2f, %.2f, %.2f", destination.X, destination.Y, destination.Z)

	// Start position tracking
	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	// Command agent to navigate using agent-specific prefix: >>>TestBot<<<
	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
	resp, err := sayCmd.Exec(s.Ctx)
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
	err = tracker.WaitForPosition(s.Ctx, agent.Name, destination, 1.0, timeout)
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
	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
	require.NoError(t, err, "get final position")

	finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}
	finalDistance := finalPos.DistanceTo(destination)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from target: %.2f blocks", finalDistance)

	assert.LessOrEqual(t, finalDistance, 1.0, "agent should be within 1 block of destination")
}

// TestMultipleDestinations tests navigation to multiple waypoints.
// Equivalent to the pre-Phase-1 TestNavigationMultipleDestinations.
func (s *NavigationRandomSuite) TestMultipleDestinations() {
	t := s.T()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("WaypointBot", "nav_waypoints")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	// Start position tracking
	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	startPos := agent.Origin

	waypoints := []models.V3{
		{X: startPos.X + 30, Y: startPos.Y, Z: startPos.Z},      // East
		{X: startPos.X + 30, Y: startPos.Y, Z: startPos.Z + 30}, // Southeast
		{X: startPos.X, Y: startPos.Y, Z: startPos.Z + 30},      // South
		{X: startPos.X, Y: startPos.Y, Z: startPos.Z},           // Back to start
	}

	for i, waypoint := range waypoints {
		logger.Logf("Waypoint %d: %.2f, %.2f, %.2f", i+1, waypoint.X, waypoint.Y, waypoint.Z)

		// Get current position
		currentX, currentY, currentZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
		require.NoError(t, err, "get current position")
		current := models.V3{X: currentX, Y: currentY, Z: currentZ}

		// Send navigation command using agent-specific prefix
		navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
		_, err = sayCmd.Exec(s.Ctx)
		require.NoError(t, err, "send navigation command")

		// Calculate timeout
		distance := current.DistanceTo(waypoint)
		timeout := max(CalculateMovementTimeout(distance), 30*time.Second)

		// Wait for arrival
		err = tracker.WaitForPosition(s.Ctx, agent.Name, waypoint, 1.0, timeout)
		require.NoError(t, err, "agent should reach waypoint %d", i+1)

		logger.Logf("Reached waypoint %d - pausing for %d Seconds", i+1, 3)
		time.Sleep(3 * time.Second) // brief pause between waypoints
	}
}

// TestSingleAgentWithPathfinding tests that a single agent can navigate to
// a destination using pathfinding, with lenient "made progress" fallback
// assertions suited to random terrain that may turn out to be impassable
// (unlike TestSingleAgent's exact-arrival check). Equivalent to the
// pre-Phase-1 TestPathfindingSingleAgent
// (navigation_pathfinding_test.go) - folded in here rather than given its
// own suite since its config (DefaultServerConfig(): WorldGenRandom,
// Peaceful, survival) matches this suite exactly.
func (s *NavigationRandomSuite) TestSingleAgentWithPathfinding() {
	t := s.T()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("PathfindingBot", "nav_pathfinding_single")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	startPos := agent.Origin
	logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startPos.X, startPos.Y, startPos.Z)

	// Define destination (can be at different elevation on random terrain).
	destination := models.V3{
		X: startPos.X + 20,
		Y: startPos.Y,
		Z: startPos.Z + 15,
	}
	logger.Logf("Target destination: %.2f, %.2f, %.2f", destination.X, destination.Y, destination.Z)

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
	resp, err := sayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send navigation command")
	logger.Logf("Command sent, response: %s", resp)

	distance := startPos.DistanceTo(destination)
	timeout := max(CalculateMovementTimeout(distance), 30*time.Second)
	logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

	err = tracker.WaitForPosition(s.Ctx, agent.Name, destination, 2.0, timeout)
	if err != nil {
		finalPos, ok := tracker.GetPosition(agent.Name)
		require.True(t, ok, "Could not get agent position")

		// Analyze movement to differentiate between failure modes.
		totalDist, progressToward, madeProgress := tracker.AnalyzeMovementProgress(
			agent.Name,
			startPos,
			destination,
			5.0, // require at least 5 blocks of progress toward target
		)

		finalDist := finalPos.DistanceTo(destination)
		logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from target: %.2f)",
			finalPos.X, finalPos.Y, finalPos.Z, finalDist)
		logger.Logf("Movement analysis: total distance traveled: %.2f blocks, net progress toward target: %.2f blocks",
			totalDist, progressToward)

		// Test FAILS if agent didn't make progress - indicates pathfinding is broken.
		require.True(t, madeProgress,
			"Agent must make meaningful progress toward target (moved %.2f blocks, progress %.2f blocks). "+
				"No progress indicates pathfinding failure, not impassable terrain.",
			totalDist, progressToward)

		logger.Logf("Note: Agent made progress (%.2f blocks toward target) but did not reach destination - terrain may be impassable",
			progressToward)
		return
	}

	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
	require.NoError(t, err, "get final position")

	finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}
	finalDistance := finalPos.DistanceTo(destination)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from target: %.2f blocks", finalDistance)

	assert.LessOrEqual(t, finalDistance, 2.0, "agent should reach destination using pathfinding")
}

// TestMultipleDestinationsWithPathfinding tests navigation to multiple
// waypoints using pathfinding, requiring only that the agent reach at
// least half of them (unlike TestMultipleDestinations, which requires all
// four). Equivalent to the pre-Phase-1 TestPathfindingMultipleDestinations
// (navigation_pathfinding_test.go) - folded in here for the same
// exact-config-match reason as TestSingleAgentWithPathfinding above.
// Agent renamed from the original's "WaypointBot" to "PathfindingWaypointBot"
// to avoid colliding with TestMultipleDestinations' own agent name in this
// same suite.
func (s *NavigationRandomSuite) TestMultipleDestinationsWithPathfinding() {
	t := s.T()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("PathfindingWaypointBot", "nav_pathfinding_waypoints")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	startPos := agent.Origin

	waypoints := []models.V3{
		{X: startPos.X + 15, Y: startPos.Y, Z: startPos.Z},      // East
		{X: startPos.X + 15, Y: startPos.Y, Z: startPos.Z + 20}, // Southeast
		{X: startPos.X, Y: startPos.Y, Z: startPos.Z + 20},      // South
	}

	successCount := 0
	for i, waypoint := range waypoints {
		logger.Logf("Waypoint %d: %.2f, %.2f, %.2f", i+1, waypoint.X, waypoint.Y, waypoint.Z)

		currentX, currentY, currentZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
		require.NoError(t, err, "get current position")
		current := models.V3{X: currentX, Y: currentY, Z: currentZ}

		navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
		_, err = sayCmd.Exec(s.Ctx)
		require.NoError(t, err, "send navigation command")

		distance := current.DistanceTo(waypoint)
		timeout := max(CalculateMovementTimeout(distance), 30*time.Second)

		err = tracker.WaitForPosition(s.Ctx, agent.Name, waypoint, 1.5, timeout)
		if err != nil {
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

	assert.GreaterOrEqual(t, successCount, len(waypoints)/2, "agent should reach at least half of waypoints with pathfinding")
}

// TestObstacles tests navigation with obstacles.
// Equivalent to the pre-Phase-1 TestNavigationObstacles (still a stub).
func (s *NavigationRandomSuite) TestObstacles() {
	s.T().Skip("Obstacle testing requires world setup - implement when needed")

	// TODO: Implement

	// This test would:
	// 1. Use RCON SetBlock to create walls/obstacles
	// 2. Command agent to navigate around them
	// 3. Verify pathfinding works correctly
}
