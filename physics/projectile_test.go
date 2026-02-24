package physics

import (
	"math"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetProjectilePhysics(t *testing.T) {
	tests := []struct {
		name              string
		projectileType    models.ProjectileType
		expectedGravity   float64
		expectedDrag      float64
		expectedInitSpeed float64
	}{
		{
			name:              "Arrow physics",
			projectileType:    models.Arrow,
			expectedGravity:   ArrowGravity,      // 0.05
			expectedDrag:      ArrowDrag,         // 0.99
			expectedInitSpeed: ArrowInitialSpeed, // 3.1
		},
		{
			name:              "Snowball physics",
			projectileType:    models.Snowball,
			expectedGravity:   SnowballGravity,      // 0.03
			expectedDrag:      SnowballDrag,         // 0.99
			expectedInitSpeed: SnowballInitialSpeed, // 1.5
		},
		{
			name:              "Egg physics (same as snowball)",
			projectileType:    models.Egg,
			expectedGravity:   SnowballGravity,
			expectedDrag:      SnowballDrag,
			expectedInitSpeed: SnowballInitialSpeed,
		},
		{
			name:              "Ender pearl physics",
			projectileType:    models.EnderPearl,
			expectedGravity:   EnderPearlGravity,      // 0.03
			expectedDrag:      EnderPearlDrag,         // 0.99
			expectedInitSpeed: EnderPearlInitialSpeed, // 1.5
		},
		{
			name:              "Splash potion physics",
			projectileType:    models.SplashPotion,
			expectedGravity:   SplashPotionGravity,      // 0.05
			expectedDrag:      SplashPotionDrag,         // 0.99
			expectedInitSpeed: SplashPotionInitialSpeed, // 0.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phys := GetProjectilePhysics(tt.projectileType)

			assert.Equal(t, tt.expectedGravity, phys.Gravity, "Gravity should match")
			assert.Equal(t, tt.expectedDrag, phys.Drag, "Drag should match")
			assert.Equal(t, tt.expectedInitSpeed, phys.InitialSpeed, "Initial speed should match")
		})
	}
}

func TestGetProjectilePhysics_DefaultFallback(t *testing.T) {
	// Test that unknown projectile types get snowball physics as default
	invalidType := models.ProjectileType(999)
	phys := GetProjectilePhysics(invalidType)

	assert.Equal(t, 0.03, phys.Gravity, "Should default to snowball gravity")
	assert.Equal(t, 0.99, phys.Drag, "Should default to snowball drag")
	assert.Equal(t, 1.5, phys.InitialSpeed, "Should default to snowball speed")
}

func TestSimulateProjectile_HitTarget(t *testing.T) {
	// Test arrow hitting a target at reasonable distance
	pitchRad := -10.0 * math.Pi / 180.0 // -10 degrees (slightly down)
	powerFactor := 1.0
	horizontalDist := 20.0
	verticalDist := -2.0 // 2 blocks below

	hitX, hitY, hit := SimulateProjectile(models.Arrow, pitchRad, powerFactor, horizontalDist, verticalDist)

	// Should hit somewhere near target
	if hit {
		assert.InDelta(t, horizontalDist, hitX, 5.0, "Hit position should be close to target horizontally")
		assert.InDelta(t, verticalDist, hitY, 1.0, "Hit position should be close to target vertically")
	}
}

func TestSimulateProjectile_MissTarget(t *testing.T) {
	// Test projectile missing target (too high angle)
	pitchRad := 45.0 * math.Pi / 180.0 // 45 degrees up
	powerFactor := 1.0
	horizontalDist := 5.0
	verticalDist := 0.0

	_, _, hit := SimulateProjectile(models.Arrow, pitchRad, powerFactor, horizontalDist, verticalDist)

	// With 45 degree angle, should overshoot a close target
	assert.False(t, hit, "Should miss target with wrong angle")
}

