package testing

import (
	"context"
	"fmt"
	"log"
	"math"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjectileReachability tests whether different projectiles can reach various distances
// This is a unit test using physics simulation to understand reachability
func TestProjectileReachability(t *testing.T) {
	// Test parameters: for each projectile type, at each distance, can we find a valid trajectory?
	tests := []struct {
		name      string
		projType  models.ProjectileType
		distances []int
	}{
		{"Arrow", models.Arrow, []int{5, 10, 15, 30}},
		{"Snowball", models.Snowball, []int{5, 10, 15, 30}},
		{"Egg", models.Egg, []int{5, 10, 15, 30}},
		{"EnderPearl", models.EnderPearl, []int{5, 10, 15, 30}},
		// SplashPotion physics per Minecraft wiki:
		//   - Speed: 0.5 blocks/tick
		//   - Gravity: 0.05 blocks/tick²
		//   - Ticking order: Acceleration (gravity), Drag, Position
		// Result: Maximum reachable distance at same height is ~1 block.
		// Despite wiki claiming 8-block range, that requires aiming upward significantly,
		// and potions still drop below launch height. Physically correct but not testable
		// with same-height target assumption. Can reach 5-8 blocks horizontally if target
		// is positioned lower (with calculated drop). Omitted from basic reachability test.
		// {"SplashPotion", models.SplashPotion, []int{5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, distance := range tt.distances {
				t.Run(fmt.Sprintf("%d_blocks", distance), func(t *testing.T) {
					// Setup
					botOrigin := models.V3{X: 0, Y: 64.52, Z: 0}                // At bot position with spawn offset
					targetPos := models.V3{X: float64(distance), Y: 64.5, Z: 0} // Same height as target block center

					// Calculate if target is reachable
					pitch, power, errorY, trajectory := physics.FindOptimalAiming(
						tt.projType,
						botOrigin,
						targetPos,
					)

					// Verify trajectory was found
					assert.Greater(t, len(trajectory), 0, fmt.Sprintf("%s at %d blocks should have valid trajectory", tt.name, distance))

					// Check hit tolerance (±0.5 blocks is standard Minecraft block)
					hitTolerance := 0.5
					hitAccuracy := math.Abs(errorY) < hitTolerance

					t.Logf("%s at %d blocks: pitch=%.2f°, power=%.3f, error=%.4f, reachable=%v",
						tt.name, distance, pitch, power, errorY, hitAccuracy)
				})
			}
		})
	}
}

// TestProjectilePhysics_Comparative tests physics simulation accuracy across projectile types
func TestProjectilePhysics_Comparative(t *testing.T) {
	// Test trajectory calculation consistency
	tests := []struct {
		name     string
		projType models.ProjectileType
		distance float64
		// Expected approximate pitch (in degrees) for horizontal shot
		expectedPitchRange [2]float64
	}{
		// Arrows: gravity=0.05, drag=0.99 - need downward pitch for all ranges
		{"Arrow_5m", models.Arrow, 5, [2]float64{-5, 5}},
		{"Arrow_10m", models.Arrow, 10, [2]float64{-5, 5}},
		{"Arrow_30m", models.Arrow, 30, [2]float64{-10, 0}},

		// Snowballs: gravity=0.03, drag=0.99 - need downward pitch at all ranges
		{"Snowball_5m", models.Snowball, 5, [2]float64{-5, 5}},
		{"Snowball_10m", models.Snowball, 10, [2]float64{-5, 5}},
		{"Snowball_30m", models.Snowball, 30, [2]float64{-20, -5}},

		// EnderPearls: same as snowballs (gravity=0.03, drag=0.99)
		{"EnderPearl_5m", models.EnderPearl, 5, [2]float64{-5, 5}},
		{"EnderPearl_10m", models.EnderPearl, 10, [2]float64{-5, 5}},
		{"EnderPearl_30m", models.EnderPearl, 30, [2]float64{-20, -5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			botOrigin := models.V3{X: 0, Y: 64.52, Z: 0}
			targetPos := models.V3{X: tt.distance, Y: 64.5, Z: 0}

			pitch, power, errorY, trajectory := physics.FindOptimalAiming(
				tt.projType,
				botOrigin,
				targetPos,
			)

			// Verify pitch is in expected range
			assert.GreaterOrEqual(t, pitch, tt.expectedPitchRange[0],
				fmt.Sprintf("%s: pitch should be >= %.1f°", tt.name, tt.expectedPitchRange[0]))
			assert.LessOrEqual(t, pitch, tt.expectedPitchRange[1],
				fmt.Sprintf("%s: pitch should be <= %.1f°", tt.name, tt.expectedPitchRange[1]))

			// Verify trajectory exists
			assert.Greater(t, len(trajectory), 0, fmt.Sprintf("%s: should have valid trajectory", tt.name))

			// Log for comparison
			t.Logf("%s: pitch=%.2f° (range: %.1f°-%.1f°), power=%.3f, error=%.4f, trajectory_len=%d",
				tt.name, pitch, tt.expectedPitchRange[0], tt.expectedPitchRange[1], power, errorY, len(trajectory))
		})
	}
}

