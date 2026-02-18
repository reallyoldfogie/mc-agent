package testing

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// TestDebugOptimalAiming traces through what pitch is selected for a simple 10m level shot
func TestDebugOptimalAiming(t *testing.T) {
	t.Logf("=== Debug Optimal Aiming ===")
	t.Logf("")
	t.Logf("Test case: 10-block horizontal distance, 0-block vertical distance (level shot)")
	t.Logf("")

	// Call FindOptimalAiming to see what pitch it selects
	// Local space target: 10 blocks forward (Z), level (Y=0)
	origin := models.V3{X: 0, Y: 0, Z: 0}
	target := models.V3{X: 0, Y: 0, Z: 10}
	pitch, power, minError, trajectory := physics.FindOptimalAiming(models.Arrow, origin, target)

	t.Logf("Result from FindOptimalAiming:")
	t.Logf("  Pitch: %.1f°", pitch)
	t.Logf("  Power: %.2f", power)
	t.Logf("  Error: %.2f blocks", minError)
	t.Logf("  Trajectory points: %d", len(trajectory))
	t.Logf("")

	// Now manually simulate what happens with that pitch
	t.Logf("Manual simulation with selected pitch (%.1f°):", pitch)

	pitchRad := pitch * math.Pi / 180.0
	arrowPhys := physics.GetProjectilePhysics(models.Arrow)
	initialSpeed := arrowPhys.InitialSpeed * power

	velY := math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	t.Logf("  Initial velocity: X/Z=%.4f, Y=%.4f", velXZ, velY)
	t.Logf("")

	// Simulate tick by tick
	posX := 0.0
	posY := 0.0
	velX := velXZ
	velYCur := velY

	t.Logf("Tick   PosX    PosY    VelX    VelY    Distance from target (10m)")
	t.Logf("----- ------- ------- ------- ------- ---------------------------")

	targetDist := 10.0
	hitFound := false

	for tick := 0; tick < 100; tick++ {
		posX += velX
		posY += velYCur

		velYCur -= arrowPhys.Gravity
		velX *= arrowPhys.Drag
		velYCur *= arrowPhys.Drag

		distFromTarget := math.Abs(posX - targetDist)

		// Check if we'd hit the target
		if posY <= 0 && posY >= -0.5 && distFromTarget <= 0.5 {
			t.Logf("%d     %.2f   %.2f   %.4f  %.4f  %.2f ✓ HIT",
				tick, posX, posY, velX, velYCur, distFromTarget)
			hitFound = true
			break
		}

		if tick < 20 || tick%5 == 0 {
			t.Logf("%d     %.2f   %.2f   %.4f  %.4f  %.2f",
				tick, posX, posY, velX, velYCur, distFromTarget)
		}

		// Stop if arrow has fallen well below ground
		if posY < -10 {
			break
		}
	}

	if !hitFound {
		t.Logf("")
		t.Logf("NO HIT FOUND - Arrow never reaches target height within range!")
		t.Logf("This explains why FindOptimalAiming returns a pitch that doesn't actually hit the target!")
	}
}
