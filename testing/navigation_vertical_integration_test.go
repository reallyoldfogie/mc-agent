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

// VerticalNavigationFlatSuite is Phase 1's (docs/plans/integration-test-shared-server/00-plan.md)
// version-parameterized suite for the vertical-navigation smoke-test
// matrix: one server per version, shared by every sub-case below, instead
// of the previous pattern where every single sub-case (9 by default, 27
// with VERTICAL_NAV_FULL=1) booted its own fresh server - by far the
// largest per-function-boot-count file converted this pass. WorldGen =
// WorldGenFlat, matching the pre-conversion test's own FlatWorldServerConfig().
// Difficulty is left at VersionWorldSuite's own Peaceful default, matching
// the original (never set explicitly).
type VerticalNavigationFlatSuite struct {
	VersionWorldSuite
}

func TestVerticalNavigationFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &VerticalNavigationFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// TestSmoke is the smoke test suite for vertical navigation. Tests one
// orientation for each structure type (9 sub-cases). Set
// VERTICAL_NAV_FULL=1 to run full coverage (all orientations, 27
// sub-cases). Equivalent to the pre-Phase-1 TestVerticalNavigationSmoke.
func (s *VerticalNavigationFlatSuite) TestSmoke() {
	t := s.T()

	// Gamerules/time/weather for stable testing - applies once, server-wide,
	// shared by every sub-case below (matches the pre-conversion test's own
	// per-server setup, just done once instead of once per sub-case).
	s.Inst.RCON.SetGamerule(s.Ctx, "doMobSpawning", "false")
	s.Inst.RCON.SetGamerule(s.Ctx, "doDaylightCycle", "false")
	s.Inst.RCON.SetTime(s.Ctx, "day")
	s.Inst.RCON.SetWeather(s.Ctx, "clear")
	time.Sleep(1 * time.Second) // Let gamerules apply

	testCases := []struct {
		name        string
		segment     CourseSegment
		orientation Orientation
	}{
		// Smoke tests (always run)
		{"LadderAscent_North", &LadderAscent{Height: 5}, North},
		{"LadderDescent_North", &LadderDescent{Height: 5}, North},

		{"StairAscent_East", &StairAscent{Steps: 5}, East},
		{"StairDescent_East", &StairDescent{Steps: 5}, East},

		{"BlockStepAscent_South", &BlockStepAscent{Steps: 4}, South},
		{"BlockStepDescent_South", &BlockStepDescent{Steps: 4}, South},

		{"VineAscent", &VineAscent{Height: 5}, North},
		{"VineDescent", &VineDescent{Height: 5}, North},

		{"Combo_StairLadderStair", NewComboSegment("Combo_StairLadderStair",
			&StairAscent{Steps: 3},
			&LadderAscent{Height: 3},
			&StairAscent{Steps: 3},
		), East},

		// Full coverage tests (only with VERTICAL_NAV_FULL=1)
		{"LadderAscent_South", &LadderAscent{Height: 5}, South},
		{"LadderAscent_East", &LadderAscent{Height: 5}, East},
		{"LadderAscent_West", &LadderAscent{Height: 5}, West},

		{"LadderDescent_South", &LadderDescent{Height: 5}, South},
		{"LadderDescent_East", &LadderDescent{Height: 5}, East},
		{"LadderDescent_West", &LadderDescent{Height: 5}, West},

		{"StairAscent_North", &StairAscent{Steps: 5}, North},
		{"StairAscent_South", &StairAscent{Steps: 5}, South},
		{"StairAscent_West", &StairAscent{Steps: 5}, West},

		{"StairDescent_North", &StairDescent{Steps: 5}, North},
		{"StairDescent_South", &StairDescent{Steps: 5}, South},
		{"StairDescent_West", &StairDescent{Steps: 5}, West},

		{"BlockStepAscent_North", &BlockStepAscent{Steps: 4}, North},
		{"BlockStepAscent_East", &BlockStepAscent{Steps: 4}, East},
		{"BlockStepAscent_West", &BlockStepAscent{Steps: 4}, West},

		{"BlockStepDescent_North", &BlockStepDescent{Steps: 4}, North},
		{"BlockStepDescent_East", &BlockStepDescent{Steps: 4}, East},
		{"BlockStepDescent_West", &BlockStepDescent{Steps: 4}, West},

		{"Combo_StairLadderStair_West", NewComboSegment("Combo_StairLadderStair_West",
			&StairAscent{Steps: 3},
			&LadderAscent{Height: 3},
			&StairAscent{Steps: 3},
		), West},
	}

	for i, tc := range testCases {
		tc := tc // Capture for closure
		testIndex := i

		t.Run(tc.name, func(t *testing.T) {
			s.runVerticalNavigationTest(t, testIndex, tc.segment, tc.orientation)
		})
	}
}

