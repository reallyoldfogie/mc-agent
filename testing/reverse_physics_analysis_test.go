package testing

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// TestReversePhysicsAnalysis determines what pitch/power combinations would produce the observed 3.4 block range
// This helps us understand what the server is actually receiving vs what we think we're sending
func TestReversePhysicsAnalysis(t *testing.T) {
	t.Logf("=== Reverse Physics Analysis ===")
	t.Logf("Observed: arrows travel ~3.4 blocks at level (0° pitch)")
	t.Logf("Predicted: should travel ~10 blocks at level with full power")
	t.Logf("")
	t.Logf("Testing what pitch/power combinations produce 3.4 block range...")
	t.Logf("")

	// We know:
	// - Gravity = 0.05 blocks/tick²
	// - Drag = 0.99 per tick
	// - InitialSpeed at full power = 3.25 blocks/tick

	arrowPhys := physics.GetProjectilePhysics(models.Arrow)
	targetDistance := 3.4 // observed distance
	horizontalDist := targetDistance
	verticalDist := 0.0 // level shot

	t.Logf("Arrow Physics Constants:")
	t.Logf("  Gravity: %.4f blocks/tick²", arrowPhys.Gravity)
	t.Logf("  Drag: %.4f per tick", arrowPhys.Drag)
	t.Logf("  InitialSpeed (full power): %.4f blocks/tick", arrowPhys.InitialSpeed)
	t.Logf("")

	// Try different power factors to see which produces 3.4 blocks
	t.Logf("Power Factor Analysis (0° pitch, level shot):")
	t.Logf("Power  InitVel  Distance  Match?")
	t.Logf("------ ------- ---------- --------")

	bestMatch := 0.0
	bestDist := 0.0
	bestPower := 0.0

	for powerFactor := 0.0; powerFactor <= 1.01; powerFactor += 0.05 {
		// Calculate what distance this power produces
		initialSpeed := arrowPhys.InitialSpeed * powerFactor

		// For level shot (0° pitch)
		distX, _, _ := physics.SimulateProjectile(models.Arrow, 0, powerFactor, horizontalDist, verticalDist)

		diff := math.Abs(distX - targetDistance)
		match := ""
		if diff < 0.5 {
			match = "✓"
			if diff < bestMatch || bestMatch == 0 {
				bestMatch = diff
				bestDist = distX
				bestPower = powerFactor
			}
		}

		t.Logf("%.2f   %.4f  %.4f      %s", powerFactor, initialSpeed, distX, match)
	}

	t.Logf("")
	t.Logf("Best match: Power=%.2f produces distance=%.2f (error=%.2f)", bestPower, bestDist, bestMatch)
	t.Logf("")

	// Now analyze at different pitches with full power to see what distance we'd get
	t.Logf("Full Power (1.0) Analysis at different pitches:")
	t.Logf("Pitch   Distance")
	t.Logf("------ ----------")

	for pitch := -45.0; pitch <= 45.0; pitch += 15.0 {
		pitchRad := pitch * math.Pi / 180.0
		distX, _, _ := physics.SimulateProjectile(models.Arrow, pitchRad, 1.0, 100.0, 0.0)
		t.Logf("%.0f°   %.2f blocks", pitch, distX)
	}

	t.Logf("")

	// What if the server is using a different initial speed?
	t.Logf("Reverse calculation: What InitialSpeed would give 3.4 blocks?")

	// At 0 pitch, velocity is purely horizontal
	// We can reverse-calculate what initial velocity would give 3.4 block distance
	var distances []struct {
		initialSpeed float64
		distance     float64
	}

	for initialSpeed := 0.5; initialSpeed <= 3.5; initialSpeed += 0.1 {
		// Simulate with this initial speed
		pos := 0.0
		vel := initialSpeed

		for tick := 0; tick < 400; tick++ {
			pos += vel
			vel *= arrowPhys.Drag

			if vel < 0.001 && tick > 10 {
				break
			}
		}

		distances = append(distances, struct {
			initialSpeed float64
			distance     float64
		}{initialSpeed, pos})
	}

	t.Logf("InitSpeed  Distance  Match?")
	t.Logf("--------- ---------- --------")

	for _, d := range distances {
		match := ""
		if math.Abs(d.distance-targetDistance) < 0.2 {
			match = "✓"
		}
		t.Logf("%.2f     %.2f       %s", d.initialSpeed, d.distance, match)
	}

	t.Logf("")
	t.Logf("=== CONCLUSION ===")
	t.Logf("The server appears to be firing arrows with significantly less power than expected.")
	t.Logf("Possible causes:")
	t.Logf("1. The bow power value isn't being communicated correctly to the server")
	t.Logf("2. The server uses a different formula for power → velocity conversion")
	t.Logf("3. There's a bug or limitation in how we're sending the bow hold action")
}