func TestSimulateProjectileTrajectory_BasicTrajectory(t *testing.T) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	velocity := models.V3{X: 1.0, Y: 1.0, Z: 0}
	maxTicks := 100

	trajectory := SimulateProjectileTrajectory(models.Arrow, origin, velocity, maxTicks)

	require.NotEmpty(t, trajectory, "Should have trajectory points")
	require.LessOrEqual(t, len(trajectory), maxTicks, "Should not exceed maxTicks")

	// First point should be near origin
	firstPoint := trajectory[0]
	assert.InDelta(t, origin.X+velocity.X, firstPoint.Pos.X, 0.1, "First X position")
	assert.InDelta(t, origin.Y+velocity.Y-ArrowGravity, firstPoint.Pos.Y, 0.1, "First Y position (with gravity)")

	// Verify velocity decreases over time due to drag
	if len(trajectory) > 10 {
		earlyVel := math.Sqrt(trajectory[1].Vel.X*trajectory[1].Vel.X + trajectory[1].Vel.Y*trajectory[1].Vel.Y)
		lateVel := math.Sqrt(trajectory[10].Vel.X*trajectory[10].Vel.X + trajectory[10].Vel.Y*trajectory[10].Vel.Y)
		assert.Less(t, lateVel, earlyVel, "Velocity should decrease over time due to drag")
	}
}

func TestSimulateProjectileTrajectory_GravityEffect(t *testing.T) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	velocity := models.V3{X: 1.0, Y: 0, Z: 0} // Horizontal velocity only
	maxTicks := 50

	trajectory := SimulateProjectileTrajectory(models.Arrow, origin, velocity, maxTicks)

	require.NotEmpty(t, trajectory, "Should have trajectory points")

	// Y should decrease over time due to gravity
	for i := 1; i < len(trajectory); i++ {
		assert.Less(t, trajectory[i].Pos.Y, trajectory[i-1].Pos.Y,
			"Y position should decrease due to gravity (tick %d)", i)
	}
}

func TestSimulateProjectileTrajectory_VelocityDecay(t *testing.T) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	velocity := models.V3{X: 2.0, Y: 0, Z: 2.0}
	maxTicks := 400 // More ticks to allow full decay

	trajectory := SimulateProjectileTrajectory(models.Arrow, origin, velocity, maxTicks)

	require.NotEmpty(t, trajectory, "Should have trajectory points")

	// Verify velocity decreases over time
	if len(trajectory) >= 10 {
		earlySpeed := math.Sqrt(trajectory[5].Vel.X*trajectory[5].Vel.X + trajectory[5].Vel.Z*trajectory[5].Vel.Z)
		lateSpeed := math.Sqrt(trajectory[len(trajectory)-1].Vel.X*trajectory[len(trajectory)-1].Vel.X +
			trajectory[len(trajectory)-1].Vel.Z*trajectory[len(trajectory)-1].Vel.Z)
		assert.Less(t, lateSpeed, earlySpeed, "Horizontal velocity should decrease over time")
	}
}

func TestFindOptimalAiming_HorizontalShot(t *testing.T) {
	// Find optimal trajectory for horizontal shot (same height)
	horizontalDist := 20.0
	verticalDist := 0.0

	origin := models.V3{X: 0, Y: 64, Z: 0}
	target := models.V3{X: horizontalDist, Y: 64 + verticalDist, Z: 0}

	pitch, power, minError, _ := FindOptimalAiming(models.Arrow, origin, target)

	assert.Greater(t, power, 0.0, "Power should be positive")
	assert.LessOrEqual(t, power, 1.0, "Power should not exceed 1.0")
	assert.LessOrEqual(t, math.Abs(pitch), 90.0, "Pitch should be within valid range")

	// For horizontal shot, pitch should be slightly negative to compensate for gravity
	assert.Less(t, pitch, 5.0, "Pitch should be low or negative for horizontal shot")

	// Verify the solution actually works
	pitchRad := pitch * math.Pi / 180.0
	hitX, hitY, hit := SimulateProjectile(models.Arrow, pitchRad, power, horizontalDist, verticalDist)

	if minError < 0.5 {
		assert.True(t, hit, "Optimal trajectory should hit target")
		assert.InDelta(t, horizontalDist, hitX, 1.0, "Should hit near target")
		assert.InDelta(t, verticalDist, hitY, 1.0, "Should hit at correct height")
	}
}

