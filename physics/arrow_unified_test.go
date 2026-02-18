package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestArrowUnifiedConvention verifies the unified pitch convention
// Negative pitch = UP (looking up in Minecraft)
// Positive pitch = DOWN (looking down in Minecraft)
// Formula: velY = -sin(pitch) × speed
func TestArrowUnifiedConvention(t *testing.T) {
	t.Logf("=== Arrow Unified Convention Test ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	t.Logf("Minecraft/Unified Convention:")
	t.Logf("  Negative pitch = looking UP (upward velocity)")
	t.Logf("  Positive pitch = looking DOWN (downward velocity)")
	t.Logf("  Formula: velY = -sin(pitch) × speed\n")

	// Test the 46-block scenario
	// Target is 46 blocks away, 1.02 blocks BELOW archer
	// To hit a target below with an upward arc, we need NEGATIVE pitch (looking up)

	origin := models.V3{X: 0, Y: 0, Z: 0}
	targetLocal := models.V3{X: 0, Y: -1.02, Z: 46}

	t.Logf("Target Scenario:")
	t.Logf("  Origin: (0, 0, 0)")
	t.Logf("  Target: (0, -1.02, 46)")
	t.Logf("  Arrow needs to go UP first, then DOWN to hit target below\n")

	t.Logf("Testing NEGATIVE pitches (looking up):\n")
	t.Logf("Pitch | VelY | Range | Peak Y | At Z=46, Y=? | Hit Target?")
	t.Logf("------|------|-------|--------|--------------|--------")

	const blockRadius = 0.5
	targetBoundsY := []float64{targetLocal.Y - blockRadius, targetLocal.Y + blockRadius}
	targetBoundsZ := []float64{targetLocal.Z - blockRadius, targetLocal.Z + blockRadius}

	for pitchDeg := -60.0; pitchDeg <= 0; pitchDeg += 5.0 {
		pitchRad := pitchDeg * math.Pi / 180.0

		// Unified formula: velY = -sin(pitch)
		velY := -math.Sin(pitchRad) * arrowPhys.InitialSpeed
		velXZ := math.Cos(pitchRad) * arrowPhys.InitialSpeed

		velocity := models.V3{X: 0, Y: velY, Z: velXZ}
		traj := SimulateProjectileTrajectory(models.Arrow, origin, velocity, 400)

		// Find range and peak height
		var maxRange, maxY float64 = 0, 0
		var yAtTargetZ float64 = 0
		var hitTarget bool = false

		for i, point := range traj {
			if point.Pos.Z > maxRange {
				maxRange = point.Pos.Z
			}
			if point.Pos.Y > maxY {
				maxY = point.Pos.Y
			}

			// Check if at target Z range
			if i > 0 && point.Pos.Z >= targetBoundsZ[0] && point.Pos.Z <= targetBoundsZ[1] {
				yAtTargetZ = point.Pos.Y

				// Check if also in Y range
				if point.Pos.Y >= targetBoundsY[0] && point.Pos.Y <= targetBoundsY[1] {
					hitTarget = true
				}
			}
		}

		hitStr := ""
		if hitTarget {
			hitStr = "✅ YES"
		}

		t.Logf("%.0f° | %.4f | %.2f | %.2f | %.2f | %s",
			pitchDeg, velY, maxRange, maxY, yAtTargetZ, hitStr)
	}

	t.Logf("\n")
}

// TestArrowNegativePitchSolution shows the CORRECT solution for 46-block target
func TestArrowNegativePitchSolution(t *testing.T) {
	t.Logf("=== Arrow: Correct Solution for 46-Block Target ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	t.Logf("Using unified Minecraft convention:")
	t.Logf("  Negative pitch = UP (looking up)")
	t.Logf("  Formula: velY = -sin(pitch) × speed\n")

	// For the reference scenario: 46 blocks away, 1.02 blocks below
	// We need a negative pitch to look up

	// From the reference document, the pitch magnitude is 6.757°
	// But in Minecraft convention, this should be NEGATIVE to go UP
	pitch := -6.757

	pitchRad := pitch * math.Pi / 180.0
	velY := -math.Sin(pitchRad) * arrowPhys.InitialSpeed
	velXZ := math.Cos(pitchRad) * arrowPhys.InitialSpeed

	t.Logf("Pitch: %.3f° (negative = looking UP)", pitch)
	t.Logf("VelY = -sin(%.3f°) × 3.0 = %.4f (upward)\n", pitch, velY)

	origin := models.V3{X: 0, Y: 0, Z: 0}
	targetLocal := models.V3{X: 0, Y: -1.02, Z: 46}

	velocity := models.V3{X: 0, Y: velY, Z: velXZ}
	traj := SimulateProjectileTrajectory(models.Arrow, origin, velocity, 400)

	// Find tick where Z ≈ 46
	t.Logf("Tick-by-Tick near target (Z≈46):\n")
	t.Logf("Tick | X | Y | Z | VelY | At Target Y?")
	t.Logf("----|------|------|------|--------|----------")

	const blockRadius = 0.5
	targetBoundsY := []float64{targetLocal.Y - blockRadius, targetLocal.Y + blockRadius}
	targetBoundsZ := []float64{targetLocal.Z - blockRadius, targetLocal.Z + blockRadius}

	for i, point := range traj {
		if point.Pos.Z >= targetBoundsZ[0]-2 && point.Pos.Z <= targetBoundsZ[1]+2 {
			inY := ""
			if point.Pos.Y >= targetBoundsY[0] && point.Pos.Y <= targetBoundsY[1] {
				inY = "✅ YES"
			} else {
				inY = "❌ NO"
			}

			t.Logf("%d | %.2f | %.2f | %.2f | %.4f | %s",
				i, point.Pos.X, point.Pos.Y, point.Pos.Z, point.Vel.Y, inY)
		}
	}

	// Check final result at Z≈46
	var closestTick int
	var closestDist float64 = math.MaxFloat64
	var closestPoint models.TrajectoryPoint

	for i, point := range traj {
		dist := math.Abs(point.Pos.Z - 46)
		if dist < closestDist {
			closestDist = dist
			closestTick = i
			closestPoint = point
		}
	}

	t.Logf("\nClosest point to Z=46:")
	t.Logf("  Tick: %d", closestTick)
	t.Logf("  Position: (%.2f, %.2f, %.2f)", closestPoint.Pos.X, closestPoint.Pos.Y, closestPoint.Pos.Z)
	t.Logf("  Target Y range: [%.2f, %.2f]", targetBoundsY[0], targetBoundsY[1])
	t.Logf("  Arrow Y at Z≈46: %.2f", closestPoint.Pos.Y)
	t.Logf("  In target? %v\n", closestPoint.Pos.Y >= targetBoundsY[0] && closestPoint.Pos.Y <= targetBoundsY[1])
}

// TestArrowReferenceDocumentConvention explains the reference document's convention
func TestArrowReferenceDocumentConvention(t *testing.T) {
	t.Logf("=== Reference Document vs Unified Convention ===\n")

	t.Logf("Reference Document (ARROW_TRAJECTORY_COMPLETE.md):")
	t.Logf("  Uses convention: positive pitch = UP")
	t.Logf("  Pitch: 6.757° (positive)")
	t.Logf("  VelY = sin(6.757°) × 3.0 = +0.3530 (upward)")
	t.Logf("  Result: Arrow goes UP to hit target below ✓ (works)\n")

	t.Logf("Minecraft/Unified Convention (in code):")
	t.Logf("  Uses convention: negative pitch = UP")
	t.Logf("  Pitch: -6.757° (negative)")
	t.Logf("  VelY = -sin(-6.757°) × 3.0 = +0.3530 (upward)")
	t.Logf("  Result: Arrow goes UP to hit target below ✓ (works)\n")

	t.Logf("CONCLUSION:")
	t.Logf("  The reference document's pitch value has the OPPOSITE SIGN")
	t.Logf("  Ref: +6.757° == Unified: -6.757°")
	t.Logf("  Both produce the same trajectory, just with opposite sign convention\n")

	t.Logf("ACTION NEEDED:")
	t.Logf("  1. FindOptimalAiming currently uses unified convention (correct)")
	t.Logf("  2. But it searches pitches 0° to 60° for level targets")
	t.Logf("  3. Should search -60° to 0° to find negative pitches that go UP")
	t.Logf("  4. OR: Update pitch range selection based on target location\n")
}