// runVerticalNavigationTest executes a single vertical navigation sub-case
// against this suite's shared server, in its own working area.
//
// Agent names are a short "VNav<index>" rather than the descriptive
// tc.name (e.g. "LadderAscent_North") deliberately: camAgentName trims its
// base to 13 characters before appending "Cam", and several of this file's
// own test-case names share the same first-13-character prefix (every
// "LadderAscent_*" orientation, for one) - their Cam companions would
// otherwise all collide on the same truncated name. The descriptive name
// is still used for the replay-file prefix, which has no such length
// limit.
func (s *VerticalNavigationFlatSuite) runVerticalNavigationTest(t *testing.T, index int, segment CourseSegment, orientation Orientation) {
	logger := NewTestLogger(t)

	agentName := fmt.Sprintf("VNav%d", index)
	leader, err := s.SpawnWorkingAreaAgentWithT(t, agentName, fmt.Sprintf("%s_%s", segment.Name(), orientation.String()))
	require.NoError(t, err, "spawn agent")

	origin := leader.Origin
	logger.Logf("Building segment %s at origin (%.0f, %.0f, %.0f) facing %s",
		segment.Name(), origin.X, origin.Y, origin.Z, orientation.String())

	// Forceload chunks BEFORE building to ensure chunks are loaded.
	chunkX1 := int(origin.X-10) >> 4
	chunkZ1 := int(origin.Z-10) >> 4
	chunkX2 := int(origin.X+20) >> 4
	chunkZ2 := int(origin.Z+20) >> 4
	forceloadCmd := fmt.Sprintf("forceload add %d %d %d %d", chunkX1<<4, chunkZ1<<4, chunkX2<<4, chunkZ2<<4)
	_, err = s.Inst.RCON.Exec(s.Ctx, forceloadCmd)
	require.NoError(t, err, "forceload chunks")
	defer s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf("forceload remove %d %d %d %d", chunkX1<<4, chunkZ1<<4, chunkX2<<4, chunkZ2<<4))
	logger.Logf("Forceloaded chunks for building area")

	time.Sleep(2 * time.Second) // Wait for chunks to load

	// Clear area first (leave ground intact at origin.Y - this suite's flat
	// world surface).
	if err := ClearArea(s.Ctx, s.Inst.RCON,
		int(origin.X)-10, int(origin.Y)+1, int(origin.Z)-10,
		int(origin.X)+20, int(origin.Y)+30, int(origin.Z)+20); err != nil {
		logger.Logf("warning: failed to clear area: %v", err)
	}

	// Build course segment.
	start, goal, err := segment.Build(s.Ctx, s.Inst.RCON, origin, orientation)
	require.NoError(t, err, "build segment")
	logger.Logf("Segment built: start=(%.2f, %.2f, %.2f) goal=(%.2f, %.2f, %.2f)",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z)

	nbt := `{Tags:["hpa_debug"],block_state:{Name:"green_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]}}`
	result, err := s.Inst.RCON.SummonEntity(s.Ctx, start.X+0.5, start.Y+0.2, start.Z+0.5, "block_display", nbt).Exec(s.Ctx)
	logger.Logf("Summon start block display result: %s", result)
	require.NoError(t, err, "summon start block display")

	nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
		buildMultilineTextDisplay([]string{"START", fmt.Sprintf("%.2f %.2f %.2f", start.X, start.Y, start.Z)}))
	result, err = s.Inst.RCON.SummonEntity(s.Ctx, start.X+0.5, start.Y+1.5, start.Z+0.5, "text_display", nbt).Exec(s.Ctx)
	logger.Logf("Summon start text display result: %s", result)
	require.NoError(t, err, "summon start text display")

	nbt = `{Tags:["hpa_debug"],block_state:{Name:"red_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]}}`
	s.Inst.RCON.SummonEntity(s.Ctx, goal.X+0.5, goal.Y+0.2, goal.Z+0.5, "block_display", nbt).Exec(s.Ctx)
	logger.Logf("Summon goal text display result: %s", result)
	require.NoError(t, err, "summon goal block display")

	nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
		buildMultilineTextDisplay([]string{"GOAL", fmt.Sprintf("%.2f %.2f %.2f", goal.X, goal.Y, goal.Z)}))
	s.Inst.RCON.SummonEntity(s.Ctx, goal.X+0.5, goal.Y+1.5, goal.Z+0.5, "text_display", nbt).Exec(s.Ctx)
	logger.Logf("Summon goal text display result: %s", result)
	require.NoError(t, err, "summon goal text display")

	// Clear inventory.
	s.Inst.RCON.Exec(s.Ctx, "clear "+leader.Name)

	// Teleport to start position.
	tpCmd := fmt.Sprintf("tp %s %.2f %.2f %.2f", leader.Name, start.X, start.Y, start.Z)
	_, err = s.Inst.RCON.Exec(s.Ctx, tpCmd)
	require.NoError(t, err, "teleport to start")
	logger.Logf("Agent teleported to start position")
	time.Sleep(1 * time.Second)

	// Start telemetry recording.
	telemetryRecorder := NewTelemetryRecorder()
	telemetryRecorder.Start(start)
	leader.Agent.SetTelemetryRecorder(telemetryRecorder)

	// Start position tracking.
	tracker := NewPositionTracker(s.Inst, 500*time.Millisecond)
	tracker.Start(s.Ctx)
	defer tracker.Stop()

	// Command navigation.
	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", goal.X, goal.Y, goal.Z)
	sayCmd := fmt.Sprintf(">>>%s<<< %s", leader.Name, navCmd)
	_, err = s.Inst.RCON.Say(s.Ctx, sayCmd).Exec(s.Ctx)
	require.NoError(t, err, "send navigation command")
	logger.Logf("Navigation command sent: %s", navCmd)

	// Calculate timeout based on segment type.
	distance := start.DistanceTo(goal)
	timeout := 60 * time.Second // Conservative timeout for vertical movement
	if distance > 10 {
		timeout = 90 * time.Second
	}
	logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

	// Wait for agent to reach goal.
	err = tracker.WaitForPosition(s.Ctx, leader.Name, goal, 1.5, timeout)
	if err != nil {
		finalPos, ok := tracker.GetPosition(leader.Name)
		if ok {
			finalDist := finalPos.DistanceTo(goal)
			logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from goal: %.2f)",
				finalPos.X, finalPos.Y, finalPos.Z, finalDist)
		}
		require.NoError(t, err, "agent should reach goal")
	}

	// Get final position via RCON.
	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, leader.Name)
	require.NoError(t, err, "get final position")
	finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}

	telemetry := telemetryRecorder.Stop(finalPos)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from goal: %.2f blocks", finalPos.DistanceTo(goal))
	logger.Logf("Telemetry: jumps=%d climb_ticks=%d sneak_ticks=%d total_ticks=%d",
		telemetry.JumpCount, telemetry.ClimbTicks, telemetry.SneakTicks, telemetry.TotalTicks)
	logger.Logf("Movement types used: %v", telemetry.MovementTypes)

	// Assertions.

	// 1. Agent reached goal within tolerance.
	finalDistance := finalPos.DistanceTo(goal)
	assert.LessOrEqual(t, finalDistance, 1.5, "agent should reach goal within 1.5 blocks")

	// 2. Telemetry assertions (based on segment expectations).
	expected := segment.GetExpectedTelemetry()

	if expected.RequireClimb {
		assert.Greater(t, telemetry.ClimbTicks, 0, "must use climbing for this segment")
	}

	if !expected.AllowJumps {
		assert.Equal(t, 0, telemetry.JumpCount, "this segment should not require jumps")
	}

	if expected.MinJumps > 0 {
		assert.GreaterOrEqual(t, telemetry.JumpCount, expected.MinJumps,
			"segment requires minimum number of jumps")
	}

	if expected.MaxJumps > 0 && telemetry.JumpCount > expected.MaxJumps {
		t.Logf("warning: jump count (%d) exceeded maximum (%d)", telemetry.JumpCount, expected.MaxJumps)
		// Don't fail, just log warning - jumps may vary with physics
	}
}
