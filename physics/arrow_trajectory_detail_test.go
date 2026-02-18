package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestArrowTrajectoryDetail shows detailed trajectory points near the 46-block target
func TestArrowTrajectoryDetail(t *testing.T) {
	t.Logf("=== Arrow Trajectory Detail: Looking for Z≈46 ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	// Reference scenario: origin at (0,0,0) in local space, target at (0, -1.02, 46)
	// This matches what FindOptimalAiming uses internally
	origin := models.V3{X: 0, Y: 0, Z: 0}
	targetLocal := models.V3{X: 0, Y: -1.02, Z: 46}

	t.Logf("Target block: X ∈ [-0.5, 0.5], Y ∈ [-1.52, -0.52], Z ∈ [45.5, 46.5]\n")

	pitch := 6.757 // From reference document
	pitchRad := pitch * math.Pi / 180.0
	initialSpeed := arrowPhys.InitialSpeed

	velY := -math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	t.Logf("Pitch: %.3f°", pitch)
	t.Logf("Initial velocity: X=0, Y=%.6f, Z=%.6f", velY, velXZ)
	t.Logf("Speed: %.6f\n", math.Sqrt(velY*velY + velXZ*velXZ))

	velocity := models.V3{X: 0, Y: velY, Z: velXZ}
	traj := SimulateProjectileTrajectory(models.Arrow, origin, velocity, 400)

	// Find points where Z is near 46
	t.Logf("Trajectory points near Z=46:\n")
	t.Logf("Tick | X | Y | Z | VelY | In Target Block?")
	t.Logf("----|------|------|------|---------|----------------")

	const blockRadius = 0.5
	targetBoundsY := []float64{targetLocal.Y - blockRadius, targetLocal.Y + blockRadius}
	targetBoundsZ := []float64{targetLocal.Z - blockRadius, targetLocal.Z + blockRadius}

	for i, point := range traj {
		// Only show points where Z is close to target Z
		if point.Pos.Z >= targetBoundsZ[0]-2 && point.Pos.Z <= targetBoundsZ[1]+2 {
			inBlock := ""
			if point.Pos.X >= -blockRadius && point.Pos.X <= blockRadius &&
				point.Pos.Y >= targetBoundsY[0] && point.Pos.Y <= targetBoundsY[1] &&
				point.Pos.Z >= targetBoundsZ[0] && point.Pos.Z <= targetBoundsZ[1] {
				inBlock = "✅ YES"
			} else {
				inBlock = "❌ NO"
			}

			t.Logf("%d | %.2f | %.2f | %.2f | %.6f | %s",
				i, point.Pos.X, point.Pos.Y, point.Pos.Z, point.Vel.Y, inBlock)
		}
	}

	t.Logf("\nTarget block Y bounds: [%.2f, %.2f]", targetBoundsY[0], targetBoundsY[1])
	t.Logf("Target block Z bounds: [%.2f, %.2f]\n", targetBoundsZ[0], targetBoundsZ[1])

	// Find min and max Y when Z is in range
	var minY, maxY float64 = math.MaxFloat64, -math.MaxFloat64
	var tickAtMinY, tickAtMaxY int

	for i, point := range traj {
		if point.Pos.Z >= targetBoundsZ[0] && point.Pos.Z <= targetBoundsZ[1] {
			if point.Pos.Y < minY {
				minY = point.Pos.Y
				tickAtMinY = i
			}
			if point.Pos.Y > maxY {
				maxY = point.Pos.Y
				tickAtMaxY = i
			}
		}
	}

	if minY != math.MaxFloat64 {
		t.Logf("When Z ∈ [45.5, 46.5]:")
		t.Logf("  Min Y: %.2f at tick %d", minY, tickAtMinY)
		t.Logf("  Max Y: %.2f at tick %d", maxY, tickAtMaxY)
		t.Logf("  Target Y range: [%.2f, %.2f]", targetBoundsY[0], targetBoundsY[1])
		t.Logf("  Arrow Y passes through target Y range? %v\n",
			minY <= targetBoundsY[1] && maxY >= targetBoundsY[0])
	}
}

// TestArrowTrajectoryComparison compares the trajectory with reference document
func TestArrowTrajectoryComparison(t *testing.T) {
	t.Logf("=== Arrow Trajectory Comparison: Go Implementation vs Reference ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	// Reference scenario from ARROW_TRAJECTORY_COMPLETE.md
	// Shot from (-7.5, 1.52, -5.5) to target (38.5, 0.5, -4.5)
	// In local space: origin (0, 0, 0), target (46 blocks horiz, -1.02 blocks vert)
	// With pitch 6.757°

	origin := models.V3{X: 0, Y: 0, Z: 0}

	pitch := 6.757
	pitchRad := pitch * math.Pi / 180.0
	initialSpeed := arrowPhys.InitialSpeed

	velY := -math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	t.Logf("Pitch: %.3f°", pitch)
	t.Logf("Initial velocity components:")
	t.Logf("  VelY = -sin(%.3f°) × %.4f = %.4f", pitch, initialSpeed, velY)
	t.Logf("  VelXZ = cos(%.3f°) × %.4f = %.4f\n", pitch, initialSpeed, velXZ)

	velocity := models.V3{X: 0, Y: velY, Z: velXZ}
	traj := SimulateProjectileTrajectory(models.Arrow, origin, velocity, 400)

	// Reference document shows specific ticks
	referenceTicks := []int{0, 1, 2, 5, 10, 15, 17}

	t.Logf("Tick-by-Tick Comparison:\n")
	t.Logf("Tick | Pos X | Pos Y | Pos Z | Vel Y | Vel Z | Status")
	t.Logf("----|-------|-------|-------|-------|-------|--------")

	for _, refTick := range referenceTicks {
		if refTick < len(traj) {
			point := traj[refTick]
			t.Logf("%d | %.2f | %.2f | %.2f | %.4f | %.4f | Generated",
				refTick, point.Pos.X, point.Pos.Y, point.Pos.Z,
				point.Vel.Y, point.Vel.Z)
		}
	}

	if len(traj) > 0 {
		lastPoint := traj[len(traj)-1]
		t.Logf("\nLast point in trajectory:")
		t.Logf("  Tick: %d", len(traj)-1)
		t.Logf("  Position: (%.2f, %.2f, %.2f)", lastPoint.Pos.X, lastPoint.Pos.Y, lastPoint.Pos.Z)
		t.Logf("  Velocity: (%.4f, %.4f, %.4f)", lastPoint.Vel.X, lastPoint.Vel.Y, lastPoint.Vel.Z)
	}

	t.Logf("\nReference document shows arrow should:")
	t.Logf("  - Reach ~46 blocks horizontal distance")
	t.Logf("  - Drop ~1.0 blocks vertically")
	t.Logf("  - Take ~17 ticks\n")

	// Find closest point to target
	targetZ := 46.0
	var closestDist float64 = math.MaxFloat64
	var closestTick int
	var closestPoint models.TrajectoryPoint

	for i, point := range traj {
		dist := math.Abs(point.Pos.Z - targetZ)
		if dist < closestDist {
			closestDist = dist
			closestTick = i
			closestPoint = point
		}
	}

	t.Logf("Arrow's closest point to Z=46:")
	t.Logf("  Tick: %d", closestTick)
	t.Logf("  Position: (%.2f, %.2f, %.2f)", closestPoint.Pos.X, closestPoint.Pos.Y, closestPoint.Pos.Z)
	t.Logf("  Distance from Z=46: %.2f blocks", closestDist)
	t.Logf("  Y at this point: %.2f (reference shows Y≈0.5 at tick 17)\n", closestPoint.Pos.Y)
}

// TestArrowPhysicsConstants verifies arrow constants against reference
func TestArrowPhysicsConstants(t *testing.T) {
	t.Logf("=== Arrow Physics Constants Verification ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	t.Logf("Go Implementation:")
	t.Logf("  Gravity: %.6f blocks/tick²", arrowPhys.Gravity)
	t.Logf("  Drag: %.6f per tick", arrowPhys.Drag)
	t.Logf("  Initial Speed: %.6f blocks/tick\n", arrowPhys.InitialSpeed)

	t.Logf("Reference (Java 1.21.8):")
	t.Logf("  Gravity: 0.050000 blocks/tick²")
	t.Logf("  Drag: 0.990000 per tick")
	t.Logf("  Initial Speed: 3.000000 blocks/tick (full draw)\n")

	const epsilon = 0.0001
	gravMatch := math.Abs(arrowPhys.Gravity-0.05) < epsilon
	dragMatch := math.Abs(arrowPhys.Drag-0.99) < epsilon
	speedMatch := math.Abs(arrowPhys.InitialSpeed-3.0) < epsilon

	t.Logf("Verification:")
	t.Logf("  Gravity match: %v", gravMatch)
	t.Logf("  Drag match: %v", dragMatch)
	t.Logf("  Speed match: %v\n", speedMatch)

	if gravMatch && dragMatch && speedMatch {
		t.Logf("✅ All physics constants match reference document\n")
	} else {
		t.Logf("❌ Physics constants do not match reference\n")
	}
}
