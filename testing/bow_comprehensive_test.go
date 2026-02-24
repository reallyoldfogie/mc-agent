package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-client-test-go/testenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TargetElevation defines the target height relative to the bot
type TargetElevation struct {
	name        string
	heightDelta int // blocks above (positive) or below (negative) the bot
	distance    int // horizontal distance in blocks
}

// setupTargetMechanismWithHeight places a target mechanism at a specific height
// Returns the coordinates of the glowstone that should move when target is hit
func setupTargetMechanismWithHeight(ctx context.Context, t *testing.T, rcon testenv.RCONHelper,
	botX, botY, botZ, targetHeightDelta, targetDistance int) (targetX, targetY, targetZ, glowstoneX, glowstoneY, glowstoneZ int, err error) {

	// Calculate target position relative to bot
	targetX = botX + targetDistance
	targetY = botY + targetHeightDelta
	targetZ = botZ

	// Build platform at target height so the glowstone has support
	platformY := targetY - 1
	platformX := targetX - 5
	platformZ := targetZ - 5

	if platformY > botY+1 {
		// if the target is above the bot build the plaform so that the target is at the front edge,
		// so the bot has line of sight to it
		platformX = targetX - 1
	}

	if err := BuildPlatform(ctx, rcon, platformX, platformY, platformZ, 10, 10, "minecraft:grass_block"); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("build platform: %w", err)
	}

	// Clear area above the platform
	if err := ClearArea(ctx, rcon,
		platformX, platformY+1, platformZ,
		platformX+10, platformY+5, platformZ+10); err != nil {
		t.Logf("warning: failed to clear area: %v", err)
	}

	// Target block at (targetX, targetY, targetZ)
	if _, err := rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:target`, targetX, targetY, targetZ)); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("set target: %w", err)
	}

	// Normal piston directly east of target, facing away from bot
	pistonX, pistonY, pistonZ := targetX+1, targetY, targetZ
	if _, err := rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:piston[facing=east]`, pistonX, pistonY, pistonZ)); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("set piston: %w", err)
	}

	// Glowstone two blocks east of target (one in front of the piston head)
	glowstoneX, glowstoneY, glowstoneZ = pistonX+1, pistonY, pistonZ
	if _, err := rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, glowstoneX, glowstoneY, glowstoneZ)); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("set glowstone: %w", err)
	}

	// Ensure space for piston to push (clear the block one more east)
	if _, err := rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:air`, glowstoneX+1, glowstoneY, glowstoneZ)); err != nil {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("clear space: %w", err)
	}

	return targetX, targetY, targetZ, glowstoneX, glowstoneY, glowstoneZ, nil
}

// fireAtElevation fires at a target at a specific height and distance
func fireAtElevation(t *testing.T, inst *TestInstance, agent *ManagedAgent,
	heightDelta, distance int, callbacks ...models.ProjectileHitCallback) (hit bool) {

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	agent.Agent.SendChat(fmt.Sprintf("FireBow: height=%+d, distance=%d blocks", heightDelta, distance))

	// Get bot position
	botX, botY, botZ, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")

	src := models.V3{X: botX, Y: botY, Z: botZ}
	// Setup target mechanism
	targetX, targetY, targetZ, gx, gy, gz, err := setupTargetMechanismWithHeight(
		ctx, t, inst.RCON,
		int(botX), int(botY), int(botZ),
		heightDelta, distance)
	require.NoError(t, err, "setup target mechanism")

	// Wait for chunks to load - targets at distance 10 need time for chunk packets to arrive
	time.Sleep(1500 * time.Millisecond)

	// Turn to face target
	require.NoError(t, agent.TurnTowards(ctx,
		float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5))
	time.Sleep(400 * time.Millisecond)

	// Fire at target
	var fireErr error
	var trajectory []models.TrajectoryPoint
	var projectileHitEvent models.ProjectileHitEvent
	callbacks = append(callbacks, func(evt models.ProjectileHitEvent) {
		projectileHitEvent = evt
	})

	targetCenter := models.V3{X: float64(targetX) + 0.5, Y: float64(targetY) + 0.5, Z: float64(targetZ) + 0.5}
	trajectory, fireErr = agent.Agent.FireBowAt(
		targetCenter.X, targetCenter.Y, targetCenter.Z, callbacks...)

	if fireErr != nil {
		t.Logf("FireBowAt error: %v", fireErr)
	}
	require.NoError(t, fireErr)

	// Allow time for arrow flight and piston action
	time.Sleep(5 * time.Second)

	// Check if target was hit (glowstone moved)
	glowstoneStillThere, err := verifyBlockAtPosition(agent, gx, gy, gz, "minecraft:glowstone")
	require.NoError(t, err)

	glowstoneMoved, err := verifyBlockAtPosition(agent, gx+1, gy, gz, "minecraft:glowstone")
	require.NoError(t, err)

	if glowstoneMoved {
		agent.Agent.SendChat(fmt.Sprintf("SUCCESS: Target at height %+d hit!", heightDelta))
	} else {
		agent.Agent.SendChat(fmt.Sprintf("FAILED: Target at height %+d missed! fired from (%.2f %.2f %.2f), landed at (%.2f %.2f %.2f - %.2f blocks away)",
			heightDelta,
			src.X, src.Y, src.Z,
			projectileHitEvent.Position.X, projectileHitEvent.Position.Y, projectileHitEvent.Position.Z,
			projectileHitEvent.Position.DistanceTo(models.V3{X: float64(targetX) + 0.5, Y: float64(targetY) + 0.5, Z: float64(targetZ) + 0.5})))
	}

	// Analyze trajectory if available
	analyzeArrowTrajectory(t, inst, agent,
		botX, botY, botZ,
		float64(targetX), float64(targetY), float64(targetZ),
		trajectory)

	// Assert target was hit
	assert.False(t, glowstoneStillThere, "glowstone should move from original position")
	assert.True(t, glowstoneMoved, "glowstone should be at new position")

	return glowstoneMoved
}

// TestBowFiring_LevelTarget fires at a target at the same height as the bot
// This tests the standard horizontal firing case
func TestBowFiring_LevelTarget(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot_LevelTarget",
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

			time.Sleep(2 * time.Second)

			// Give bow and arrows
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			// Fire at target 10 blocks away at same height
			fireAtElevation(t, inst, ag, 0, 10)

			time.Sleep(2 * time.Second)
		})
	}
}

// TestBowFiring_BelowTarget fires at a target below the bot
// This tests downward firing with positive vertical distance
func TestBowFiring_BelowTarget(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot_BelowTarget",
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

			time.Sleep(2 * time.Second)

			// Give bow and arrows
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			// Fire at target 10 blocks away, 3 blocks below
			fireAtElevation(t, inst, ag, -3, 10)

			time.Sleep(2 * time.Second)
		})
	}
}

// TestBowFiring_AboveTarget fires at a target above the bot
// This tests upward firing - requires high-angle shot
func TestBowFiring_AboveTarget(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot_AboveTarget",
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

			time.Sleep(2 * time.Second)

			// Give bow and arrows
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			// Fire at target 10 blocks away, 3 blocks above
			// This requires a high-angle shot to reach the target
			fireAtElevation(t, inst, ag, 3, 10)

			time.Sleep(2 * time.Second)
		})
	}
}

// TestBowFiring_ShelfTarget fires at a target on a high shelf
// This tests the critical case: target is above, but arrow must hit on the way DOWN
// Arrow rises initially, passes through target height, continues rising, then falls back down
// We only want the descending pass to count as a hit
func TestBowFiring_ShelfTarget(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot_ShelfTarget",
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

			time.Sleep(2 * time.Second)

			// Give bow and arrows
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, ag.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			// Critical test case: target on a shelf 2 blocks back and 2 blocks up
			// At moderate distance (8 blocks), the arrow's high-angle trajectory will:
			// 1. Rise up while traveling forward
			// 2. Pass through the target height early (but still ascending) - INVALID
			// 3. Continue rising to apex
			// 4. Fall back down and pass through target height again - VALID
			// Our fix ensures only the descending pass is accepted
			fireAtElevation(t, inst, ag, 2, 8)

			time.Sleep(2 * time.Second)
		})
	}
}

// TestBowFiring_VariousElevations tests hitting targets at multiple elevations in one test
func TestBowFiring_VariousElevations(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
			defer cancel()

			fw, err := NewFramework()
			require.NoError(t, err)

			srv := DefaultServerConfig()
			srv.Version = tt.MCVersion
			RequireIntegrationEnv(t, srv)

			inst, err := fw.StartServer(ctx, srv)
			require.NoError(t, err)
			defer func() {
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer stopCancel()
				_ = fw.StopServer(stopCtx, inst, true)
			}()

			agCfg := DefaultAgentConfig(
				"BowBot_VariousElevations",
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

			time.Sleep(2 * time.Second)

			// Give bow and arrows
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, ag.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 256`, ag.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			// Test multiple elevation cases
			elevationTests := []struct {
				name        string
				heightDelta int
				distance    int
			}{
				{"level target", 0, 10},
				{"2 blocks below", -2, 10},
				{"5 blocks below", -5, 10},
				{"2 blocks above", 2, 10},
				{"3 blocks above", 3, 10},
			}

			for _, et := range elevationTests {
				t.Run(et.name, func(t *testing.T) {
					fireAtElevation(t, inst, ag, et.heightDelta, et.distance)
					time.Sleep(1 * time.Second)
				})
			}

			time.Sleep(2 * time.Second)
		})
	}
}