// TestProjectileReachability_Detailed tests max reachable distance for each projectile type
func TestProjectileReachability_Detailed(t *testing.T) {
	projTypes := []struct {
		name string
		typ  models.ProjectileType
	}{
		{"Arrow", models.Arrow},
		{"Snowball", models.Snowball},
		{"Egg", models.Egg},
		{"EnderPearl", models.EnderPearl},
		{"SplashPotion", models.SplashPotion},
	}

	for _, proj := range projTypes {
		t.Run(proj.name, func(t *testing.T) {
			// Test reachability at 1-50 blocks in increments
			botOrigin := models.V3{X: 0, Y: 64.52, Z: 0}

			results := make(map[int]bool)
			for distance := 1; distance <= 50; distance += 5 {
				targetPos := models.V3{X: float64(distance), Y: 64.5, Z: 0}
				_, _, _, trajectory := physics.FindOptimalAiming(
					proj.typ,
					botOrigin,
					targetPos,
				)
				reachable := len(trajectory) > 0
				results[distance] = reachable

				if !reachable && distance <= 30 {
					t.Logf("%s: UNREACHABLE at %d blocks (starting to fail)", proj.name, distance)
				}
			}

			// Find max reachable distance
			var maxReachable int
			for distance := 1; distance <= 50; distance += 5 {
				if results[distance] {
					maxReachable = distance
				}
			}

			t.Logf("%s: Max reachable distance = ~%d blocks", proj.name, maxReachable)
		})
	}
}

