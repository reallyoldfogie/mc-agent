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
	"github.com/stretchr/testify/suite"
)

// ProjectileFlatSuite covers every live-server bow/projectile-throwing test
// (previously spread across bow_firing_test.go, bow_comprehensive_test.go,
// bow_power_comparison_test.go, arrow_physics_calibration_test.go, and
// projectile_throwing_test.go's own live-server functions). None of these
// tests touch ScreenMgr or any container GUI, so nothing here is
// window-ID-constrained.
//
// WorldGen = WorldGenFlat, NOT the WorldGenRandom every pre-conversion
// function in this cluster actually used (DefaultServerConfig()'s own
// default) - a deliberate, evidence-based deviation from "preserve the
// original config," not an assumption. The first live run of this suite
// used WorldGenRandom (reasoning: every test already builds its own target
// platform, so preserving the original config seemed to cost nothing) and
// failed hard: ThrowProjectileAt's own trajectory solver reported "ALL
// TRAJECTORIES BLOCKED" for every EnderPearlRange distance at one working
// area, and several fireAt/fireAtElevation shots (bow tests, which DO clear
// a platform around the target) still missed. Root cause confirmed by
// inspecting the logged bot positions and command-block placements: only
// the immediate target vicinity gets flattened (setupTargetMechanism/
// setupTargetMechanismWithHeight's own BuildPlatform+ClearArea calls) - the
// rest of a 10-60 block flight path crosses whatever real, unflattened
// random terrain happens to lie in between, at whatever working-area X
// offset a given test method draws. The pre-conversion, one-server-per-test
// versions of these tests never hit this because every one of them ran
// solo at the world's natural spawn point - a shared suite's working areas
// spread out along X (see NextWorkingAreaOffset), eventually reaching
// terrain a solo spawn-adjacent test never had to fly over. Unlike
// NavigationRandomSuite (which specifically wants to exercise pathfinding
// over real terrain), nothing about a ballistics test's own assertions
// depends on the terrain being "real" - an open, obstruction-free flat
// world removes this entire failure class without weakening what's being
// tested.
type ProjectileFlatSuite struct {
	VersionWorldSuite
}

func TestProjectileFlatSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &ProjectileFlatSuite{}
		s.WorldGen = WorldGenFlat
		return s
	})
}

