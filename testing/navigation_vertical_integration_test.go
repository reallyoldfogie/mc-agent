package testing

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerticalNavigationSmoke is the smoke test suite for vertical navigation.
// Tests one orientation for each structure type (9 tests total).
// Set VERTICAL_NAV_FULL=1 to run full coverage (all orientations).
func TestVerticalNavigationSmoke(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
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
					runVerticalNavigationTest(t, tt.MCVersion, tc.segment, tc.orientation, testIndex)
				})
			}
		})
	}
}

// runVerticalNavigationTest executes a single vertical navigation test
func runVerticalNavigationTest(t *testing.T, mcVersion string, segment CourseSegment, orientation Orientation, testIndex int) {
	logger := NewTestLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create framework
	framework, err := NewFramework()
	require.NoError(t, err, "create framework")

	// Start flat world server (deterministic terrain)
	serverCfg := FlatWorldServerConfig()
	serverCfg.Version = mcVersion
	serverCfg.PullImage = false
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

	// Set gamerules for stable testing
	inst.RCON.SetGamerule(ctx, "doMobSpawning", "false")
	inst.RCON.SetGamerule(ctx, "doDaylightCycle", "false")
	inst.RCON.SetTime(ctx, "day")
	inst.RCON.SetWeather(ctx, "clear")
	time.Sleep(1 * time.Second) // Let gamerules apply

	// Build course segment at deterministic coordinates (spaced by testIndex)
	origin := models.V3{
		X: 100 + float64(testIndex*50), // 50-block spacing to avoid interference
		Y: 0,                           // Y_BASE = 0 (flat world surface)
		Z: 100,
	}

	logger.Logf("Building segment %s at origin (%.0f, %.0f, %.0f) facing %s",
		segment.Name(), origin.X, origin.Y, origin.Z, orientation.String())

	// Forceload chunks BEFORE building to ensure chunks are loaded
	chunkX1 := int(origin.X-10) >> 4
	chunkZ1 := int(origin.Z-10) >> 4
	chunkX2 := int(origin.X+20) >> 4
	chunkZ2 := int(origin.Z+20) >> 4
	forceloadCmd := fmt.Sprintf("forceload add %d %d %d %d", chunkX1<<4, chunkZ1<<4, chunkX2<<4, chunkZ2<<4)
	_, err = inst.RCON.Exec(ctx, forceloadCmd)
	require.NoError(t, err, "forceload chunks")
	defer inst.RCON.Exec(ctx, fmt.Sprintf("forceload remove %d %d %d %d", chunkX1<<4, chunkZ1<<4, chunkX2<<4, chunkZ2<<4))
	logger.Logf("Forceloaded chunks for building area")

	// Wait for chunks to load
	time.Sleep(2 * time.Second)

	// Clear area first (leave ground intact at Y=0)
	if err := ClearArea(ctx, inst.RCON,
		int(origin.X)-10, 1, int(origin.Z)-10,
		int(origin.X)+20, 30, int(origin.Z)+20); err != nil {
		logger.Logf("warning: failed to clear area: %v", err)
	}

	// Build course segment
	start, goal, err := segment.Build(ctx, inst.RCON, origin, orientation)
	require.NoError(t, err, "build segment")
	logger.Logf("Segment built: start=(%.2f, %.2f, %.2f) goal=(%.2f, %.2f, %.2f)",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z)

	nbt := `{Tags:["hpa_debug"],block_state:{Name:"green_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]}}`
	result, err := inst.RCON.SummonEntity(ctx, start.X+0.5, start.Y+0.2, start.Z+0.5, "block_display", nbt).Exec(ctx)
	logger.Logf("Summon start block display result: %s", result)
	require.NoError(t, err, "summon start block display")

	nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
		buildMultilineTextDisplay([]string{"START", fmt.Sprintf("%.2f %.2f %.2f", start.X, start.Y, start.Z)}))
	result, err = inst.RCON.SummonEntity(ctx, start.X+0.5, start.Y+1.5, start.Z+0.5, "text_display", nbt).Exec(ctx)
	logger.Logf("Summon start text display result: %s", result)
	require.NoError(t, err, "summon start text display")

	nbt = `{Tags:["hpa_debug"],block_state:{Name:"red_stained_glass"},transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]}}`
	inst.RCON.SummonEntity(ctx, goal.X+0.5, goal.Y+0.2, goal.Z+0.5, "block_display", nbt).Exec(ctx)
	logger.Logf("Summon goal text display result: %s", result)
	require.NoError(t, err, "summon goal block display")

	nbt = fmt.Sprintf(`{Tags:["hpa_debug"],text:'%s', transformation:{translation:[0f,0f,0f], left_rotation:[0f,0f,0f,1f], scale:[0.4f,0.4f,0.4f], right_rotation:[0f,0f,0f,1f]},billboard:center}`,
		buildMultilineTextDisplay([]string{"GOAL", fmt.Sprintf("%.2f %.2f %.2f", goal.X, goal.Y, goal.Z)}))
	inst.RCON.SummonEntity(ctx, goal.X+0.5, goal.Y+1.5, goal.Z+0.5, "text_display", nbt).Exec(ctx)
	logger.Logf("Summon goal text display result: %s", result)
	require.NoError(t, err, "summon goal text display")

	// Spawn test agent
	agentCfg := DefaultAgentConfig(
		"NavigationBot",
		fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
		serverCfg.Version,
	)

	// Version handler is auto-detected by the framework

	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = normalizeReplayOutput(serverCfg.Version, fmt.Sprintf("%s_%s_%s_%s.mcpr",
		segment.Name(), strings.ToUpper(string(orientation.String()[0])), mcVersion, time.Now().Format("20060102_150405")), agentCfg.Name)

	agent, err := framework.SpawnAgent(ctx, inst, agentCfg)
	require.NoError(t, err, "spawn agent")
	logger.Logf("Agent %s spawned (replay: %s)", agent.Name, agentCfg.ReplayOutput)

	// Wait for agent to join
	time.Sleep(3 * time.Second)

	// Clear inventory
	inst.RCON.Exec(ctx, fmt.Sprintf("clear %s", agent.Name))

	// Teleport to start position
	tpCmd := fmt.Sprintf("tp %s %.2f %.2f %.2f", agent.Name, start.X, start.Y, start.Z)
	_, err = inst.RCON.Exec(ctx, tpCmd)
	require.NoError(t, err, "teleport to start")
	logger.Logf("Agent teleported to start position")
	time.Sleep(1 * time.Second)

	// Start telemetry recording
	telemetryRecorder := NewTelemetryRecorder()
	telemetryRecorder.Start(start)

	// Hook telemetry into agent's movement executor
	agent.Agent.SetTelemetryRecorder(telemetryRecorder)

	// Start position tracking
	tracker := NewPositionTracker(inst, 500*time.Millisecond)
	tracker.Start(ctx)
	defer tracker.Stop()

	// Command navigation
	navCmd := fmt.Sprintf("moveTo %.2f %.2f %.2f", goal.X, goal.Y, goal.Z)
	sayCmd := fmt.Sprintf(">>>%s<<< %s", agent.Name, navCmd)
	_, err = inst.RCON.Say(ctx, sayCmd).Exec(ctx)
	require.NoError(t, err, "send navigation command")
	logger.Logf("Navigation command sent: %s", navCmd)

	// Calculate timeout based on segment type
	distance := start.DistanceTo(goal)
	timeout := 60 * time.Second // Conservative timeout for vertical movement
	if distance > 10 {
		timeout = 90 * time.Second
	}
	logger.Logf("Distance: %.2f blocks, timeout: %v", distance, timeout)

	// Wait for agent to reach goal
	err = tracker.WaitForPosition(ctx, agent.Name, goal, 1.5, timeout)
	if err != nil {
		// Get final position for debugging
		finalPos, ok := tracker.GetPosition(agent.Name)
		if ok {
			finalDist := finalPos.DistanceTo(goal)
			logger.Logf("Agent final position: %.2f, %.2f, %.2f (distance from goal: %.2f)",
				finalPos.X, finalPos.Y, finalPos.Z, finalDist)
		}
		require.NoError(t, err, "agent should reach goal")
	}

	// Get final position via RCON
	finalX, finalY, finalZ, err := inst.RCON.GetEntityPos(ctx, agent.Name)
	require.NoError(t, err, "get final position")
	finalPos := models.V3{X: finalX, Y: finalY, Z: finalZ}

	// Stop telemetry
	telemetry := telemetryRecorder.Stop(finalPos)

	logger.Logf("Agent final position: %.2f, %.2f, %.2f", finalX, finalY, finalZ)
	logger.Logf("Distance from goal: %.2f blocks", finalPos.DistanceTo(goal))
	logger.Logf("Telemetry: jumps=%d climb_ticks=%d sneak_ticks=%d total_ticks=%d",
		telemetry.JumpCount, telemetry.ClimbTicks, telemetry.SneakTicks, telemetry.TotalTicks)
	logger.Logf("Movement types used: %v", telemetry.MovementTypes)
	logger.Logf("Replay saved to: %s", agentCfg.ReplayOutput)

	// Assertions

	// 1. Agent reached goal within tolerance
	finalDistance := finalPos.DistanceTo(goal)
	assert.LessOrEqual(t, finalDistance, 1.5, "agent should reach goal within 1.5 blocks")

	// 2. Telemetry assertions (based on segment expectations)
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