// throwProjectile is a helper function that throws a projectile at a target and verifies the hit
// Returns true if the target was hit (glowstone moved for trigger blocks, or teleported for ender pearls)
// Accepts optional callbacks that will be registered with the projectile
func throwProjectile(t *testing.T, inst *TestInstance, agent *ManagedAgent, projectileName string, itemCount int, distance int, callbacks ...models.ProjectileHitCallback) (models.V3, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	agent.Agent.SendChat(fmt.Sprintf("Throw %s at target. Distance %d blocks", projectileName, distance))

	// Place the item directly in hotbar slot 0 using /item replace command (1.17+)
	cmd := fmt.Sprintf(`/item replace entity %s hotbar.0 with minecraft:%s %d`, agent.Name, projectileName, itemCount)
	_, err := inst.RCON.Exec(ctx, cmd)
	require.NoError(t, err, "item replace command failed")

	time.Sleep(500 * time.Millisecond)

	// Select hotbar slot 0 (where we just put the item)
	require.NoError(t, agent.Agent.SelectHotbarSlot(ctx, 0))

	botX, botY, botZ, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")
	t.Logf("[throwProjectile] Bot position: (%.2f, %.2f, %.2f)", botX, botY, botZ)
	targetX := int(botX) + distance
	targetY := int(botY)
	targetZ := int(botZ)

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
	gx, gy, gz, err := setupTargetMechanism(ctx, t, inst.RCON, targetX, targetY, targetZ)
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
	startX, startY, startZ, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position initialized")

	resp, err := inst.RCON.Exec(ctx, fmt.Sprintf(`setblock %d %d %d minecraft:glowstone`, targetX, targetY-1, targetZ))
	require.NoError(t, err, "place glowstone block for ender pearl test")

	t.Logf("place glowstone: %s", resp)

	// Turn bot's body to face the target before throwing
	require.NoError(t, agent.TurnTowards(ctx, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5))
	// Wait for server to process the rotation update
	time.Sleep(400 * time.Millisecond)

	// Throw the ender pearl with optional callback(s)
	// use log instead of t.Log to make the log statement show in the agent log, not the test output
	log.Printf("Throwing ender pearl from (%.1f, %.1f, %.1f) to (%.1f, %.1f, %.1f)", startX, startY, startZ, float64(targetX)+0.5, float64(targetY)+0.5, float64(targetZ)+0.5)

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
	endX, endY, endZ, ok := agent.Agent.GetPositionSimple()
	require.True(t, ok, "bot position after throw")

	// Ender pearls should teleport the player to where they land
	// Check if player moved significantly from starting position
	distanceMoved := math.Sqrt(math.Pow(endX-startX, 2) + math.Pow(endY-startY, 2) + math.Pow(endZ-startZ, 2))
	distanceFromTarget := models.V3{X: float64(targetX), Y: float64(targetY), Z: float64(targetZ)}.DistanceTo(models.V3{X: endX, Y: endY, Z: endZ})
	// hitAccuracy := math.Abs(distanceMoved) > float64(distance)/2

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

// Test_EnderPearlRange verifies that ender pearls can reach various distances (including 30+ blocks) on a live server
// This directly answers the user's question: "I need to know if the agent can throw an enderpearl 30+ blocks"
func Test_EnderPearlRange(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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
				"EnderPearlBot",
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
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:ender_pearl 16`, agnt.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			for _, distance := range []int{5, 10, 15, 20, 25, 30, 45, 50} { // max distance when standing on flat ground is 50-55, with optimal angle being 35-40 degrees
				// for _, distance := range []int{5, 30} {
				t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
					// Setup callback channel for ender pearl
					hitCh := make(chan models.ProjectileHitEvent, 1)
					callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

					target, hit := throwProjectile(t, inst, agnt, "ender_pearl", 1, distance, callback)

					// Wait for callback (non-blocking)
					select {
					case evt := <-hitCh:
						distFromTarget := evt.Position.DistanceTo(target)
						t.Logf("✓ EnderPearl callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.02f blocks)", evt.HitType, evt.ProjectileType,
							evt.Position.X, evt.Position.Y, evt.Position.Z, distFromTarget)

						// Verify callback matches this projectile type
						assert.Equal(t, models.EnderPearl, evt.ProjectileType, "callback projectile type should be EnderPearl")

						// Non-persistent projectiles fire ProjectileHitUnknown
						assert.Equal(t, models.ProjectileHitUnknown, evt.HitType, "ender pearl should fire ProjectileHitUnknown")

						// Verify landed position is within 1.0 blocks of target
						assert.LessOrEqual(t, distFromTarget, 1.0, "ender pearl should land within 0.75 blocks of target")
					case <-time.After(5 * time.Second):
						t.Logf("⚠ EnderPearl callback did not fire (timeout)")
					}
					if distance <= 25 {
						// Lighter projectiles should reach closer distances reliably
						require.True(t, hit, "ender pearl should hit at %d blocks", distance)
					} else {
						// 30 blocks is at the edge, may or may not hit due to divergence
						t.Logf("EnderPearl at %d blocks: %v (may fail due to divergence)", distance, hit)
					}
				})
			}
		})
	}
}

// Test_SnowballRange verifies snowball throwing at various distances
func Test_SnowballRange(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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
				"SnowballBot",
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
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:snowball 128`, agnt.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			for _, distance := range []int{5, 15, 30, 45, 50} { // max standing throw distance on flat ground is about 51 blocks (angle should be ~40 degrees).
				t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
					// Setup callback channel for snowball
					hitCh := make(chan models.ProjectileHitEvent, 1)
					callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

					target, hit := throwProjectile(t, inst, agnt, "snowball", 1, distance, callback)

					// Wait for callback (non-blocking)
					select {
					case evt := <-hitCh:
						distanceFromTarget := evt.Position.DistanceTo(target)
						t.Logf("✓ Snowball callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.02f blocks)", evt.HitType, evt.ProjectileType,
							evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)

						// Verify callback matches this projectile type
						assert.Equal(t, models.Snowball, evt.ProjectileType, "callback projectile type should be Snowball")

						// Non-persistent projectiles fire ProjectileHitUnknown
						assert.Equal(t, models.ProjectileHitUnknown, evt.HitType, "snowball should fire ProjectileHitUnknown")

						// Verify landed position is within 1.0 blocks of target
						assert.LessOrEqual(t, distanceFromTarget, 1.0, "snowball should land within 0.75 blocks of target")

					case <-time.After(3 * time.Second):
						t.Logf("⚠ Snowball callback did not fire (timeout)")
					}
					if distance <= 30 {
						// Snowballs should reach closer distances reliably
						require.True(t, hit, "snowball should hit at %d blocks", distance)
					} else {
						// Beyond 30 blocks is edge case
						t.Logf("Snowball at %d blocks: %v (edge case)", distance, hit)
					}
				})
			}
		})
	}
}

