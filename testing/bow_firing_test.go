package testing

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// analyzeArrowTrajectory parses arrow trajectory from agent logs and compares with predictions
func analyzeArrowTrajectory(t *testing.T, inst *TestInstance, agent *ManagedAgent,
	botX, botY, botZ float64,
	targetX, targetY, targetZ float64,
	trajectory []models.TrajectoryPoint) {
	if inst.AgentLogFile == "" {
		t.Logf("[TEST] No agent log file path available for trajectory analysis")
		return
	}

	logFile := inst.AgentLogFile

	// Parse trajectory from logs
	positions, err := AnalyzeArrowTrajectory(logFile)
	if err != nil {
		t.Logf("[TEST] Failed to analyze arrow trajectory: %v", err)
		return
	}

	if len(positions) == 0 {
		t.Logf("[TEST] No arrow trajectory data found in logs")
		return
	}

	// Compare with predictions
	botOrigin := models.V3{X: botX, Y: botY, Z: botZ}
	targetOrigin := models.V3{X: targetX, Y: targetY, Z: targetZ}
	CompareTrajectories(positions, botOrigin, targetOrigin, trajectory)
}

// setupTargetMechanism places:
// - a target block (to be hit by arrow)
// - a piston adjacent to the target, facing away from it
// - a glowstone block in front of the piston to be pushed
// Returns the original glowstone coordinates.
func setupTargetMechanism(ctx context.Context, t *testing.T, rcon testenv.RCONHelper, targetX, targetY, targetZ int) (glowstoneX, glowstoneY, glowstoneZ int, err error) {
	platformY := targetY - 1 // Platform is below target
	platformX := targetX - 10
	platformZ := targetZ - 10

	// Build the platform first so agent doesn't fall
	BuildPlatform(ctx, rcon, platformX, platformY, platformZ, 20, 20, "minecraft:grass_block")

	// Clear area above the platform
	if err := ClearArea(ctx, rcon,
		platformX, platformY+1, platformZ,
		platformX+20, platformY+5, platformZ+20); err != nil {
		t.Logf("warning: failed to clear area: %v", err)
	}

	// Target block at (targetX, targetY, targetZ)
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:target`, targetX, targetY, targetZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("set target: %w", err)
	}

	// Normal piston directly east of target, facing west (toward the target)
	pistonX, pistonY, pistonZ := targetX+1, targetY, targetZ
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:piston[facing=east]`, pistonX, pistonY, pistonZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("set piston: %w", err)
	}

	// Glowstone two blocks east of target (one in front of the piston head)
	glowstoneX, glowstoneY, glowstoneZ = pistonX+1, pistonY, pistonZ
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, glowstoneX, glowstoneY, glowstoneZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("set glowstone: %w", err)
	}

	// Ensure space for piston to push (clear the block one more east)
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:air`, glowstoneX+1, glowstoneY, glowstoneZ)); err != nil {
		return 0, 0, 0, fmt.Errorf("clear space: %w", err)
	}
	return glowstoneX, glowstoneY, glowstoneZ, nil
}

// verifyBlockAtPosition returns true if block is at (gx,gy,gz) using agent's world data.
// This checks the blocks the agent received in chunk updates, so it knows immediately
// when blocks are modified (like when a piston pushes glowstone).
func verifyBlockAtPosition(agent *ManagedAgent, gx, gy, gz int, expectedBlockName string) (bool, error) {
	// Get the block from the agent's world view
	blockName := agent.Agent.BlockNameAt(gx, gy, gz)

	// Check if the block name matches (handle both "glowstone" and "minecraft:glowstone")
	expectedName := expectedBlockName
	if !strings.Contains(expectedName, ":") {
		expectedName = "minecraft:" + expectedBlockName
	}

	log.Printf("[TEST] verifyBlockAtPosition: expected=%s, got=%s at (%d, %d, %d)", expectedName, blockName, gx, gy, gz)

	return blockName == expectedName, nil
}

// fireAt builds a target mechanism in front of the bot and fires the bow at it.
// fireAt fires an arrow at a target distance and optionally validates via callback
// Accepts optional callbacks that will be registered with the arrow projectile
func fireAt(t *testing.T, inst *TestInstance, agent *ManagedAgent, distance int, callbacks ...models.ProjectileHitCallback) (target models.V3) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	agent.Agent.SendChat(fmt.Sprintf("FireBow at target. Distance %d blocks", distance))

	// Make sure bow is equipped explicitly (active slot cannot be assumed)
	require.NoError(t, agent.EquipItemByName(ctx, "minecraft:bow"))

	botX, botY, botZ, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	// Use math.Floor() to properly convert world coordinates to block coordinates
	// int() truncates towards zero, which breaks negative coordinates (e.g., int(-0.50) = 0, not -1)
	// math.Floor() properly rounds down for all values
	targetX := int(math.Floor(botX)) + distance
	targetY := int(math.Floor(botY))
	targetZ := int(math.Floor(botZ))

	target = models.V3{X: float64(targetX), Y: float64(targetY), Z: float64(targetZ)}

	gx, gy, gz, err := setupTargetMechanism(ctx, t, inst.RCON, targetX, targetY, targetZ)
	require.NoError(t, err, "build target mechanism")

	time.Sleep(400 * time.Millisecond)

	// Turn bot's body to face the target before firing
	require.NoError(t, agent.TurnTowards(ctx, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5))
	// Wait for server to process the rotation update
	time.Sleep(400 * time.Millisecond)

	var trajectory []models.TrajectoryPoint
	var fireErr error

	packetWriter := agent.Agent.GetPacketLogWriter()

	var projectileHitEvent models.ProjectileHitEvent
	callbacks = append(callbacks, func(evt models.ProjectileHitEvent) {
		projectileHitEvent = evt
		fmt.Fprintf(packetWriter, ">>>>> End FireBowAtDebug %d blocks <<<<<\n", distance)
	})

	// Fire bow with optional callback(s)
	fmt.Fprintf(packetWriter, ">>>>> Start FireBowAtDebug %d blocks <<<<<\n", distance)
	if traj, err := agent.Agent.FireBowAtDebug(float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5, callbacks...); err == nil {
		trajectory = traj
		fireErr = err
	} else {
		fireErr = err
	}

	// Fire using the new API at the center of the target block
	if fireErr != nil {
		t.Logf("FireBowAt error: %v", fireErr)
	}
	require.NoError(t, fireErr)

	// Allow time for arrow flight and piston action
	time.Sleep(5 * time.Second)

	glowstoneStillThere, err := verifyBlockAtPosition(agent, gx, gy, gz, "minecraft:glowstone")
	require.NoError(t, err, "verify original glowstone position")
	glowstoneMoved, err := verifyBlockAtPosition(agent, gx+1, gy, gz, "minecraft:glowstone")
	require.NoError(t, err, "verify glowstone moved")

	distFromTarget := projectileHitEvent.Position.DistanceTo(target)
	t.Logf("ProjectileHitEvent: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.02f blocks)", projectileHitEvent.HitType, projectileHitEvent.ProjectileType,
		projectileHitEvent.Position.X, projectileHitEvent.Position.Y, projectileHitEvent.Position.Z, distFromTarget)

	if glowstoneMoved {
		agent.Agent.SendChat("Success: Target hit and glowstone moved!")
	} else {
		agent.Agent.SendChat(fmt.Sprintf("Failure: Target not hit, glowstone did not move. (landed at %.2f, %.2f, %.2f - %.2f blocks away)", projectileHitEvent.Position.X, projectileHitEvent.Position.Y, projectileHitEvent.Position.Z, distFromTarget))
	}

	// Analyze arrow trajectory from logs
	analyzeArrowTrajectory(t, inst, agent, float64(botX)+0.5, float64(botY)+0.5, float64(botZ)+0.5,
		float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5, trajectory)

	assert.False(t, glowstoneStillThere, "glowstone should have moved from original position if target was hit")
	assert.True(t, glowstoneMoved, "glowstone should be at new position if target was hit")
	return target
}

// TestBowFiring_FireBowAt verifies FireBowAt hits a target block that triggers a piston
func TestBowFiring_FireBowAt(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.mcVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot_FireBowAt",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				srv.Version,
			)
			ag, err := fw.SpawnAgent(ctx, inst, agCfg)
			require.NoError(t, err)
			defer func() {
				if ag != nil && ag.BotClient() != nil {
					_ = ag.BotClient().Close()
				}
			}()

			// Let the agent join fully
			time.Sleep(2 * time.Second)

			// Give bow and arrows
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)

			// Setup callback channel for arrow
			hitCh := make(chan models.ProjectileHitEvent, 1)
			callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

			target := fireAt(t, inst, ag, 10, callback)

			// Wait for callback (non-blocking)
			select {
			case evt := <-hitCh:
				distanceFromTarget := evt.Position.DistanceTo(target)

				t.Logf("✓ Arrow callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.2f blocks away)",
					evt.HitType, evt.ProjectileType, evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)
				// Verify callback matches this projectile type
				assert.Equal(t, models.Arrow, evt.ProjectileType, "callback projectile type should be Arrow")
			case <-time.After(5 * time.Second):
				t.Logf("⚠ Arrow callback did not fire (timeout)")
			}

			time.Sleep(3 * time.Second) // give a little time after the test for inspection etc.
		})
	}
}

// TestBowFiring_MultipleDistances hits targets at multiple ranges
func TestBowFiring_MultipleDistances(t *testing.T) {
	for _, tt := range standardVersionTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.mcVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot_MultiDist",
				fmt.Sprintf("%s:%d", inst.Server.Host, inst.Server.HostServerPort),
				srv.Version,
			)
			agnt, err := fw.SpawnAgent(ctx, inst, agCfg)
			require.NoError(t, err)
			defer func() {
				if agnt != nil && agnt.BotClient() != nil {
					_ = agnt.BotClient().Close()
				}
			}()

			// Join + give items
			time.Sleep(2 * time.Second)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, agnt.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 128`, agnt.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			for _, distance := range []int{5, 10, 15, 30} {
				t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
					// Setup callback channel for arrow
					hitCh := make(chan models.ProjectileHitEvent, 1)
					callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

					fireAt(t, inst, agnt, distance, callback)

					// Wait for callback (non-blocking)
					select {
					case evt := <-hitCh:
						t.Logf("✓ Arrow callback at %d blocks: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f)", distance, evt.HitType, evt.ProjectileType, evt.Position.X, evt.Position.Y, evt.Position.Z)
						// Verify callback matches this projectile type
						assert.Equal(t, models.Arrow, evt.ProjectileType, "callback projectile type should be Arrow")
					case <-time.After(5 * time.Second):
						t.Logf("⚠ Arrow callback at %d blocks did not fire (timeout)", distance)
					}
				})
			}
		})
	}
}