// --- Shared helpers (moved unchanged from bow_firing_test.go/
// bow_comprehensive_test.go/projectile_throwing_test.go - every one of these
// already took *TestInstance/*ManagedAgent parameters rather than
// *StandaloneTestEnv, so no generalization was needed to call them from
// suite methods, unlike most other conversions in this package.) ---

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
func setupTargetMechanism(ctx context.Context, t *testing.T, rcon testenv.RCONHelper, targetX, targetY, targetZ int) (glowstonePos, platformPos models.V3, err error) {
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
		return models.V3{}, models.V3{}, fmt.Errorf("set target: %w", err)
	}

	// Normal piston directly east of target, facing west (toward the target)
	pistonX, pistonY, pistonZ := targetX+1, targetY, targetZ
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:piston[facing=east]`, pistonX, pistonY, pistonZ)); err != nil {
		return models.V3{}, models.V3{}, fmt.Errorf("set piston: %w", err)
	}

	// Glowstone two blocks east of target (one in front of the piston head)
	glowstoneX, glowstoneY, glowstoneZ := pistonX+1, pistonY, pistonZ
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, glowstoneX, glowstoneY, glowstoneZ)); err != nil {
		return models.V3{}, models.V3{}, fmt.Errorf("set glowstone: %w", err)
	}

	// Ensure space for piston to push (clear the block one more east)
	if _, err = rcon.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:air`, glowstoneX+1, glowstoneY, glowstoneZ)); err != nil {
		return models.V3{}, models.V3{}, fmt.Errorf("clear space: %w", err)
	}
	return models.V3{X: float64(glowstoneX), Y: float64(glowstoneY), Z: float64(glowstoneZ)}, models.V3{X: float64(platformX), Y: float64(platformY), Z: float64(platformZ)}, nil
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
func fireAt(ctx context.Context, t *testing.T, inst *TestInstance, agent *ManagedAgent, distance int, callbacks ...models.ProjectileHitCallback) (target models.V3) {
	// Create child context with additional timeout as backup, but inherit from parent
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	agent.Agent.SendChat(fmt.Sprintf("FireBow at target. Distance %d blocks", distance))

	// Make sure bow is equipped explicitly (active slot cannot be assumed)
	require.NoError(t, agent.EquipItemByName(ctx, "minecraft:bow"))

	botPos, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	botX, botY, botZ := botPos.X, botPos.Y, botPos.Z
	// Use math.Floor() to properly convert world coordinates to block coordinates
	// int() truncates towards zero, which breaks negative coordinates (e.g., int(-0.50) = 0, not -1)
	// math.Floor() properly rounds down for all values
	targetX := int(math.Floor(botX)) + distance
	targetY := int(math.Floor(botY))
	targetZ := int(math.Floor(botZ))

	target = models.V3{X: float64(targetX), Y: float64(targetY), Z: float64(targetZ)}

	glowstonePos, platformPos, err := setupTargetMechanism(ctx, t, inst.RCON, targetX, targetY, targetZ)
	defer func() {
		platformX, platformY, platformZ := platformPos.Floor()
		// Clear area above the platform before the next test to prevent interference (non-blocking cleanup)
		if err := ClearArea(ctx, inst.RCON,
			platformX, platformY+1, platformZ,
			platformX+20, platformY+5, platformZ+20); err != nil {
			t.Logf("warning: failed to clear area (Post Test): %v", err)
		}
	}()
	require.NoError(t, err, "build target mechanism")

	time.Sleep(400 * time.Millisecond)

	cmd := fmt.Sprintf(`/setblock %d %d %d repeating_command_block[facing=up]{Command:"execute at @e[type=arrow] run particle minecraft:flame ~ ~ ~ 0 0 0 0.01 1"} replace`, int(math.Floor(botX)), int(math.Floor(botY-1)), int(math.Floor(botZ)))
	cmdBlockResponse, err := inst.RCON.Exec(ctx, cmd)
	t.Logf("%s => %s", cmd, cmdBlockResponse)
	require.NoError(t, err, "give command failed")

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
	fmt.Fprintf(packetWriter, ">>>>> Start FireBowAt %d blocks <<<<<\n", distance)
	if traj, err := agent.Agent.FireBowAt(context.Background(), float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5, callbacks...); err == nil {
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

	gx, gy, gz := glowstonePos.Floor()
	glowstoneStillThere, err := verifyBlockAtPosition(agent, gx, gy, gz, "minecraft:glowstone")
	require.NoError(t, err, "verify original glowstone position")
	glowstoneMoved, err := verifyBlockAtPosition(agent, gx+1, gy, gz, "minecraft:glowstone")
	require.NoError(t, err, "verify glowstone moved")

	distFromTarget := projectileHitEvent.Position.DistanceTo(target)
	hitEntityIDStr := "none"
	if projectileHitEvent.HitEntityID >= 0 {
		hitEntityIDStr = fmt.Sprintf("%d", projectileHitEvent.HitEntityID)
	}
	t.Logf("ProjectileHitEvent: HitType=%v, ProjectileType=%v, HitResult=%s, HitEntityID=%s, landed=(%.2f %.2f %.2f - %.02f blocks)", projectileHitEvent.HitType, projectileHitEvent.ProjectileType, projectileHitEvent.HitResult, hitEntityIDStr,
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

// fireAtElevation fires at a target at a specific height and distance
func fireAtElevation(t *testing.T, inst *TestInstance, agent *ManagedAgent,
	heightDelta, distance int, callbacks ...models.ProjectileHitCallback) (hit bool) {

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	agent.Agent.SendChat(fmt.Sprintf("FireBow: height=%+d, distance=%d blocks", heightDelta, distance))

	// Get bot position
	src, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")

	botX, botY, botZ := src.X, src.Y, src.Z
	// Setup target mechanism
	targetX, targetY, targetZ, gx, gy, gz, err := setupTargetMechanismWithHeight(
		ctx, t, inst.RCON,
		int(botX), int(botY), int(botZ),
		heightDelta, distance)
	require.NoError(t, err, "setup target mechanism")

	// Wait for chunks to load - targets at distance 10 need time for chunk
	// packets to arrive. 1500ms (this function's original value) was tuned
	// for a solo, spawn-adjacent server; live-testing this suite found
	// "Bot doesn't have line of sight to the target" failures at every
	// elevation angle once a test method's working area landed far enough
	// from spawn (each method gets a fresh, never-before-loaded chunk
	// region - see NextWorkingAreaOffset) - a real chunk-delivery race, not
	// a gameplay miss. Bumped to give a shared server's later, farther-out
	// working areas the same margin a solo near-spawn server always had.
	time.Sleep(3 * time.Second)

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
	// Retry once on "no line of sight" specifically: the 3-second wait above
	// is a best-effort margin for chunk delivery, not a guarantee, and
	// live-testing found it still occasionally insufficient for the
	// steeper up/down angles this function fires at. Any other error fails
	// immediately, same as before.
	for attempt := 0; attempt < 2; attempt++ {
		trajectory, fireErr = agent.Agent.FireBowAt(context.Background(),
			targetCenter.X, targetCenter.Y, targetCenter.Z, callbacks...)
		if fireErr == nil || !strings.Contains(fireErr.Error(), "line of sight") {
			break
		}
		t.Logf("FireBowAt error (attempt %d): %v - retrying after extra chunk-load wait", attempt+1, fireErr)
		time.Sleep(3 * time.Second)
	}

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

// throwProjectile is a helper function that throws a projectile at a target and verifies the hit
// Returns true if the target was hit (glowstone moved for trigger blocks, or teleported for ender pearls)
// Accepts optional callbacks that will be registered with the projectile
func throwProjectile(ctx context.Context, t *testing.T, inst *TestInstance, agent *ManagedAgent, projectileName string, itemCount int, distance int, callbacks ...models.ProjectileHitCallback) (models.V3, bool) {
	// Create child context with additional timeout as backup, but inherit from parent
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	agent.Agent.SendChat(fmt.Sprintf("Throw %s at target. Distance %d blocks", projectileName, distance))

	// Give items to player inventory using /give command
	// Note: /item replace is server-side only and doesn't send client inventory update packets
	// /give command automatically sends inventory update so client sees the items
	cmd := fmt.Sprintf(`give %s minecraft:%s %d`, agent.Name, projectileName, itemCount)
	giveResponse, err := inst.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "give command failed")

	t.Logf("[throwProjectile] %s => %s", cmd, giveResponse)
	t.Logf("[throwProjectile] Gave %s to inventory via RCON: %s", projectileName, cmd)

	// Wait for inventory packet to arrive with the placed item
	// This is more robust than fixed sleep times as it waits for actual inventory sync
	fullItemName := fmt.Sprintf("minecraft:%s", projectileName)
	slot, waitErr := agent.Agent.WaitForHotbarItem(ctx, fullItemName, 3000) // max 3 seconds
	if waitErr != nil {
		// Log detailed diagnostic info if wait fails
		t.Logf("[throwProjectile] WaitForHotbarItem failed for %s: %v (may proceed anyway)", fullItemName, waitErr)
		// Don't fail yet - SelectHotbarSlot(0) should still work since we sent the RCON command
		slot = 0
	} else {
		t.Logf("[throwProjectile] Found %s in hotbar slot %d", projectileName, slot)
	}

	// Select hotbar slot 0 (where we just put the item)
	require.NoError(t, agent.Agent.SelectHotbarSlot(ctx, slot))

	// Give a moment for the selection to be acknowledged
	time.Sleep(200 * time.Millisecond)

	botPos, initialized := agent.Agent.GetPositionSimple()
	require.True(t, initialized, "bot position initialized")
	t.Logf("[throwProjectile] Bot position: %s", botPos)
	targetX := int(botPos.X) + distance
	targetY := int(botPos.Y)
	targetZ := int(botPos.Z)

	target := models.V3{X: float64(targetX), Y: float64(targetY), Z: float64(targetZ)}
	t.Logf("[throwProjectile] Target: (%d, %d, %d) = (%.1f, %.1f, %.1f)", targetX, targetY, targetZ, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5)

	// Map projectile name to physics.ProjectileType
	var projType models.ProjectileType
	switch projectileName {
	case "snowball":
		projType = models.Snowball
	case "ender_pearl":
		projType = models.EnderPearl
	case "egg":
		projType = models.Egg
	case "splash_potion":
		projType = models.SplashPotion
	case "wind_charge":
		projType = models.WindCharge
	default:
		t.Fatalf("Unknown projectile type: %s", projectileName)
	}

	// For ender pearls, verify by checking if player teleported
	if projType == models.EnderPearl {
		return throwEnderPearl(t, inst, agent, ctx, targetX, targetY, targetZ, distance, callbacks...)
	}

	// For other projectiles, use the target block mechanism
	glowstonePos, platformPos, err := setupTargetMechanism(ctx, t, inst.RCON, targetX, targetY, targetZ)
	defer func() {
		platformX, platformY, platformZ := platformPos.Floor()
		// Clear area above the platform before the next test to prevent interference (non-blocking cleanup)
		if err := ClearArea(ctx, inst.RCON,
			platformX, platformY+1, platformZ,
			platformX+20, platformY+5, platformZ+20); err != nil {
			t.Logf("warning: failed to clear area (Post Test): %v", err)
		}
	}()
	require.NoError(t, err, "build target mechanism")

	time.Sleep(400 * time.Millisecond)

	// Turn bot's body to face the target before throwing
	require.NoError(t, agent.TurnTowards(ctx, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5))
	// Wait for server to process the rotation update
	time.Sleep(400 * time.Millisecond)

	packetWriter := agent.Agent.GetPacketLogWriter()

	callbacks = append(callbacks, func(evt models.ProjectileHitEvent) {
		fmt.Fprintf(packetWriter, ">>>>> %s End ThrowProjectileAt %d blocks <<<<<\n", projectileName, distance)
	})

	fmt.Fprintf(packetWriter, ">>>>> %s Start ThrowProjectileAt %d blocks <<<<<\n", projectileName, distance)
	// Throw the projectile with optional callback(s)
	// (Diagnostic report will be logged during trajectory validation)
	_, throwErr := agent.Agent.ThrowProjectileAt(ctx, projType, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5, callbacks...)

	if throwErr != nil {
		t.Logf("ThrowProjectileAt error: %v", throwErr)
	}
	require.NoError(t, throwErr)

	// Allow time for projectile flight and piston action
	time.Sleep(5 * time.Second)

	gx, gy, gz := glowstonePos.Floor()
	_, err = verifyBlockAtPosition(agent, gx, gy, gz, "minecraft:glowstone")
	require.NoError(t, err, "verify original glowstone position")
	glowstoneMoved, err := verifyBlockAtPosition(agent, gx+1, gy, gz, "minecraft:glowstone")
	require.NoError(t, err, "verify glowstone moved")

	if glowstoneMoved {
		agent.Agent.SendChat(fmt.Sprintf("Success: %s hit target at %d blocks!", projectileName, distance))
		return target, true
	} else {
		agent.Agent.SendChat(fmt.Sprintf("Failure: %s did not hit target at %d blocks", projectileName, distance))
		return target, false
	}
}

// throwEnderPearl verifies an ender pearl throw by checking if the player teleported to the target location
// Ender pearls teleport the player, so we check the final position instead of using a target block
// Accepts optional callbacks that will be registered with the projectile
func throwEnderPearl(t *testing.T, inst *TestInstance, agent *ManagedAgent, ctx context.Context, targetX, targetY, targetZ int, distance int, callbacks ...models.ProjectileHitCallback) (models.V3, bool) {
	target := models.V3{X: float64(targetX), Y: float64(targetY), Z: float64(targetZ)}

	var projectileHitEvent models.ProjectileHitEvent
	callbacks = append(callbacks, func(evt models.ProjectileHitEvent) {
		projectileHitEvent = evt
	})

	// Get position before throw
	startPos, initialized := agent.Agent.GetPositionSimple()
	require.True(t, initialized, "bot position initialized")

	resp, err := inst.RCON.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, targetX, targetY-1, targetZ))
	require.NoError(t, err, "place glowstone block for ender pearl test")

	t.Logf("place glowstone: %s", resp)

	// Turn bot's body to face the target before throwing
	require.NoError(t, agent.TurnTowards(ctx, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5))
	// Wait for server to process the rotation update
	time.Sleep(400 * time.Millisecond)

	// Throw the ender pearl with optional callback(s)
	// use log instead of t.Log to make the log statement show in the agent log, not the test output
	log.Printf("Throwing ender pearl from %s to (%.1f, %.1f, %.1f)", startPos, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5)

	packetWriter := agent.Agent.GetPacketLogWriter()
	callbacks = append(callbacks, func(evt models.ProjectileHitEvent) {
		fmt.Fprintf(packetWriter, ">>>>> End EnderPearl ThrowProjectileAt %d blocks <<<<<\n", distance)
	})

	fmt.Fprintf(packetWriter, ">>>>> Start EnderPearl ThrowProjectileAt %d blocks <<<<<\n", distance)

	_, throwErr := agent.Agent.ThrowProjectileAt(ctx, models.EnderPearl, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5, callbacks...)

	if throwErr != nil {
		t.Logf("ThrowProjectileAt error: %v", throwErr)
	}
	require.NoError(t, throwErr)
	log.Printf("Ender pearl thrown, waiting for teleport...")

	// Allow time for pearl to travel and player to teleport
	time.Sleep(5 * time.Second)

	// Check final position
	endPos, initialized := agent.Agent.GetPositionSimple()
	require.True(t, initialized, "bot position after throw")

	// Ender pearls should teleport the player to where they land
	// Check if player moved significantly from starting position
	distanceMoved := startPos.DistanceTo(endPos)
	distanceFromTarget := models.V3{X: float64(targetX), Y: float64(targetY), Z: float64(targetZ)}.DistanceTo(endPos)

	if math.Abs(distanceFromTarget) < .5 { // expect player to be within 0.5 blocks of target
		agent.Agent.SendChat(fmt.Sprintf("Success: EnderPearl teleported player ~%.1f blocks away (%.1f blocks total) from target (target was %d blocks from source)", distanceFromTarget, distanceMoved, distance))
		return target, true
	} else {
		pearlDistanceFromTarget := projectileHitEvent.Position.DistanceTo(target)
		agent.Agent.SendChat(fmt.Sprintf("Failure: EnderPearl did not teleport player close enough to the target (moved %.1f, target was %d blocks) pearl landed at (%.2f %.2f %.2f) - %.2f blocks from target",
			distanceMoved, distance,
			projectileHitEvent.Position.X, projectileHitEvent.Position.Y, projectileHitEvent.Position.Z,
			pearlDistanceFromTarget,
		))
		return target, false
	}
}

// --- Suite methods (bow_firing_test.go's 2 functions) ---

// TestFireBowAt verifies FireBowAt hits a target block that triggers a
// piston. Equivalent to the original TestBowFiring_FireBowAt.
func (s *ProjectileFlatSuite) TestFireBowAt() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("FireBowAtBot", "fire_bow_at")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, leader.Name))
	require.NoError(t, err)

	hitCh := make(chan models.ProjectileHitEvent, 1)
	callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

	target := fireAt(s.Ctx, t, s.Inst, leader.ManagedAgent, 10, callback)

	select {
	case evt := <-hitCh:
		distanceFromTarget := evt.Position.DistanceTo(target)
		t.Logf("✓ Arrow callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.2f blocks away)",
			evt.HitType, evt.ProjectileType, evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)
		assert.Equal(t, models.Arrow, evt.ProjectileType, "callback projectile type should be Arrow")
	case <-time.After(5 * time.Second):
		t.Logf("⚠ Arrow callback did not fire (timeout)")
	}
}