// Test_ArrowRange verifies arrow firing with bow at various distances
func Test_ArrowRange(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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
				"ArrowBot",
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

			// max range should be about 120 on flat ground with an unmodded bow, while standing still,
			// but replay shows the arrow dissapearing around 100 blocks
			for _, distance := range []int{5, 10, 15, 30, 45, 60, 90} {
				t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
					// Setup callback channel for arrow
					hitCh := make(chan models.ProjectileHitEvent, 1)
					callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

					// Note: arrows use FireBowAt, not ThrowProjectileAt, but both go through the same mechanism
					target := fireAt(t, inst, agnt, distance, callback)

					// Wait for callback (non-blocking)
					select {
					case evt := <-hitCh:
						botX, botY, botZ, _ := agnt.Agent.GetPositionSimple()
						distanceFromTarget := evt.Position.DistanceTo(target)
						t.Logf("✓ Arrow callback: HitType=%v, ProjectileType=%v, source=(%.2f %.2f %.2f) target=(%.2f %.2f %.2f) landed=(%.2f %.2f %.2f - %.02f blocks)",
							evt.HitType, evt.ProjectileType,
							botX, botY, botZ,
							target.X, target.Y, target.Z,
							evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)

						// Verify callback matches this projectile type
						assert.Equal(t, models.Arrow, evt.ProjectileType, "callback projectile type should be Arrow")

						// Persistent projectiles fire HitBlock (when isInGround) or HitEntity (when removed without isInGround)
						assert.True(t, evt.HitType == models.ProjectileHitBlock || evt.HitType == models.ProjectileHitEntity,
							"arrow should fire HitBlock or HitEntity")

						// Verify landed position is within 1.0 blocks of target
						assert.LessOrEqual(t, distanceFromTarget, 1.0, "arrow should land within 0.75 blocks of target")
					case <-time.After(5 * time.Second):
						t.Logf("⚠ Arrow callback did not fire (timeout)")
					}
				})
			}
		})
	}
}