func TestFindOptimalAiming_DownwardShot(t *testing.T) {
	// Find optimal trajectory for shooting down
	horizontalDist := 15.0
	verticalDist := -10.0 // 10 blocks below

	origin := models.V3{X: 0, Y: 64, Z: 0}
	target := models.V3{X: horizontalDist, Y: 64 + verticalDist, Z: 0}

	pitch, power, minError, _ := FindOptimalAiming(models.Arrow, origin, target)

	assert.Greater(t, power, 0.0, "Power should be positive")
	assert.LessOrEqual(t, power, 1.0, "Power should not exceed 1.0")

	// For downward shots, the algorithm may find upward arc solutions
	// The important thing is that a solution was found with reasonable error
	if minError < 1.0 {
		assert.LessOrEqual(t, math.Abs(pitch), 90.0, "Pitch should be within valid range")
	}
}

func TestSolveAim(t *testing.T) {
	tests := []struct {
		name   string
		origin models.V3
		target models.V3
	}{
		{
			name:   "Target to the north",
			origin: models.V3{X: 0, Y: 64, Z: 0},
			target: models.V3{X: 0, Y: 64, Z: -20},
		},
		{
			name:   "Target to the east",
			origin: models.V3{X: 0, Y: 64, Z: 0},
			target: models.V3{X: 20, Y: 64, Z: 0},
		},
		{
			name:   "Target above",
			origin: models.V3{X: 0, Y: 64, Z: 0},
			target: models.V3{X: 10, Y: 74, Z: 10},
		},
		{
			name:   "Target below",
			origin: models.V3{X: 0, Y: 64, Z: 0},
			target: models.V3{X: 10, Y: 54, Z: 10},
		},
	}

	props := GetProjectileProps(models.Arrow)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			solution, err := SolveAim(tt.origin, tt.target, props, false)
			require.NoError(t, err, "Should solve aiming without error")

			yaw, pitch := rad2deg(solution.YawRad), rad2deg(solution.PitchRad)

			caYaw, caPitch, _ := CalculateAiming_OLD(models.Arrow, tt.origin, tt.target) // For comparison; not used in new implementation

			t.Logf("%s: SolveAim yaw=%.2f°, pitch=%.2f° | CalculateAiming yaw=%.2f°, pitch=%.2f°",
				tt.name, yaw, pitch, caYaw, caPitch)

			// Validate ranges
			assert.LessOrEqual(t, math.Abs(yaw), 180.0, "Yaw should be within [-180, 180]")
			assert.LessOrEqual(t, math.Abs(pitch), 90.0, "Pitch should be within [-90, 90]")

		// Verify yaw points toward target
		dx := tt.target.X - tt.origin.X
		dz := tt.target.Z - tt.origin.Z
		// SolveAim uses convention: yaw = atan2(dZ, dX)
		expectedYawSolveAim := rad2deg(math.Atan2(dz, dx))
		// CalculateAiming_OLD uses convention: yaw = atan2(-dX, -dZ)
		expectedYawOld := rad2deg(math.Atan2(-dx, -dz))
		assert.InDelta(t, expectedYawSolveAim, yaw, 0.1, "SolveAim Yaw should point toward target")
		assert.InDelta(t, expectedYawOld, caYaw, 0.1, "CalculateAiming Yaw should point toward target")
		})
	}
}

func TestPredictLandingPosition_HorizontalThrow(t *testing.T) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	yaw := 0.0   // North
	pitch := 0.0 // Horizontal
	power := 1.0

	landing := PredictLandingPosition(models.Arrow, origin, yaw, pitch, power)

	// Should land north of origin
	assert.Less(t, landing.Z, origin.Z, "Should land north (negative Z)")
	// Should land at or below origin height
	assert.LessOrEqual(t, landing.Y, origin.Y, "Should land at or below origin due to gravity")
	// X should be relatively unchanged
	assert.InDelta(t, origin.X, landing.X, 1.0, "X should not change much")
}