// TestMultipleDistances hits targets at multiple ranges. Equivalent to the
// original TestBowFiring_MultipleDistances.
func (s *ProjectileFlatSuite) TestMultipleDistances() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("MultiDistBot", "fire_bow_multi_dist")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 128`, leader.Name))
	require.NoError(t, err)

	for _, distance := range []int{5, 10, 15, 30} {
		t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
			hitCh := make(chan models.ProjectileHitEvent, 1)
			callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

			fireAt(s.Ctx, t, s.Inst, leader.ManagedAgent, distance, callback)

			select {
			case evt := <-hitCh:
				t.Logf("✓ Arrow callback at %d blocks: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f)", distance, evt.HitType, evt.ProjectileType, evt.Position.X, evt.Position.Y, evt.Position.Z)
				assert.Equal(t, models.Arrow, evt.ProjectileType, "callback projectile type should be Arrow")
			case <-time.After(5 * time.Second):
				t.Logf("⚠ Arrow callback at %d blocks did not fire (timeout)", distance)
			}
		})
	}
}

// --- Suite methods (bow_comprehensive_test.go's 5 functions) ---

// TestLevelTarget fires at a target at the same height as the bot. Equivalent
// to the original TestBowFiring_LevelTarget.
func (s *ProjectileFlatSuite) TestLevelTarget() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("LevelTargetBot", "fire_bow_level_target")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, leader.Name))
	require.NoError(t, err)

	fireAtElevation(t, s.Inst, leader.ManagedAgent, 0, 10)
}

// TestBelowTarget fires at a target below the bot. Equivalent to the
// original TestBowFiring_BelowTarget.
func (s *ProjectileFlatSuite) TestBelowTarget() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("BelowTargetBot", "fire_bow_below_target")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, leader.Name))
	require.NoError(t, err)

	fireAtElevation(t, s.Inst, leader.ManagedAgent, -3, 10)
}

// TestAboveTarget fires at a target above the bot - requires a high-angle
// shot. Equivalent to the original TestBowFiring_AboveTarget.
func (s *ProjectileFlatSuite) TestAboveTarget() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("AboveTargetBot", "fire_bow_above_target")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, leader.Name))
	require.NoError(t, err)

	fireAtElevation(t, s.Inst, leader.ManagedAgent, 3, 10)
}

// TestShelfTarget fires at a target on a high shelf: the arrow must hit on
// the way down, not the way up. Equivalent to the original
// TestBowFiring_ShelfTarget.
func (s *ProjectileFlatSuite) TestShelfTarget() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ShelfTargetBot", "fire_bow_shelf_target")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, leader.Name))
	require.NoError(t, err)

	fireAtElevation(t, s.Inst, leader.ManagedAgent, 2, 8)
}

// TestVariousElevations tests hitting targets at multiple elevations in one
// test. Equivalent to the original TestBowFiring_VariousElevations.
func (s *ProjectileFlatSuite) TestVariousElevations() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("VariousElevBot", "fire_bow_various_elevations")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 256`, leader.Name))
	require.NoError(t, err)

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
			fireAtElevation(t, s.Inst, leader.ManagedAgent, et.heightDelta, et.distance)
			time.Sleep(1 * time.Second)
		})
	}
}

