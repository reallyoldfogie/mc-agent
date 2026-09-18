package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// NavigationFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for flat-world movement-command tests: one
// server per version, shared by every test method below, instead of the
// previous per-test-function StartServer/StopServer pattern. World gen is
// WorldGenFlat because these tests specifically exercise basic movement
// commands (lineTo/moveForward/moveToAndSneak) without pathfinding, and want
// predictable, uniform terrain - see NavigationRandomSuite in
// navigation_test.go for the random-terrain counterpart that shares a
// *different* server.
type NavigationFlatSuite struct {
	VersionWorldSuite
}

func TestNavigationFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &NavigationFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// TestSingleAgent tests that a single agent can navigate using basic
// movement commands on flat terrain. Equivalent to the pre-Phase-1
// TestFlatMovementSingleAgent.
//
// Marked t.Parallel() (Phase 2 of docs/plans/integration-test-shared-server/00-plan.md):
// safe because SpawnWorkingAreaAgent gives every test method its own
// working area, and WorldGenFlat means every working area sits on uniform,
// deterministic terrain (no shared-Y terrain risk the way WorldGenRandom
// suites have - see VersionWorldSuite's own doc comment). See
// docs/plans/integration-test-shared-server/04-phase2-parallelism.md for
// the concurrency level actually measured safe on this machine.
func (s *NavigationFlatSuite) TestSingleAgent() {
	t := s.T()
	t.Parallel()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("FlatMovementBot", "flat_movement_single")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	startPos := agent.Origin
	logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startPos.X, startPos.Y, startPos.Z)

	// Define destination on the same Y level (10 blocks east)
	destination := models.V3{
		X: startPos.X + 10,
		Y: startPos.Y,
		Z: startPos.Z,
	}
	logger.Logf("Target destination: %.2f, %.2f, %.2f", destination.X, destination.Y, destination.Z)

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	// Send lineTo command (straight-line for flat world)
	navCmd := fmt.Sprintf("lineTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
	resp, err := sayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send navigation command")
	logger.Logf("Command sent, response: %s", resp)

	distance := startPos.DistanceTo(destination)
	timeout := CalculateMovementTimeout(distance)
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

	err = tracker.WaitForPosition(s.Ctx, agent.Name, destination, 1.0, timeout)
	if err != nil {
		finalPos, ok := tracker.GetPosition(agent.Name)
		if ok {
			finalDist := finalPos.DistanceTo(destination)
			logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from target: %.2f)",
				finalPos.X, finalPos.Y, finalPos.Z, finalDist)
		}
		require.NoError(t, err, "agent should reach destination on flat terrain without pathfinding")
	}

	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
	require.NoError(t, err, "get final position")

	finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}
	finalDistance := finalPos.DistanceTo(destination)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from target: %.2f blocks", finalDistance)

	assert.LessOrEqual(t, finalDistance, 1.0, "agent should be within 1 block of destination on flat terrain")
}

