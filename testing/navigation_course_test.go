package testing

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// NavigationCourseFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the NavigationCourse waypoint meta-test:
// one server per version, shared by every test method below (currently
// one), instead of the previous per-test-function StartServer/StopServer
// pattern. WorldGen = WorldGenFlat and Difficulty = DifficultyEasy,
// matching the pre-conversion test's own
// setupStandaloneTestWithModeAndBlockPlacement call exactly. Kept as its
// own new suite rather than folded into an existing Flat/Easy suite
// (several already share this exact config) - this session's convention
// has been one suite per source file/feature, not merging on config match
// alone (see docs/plans/integration-test-shared-server/22-phase1-drop-conversion.md
// for the same reasoning applied to drop_test.go).
//
// This test's own NavigationCourse mechanism depends on command blocks -
// the exact thing docs/plans/integration-test-shared-server/16-phase1-elytra-conversion.md
// found SharedServerConfig() silently disabled (ENABLE_COMMAND_BLOCK
// defaults off on itzg/minecraft-server) and fixed. That fix already
// landed in working_area.go before this conversion, so this suite works
// correctly on the first try - no rediscovery needed.
type NavigationCourseFlatSuite struct {
	VersionWorldSuite
}

func TestNavigationCourseFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &NavigationCourseFlatSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		return s
	})
}

// TestMoveToVisitsWaypointsInOrder is Phase 9's meta-test (see
// PHASE_9_PLAN.md §5): it proves the NavigationCourse tool itself works
// before any other test depends on it, using plain pathfinding-based
// walking - the simplest, most deterministic movement mode - rather than
// something timing-sensitive like elytra flight. Equivalent to the
// pre-Phase-1 TestNavigationCourse_MoveToVisitsWaypointsInOrder.
func (s *NavigationCourseFlatSuite) TestMoveToVisitsWaypointsInOrder() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("NavCourseBot", "navigation_course_moveto")
	require.NoError(t, err, "spawn agent")

	clearX := int(math.Floor(leader.Origin.X))
	clearZ := int(math.Floor(leader.Origin.Z))
	clearY := int(math.Floor(leader.Origin.Y))
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, clearX-10, clearY+1, clearZ-5, clearX+10, clearY+10, clearZ+45), "clear course area")

	waypoints := []CourseWaypoint{
		{Pos: models.V3{X: leader.Origin.X, Y: leader.Origin.Y, Z: leader.Origin.Z + 15}},
		{Pos: models.V3{X: leader.Origin.X, Y: leader.Origin.Y, Z: leader.Origin.Z + 30}},
		{Pos: models.V3{X: leader.Origin.X, Y: leader.Origin.Y, Z: leader.Origin.Z + 40}},
	}

	const walkingRadius = 2.0
	course, err := SetupNavigationCourse(s.Ctx, s.Inst.RCON, leader.Name, walkingRadius, waypoints)
	require.NoError(t, err, "set up navigation course")
	defer func() { _ = course.Cleanup(s.Ctx, s.Inst.RCON) }()

	for i, wp := range waypoints {
		err := leader.Agent.MoveTo(s.Ctx, wp.Pos.X, wp.Pos.Y, wp.Pos.Z, false)
		require.NoError(t, err, "moveTo waypoint %d", i)
	}

	results, err := course.Results(s.Ctx, s.Inst.RCON)
	require.NoError(t, err, "query course results")
	require.Len(t, results, len(waypoints))

	var lastTriggeredTime int64
	for i, res := range results {
		t.Logf("waypoint %d: TriggeredTime=%d", i, res.TriggeredTime)
		assert.Greater(t, res.TriggeredTime, int64(0), "waypoint %d should have been reached", i)
		assert.GreaterOrEqual(t, res.TriggeredTime, lastTriggeredTime, "waypoint %d should not have been reached before the previous one", i)
		lastTriggeredTime = res.TriggeredTime
	}
}