// --- Suite methods (bow_power_comparison_test.go's 1 function, and
// arrow_physics_calibration_test.go's 1 function - both need the same kind
// of elevated, terrain-agnostic platform) ---

// teleportToElevatedPlatform teleports leader to a fixed, high elevation
// (y=100) above leader's own working area and builds a flat bedrock
// platform under it, matching what the pre-conversion TestBowPowerComparison/
// TestArrowPhysicsCalibration each did at the literal world origin (0, 100,
// 0), since each owned its entire server - a shared server has more than
// one working area active at once, so this instead builds that same kind of
// platform over leader's own working-area X/Z, so it can't collide with
// another test method's own working area.
func teleportToElevatedPlatform(s *ProjectileFlatSuite, leader *WorkingAreaAgent) (originX, originZ int) {
	t := s.T()

	originX = int(math.Floor(leader.Origin.X))
	originZ = int(math.Floor(leader.Origin.Z))

	_, err := s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`teleport %s %d 100 %d`, leader.Name, originX, originZ))
	require.NoError(t, err, "teleport to elevated platform")
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`fill %d 99 %d %d 99 %d minecraft:bedrock`,
		originX-50, originZ-50, originX+50, originZ+50))
	require.NoError(t, err, "build elevated bedrock platform")
	// Clear well above the platform too, for the same reason every other
	// method's setupTargetMechanism/setupTargetMechanismWithHeight call
	// clears above their own platform: flat-world ground is still solid a
	// short distance up from y=64, and the fill above only replaces one
	// exact layer (y=99), not whatever occupies the space between it and
	// this working area's actual ground.
	require.NoError(t, ClearArea(s.Ctx, s.Inst.RCON, originX-50, 100, originZ-50, originX+50, 140, originZ+50), "clear headroom above elevated platform")
	return originX, originZ
}