// TestMultipleDestinations tests navigation to multiple waypoints on flat
// terrain. Equivalent to the pre-Phase-1 TestFlatMovementMultipleDestinations.
// Marked t.Parallel() - see TestSingleAgent's doc comment for why this is safe.
func (s *NavigationFlatSuite) TestMultipleDestinations() {
	t := s.T()
	t.Parallel()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("WaypointBot", "flat_waypoints")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	startPos := agent.Origin

	waypoints := []models.V3{
		{X: startPos.X + 10, Y: startPos.Y, Z: startPos.Z},      // East
		{X: startPos.X + 10, Y: startPos.Y, Z: startPos.Z + 10}, // Southeast
		{X: startPos.X, Y: startPos.Y, Z: startPos.Z + 10},      // South
		{X: startPos.X, Y: startPos.Y, Z: startPos.Z},           // Back to start
	}

	for i, waypoint := range waypoints {
		logger.Logf("Waypoint %d: %.2f, %.2f, %.2f", i+1, waypoint.X, waypoint.Y, waypoint.Z)

		currentX, currentY, currentZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
		require.NoError(t, err, "get current position")
		current := models.V3{X: currentX, Y: currentY, Z: currentZ}

		navCmd := fmt.Sprintf("lineTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd))
		_, err = sayCmd.Exec(s.Ctx)
		require.NoError(t, err, "send navigation command")

		distance := current.DistanceTo(waypoint)
		timeout := CalculateMovementTimeout(distance)
		if timeout < 30*time.Second {
			timeout = 30 * time.Second
		}

		err = tracker.WaitForPosition(s.Ctx, agent.Name, waypoint, 1.0, timeout)
		require.NoError(t, err, "agent should reach waypoint %d on flat terrain", i+1)

		logger.Logf("Reached waypoint %d", i+1)
	}
}

// TestVerticalMovement tests vertical movement on flat terrain. Equivalent
// to the pre-Phase-1 TestFlatMovementVertical.
func (s *NavigationFlatSuite) TestVerticalMovement() {
	t := s.T()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("VerticalBot", "flat_vertical")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	startX, startY, startZ := agent.Origin.X, agent.Origin.Y, agent.Origin.Z

	// Forceload chunks before building to ensure chunks are loaded
	chunkX := int(startX) >> 4
	chunkZ := int(startZ) >> 4
	forceloadCmd := fmt.Sprintf("forceload add %d %d %d %d", (chunkX-1)<<4, (chunkZ-1)<<4, (chunkX+1)<<4, (chunkZ+1)<<4)
	_, err = s.Inst.RCON.Exec(s.Ctx, forceloadCmd)
	if err != nil {
		logger.Logf("Warning: Failed to forceload chunks: %v", err)
	}
	time.Sleep(1 * time.Second)

	logger.Logf("Building ladder at agent position for vertical movement test")
	ladderHeight := 5
	for y := int(startY); y <= int(startY)+ladderHeight; y++ {
		_, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", int(startX)+1, y, int(startZ)))
		if err != nil {
			logger.Logf("Warning: Failed to place backing wall at (%d, %d, %d): %v", int(startX)+1, y, int(startZ), err)
		}
	}
	for y := int(startY); y <= int(startY)+ladderHeight; y++ {
		_, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:ladder[facing=west]", int(startX), y, int(startZ)))
		if err != nil {
			logger.Logf("Warning: Failed to place ladder at (%d, %d, %d): %v", int(startX), y, int(startZ), err)
		}
	}
	logger.Logf("Ladder built, waiting for chunks to sync")
	time.Sleep(2 * time.Second)

	ladderCenterX := float64(int(startX)) + 0.5
	ladderCenterZ := float64(int(startZ)) + 0.5
	tpCmd := fmt.Sprintf("tp %s %.2f %.2f %.2f", agent.Name, ladderCenterX, startY, ladderCenterZ)
	_, err = s.Inst.RCON.Exec(s.Ctx, tpCmd)
	if err != nil {
		logger.Logf("Warning: Failed to teleport agent to ladder center: %v", err)
	}
	logger.Logf("Teleported agent to ladder center (%.2f, %.2f, %.2f)", ladderCenterX, startY, ladderCenterZ)
	time.Sleep(500 * time.Millisecond)

	upDistance := 3.0
	targetY := startY + upDistance
	moveCmd := fmt.Sprintf("moveToAndSneak %.2f %.2f %.2f", ladderCenterX, targetY, ladderCenterZ)
	sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, moveCmd))
	_, err = sayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send moveToAndSneak command")

	logger.Logf("Sent moveToAndSneak to (%.2f, %.2f, %.2f)", ladderCenterX, targetY, ladderCenterZ)

	time.Sleep(5 * time.Second)

	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
	require.NoError(t, err, "get final position")

	yChange := finalY - startY
	logger.Logf("Y position change: %.2f (expected ~%.2f)", yChange, upDistance)

	assert.Greater(t, yChange, upDistance*0.5, "agent should move up at least 50%% of requested distance")
	assert.InDelta(t, ladderCenterX, finalX, 0.25, "X position should remain near ladder center")
	assert.InDelta(t, ladderCenterZ, finalZ, 0.25, "Z position should remain near ladder center")
}

