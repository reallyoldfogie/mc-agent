package physics

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestFindOptimalAimingSimple tests FindOptimalAiming finds solutions for simple cases
func TestFindOptimalAimingSimple(t *testing.T) {
	t.Logf("=== Testing FindOptimalAiming with New Pitch Range ===\n")

	testCases := []struct {
		name     string
		distance float64
		vertDist float64
	}{
		{"5 blocks level", 5.0, 0.0},
		{"10 blocks level", 10.0, 0.0},
		{"15 blocks level", 15.0, 0.0},
		{"30 blocks level", 30.0, 0.0},
		{"46 blocks below", 46.0, -1.02},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origin := models.V3{X: 0, Y: 1.52, Z: 0}
			target := models.V3{X: tc.distance, Y: 1.52 + tc.vertDist, Z: 0}

			pitch, power, minError, traj := FindOptimalAiming(models.Arrow, origin, target)

			if len(traj) == 0 {
				t.Logf("❌ FAIL: No trajectory found")
				t.Logf("   Pitch: %.1f°, Power: %.2f, Error: %.4f\n", pitch, power, minError)
			} else {
				t.Logf("✅ FOUND: Pitch=%.1f°, Power=%.2f, Error=%.4f", pitch, power, minError)
				t.Logf("   Trajectory length: %d ticks", len(traj))
				if len(traj) > 0 {
					lastPoint := traj[len(traj)-1]
					t.Logf("   Final position: (%.2f, %.2f, %.2f)", lastPoint.Pos.X, lastPoint.Pos.Y, lastPoint.Pos.Z)
				}
				t.Logf("")
			}
		})
	}
}