// TestPowerComparison compares actual arrow behavior from FireBowAt across
// several distances, cross-checked against the agent's own trajectory log.
// Equivalent to the original TestBowPowerComparison.
func (s *ProjectileFlatSuite) TestPowerComparison() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("PowerComparisonBot", "fire_bow_power_comparison")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, leader.Name))
	require.NoError(t, err)

	teleportToElevatedPlatform(s, leader)

	time.Sleep(2 * time.Second)
	time.Sleep(3 * time.Second)

	t.Logf("Starting bow power comparison tests")

	testCases := []struct {
		distance float64
		label    string
	}{
		{10, "10m"},
		{20, "20m"},
		{30, "30m"},
		{40, "40m"},
	}

	for _, tc := range testCases {
		t.Run(tc.label, func(t *testing.T) {
			botPos, ok := leader.Agent.GetPositionSimple()
			require.True(t, ok, "bot position initialized")
			botX, botY, botZ := botPos.X, botPos.Y, botPos.Z

			targetX := botX + tc.distance
			targetY := botY
			targetZ := botZ

			t.Logf("Testing at distance %.1f: target=(%.1f, %.1f, %.1f)", tc.distance, targetX, targetY, targetZ)

			blockX := int(math.Floor(targetX))
			blockY := int(math.Floor(targetY))
			blockZ := int(math.Floor(targetZ))

			cmd := fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, blockX, blockY, blockZ)
			response, err := s.Inst.RCON.Exec(s.Ctx, cmd)
			require.NoError(t, err, "place target glowstone block")
			t.Logf("%s => %s", cmd, response)

			time.Sleep(3 * time.Second)

			t.Logf("  Firing with FireBowAt...")
			traj, err := leader.Agent.FireBowAt(context.Background(), float64(blockX)+0.5, float64(blockY)+0.5, float64(blockZ)+0.5)
			assert.NoError(t, err)
			t.Logf("    Trajectory: %d points", len(traj))

			time.Sleep(3 * time.Second)

			actualStandard, err := AnalyzeArrowTrajectory(s.Inst.AgentLogFile)
			if err != nil {
				t.Logf("    Warning: Could not analyze standard trajectory: %v", err)
			} else {
				t.Logf("    Standard: actual=%d points", len(actualStandard))
				if len(actualStandard) > 0 {
					lastPos := actualStandard[len(actualStandard)-1]
					distTraveled := math.Sqrt(
						(lastPos.X-botX)*(lastPos.X-botX) +
							(lastPos.Z-botZ)*(lastPos.Z-botZ))
					t.Logf("    Standard: arrow traveled %.1f blocks (predicted %.1f)", distTraveled, tc.distance)
				}
			}
			cmd = fmt.Sprintf(`setblock %d %d %d minecraft:air`, blockX, blockY, blockZ)
			response, err = s.Inst.RCON.Exec(s.Ctx, cmd)
			require.NoError(t, err, "place target air block")
			t.Logf("%s => %s", cmd, response)

			time.Sleep(2 * time.Second)
		})
	}
}

