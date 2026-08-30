package testing

import (
	"context"
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNavigationCourse_MoveToVisitsWaypointsInOrder is Phase 9's meta-test
// (see PHASE_9_PLAN.md §5): it proves the NavigationCourse tool itself
// works before any other test depends on it, using plain pathfinding-based
// walking - the simplest, most deterministic movement mode - rather than
// something timing-sensitive like elytra flight.
func TestNavigationCourse_MoveToVisitsWaypointsInOrder(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			env := setupStandaloneTestWithModeAndBlockPlacement(t, "navigation_course_moveto", "survival", false, tt.MCVersion, DifficultyEasy, false)
			defer env.Cancel()

			ctx := context.Background()

			botPos, ok := env.Agent.Agent.GetPositionSimple()
			require.True(t, ok, "agent position should be initialized")

			clearX := int(math.Floor(botPos.X))
			clearZ := int(math.Floor(botPos.Z))
			clearY := int(math.Floor(botPos.Y))
			require.NoError(t, ClearArea(ctx, env.Inst.RCON, clearX-10, clearY+1, clearZ-5, clearX+10, clearY+10, clearZ+45), "clear course area")

			waypoints := []CourseWaypoint{
				{Pos: models.V3{X: botPos.X, Y: botPos.Y, Z: botPos.Z + 15}},
				{Pos: models.V3{X: botPos.X, Y: botPos.Y, Z: botPos.Z + 30}},
				{Pos: models.V3{X: botPos.X, Y: botPos.Y, Z: botPos.Z + 40}},
			}

			const walkingRadius = 2.0
			course, err := SetupNavigationCourse(ctx, env.Inst.RCON, env.BotName, walkingRadius, waypoints)
			require.NoError(t, err, "set up navigation course")
			defer func() { _ = course.Cleanup(context.Background(), env.Inst.RCON) }()

			for i, wp := range waypoints {
				err := env.Agent.Agent.MoveTo(ctx, wp.Pos.X, wp.Pos.Y, wp.Pos.Z, false)
				require.NoError(t, err, "moveTo waypoint %d", i)
			}

			results, err := course.Results(ctx, env.Inst.RCON)
			require.NoError(t, err, "query course results")
			require.Len(t, results, len(waypoints))

			var lastTriggeredTime int64
			for i, res := range results {
				t.Logf("waypoint %d: TriggeredTime=%d", i, res.TriggeredTime)
				assert.Greater(t, res.TriggeredTime, int64(0), "waypoint %d should have been reached", i)
				assert.GreaterOrEqual(t, res.TriggeredTime, lastTriggeredTime, "waypoint %d should not have been reached before the previous one", i)
				lastTriggeredTime = res.TriggeredTime
			}
		})
	}
}
