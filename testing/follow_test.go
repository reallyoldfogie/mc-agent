package testing

import (
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/movement"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type movementExecutorTypeProvider interface {
	MovementExecutorType() movement.ExecutorType
}

func requirePhysicsExecutor(t *testing.T, agent *ManagedAgent) {
	t.Helper()
	provider, ok := agent.Agent.(movementExecutorTypeProvider)
	require.True(t, ok, "agent does not expose movement executor type")
	require.Equal(t, movement.PhysicsExecutor, provider.MovementExecutorType(), "agent should use physics movement executor")
}

// FollowFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for follow-behavior tests: one server per
// version, shared by every test method below, instead of the previous
// per-test-function StartServer/StopServer pattern. WorldGen = WorldGenFlat,
// matching every pre-conversion TestFollow* function's own choice
// (FlatWorldServerConfig, "for reliable spawn locations"/predictable
// terrain).
//
// Every test here spawns multiple agents (a leader plus one or more
// followers) that need to start NEAR each other - not each independently
// offset 256+ blocks apart the way single-agent NavigationFlatSuite/
// NavigationRandomSuite tests are. Only the leader claims this test's one
// working area (SpawnWorkingAreaAgent); every follower is placed near the
// leader's settled Origin via SpawnAgentNear instead of claiming its own
// separate area - see docs/plans/integration-test-shared-server/07-phase1-follow-conversion.md
// for why the original (single-call-per-agent) SpawnWorkingAreaAgent
// semantics would have been wrong for this file specifically, and why that
// gap was fixed in testing/version_world_suite.go rather than worked around
// here.
type FollowFlatSuite struct {
	VersionWorldSuite
}

func TestFollowFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &FollowFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// TestSingleAgent tests that a single follower agent can follow a leader to
// a destination. Equivalent to the pre-Phase-1 TestFollowSingleAgent.
func (s *FollowFlatSuite) TestSingleAgent() {
	t := s.T()
	logger := NewTestLogger(t)

	leader, err := s.SpawnWorkingAreaAgent("SingleLeader", "follow_test_leader")
	require.NoError(t, err, "spawn leader agent")
	logger.Logf("Leader agent spawned at working area (%.2f, %.2f, %.2f)", leader.Origin.X, leader.Origin.Y, leader.Origin.Z)
	requirePhysicsExecutor(t, leader.ManagedAgent)

	// Placed 6 blocks west, 3 blocks north of the leader - the exact
	// relative offset the pre-conversion original used
	// ("leaderStartX-6, leaderStartY, leaderStartZ-3"), now expressed via
	// SpawnAgentNear instead of a follow-up RCON teleport.
	follower, err := s.SpawnAgentNear("SingleFollower", "follow_test_follower", leader.Origin, -6, -3)
	require.NoError(t, err, "spawn follower agent")
	logger.Logf("Follower agent spawned near leader at (%.2f, %.2f, %.2f)", follower.Origin.X, follower.Origin.Y, follower.Origin.Z)
	requirePhysicsExecutor(t, follower.ManagedAgent)

	leaderStart := leader.Origin
	followerStart := follower.Origin
	logger.Logf("Leader start: (%.2f, %.2f, %.2f)", leaderStart.X, leaderStart.Y, leaderStart.Z)
	logger.Logf("Follower start: (%.2f, %.2f, %.2f)", followerStart.X, followerStart.Y, followerStart.Z)

	// Validate spawn locations are mutually reachable
	validator := NewSpawnValidator()
	validator.RecordPosition(leader.Name, leaderStart)
	validator.RecordPosition(follower.Name, followerStart)
	report := validator.Validate(s.Ctx)
	logger.Logf("Spawn Validation Report:\n%s", report.String())
	require.True(t, report.AllReachable, "both agents must spawn in mutually reachable locations")

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	followCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
	resp, err := followCmd.Exec(s.Ctx)
	require.NoError(t, err, "send follow command to follower")
	logger.Logf("Follow command sent, response: %s", resp)

	time.Sleep(2 * time.Second)

	destination := models.V3{
		X: leaderStart.X + 20,
		Y: leaderStart.Y,
		Z: leaderStart.Z + 10,
	}
	logger.Logf("Leader destination: (%.2f, %.2f, %.2f)", destination.X, destination.Y, destination.Z)

	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	navSayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
	_, err = navSayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send navigation command to leader")

	distance := leaderStart.DistanceTo(destination)
	timeout := CalculateMovementTimeout(distance)
	timeout *= 2 // double timeout to account for follower lag
	if timeout < 60*time.Second {
		timeout = 60 * time.Second
	}

	logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

	err = tracker.WaitForPosition(s.Ctx, leader.Name, destination, 1.0, timeout)
	if err != nil {
		leaderPos, ok := tracker.GetPosition(leader.Name)
		if ok {
			logger.Logf("Leader final position: %.2f, %.2f, %.2f (distance: %.2f)",
				leaderPos.X, leaderPos.Y, leaderPos.Z, leaderPos.DistanceTo(destination))
		}
		require.NoError(t, err, "leader should reach destination")
	}
	logger.Logf("Leader reached destination")

	time.Sleep(15 * time.Second)

	leaderX, leaderY, leaderZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, leader.Name)
	require.NoError(t, err, "get leader final position")

	followerX, followerY, followerZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, follower.Name)
	require.NoError(t, err, "get follower final position")

	leaderFinalPos := models.V3{X: leaderX, Y: leaderY, Z: leaderZ}
	followerFinalPos := models.V3{X: followerX, Y: followerY, Z: followerZ}

	distanceBetween := leaderFinalPos.DistanceTo(followerFinalPos)

	logger.Logf("Leader final: (%.2f, %.2f, %.2f)", leaderX, leaderY, leaderZ)
	logger.Logf("Follower final: (%.2f, %.2f, %.2f)", followerX, followerY, followerZ)
	logger.Logf("Distance between agents: %.2f blocks", distanceBetween)

	assert.LessOrEqual(t, distanceBetween, 5.0, "follower should stay within 5 blocks of leader")

	followerTravelDistance := followerStart.DistanceTo(followerFinalPos)
	logger.Logf("Follower traveled: %.2f blocks", followerTravelDistance)

	assert.Greater(t, followerTravelDistance, distance*0.5, "follower should travel at least 50%% of leader's path")
}