// TestArrowPhysicsCalibration fires arrows at several pitches from a fixed
// elevated platform and back-calculates gravity/drag from the observed
// trajectory, comparing against physics/projectile.go's own predictions.
// Equivalent to the original TestArrowPhysicsCalibration
// (arrow_physics_calibration_test.go, which now keeps only the
// calibrateArrowPhysics/analyzePhysicsFromTrajectory helpers this method
// calls).
func (s *ProjectileFlatSuite) TestArrowPhysicsCalibration() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("CalibrationBot", "arrow_physics_calibration")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 64`, leader.Name))
	require.NoError(t, err)

	teleportToElevatedPlatform(s, leader)
	time.Sleep(500 * time.Millisecond)

	t.Logf("Starting arrow physics calibration")

	// Test multiple pitches to calibrate. Each pitch produces different
	// trajectory data for physics analysis.
	testCases := []struct {
		pitch float32
		yaw   float32
		label string
	}{
		{0, 0, "Pitch_0_Level"},
		{15, 0, "Pitch_15_Up"},
		{30, 0, "Pitch_30_Up"},
		{45, 0, "Pitch_45_Up"},
		{-15, 0, "Pitch_-15_Down"},
		{-30, 0, "Pitch_-30_Down"},
		{-45, 0, "Pitch_-45_Down"},
	}
	var calibrations []*ArrowPhysicsCalibration

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			cal, err := calibrateArrowPhysics(t, s.Inst, leader.ManagedAgent, tc.pitch, tc.yaw, tc.label)
			if err != nil {
				t.Logf("Failed to calibrate %s: %v", tc.label, err)
				return
			}
			calibrations = append(calibrations, cal)
		})

		// Wait between shots to avoid packet overlap
		time.Sleep(2 * time.Second)
	}

	// Report summary
	if len(calibrations) > 0 {
		t.Logf("\n=== Arrow Physics Calibration Summary for %s ===", s.Version)
		t.Logf("Test\t\t\tMeas.Grav\tPred.Grav\tGrav%%Err\tMeas.Drag\tPred.Drag\tDrag%%Err")

		var totalGravError, totalDragError float64
		for _, cal := range calibrations {
			if cal == nil {
				continue
			}
			t.Logf("%.0f/%.0f\t\t%.6f\t%.6f\t%.1f%%\t%.6f\t%.6f\t%.1f%%",
				cal.Pitch, cal.Pitch, cal.MeasuredGrav, cal.PredictedGrav, cal.GravError,
				cal.MeasuredDrag, cal.PredictedDrag, cal.DragError)
			totalGravError += cal.GravError
			totalDragError += cal.DragError
		}

		if len(calibrations) > 0 {
			avgGravError := totalGravError / float64(len(calibrations))
			avgDragError := totalDragError / float64(len(calibrations))
			t.Logf("\nAverage Errors: Gravity=%.1f%%, Drag=%.1f%%", avgGravError, avgDragError)

			if avgGravError > 5 || avgDragError > 5 {
				t.Logf("WARNING: Physics calibration shows significant deviation from predictions!")
				t.Logf("Consider updating physics constants in physics/projectile.go")
			}
		}
	}
}

// --- Suite methods (projectile_throwing_test.go's 5 live-server functions;
// its 3 physics-only unit tests stay in that file, unchanged) ---

// TestEnderPearlRange verifies that ender pearls can reach various distances
// (including 30+ blocks). Equivalent to the original Test_EnderPearlRange.
func (s *ProjectileFlatSuite) TestEnderPearlRange() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("EnderPearlBot", "ender_pearl_range")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:ender_pearl 16`, leader.Name))
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	for _, distance := range []int{5, 10, 15, 20, 25, 30, 45, 50} { // max distance when standing on flat ground is 50-55, with optimal angle being 35-40 degrees
		t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
			hitCh := make(chan models.ProjectileHitEvent, 1)
			callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

			target, hit := throwProjectile(s.Ctx, t, s.Inst, leader.ManagedAgent, "ender_pearl", 1, distance, callback)

			select {
			case evt := <-hitCh:
				distFromTarget := evt.Position.DistanceTo(target)
				t.Logf("✓ EnderPearl callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.02f blocks)", evt.HitType, evt.ProjectileType,
					evt.Position.X, evt.Position.Y, evt.Position.Z, distFromTarget)

				assert.Equal(t, models.EnderPearl, evt.ProjectileType, "callback projectile type should be EnderPearl")
				assert.Equal(t, models.ProjectileHitUnknown, evt.HitType, "ender pearl should fire ProjectileHitUnknown")
				assert.True(t, evt.IsValidHit,
					"ender pearl should be valid hit (direct distance=%.2f, acceptance=1.0, trajectory=%v, targetSet=%v)",
					distFromTarget, evt.TrajectoryHit, evt.TargetSet)
			case <-time.After(5 * time.Second):
				t.Logf("⚠ EnderPearl callback did not fire (timeout)")
			}
			if distance <= 25 {
				require.True(t, hit, "ender pearl should hit at %d blocks", distance)
			} else {
				t.Logf("EnderPearl at %d blocks: %v (may fail due to divergence)", distance, hit)
			}
		})
	}
}