// TestLongLadderClimbAndHold tests climbing a long ladder (25 blocks) and
// holding position near the top. Equivalent to the pre-Phase-1
// TestLongLadderClimbAndHold.
func (s *NavigationFlatSuite) TestLongLadderClimbAndHold() {
	t := s.T()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("LongClimbBot", "long_ladder_climb")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	startX, startY, startZ := agent.Origin.X, agent.Origin.Y, agent.Origin.Z
	logger.Logf("Agent starting position: %.2f, %.2f, %.2f", startX, startY, startZ)

	chunkX := int(startX) >> 4
	chunkZ := int(startZ) >> 4
	forceloadCmd := fmt.Sprintf("forceload add %d %d %d %d", (chunkX-1)<<4, (chunkZ-1)<<4, (chunkX+1)<<4, (chunkZ+1)<<4)
	_, err = s.Inst.RCON.Exec(s.Ctx, forceloadCmd)
	if err != nil {
		logger.Logf("Warning: Failed to forceload chunks: %v", err)
	}
	time.Sleep(1 * time.Second)

	ladderHeight := 25
	logger.Logf("Building %d-block tall ladder", ladderHeight)

	for y := int(startY); y <= int(startY)+ladderHeight; y++ {
		_, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:stone", int(startX)+1, y, int(startZ)))
		if err != nil {
			logger.Logf("Warning: Failed to place backing wall at Y=%d: %v", y, err)
		}
	}
	for y := int(startY); y <= int(startY)+ladderHeight; y++ {
		_, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("setblock %d %d %d minecraft:ladder[facing=west]", int(startX), y, int(startZ)))
		if err != nil {
			logger.Logf("Warning: Failed to place ladder at Y=%d: %v", y, err)
		}
	}
	logger.Logf("Ladder built, waiting for chunks to sync")
	time.Sleep(2 * time.Second)

	ladderCenterX := float64(int(startX)) + 0.5
	ladderCenterZ := float64(int(startZ)) + 0.5
	tpCmd := fmt.Sprintf("tp %s %.2f %.2f %.2f", agent.Name, ladderCenterX, startY, ladderCenterZ)
	_, err = s.Inst.RCON.Exec(s.Ctx, tpCmd)
	require.NoError(t, err, "teleport agent to ladder")
	logger.Logf("Teleported agent to ladder center")
	time.Sleep(500 * time.Millisecond)

	targetY := startY + float64(ladderHeight) - 2.0
	logger.Logf("Target Y: %.2f (ladder top minus 2 blocks)", targetY)

	moveCmd := fmt.Sprintf("moveToAndSneak %.2f %.2f %.2f", ladderCenterX, targetY, ladderCenterZ)
	sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, moveCmd))
	_, err = sayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send moveToAndSneak command")
	logger.Logf("Sent moveToAndSneak to (%.2f, %.2f, %.2f)", ladderCenterX, targetY, ladderCenterZ)

	time.Sleep(30 * time.Second)

	climbX, climbY, climbZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
	require.NoError(t, err, "get position after climb")
	logger.Logf("models.V3 after climb: %.2f, %.2f, %.2f", climbX, climbY, climbZ)

	climbHeight := climbY - startY
	logger.Logf("Climb height achieved: %.2f blocks (target: %.2f)", climbHeight, targetY-startY)

	expectedClimb := targetY - startY
	assert.Greater(t, climbHeight, expectedClimb*0.8, "agent should climb at least 80%% of target height")

	logger.Logf("Waiting 10 seconds to verify agent holds position...")
	time.Sleep(10 * time.Second)

	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
	require.NoError(t, err, "get final position")
	logger.Logf("Final position after hold: %.2f, %.2f, %.2f", finalX, finalY, finalZ)

	yDrop := climbY - finalY
	logger.Logf("Y drop during hold period: %.2f blocks", yDrop)
	assert.LessOrEqual(t, yDrop, 0.5, "agent should hold position on ladder (not fall more than 0.5 blocks)")

	finalHeight := finalY - startY
	logger.Logf("Final height from start: %.2f blocks", finalHeight)
	assert.Greater(t, finalHeight, expectedClimb*0.7, "agent should maintain at least 70%% of target height after hold period")

	assert.InDelta(t, ladderCenterX, finalX, 0.5, "X position should remain within ladder area")
	assert.InDelta(t, ladderCenterZ, finalZ, 0.5, "Z position should remain within ladder area")
}

// TestForwardCommand tests the moveForward command on flat terrain.
// Equivalent to the pre-Phase-1 TestFlatMovementForwardCommand.
// Marked t.Parallel() - see TestSingleAgent's doc comment for why this is safe.
func (s *NavigationFlatSuite) TestForwardCommand() {
	t := s.T()
	t.Parallel()
	logger := NewTestLogger(t)

	agent, err := s.SpawnWorkingAreaAgent("ForwardBot", "flat_forward")
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned at working area (%.2f, %.2f, %.2f)", agent.Name, agent.Origin.X, agent.Origin.Y, agent.Origin.Z)

	startPos := agent.Origin

	distance := 5.0
	moveCmd := fmt.Sprintf("moveForward %.2f", distance)
	sayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", agent.Name, moveCmd))
	_, err = sayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send moveForward command")

	logger.Logf("Sent moveForward %.2f command", distance)

	time.Sleep(5 * time.Second)

	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agent.Name)
	require.NoError(t, err, "get final position")

	finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}
	moved := startPos.DistanceTo(finalPos)

	logger.Logf("Distance moved: %.2f blocks (requested %.2f)", moved, distance)

	assert.Greater(t, moved, distance*0.5, "agent should move forward at least 50%% of requested distance")
}