// TestMultipleAgents tests multiple agents following a single leader.
// Equivalent to the pre-Phase-1 TestFollowMultipleAgents. The pre-conversion
// original didn't explicitly position its 3 followers relative to the
// leader - every agent joining a fresh, single-purpose server naturally
// spawns at (nearly) the same world spawn point, so "near the leader" was
// free. A shared working area doesn't have a single fixed spawn point in
// that sense once offset, so each follower is explicitly placed a few
// blocks from the leader via SpawnAgentNear (small, distinct offsets so the
// 3 followers don't stack on top of each other or the leader).
func (s *FollowFlatSuite) TestMultipleAgents() {
	t := s.T()
	logger := NewTestLogger(t)

	leader, err := s.SpawnWorkingAreaAgent("MultiLeader", "multi_follow_leader")
	require.NoError(t, err, "spawn leader")
	logger.Logf("Leader spawned at working area (%.2f, %.2f, %.2f)", leader.Origin.X, leader.Origin.Y, leader.Origin.Z)

	type followerSpec struct {
		name   string
		dx, dz float64
	}
	specs := []followerSpec{
		{"Follower1", -3, -3},
		{"Follower2", -3, 3},
		{"Follower3", 3, -3},
	}

	followers := make([]*WorkingAreaAgent, 0, len(specs))
	for _, spec := range specs {
		follower, err := s.SpawnAgentNear(spec.name, "multi_follow_"+spec.name, leader.Origin, spec.dx, spec.dz)
		require.NoError(t, err, "spawn follower %s", spec.name)
		followers = append(followers, follower)
		logger.Logf("Follower %s spawned near leader at (%.2f, %.2f, %.2f)", spec.name, follower.Origin.X, follower.Origin.Y, follower.Origin.Z)
	}

	leaderStart := leader.Origin

	validator := NewSpawnValidator()
	validator.RecordPosition(leader.Name, leaderStart)
	for _, follower := range followers {
		validator.RecordPosition(follower.Name, follower.Origin)
	}
	report := validator.Validate(s.Ctx)
	logger.Logf("Spawn Validation Report:\n%s", report.String())
	require.True(t, report.AllReachable, "all agents must spawn in mutually reachable locations")

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	for _, follower := range followers {
		followCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
		_, err := followCmd.Exec(s.Ctx)
		require.NoError(t, err, "send follow command to %s", follower.Name)
		time.Sleep(500 * time.Millisecond) // stagger commands
	}

	logger.Logf("All followers commanded to follow leader")

	time.Sleep(2 * time.Second)

	destination := models.V3{
		X: leaderStart.X + 15,
		Y: leaderStart.Y,
		Z: leaderStart.Z + 15,
	}

	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", destination.X, destination.Y, destination.Z)
	navSayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
	_, err = navSayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send navigation command to leader")

	distance := leaderStart.DistanceTo(destination)
	timeout := CalculateMovementTimeout(distance) * 3 // extra time for multiple agents
	if timeout < 90*time.Second {
		timeout = 90 * time.Second
	}

	err = tracker.WaitForPosition(s.Ctx, leader.Name, destination, 1.0, timeout)
	require.NoError(t, err, "leader should reach destination")
	logger.Logf("Leader reached destination")

	time.Sleep(10 * time.Second)

	leaderX, leaderY, leaderZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, leader.Name)
	require.NoError(t, err, "get leader final position")
	leaderFinal := models.V3{X: leaderX, Y: leaderY, Z: leaderZ}

	for _, follower := range followers {
		followerX, followerY, followerZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, follower.Name)
		require.NoError(t, err, "get %s final position", follower.Name)

		followerPos := models.V3{X: followerX, Y: followerY, Z: followerZ}
		distToLeader := followerPos.DistanceTo(leaderFinal)

		logger.Logf("%s final: (%.2f, %.2f, %.2f), distance to leader: %.2f",
			follower.Name, followerX, followerY, followerZ, distToLeader)

		assert.LessOrEqual(t, distToLeader, 7.0, "%s should be within 7 blocks of leader", follower.Name)
	}
}