// TestSnowballRange verifies snowball throwing at various distances.
// Equivalent to the original Test_SnowballRange.
func (s *ProjectileFlatSuite) TestSnowballRange() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("SnowballBot", "snowball_range")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:snowball 128`, leader.Name))
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	for _, distance := range []int{5, 15, 30, 45, 50} { // max standing throw distance on flat ground is about 51 blocks (angle should be ~40 degrees).
		t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
			hitCh := make(chan models.ProjectileHitEvent, 1)
			callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

			target, hit := throwProjectile(s.Ctx, t, s.Inst, leader.ManagedAgent, "snowball", 1, distance, callback)

			select {
			case evt := <-hitCh:
				distanceFromTarget := evt.Position.DistanceTo(target)
				t.Logf("✓ Snowball callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.02f blocks)", evt.HitType, evt.ProjectileType,
					evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)

				assert.Equal(t, models.Snowball, evt.ProjectileType, "callback projectile type should be Snowball")
				assert.Equal(t, models.ProjectileHitUnknown, evt.HitType, "snowball should fire ProjectileHitUnknown")
				assert.True(t, evt.IsValidHit,
					"snowball should be valid hit (direct distance=%.2f, acceptance=0.5, trajectory=%v, targetSet=%v)",
					distanceFromTarget, evt.TrajectoryHit, evt.TargetSet)

			case <-time.After(3 * time.Second):
				t.Logf("⚠ Snowball callback did not fire (timeout)")
			}
			if distance <= 30 {
				require.True(t, hit, "snowball should hit at %d blocks", distance)
			} else {
				t.Logf("Snowball at %d blocks: %v (edge case)", distance, hit)
			}
		})
	}
}

// TestArrowRange verifies arrow firing with a bow at various distances.
// Equivalent to the original Test_ArrowRange.
func (s *ProjectileFlatSuite) TestArrowRange() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("ArrowRangeBot", "arrow_range")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 128`, leader.Name))
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	// max range should be about 120 on flat ground with an unmodded bow, while standing still,
	// but replay shows the arrow dissapearing 60 and 100 blocks
	for _, distance := range []int{5, 10, 15, 30, 45, 60} {
		t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
			hitCh := make(chan models.ProjectileHitEvent, 1)
			callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

			// Note: arrows use FireBowAt, not ThrowProjectileAt, but both go through the same mechanism
			target := fireAt(s.Ctx, t, s.Inst, leader.ManagedAgent, distance, callback)

			select {
			case evt := <-hitCh:
				pos, _ := leader.Agent.GetPositionSimple()
				botX, botY, botZ := pos.X, pos.Y, pos.Z
				distanceFromTarget := evt.Position.DistanceTo(target)
				t.Logf("✓ Arrow callback: HitType=%v, ProjectileType=%v, source=(%.2f %.2f %.2f) target=(%.2f %.2f %.2f) landed=(%.2f %.2f %.2f - %.02f blocks)",
					evt.HitType, evt.ProjectileType,
					botX, botY, botZ,
					target.X, target.Y, target.Z,
					evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)

				assert.Equal(t, models.Arrow, evt.ProjectileType, "callback projectile type should be Arrow")
				assert.True(t, evt.HitType == models.ProjectileHitBlock || evt.HitType == models.ProjectileHitEntity,
					"arrow should fire HitBlock or HitEntity")
				assert.True(t, evt.IsValidHit,
					"arrow should be valid hit (direct distance=%.2f, acceptance=0.5, trajectory=%v, targetSet=%v)",
					distanceFromTarget, evt.TrajectoryHit, evt.TargetSet)
			case <-time.After(5 * time.Second):
				t.Logf("⚠ Arrow callback did not fire (timeout)")
			}
		})
	}
}

