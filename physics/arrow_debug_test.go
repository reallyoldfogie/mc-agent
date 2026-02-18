package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestArrowDebugLevelShot examines why level shots (0° pitch) fail for arrows
func TestArrowDebugLevelShot(t *testing.T) {
	t.Logf("=== Arrow Debug: Level Shot Analysis ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	// Test case from reference: 46 blocks horizontal
	t.Logf("Reference scenario: 46 blocks horizontal, level height")
	t.Logf("Arrow physics: speed=%.4f, gravity=%.4f, drag=%.4f\n",
		arrowPhys.InitialSpeed, arrowPhys.Gravity, arrowPhys.Drag)

	// At 0° pitch, velocity should be purely horizontal
	pitch := 0.0
	pitchRad := pitch * math.Pi / 180.0
	initialSpeed := arrowPhys.InitialSpeed * 1.0 // full power

	velY := -math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	t.Logf("At pitch=%.1f°:", pitch)
	t.Logf("  sin(%.1f°) = %.6f", pitch, math.Sin(pitchRad))
	t.Logf("  -sin(%.1f°) = %.6f", pitch, -math.Sin(pitchRad))
	t.Logf("  cos(%.1f°) = %.6f", pitch, math.Cos(pitchRad))
	t.Logf("  Initial velY (vertical) = %.6f", velY)
	t.Logf("  Initial velXZ (horizontal) = %.6f\n", velXZ)

	// Simulate trajectory
	velocity := models.V3{X: 0, Y: velY, Z: velXZ}
	origin := models.V3{X: 0, Y: 0, Z: 0}

	t.Logf("Simulating from origin=(%.2f, %.2f, %.2f)", origin.X, origin.Y, origin.Z)
	t.Logf("With initial velocity=(%.4f, %.4f, %.4f)\n", velocity.X, velocity.Y, velocity.Z)

	model := GetProjectilePhysicsModel(models.Arrow)
	pos := origin
	vel := velocity

	t.Logf("Tick | X | Y | Z | VelX | VelY | VelZ | Speed")
	t.Logf("----|-------|-------|-------|--------|--------|--------|--------")

	for tick := 0; tick < 30; tick++ {
		pos, vel = TickProjectile(pos, vel, model)

		speed := math.Sqrt(vel.X*vel.X + vel.Y*vel.Y + vel.Z*vel.Z)
		t.Logf("%d | %.2f | %.2f | %.2f | %.6f | %.6f | %.6f | %.4f",
			tick, pos.X, pos.Y, pos.Z, vel.X, vel.Y, vel.Z, speed)

		// Stop if velocity is negligible
		if speed < 0.001 {
			t.Logf("\nVelocity negligible, stopping simulation")
			break
		}

		// Stop if position is way below ground
		if pos.Y < -10 {
			t.Logf("\nPosition below ground, stopping simulation")
			break
		}
	}

	t.Logf("\n")
}

// TestArrowDebugUpwardShot examines why upward shots (15° pitch) work well
func TestArrowDebugUpwardShot(t *testing.T) {
	t.Logf("=== Arrow Debug: Upward Shot Analysis ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	// Test case: 15° pitch (working well)
	t.Logf("Test scenario: 15° pitch")
	t.Logf("Arrow physics: speed=%.4f, gravity=%.4f, drag=%.4f\n",
		arrowPhys.InitialSpeed, arrowPhys.Gravity, arrowPhys.Drag)

	pitch := 15.0
	pitchRad := pitch * math.Pi / 180.0
	initialSpeed := arrowPhys.InitialSpeed * 1.0 // full power

	velY := -math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	t.Logf("At pitch=%.1f°:", pitch)
	t.Logf("  sin(%.1f°) = %.6f", pitch, math.Sin(pitchRad))
	t.Logf("  -sin(%.1f°) = %.6f", pitch, -math.Sin(pitchRad))
	t.Logf("  cos(%.1f°) = %.6f", pitch, math.Cos(pitchRad))
	t.Logf("  Initial velY (vertical) = %.6f", velY)
	t.Logf("  Initial velXZ (horizontal) = %.6f\n", velXZ)

	// Simulate trajectory
	velocity := models.V3{X: 0, Y: velY, Z: velXZ}
	origin := models.V3{X: 0, Y: 0, Z: 0}

	t.Logf("Simulating from origin=(%.2f, %.2f, %.2f)", origin.X, origin.Y, origin.Z)
	t.Logf("With initial velocity=(%.4f, %.4f, %.4f)\n", velocity.X, velocity.Y, velocity.Z)

	model := GetProjectilePhysicsModel(models.Arrow)
	pos := origin
	vel := velocity

	t.Logf("Tick | X | Y | Z | VelX | VelY | VelZ | Speed | Distance from Z=46")
	t.Logf("----|-------|-------|-------|--------|--------|--------|--------|------------------")

	targetZ := 46.0
	for tick := 0; tick < 50; tick++ {
		pos, vel = TickProjectile(pos, vel, model)

		speed := math.Sqrt(vel.X*vel.X + vel.Y*vel.Y + vel.Z*vel.Z)
		distFromTarget := math.Abs(pos.Z - targetZ)

		t.Logf("%d | %.2f | %.2f | %.2f | %.6f | %.6f | %.6f | %.4f | %.2f",
			tick, pos.X, pos.Y, pos.Z, vel.X, vel.Y, vel.Z, speed, distFromTarget)

		// Stop if velocity is negligible
		if speed < 0.001 {
			t.Logf("\nVelocity negligible, stopping simulation")
			break
		}

		// Stop if position is way below ground
		if pos.Y < -10 {
			t.Logf("\nPosition below ground, stopping simulation")
			break
		}
	}

	t.Logf("\n")
}

// TestArrowDebugCoordinateSystem verifies pitch sign convention
func TestArrowDebugCoordinateSystem(t *testing.T) {
	t.Logf("=== Arrow Debug: Coordinate System Verification ===\n")

	t.Logf("Minecraft pitch convention:")
	t.Logf("  Negative pitch = looking UP (should have positive Y velocity)")
	t.Logf("  Positive pitch = looking DOWN (should have negative Y velocity)")
	t.Logf("  0° = level/horizontal\n")

	testPitches := []float64{-45, -30, -15, 0, 15, 30, 45}
	initialSpeed := 3.0

	t.Logf("Pitch | Sin | -Sin | Expected VelY | Actual Formula | Result")
	t.Logf("------|------|------|---------------|----------------|--------")

	for _, pitch := range testPitches {
		pitchRad := pitch * math.Pi / 180.0
		sinVal := math.Sin(pitchRad)
		negSinVal := -math.Sin(pitchRad)
		velY := negSinVal * initialSpeed

		expected := ""
		if pitch < 0 {
			expected = "POSITIVE (up)"
		} else if pitch > 0 {
			expected = "NEGATIVE (down)"
		} else {
			expected = "ZERO (level)"
		}

		result := ""
		if velY > 0.001 {
			result = "UP ✅"
		} else if velY < -0.001 {
			result = "DOWN ✅"
		} else {
			result = "LEVEL ✅"
		}

		t.Logf("%.0f | %.3f | %.3f | %s | velY = -sin(pitch)*speed = %.3f | %s",
			pitch, sinVal, negSinVal, expected, velY, result)
	}

	t.Logf("\n")
}

// TestArrowDebugFindOptimalAiming checks what pitch is being tested for a 46-block target
func TestArrowDebugFindOptimalAiming(t *testing.T) {
	t.Logf("=== Arrow Debug: FindOptimalAiming for 46 Blocks ===\n")

	arrowPhys := GetProjectilePhysics(models.Arrow)

	// Reference scenario: 46 blocks, 1.02 blocks down
	origin := models.V3{X: 0, Y: 1.52, Z: 0}
	target := models.V3{X: 46, Y: 0.5, Z: 0}

	t.Logf("Origin: (%.2f, %.2f, %.2f)", origin.X, origin.Y, origin.Z)
	t.Logf("Target: (%.2f, %.2f, %.2f)", target.X, target.Y, target.Z)
	t.Logf("Horizontal distance: %.2f blocks", target.X-origin.X)
	t.Logf("Vertical distance: %.2f blocks\n", target.Y-origin.Y)

	t.Logf("Testing pitches from 0° to 60°...\n")

	// Manually test several pitches to see what happens
	testPitches := []float64{0, 5, 6.757, 10, 15, 20, 30, 45}

	for _, pitch := range testPitches {
		pitchRad := pitch * math.Pi / 180.0

		// Calculate velocity for this pitch
		initialSpeed := arrowPhys.InitialSpeed * 1.0 // full power
		velY := -math.Sin(pitchRad) * initialSpeed
		velXZ := math.Cos(pitchRad) * initialSpeed

		// Simulate in local space (origin at 0,0,0, target at (46, -1.02, 0))
		velocity := models.V3{X: 0, Y: velY, Z: velXZ}
		localOrigin := models.V3{X: 0, Y: 0, Z: 0}

		traj := SimulateProjectileTrajectory(models.Arrow, localOrigin, velocity, 400)

		// Check if any point hits the target
		const blockRadius = 0.5
		targetLocal := models.V3{X: 0, Y: -1.02, Z: 46.0}

		hitFound := false
		var hitTick int
		var hitPoint models.TrajectoryPoint

		for i, point := range traj {
			if point.Pos.X >= -blockRadius && point.Pos.X <= blockRadius &&
				point.Pos.Y >= targetLocal.Y-blockRadius && point.Pos.Y <= targetLocal.Y+blockRadius &&
				point.Pos.Z >= targetLocal.Z-blockRadius && point.Pos.Z <= targetLocal.Z+blockRadius {
				hitFound = true
				hitTick = i
				hitPoint = point
				break
			}
		}

		t.Logf("Pitch=%.3f°: ", pitch)
		if hitFound {
			t.Logf("✅ HIT at tick %d", hitTick)
			t.Logf("  Hit position: (%.2f, %.2f, %.2f)", hitPoint.Pos.X, hitPoint.Pos.Y, hitPoint.Pos.Z)
			t.Logf("  Hit velocity: (%.4f, %.4f, %.4f)", hitPoint.Vel.X, hitPoint.Vel.Y, hitPoint.Vel.Z)
		} else {
			t.Logf("❌ MISS - max range=%.2f, max ticks=%d",
				func() float64 {
					if len(traj) == 0 {
						return 0
					}
					return traj[len(traj)-1].Pos.Z
				}(),
				len(traj))
		}
	}

	t.Logf("\n")
}