// TestDynamicTarget tests following a leader that changes direction.
// Equivalent to the pre-Phase-1 TestFollowDynamicTarget. Same "explicit
// SpawnAgentNear placement, original relied on shared spawn point" note as
// TestMultipleAgents.
func (s *FollowFlatSuite) TestDynamicTarget() {
	t := s.T()
	logger := NewTestLogger(t)

	leader, err := s.SpawnWorkingAreaAgent("DynamicLeader", "dynamic_follow_leader")
	require.NoError(t, err, "spawn leader")
	logger.Logf("Leader spawned at working area (%.2f, %.2f, %.2f)", leader.Origin.X, leader.Origin.Y, leader.Origin.Z)

	follower, err := s.SpawnAgentNear("DynamicFollower", "dynamic_follow_follower", leader.Origin, -3, -3)
	require.NoError(t, err, "spawn follower")
	logger.Logf("Follower spawned near leader at (%.2f, %.2f, %.2f)", follower.Origin.X, follower.Origin.Y, follower.Origin.Z)

	leaderStart := leader.Origin

	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	followCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
	_, err = followCmd.Exec(s.Ctx)
	require.NoError(t, err, "send follow command")
	time.Sleep(2 * time.Second)

	waypoints := []models.V3{
		{X: leaderStart.X + 10, Y: leaderStart.Y, Z: leaderStart.Z},
		{X: leaderStart.X + 10, Y: leaderStart.Y, Z: leaderStart.Z + 10},
		{X: leaderStart.X, Y: leaderStart.Y, Z: leaderStart.Z + 10},
	}

	for i, waypoint := range waypoints {
		logger.Logf("Leader waypoint %d: (%.2f, %.2f, %.2f)", i+1, waypoint.X, waypoint.Y, waypoint.Z)

		navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", waypoint.X, waypoint.Y, waypoint.Z)
		navSayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
		_, err = navSayCmd.Exec(s.Ctx)
		require.NoError(t, err, "send navigation command for waypoint %d", i+1)

		time.Sleep(5 * time.Second)

		logger.Logf("Leader moving to waypoint %d", i+1)
	}

	time.Sleep(10 * time.Second)

	leaderX, leaderY, leaderZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, leader.Name)
	require.NoError(t, err, "get leader final position")

	followerX, followerY, followerZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, follower.Name)
	require.NoError(t, err, "get follower final position")

	leaderFinal := models.V3{X: leaderX, Y: leaderY, Z: leaderZ}
	followerFinal := models.V3{X: followerX, Y: followerY, Z: followerZ}

	distance := leaderFinal.DistanceTo(followerFinal)
	logger.Logf("Leader final: (%.2f, %.2f, %.2f)", leaderX, leaderY, leaderZ)
	logger.Logf("Follower final: (%.2f, %.2f, %.2f)", followerX, followerY, followerZ)
	logger.Logf("Final distance: %.2f blocks", distance)

	assert.LessOrEqual(t, distance, 8.0, "follower should stay within 8 blocks despite direction changes")
}

// TestStopCommand tests that a follower can stop following. Equivalent to
// the pre-Phase-1 TestFollowStopCommand.
func (s *FollowFlatSuite) TestStopCommand() {
	t := s.T()
	logger := NewTestLogger(t)

	leader, err := s.SpawnWorkingAreaAgent("StopLeader", "stop_follow_leader")
	require.NoError(t, err, "spawn leader")
	logger.Logf("Leader spawned at working area (%.2f, %.2f, %.2f)", leader.Origin.X, leader.Origin.Y, leader.Origin.Z)

	follower, err := s.SpawnAgentNear("StopFollower", "stop_follow_follower", leader.Origin, -3, -3)
	require.NoError(t, err, "spawn follower")
	logger.Logf("Follower spawned near leader at (%.2f, %.2f, %.2f)", follower.Origin.X, follower.Origin.Y, follower.Origin.Z)

	followCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< follow %s", follower.Name, leader.Name))
	_, err = followCmd.Exec(s.Ctx)
	require.NoError(t, err, "send follow command")
	time.Sleep(2 * time.Second)

	_, followerY1, _, err := s.Inst.RCON.GetEntityPos(s.Ctx, follower.Name)
	require.NoError(t, err, "get follower position before stop")

	stopCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< stopFollow", follower.Name))
	_, err = stopCmd.Exec(s.Ctx)
	require.NoError(t, err, "send stop follow command")
	time.Sleep(2 * time.Second)

	leaderX, leaderY, leaderZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, leader.Name)
	require.NoError(t, err, "get leader position")

	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", leaderX+20, leaderY, leaderZ)
	navSayCmd := s.Inst.RCON.Say(s.Ctx, fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd))
	_, err = navSayCmd.Exec(s.Ctx)
	require.NoError(t, err, "send navigation command to leader")

	time.Sleep(8 * time.Second)

	_, followerY2, _, err := s.Inst.RCON.GetEntityPos(s.Ctx, follower.Name)
	require.NoError(t, err, "get follower position after stop")

	yChange := followerY2 - followerY1
	logger.Logf("Follower Y change after stop: %.2f", yChange)

	assert.LessOrEqual(t, yChange, 2.0, "follower should stay relatively still after stop command")
}