// TestProjectileHitCallback verifies that projectile hit callbacks fire
// correctly - the queue-based pending callback implementation, for multiple
// callbacks in a row. Equivalent to the original
// Test_ProjectileHitCallback.
func (s *ProjectileFlatSuite) TestProjectileHitCallback() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("CallbackBot", "projectile_hit_callback")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:snowball 16`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:bow`, leader.Name))
	require.NoError(t, err)
	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:arrow 16`, leader.Name))
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	// Test queue-based callback matching:
	// Fire multiple projectiles rapidly and verify each callback matches its projectile type
	t.Log("Testing queue-based callback implementation - firing multiple projectiles")

	// Test 1: Verify Snowball callbacks are queued and matched correctly
	t.Run("Snowball_Queue_Callback", func(t *testing.T) {
		hitCh := make(chan models.ProjectileHitEvent, 1)
		callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

		botPos, initialized := leader.Agent.GetPositionSimple()
		require.True(t, initialized, "bot position initialized")

		// Throw snowball with callback
		// Just throw in any direction - we're testing queue matching, not aiming accuracy
		_, err := leader.Agent.ThrowProjectileAt(s.Ctx, models.Snowball, botPos.X+5, botPos.Y, botPos.Z+5, callback)
		require.NoError(t, err, "ThrowProjectileAt should succeed")

		select {
		case event := <-hitCh:
			t.Logf("✓ Snowball callback fired: HitType=%v, ProjectileType=%v",
				event.HitType, event.ProjectileType)
			assert.Equal(t, models.Snowball, event.ProjectileType, "callback should match Snowball projectile type")
			assert.Equal(t, models.ProjectileHitUnknown, event.HitType, "non-persistent should fire HitUnknown")
		case <-time.After(15 * time.Second):
			t.Fatalf("Snowball callback did not fire within 15 seconds - queue matching may be broken")
		}
	})

	// Test 2: Verify Arrow callbacks are queued and matched correctly
	// No delay needed - pendingProjectiles queue supports multiple callbacks
	t.Run("Arrow_Queue_Callback", func(t *testing.T) {
		hitCh := make(chan models.ProjectileHitEvent, 1)
		callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

		botPos, initialized := leader.Agent.GetPositionSimple()
		require.True(t, initialized, "bot position initialized")

		// Ensure bow and arrow are ready
		// Use /item replace (1.17+) instead of /replaceitem (deprecated)
		cmd := fmt.Sprintf(`/item replace entity %s hotbar.0 with minecraft:bow`, leader.Name)
		resp, err := s.Inst.RCON.Exec(s.Ctx, cmd)
		require.NoError(t, err, "equip bow command failed: %s", resp)
		t.Logf("%s => %s", cmd, resp)

		cmd = fmt.Sprintf(`/give %s minecraft:arrow 64`, leader.Name)
		resp, err = s.Inst.RCON.Exec(s.Ctx, cmd)
		require.NoError(t, err)
		t.Logf("%s => %s", cmd, resp)
		time.Sleep(500 * time.Millisecond)

		// CRITICAL: Select hotbar slot 0 to equip the bow in agent's hand
		require.NoError(t, leader.Agent.SelectHotbarSlot(s.Ctx, 0), "must equip bow")
		time.Sleep(200 * time.Millisecond)

		// Fire bow with callback - fire DOWNWARD to guarantee hitting ground
		// This ensures arrow will land quickly and trigger a callback
		_, err = leader.Agent.FireBowAt(context.Background(), botPos.X, botPos.Y-5, botPos.Z, callback)
		require.NoError(t, err, "FireBowAt should succeed")

		select {
		case event := <-hitCh:
			t.Logf("✓ Arrow callback fired: HitType=%v, ProjectileType=%v",
				event.HitType, event.ProjectileType)
			assert.Equal(t, models.Arrow, event.ProjectileType, "callback should match Arrow projectile type")
			assert.True(t, event.HitType == models.ProjectileHitBlock || event.HitType == models.ProjectileHitEntity,
				"arrow should fire HitBlock or HitEntity")
		case <-time.After(10 * time.Second):
			t.Fatalf("Arrow callback did not fire within 10 seconds")
		}
	})
}

// TestWindChargeRange verifies wind charges can be thrown at various
// distances. Wind charges are available in Minecraft 1.21+ and have unique
// physics (constant velocity, no gravity). Equivalent to the original
// Test_WindChargeRange.
//
// IMPORTANT: Projectile Randomness Considerations
// In Minecraft, player-fired projectiles include inherent randomness to simulate inaccuracy:
// - Formula: deviation = random.nextGaussian() * 0.0075 * inaccuracy
// - Expected spread: ~0.3 blocks at all distances (pre-1.21.6)
// - 1.21.6+ provides zero spread for first 2 ticks, then gradual increase
//
// See docs/PROJECTILE_RANDOMNESS.md for detailed analysis.
func (s *ProjectileFlatSuite) TestWindChargeRange() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("WindChargeBot", "wind_charge_range")
	require.NoError(t, err, "spawn agent")

	_, err = s.Inst.RCON.Exec(s.Ctx, fmt.Sprintf(`give %s minecraft:wind_charge 64`, leader.Name))
	require.NoError(t, err)
	time.Sleep(1 * time.Second)

	for _, distance := range []int{5, 15, 30, 45} {
		t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
			hitCh := make(chan models.ProjectileHitEvent, 1)
			callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

			target, hit := throwProjectile(s.Ctx, t, s.Inst, leader.ManagedAgent, "wind_charge", 1, distance, callback)

			select {
			case evt := <-hitCh:
				distanceFromTarget := evt.Position.DistanceTo(target)
				t.Logf("✓ Wind charge callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.02f blocks)", evt.HitType, evt.ProjectileType,
					evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)

				assert.Equal(t, models.WindCharge, evt.ProjectileType, "callback projectile type should be WindCharge")
				assert.Equal(t, models.ProjectileHitUnknown, evt.HitType, "wind charge should fire ProjectileHitUnknown")
				assert.True(t, evt.IsValidHit,
					"wind charge should be valid hit (direct distance=%.2f, acceptance=1.5, trajectory=%v, targetSet=%v)",
					distanceFromTarget, evt.TrajectoryHit, evt.TargetSet)

			case <-time.After(3 * time.Second):
				t.Logf("⚠ Wind charge callback did not fire (timeout)")
			}
			if distance <= 30 {
				require.True(t, hit, "wind charge should hit at %d blocks", distance)
			} else {
				t.Logf("Wind charge at %d blocks: %v (edge case)", distance, hit)
			}
		})
	}
}