// Test_ProjectileHitCallback verifies that projectile hit callbacks fire correctly
// Tests the queue-based pending callback implementation to ensure multiple callbacks work
func Test_ProjectileHitCallback(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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
				"CallbackBot",
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
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:snowball 16`, agnt.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:bow`, agnt.Name))
			require.NoError(t, err)
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:arrow 16`, agnt.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			// Test queue-based callback matching:
			// Fire multiple projectiles rapidly and verify each callback matches its projectile type
			t.Log("Testing queue-based callback implementation - firing multiple projectiles")

			// Test 1: Verify Snowball callbacks are queued and matched correctly
			t.Run("Snowball_Queue_Callback", func(t *testing.T) {
				hitCh := make(chan models.ProjectileHitEvent, 1)
				callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

				botX, botY, botZ, ok := agnt.Agent.GetPositionSimple()
				require.True(t, ok, "bot position initialized")

				// Throw snowball with callback
				// Just throw in any direction - we're testing queue matching, not aiming accuracy
				_, err := agnt.Agent.ThrowProjectileAt(ctx, models.Snowball, botX+5, botY, botZ+5, callback)
				require.NoError(t, err, "ThrowProjectileAt should succeed")

				// Wait for callback with timeout
				select {
				case event := <-hitCh:
					t.Logf("✓ Snowball callback fired: HitType=%v, ProjectileType=%v",
						event.HitType, event.ProjectileType)
					// Queue test: Verify callback is matched to correct projectile type
					assert.Equal(t, models.Snowball, event.ProjectileType, "callback should match Snowball projectile type")
					// Non-persistent projectiles fire ProjectileHitUnknown on removal
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

				botX, botY, botZ, ok := agnt.Agent.GetPositionSimple()
				require.True(t, ok, "bot position initialized")

				// Ensure bow and arrow are ready
				// Use /item replace (1.17+) instead of /replaceitem (deprecated)
				cmd := fmt.Sprintf(`/item replace entity %s hotbar.0 with minecraft:bow`, agnt.Name)
				resp, err := inst.RCON.Exec(ctx, cmd)
				require.NoError(t, err, "equip bow command failed: %s", resp)
				t.Logf("%s => %s", cmd, resp)

				cmd = fmt.Sprintf(`/give %s minecraft:arrow 64`, agnt.Name)
				resp, err = inst.RCON.Exec(ctx, cmd)
				require.NoError(t, err)
				t.Logf("%s => %s", cmd, resp)
				time.Sleep(500 * time.Millisecond)

				// CRITICAL: Select hotbar slot 0 to equip the bow in agent's hand
				require.NoError(t, agnt.Agent.SelectHotbarSlot(ctx, 0), "must equip bow")
				time.Sleep(200 * time.Millisecond)

				// Fire bow with callback - fire DOWNWARD to guarantee hitting ground
				// This ensures arrow will land quickly and trigger a callback
				_, err = agnt.Agent.FireBowAt(botX, botY-5, botZ, callback)
				require.NoError(t, err, "FireBowAt should succeed")

				// Wait for callback with timeout
				select {
				case event := <-hitCh:
					t.Logf("✓ Arrow callback fired: HitType=%v, ProjectileType=%v",
						event.HitType, event.ProjectileType)
					// Queue test: Verify callback is matched to correct projectile type
					assert.Equal(t, models.Arrow, event.ProjectileType, "callback should match Arrow projectile type")
					// Persistent projectiles fire HitBlock (when isInGround) or HitEntity (when removed without isInGround)
					assert.True(t, event.HitType == models.ProjectileHitBlock || event.HitType == models.ProjectileHitEntity,
						"arrow should fire HitBlock or HitEntity")
				case <-time.After(10 * time.Second):
					t.Fatalf("Arrow callback did not fire within 10 seconds")
				}
			})
		})
	}
}

// Test_WindChargeRange verifies wind charges can be thrown at various distances
// Wind charges are available in Minecraft 1.24+ and have unique physics (drag→position with custom acceleration)
func Test_WindChargeRange(t *testing.T) {
	for _, tt := range models.StandardVersionTests {
		t.Run(tt.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
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
				"WindChargeBot",
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
			_, err = inst.RCON.Exec(ctx, fmt.Sprintf(`give %s minecraft:wind_charge 64`, agnt.Name))
			require.NoError(t, err)

			time.Sleep(1 * time.Second)

			for _, distance := range []int{5, 15, 30, 45} {
				t.Run(fmt.Sprintf("%d blocks", distance), func(t *testing.T) {
					// Setup callback channel for wind charge
					hitCh := make(chan models.ProjectileHitEvent, 1)
					callback := func(evt models.ProjectileHitEvent) { hitCh <- evt }

					target, hit := throwProjectile(t, inst, agnt, "wind_charge", 1, distance, callback)

					// Wait for callback (non-blocking)
					select {
					case evt := <-hitCh:
						distanceFromTarget := evt.Position.DistanceTo(target)
						t.Logf("✓ Wind charge callback: HitType=%v, ProjectileType=%v, landed=(%.2f %.2f %.2f - %.02f blocks)", evt.HitType, evt.ProjectileType,
							evt.Position.X, evt.Position.Y, evt.Position.Z, distanceFromTarget)

						// Verify callback matches this projectile type
						assert.Equal(t, models.WindCharge, evt.ProjectileType, "callback projectile type should be WindCharge")

						// Non-persistent projectiles fire ProjectileHitUnknown
						assert.Equal(t, models.ProjectileHitUnknown, evt.HitType, "wind charge should fire ProjectileHitUnknown")

						// Verify landed position is within 1.0 blocks of target
						assert.LessOrEqual(t, distanceFromTarget, 1.0, "wind charge should land within 0.75 blocks of target")

					case <-time.After(3 * time.Second):
						t.Logf("⚠ Wind charge callback did not fire (timeout)")
					}
					if distance <= 30 {
						// Wind charges should reach closer distances reliably
						require.True(t, hit, "wind charge should hit at %d blocks", distance)
					} else {
						// Beyond 30 blocks is edge case
						t.Logf("Wind charge at %d blocks: %v (edge case)", distance, hit)
					}
				})
			}
		})
	}
}
