package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// TestPhysicsOrderComparison compares the two update orders at various distances
func TestPhysicsOrderComparison(t *testing.T) {
	testCases := []struct {
		name           string
		targetDist     float64
		expectedPitch  float64 // approximate expected pitch in degrees
		allowableError float64
	}{
		{
			name:           "5 blocks horizontal",
			targetDist:     5.0,
			expectedPitch:  10.9,
			allowableError: 2.0,
		},
		{
			name:           "10 blocks horizontal",
			targetDist:     10.0,
			expectedPitch:  4.5,
			allowableError: 1.5,
		},
		{
			name:           "15 blocks horizontal",
			targetDist:     15.0,
			expectedPitch:  1.8,
			allowableError: 1.5,
		},
		{
			name:           "30 blocks horizontal",
			targetDist:     30.0,
			expectedPitch:  -2.7,
			allowableError: 2.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origin := models.V3{X: 4.5, Y: 1.5, Z: 1.5}
			target := models.V3{X: 4.5 + tc.targetDist, Y: 0.5, Z: 1.5}

			// Test the new solver
			props := GetProjectileProps(models.Arrow)
			solution, err := SolveAim(origin, target, props, false)
			require.NoError(t, err, "SolveAim should find solution")

			pitchDeg := solution.PitchRad * 180 / math.Pi
			t.Logf("Distance %.1f: SolveAim pitch=%.2f°, errorY=%.4f",
				tc.targetDist, pitchDeg, solution.ErrorY)

			// Verify solution is close to expected
			require.InDelta(t, tc.expectedPitch, pitchDeg, tc.allowableError,
				"Pitch should be close to expected value")

			// Now test the trajectory simulation with that pitch
			// Get initial velocity from solution
			t.Logf("  V0=(%.4f, %.4f, %.4f), Tick=%d", solution.V0.X, solution.V0.Y, solution.V0.Z, solution.Tick)

			// Simulate trajectory
			traj := SimulateProjectileTrajectory(models.Arrow, origin, solution.V0, 400)
			t.Logf("  Simulated %d trajectory points", len(traj))

			// Check if trajectory actually hits the target
			// Need larger tolerance because SimulateProjectileTrajectory samples discrete ticks,
			// while evalAtPitch interpolates to exact crossing point
			tolerance := 1.5
			hitFound := false
			closestDist := math.MaxFloat64
			var closestTick int
			var closestPos models.V3

			for _, point := range traj {
				dx := point.Pos.X - target.X
				dy := point.Pos.Y - target.Y
				dz := point.Pos.Z - target.Z
				dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

				if dist < closestDist {
					closestDist = dist
					closestTick = point.Tick
					closestPos = point.Pos
				}

				if dist <= tolerance {
					t.Logf("  Hit at tick %d: pos=(%.2f,%.2f,%.2f), target=(%.2f,%.2f,%.2f), dist=%.2f",
						point.Tick, point.Pos.X, point.Pos.Y, point.Pos.Z,
						target.X, target.Y, target.Z, dist)
					hitFound = true
					break
				}
			}

			if !hitFound {
				t.Logf("  Closest: tick=%d, pos=(%.2f,%.2f,%.2f), dist=%.2f",
					closestTick, closestPos.X, closestPos.Y, closestPos.Z, closestDist)
			}

			require.True(t, hitFound,
				"Trajectory should hit target at distance %.1f with pitch %.2f°",
				tc.targetDist, pitchDeg)
		})
	}
}