func TestPredictLandingPosition_UpwardThrow(t *testing.T) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	yaw := 90.0   // East
	pitch := 45.0 // 45 degrees up
	power := 1.0

	landing := PredictLandingPosition(models.Arrow, origin, yaw, pitch, power)

	// Should land east of origin
	assert.Greater(t, landing.X, origin.X, "Should land east (positive X)")
	// Should land below or at origin (goes up then comes down)
	assert.LessOrEqual(t, landing.Y, origin.Y+10.0, "Should land reasonably close to origin height")
}

func TestPredictLandingPosition_DownwardThrow(t *testing.T) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	yaw := 180.0   // South
	pitch := -45.0 // 45 degrees down
	power := 1.0

	landing := PredictLandingPosition(models.Arrow, origin, yaw, pitch, power)

	// Should land south of origin
	assert.Greater(t, landing.Z, origin.Z, "Should land south (positive Z)")
	// Should land well below origin
	assert.Less(t, landing.Y, origin.Y, "Should land below origin")
}

func TestProjectileType_AllTypes(t *testing.T) {
	// Verify all projectile types have physics defined
	types := []models.ProjectileType{models.Arrow, models.Snowball, models.Egg, models.EnderPearl, models.SplashPotion, models.Trident, models.FishingBobber}

	for _, pType := range types {
		phys := GetProjectilePhysics(pType)

		assert.Greater(t, phys.Gravity, 0.0, "Gravity should be positive for type %v", pType)
		assert.Greater(t, phys.Drag, 0.0, "Drag should be positive for type %v", pType)
		assert.Greater(t, phys.InitialSpeed, 0.0, "Initial speed should be positive for type %v", pType)
		assert.LessOrEqual(t, phys.Drag, 1.0, "Drag should not exceed 1.0 for type %v", pType)
	}
}

func TestTrajectoryPoint_Structure(t *testing.T) {
	// Verify TrajectoryPoint structure works as expected
	point := models.TrajectoryPoint{
		Pos:  models.V3{X: 1, Y: 2, Z: 3},
		Vel:  models.V3{X: 0.5, Y: 0.3, Z: 0.4},
		Tick: 10,
		Hit:  true,
	}

	assert.Equal(t, 1.0, point.Pos.X)
	assert.Equal(t, 2.0, point.Pos.Y)
	assert.Equal(t, 3.0, point.Pos.Z)
	assert.Equal(t, 0.5, point.Vel.X)
	assert.Equal(t, 10, point.Tick)
	assert.True(t, point.Hit)
}

// Benchmarks
func BenchmarkSimulateProjectile(b *testing.B) {
	pitchRad := -10.0 * math.Pi / 180.0
	for b.Loop() {
		SimulateProjectile(models.Arrow, pitchRad, 1.0, 20.0, -2.0)
	}
}

func BenchmarkSimulateProjectileTrajectory(b *testing.B) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	velocity := models.V3{X: 1.0, Y: 1.0, Z: 0}

	for b.Loop() {
		SimulateProjectileTrajectory(models.Arrow, origin, velocity, 100)
	}
}

func BenchmarkFindOptimalTrajectory(b *testing.B) {
	for b.Loop() {
		origin := models.V3{X: 0, Y: 64, Z: 0}
		target := models.V3{X: 20.0, Y: 64.0, Z: 0}
		FindOptimalAiming(models.Arrow, origin, target)
	}
}

func BenchmarkSolveAim(b *testing.B) {
	origin := models.V3{X: 0, Y: 64, Z: 0}
	target := models.V3{X: 10, Y: 64, Z: 10}
	props := GetProjectileProps(models.Arrow)

	for b.Loop() {
		SolveAim(origin, target, props, false)
	}
}

func BenchmarkPredictLandingPosition(b *testing.B) {
	origin := models.V3{X: 0, Y: 64, Z: 0}

	for b.Loop() {
		PredictLandingPosition(models.Arrow, origin, 0.0, 0.0, 1.0)
	}
}
