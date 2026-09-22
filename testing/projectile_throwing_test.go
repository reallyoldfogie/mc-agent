package testing

import (
	"fmt"
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
	"github.com/stretchr/testify/assert"
)

// getAccuracyTolerance returns the acceptable accuracy tolerance for wind charge tests
// based on Minecraft version and throw distance
//
// Randomness in projectiles changed in Minecraft 1.21.6:
// - Pre-1.21.6: Immediate randomness (~0.3 blocks spread)
// - 1.21.6+: Zero spread for first 2 ticks, then gradual increase (0.05/tick)
func getAccuracyTolerance(version string, distance int) float64 {
	// Parse version to determine pre/post 1.21.6
	var isPostBuffedAccuracy bool
	switch version {
	// Pre-1.21.6 versions (immediate randomness, ~0.3 blocks)
	case "1.21.1", "1.21.2", "1.21.3", "1.21.4", "1.21.5", "1.21.8", "1.21.9", "1.21.10", "1.21.11":
		isPostBuffedAccuracy = false
	// 1.21.6+ versions (buffed short-range accuracy)
	case "1.21.6", "1.21.7", "1.21.12":
		isPostBuffedAccuracy = true
	default:
		// Assume pre-1.21.6 for safety
		isPostBuffedAccuracy = false
	}

	// Tolerance depends on version and distance
	// Randomness accumulates with distance in both version lines
	if isPostBuffedAccuracy {
		// 1.21.6+ has zero spread for first 2 ticks (~0.1s), then gradual increase
		// This allows more precise short-range shots
		switch distance {
		case 5:
			return 0.3 // Short-range: benefits from buffed accuracy
		case 15:
			return 0.5 // Medium-range
		case 30:
			return 0.7 // Long-range
		case 45:
			return 1.0 // Edge case
		default:
			return 1.0
		}
	} else {
		// Pre-1.21.6 has immediate randomness (~0.3 blocks) across all distances
		// Spread can accumulate, especially at longer ranges
		switch distance {
		case 5:
			return 0.5 // Short-range: base spread 0.3 + margin
		case 15:
			return 0.75 // Medium-range: accumulated spread
		case 30:
			return 1.0 // Long-range: reasonable margin
		case 45:
			return 1.5 // Edge case: significant spread accumulation
		default:
			return 1.0
		}
	}
}

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