// TestDirectComparison simulates the same way evalAtPitch does and compares to SimulateProjectileTrajectory
func TestDirectComparison(t *testing.T) {
	origin := models.V3{X: 4.5, Y: 1.5, Z: 1.5}
	target := models.V3{X: 34.5, Y: 0.5, Z: 1.5} // 30 blocks

	phys := GetProjectilePhysics(models.Arrow)
	props := ProjectileProps{
		Type:     models.Arrow,
		Speed:    phys.InitialSpeed,
		Drag:     phys.Drag,
		Gravity:  phys.Gravity,
		MaxTicks: 400,
		Order:    OrderMoveDragGravity,
	}

	solution, err := SolveAim(origin, target, props, false)
	require.NoError(t, err)

	pitchDeg := solution.PitchRad * 180 / math.Pi
	yawDeg := solution.YawRad * 180 / math.Pi
	t.Logf("SolveAim solution: pitch=%.2f°, yaw=%.2f°, V0=(%.4f,%.4f,%.4f), tick=%d",
		pitchDeg, yawDeg, solution.V0.X, solution.V0.Y, solution.V0.Z, solution.Tick)

	// Now simulate using SimulateProjectileTrajectory
	traj := SimulateProjectileTrajectory(models.Arrow, origin, solution.V0, 400)
	t.Logf("SimulateProjectileTrajectory: %d points", len(traj))

	// Check ticks 8, 9, 10 (around where SolveAim said it crosses)
	for _, point := range traj {
		if point.Tick >= 8 && point.Tick <= 11 {
			dist := math.Sqrt(
				(point.Pos.X-target.X)*(point.Pos.X-target.X) +
					(point.Pos.Y-target.Y)*(point.Pos.Y-target.Y) +
					(point.Pos.Z-target.Z)*(point.Pos.Z-target.Z))
			t.Logf("  Tick %d: pos=(%.2f,%.2f,%.2f), vel=(%.4f,%.4f,%.4f), dist=%.2f",
				point.Tick, point.Pos.X, point.Pos.Y, point.Pos.Z,
				point.Vel.X, point.Vel.Y, point.Vel.Z, dist)
		}
	}

	// Now let's manually trace through what SolveAim's evalAtPitch should produce at each step
	t.Logf("\nManual trace (matching evalAtPitch):")
	pos := origin
	vel := solution.V0
	for step := 1; step <= 12; step++ {
		// This is the OrderMoveDragGravity update
		pos = pos.Add(vel)
		vel = vel.Mul(props.Drag)
		vel.Y -= props.Gravity

		dist := math.Sqrt(
			(pos.X-target.X)*(pos.X-target.X) +
				(pos.Y-target.Y)*(pos.Y-target.Y) +
				(pos.Z-target.Z)*(pos.Z-target.Z))

		if step >= 8 && step <= 11 {
			t.Logf("  Step %d: pos=(%.2f,%.2f,%.2f), vel=(%.4f,%.4f,%.4f), dist=%.2f",
				step, pos.X, pos.Y, pos.Z, vel.X, vel.Y, vel.Z, dist)
		}
	}
}

// TestUpdateOrderDifference tests if the update order matters
func TestUpdateOrderDifference(t *testing.T) {
	origin := models.V3{X: 0, Y: 1.5, Z: 0}
	target := models.V3{X: 30, Y: 0.5, Z: 0}

	phys := GetProjectilePhysics(models.Arrow)
	props := ProjectileProps{
		Type:     models.Arrow,
		Speed:    phys.InitialSpeed,
		Drag:     phys.Drag,
		Gravity:  phys.Gravity,
		MaxTicks: 400,
		Order:    OrderMoveDragGravity,
	}

	// Get the pitch that SolveAim found
	solution, err := SolveAim(origin, target, props, false)
	require.NoError(t, err)

	pitchDeg := solution.PitchRad * 180 / math.Pi
	t.Logf("For 30-block distance: pitch=%.2f°, errorY=%.4f", pitchDeg, solution.ErrorY)

	// Now test both update orders at that pitch
	testUpdateOrder := func(order UpdateOrder, name string) (landingY float64, reachedRange bool) {
		props.Order = order

		// Re-evaluate with this order
		yaw := 0.0
		sol := evalAtPitch(origin, target, yaw, solution.PitchRad, props)

		return sol.ErrorY, sol.Tick > 0
	}

	errorMDG, reachedMDG := testUpdateOrder(OrderMoveDragGravity, "MoveDragGravity")
	errorGDM, reachedGDM := testUpdateOrder(OrderGravityDragMove, "GravityDragMove")

	t.Logf("OrderMoveDragGravity: reached=%v, errorY=%.4f", reachedMDG, errorMDG)
	t.Logf("OrderGravityDragMove: reached=%v, errorY=%.4f", reachedGDM, errorGDM)

	// The error should be small for whichever order is correct
	if math.Abs(errorMDG) < math.Abs(errorGDM) {
		t.Logf("OrderMoveDragGravity is more accurate (smaller error)")
	} else if math.Abs(errorGDM) < math.Abs(errorMDG) {
		t.Logf("OrderGravityDragMove is more accurate (smaller error)")
	} else {
		t.Logf("Both orders have similar accuracy")
	}
}
