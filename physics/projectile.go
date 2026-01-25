package physics

import "math"

// ProjectileType represents different types of throwable projectiles
type ProjectileType int

const (
	Arrow ProjectileType = iota
	Snowball
	Egg
	EnderPearl
	SplashPotion
	Trident
	FishingBobber
)

// ProjectilePhysics defines physics constants for a projectile type
type ProjectilePhysics struct {
	Gravity      float64 // Gravitational acceleration (blocks/tick²)
	Drag         float64 // Air resistance multiplier
	InitialSpeed float64 // Base throw/shoot speed
}

// GetProjectilePhysics returns physics constants for a projectile type
func GetProjectilePhysics(pType ProjectileType) ProjectilePhysics {
	switch pType {
	case Arrow:
		return ProjectilePhysics{
			Gravity:      ArrowGravity,      // 0.05
			Drag:         ArrowDrag,         // 0.99
			InitialSpeed: ArrowInitialSpeed, // 3.1
		}
	case Snowball, Egg:
		return ProjectilePhysics{
			Gravity:      SnowballGravity,      // 0.03
			Drag:         SnowballDrag,         // 0.99
			InitialSpeed: SnowballInitialSpeed, // 1.5
		}
	case EnderPearl:
		return ProjectilePhysics{
			Gravity:      EnderPearlGravity,      // 0.03
			Drag:         EnderPearlDrag,         // 0.99
			InitialSpeed: EnderPearlInitialSpeed, // 1.5
		}
	case SplashPotion:
		return ProjectilePhysics{
			Gravity:      SplashPotionGravity,      // 0.05
			Drag:         SplashPotionDrag,         // 0.99
			InitialSpeed: SplashPotionInitialSpeed, // 0.5
		}
	default:
		// Default to snowball physics
		return ProjectilePhysics{
			Gravity:      0.03,
			Drag:         0.99,
			InitialSpeed: 1.5,
		}
	}
}

// TrajectoryPoint represents a point along a projectile's trajectory
type TrajectoryPoint struct {
	Pos  V3   // Position in 3D space
	Vel  V3   // Velocity at this point
	Tick int  // Tick number
	Hit  bool // Whether projectile hit target
}

// SimulateProjectile simulates projectile trajectory for a given pitch, power, and target.
// Returns the landing position and whether it hit the target area.
func SimulateProjectile(pType ProjectileType, pitchRad, powerFactor float64,
	targetHorizontalDist, targetVerticalDist float64) (hitX, hitY float64, hit bool) {

	phys := GetProjectilePhysics(pType)

	// Calculate initial speed based on power factor
	initialSpeed := phys.InitialSpeed * powerFactor

	velY := math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	posX, posY := 0.0, 0.0 // Simplified 2D simulation (horizontal distance, vertical position)

	// Simulate up to 400 ticks (20 seconds)
	for tick := 0; tick < 400; tick++ {
		posX += velXZ
		posY += velY

		velY -= phys.Gravity
		velY *= phys.Drag
		velXZ *= phys.Drag

		// Check if near target height (within 0.5 blocks vertically)
		if posY <= targetVerticalDist && posY >= targetVerticalDist-0.5 {
			// Check if within horizontal range (within 0.5 blocks)
			if math.Abs(posX) >= math.Abs(targetHorizontalDist)-0.5 &&
				math.Abs(posX) <= math.Abs(targetHorizontalDist)+0.5 {
				return math.Abs(posX), posY, true
			}
		}

		// Early termination if trajectory is clearly wrong
		if posY < targetVerticalDist-10 || math.Abs(posX) > math.Abs(targetHorizontalDist)+20 {
			break
		}
	}

	return math.Abs(posX), posY, false
}

// SimulateProjectileTrajectory simulates full 3D projectile trajectory.
// Returns a slice of trajectory points for visualization or collision detection.
func SimulateProjectileTrajectory(pType ProjectileType, origin, velocity V3, maxTicks int) []TrajectoryPoint {
	phys := GetProjectilePhysics(pType)

	pos := origin
	vel := velocity
	trajectory := make([]TrajectoryPoint, 0, maxTicks)

	for tick := 0; tick < maxTicks; tick++ {
		// Update position
		pos.X += vel.X
		pos.Y += vel.Y
		pos.Z += vel.Z

		// Update velocity
		vel.Y -= phys.Gravity
		vel.X *= phys.Drag
		vel.Y *= phys.Drag
		vel.Z *= phys.Drag

		trajectory = append(trajectory, TrajectoryPoint{
			Pos:  pos,
			Vel:  vel,
			Tick: tick,
			Hit:  false,
		})

		// Stop if velocity is negligible
		if math.Abs(vel.X) < 0.001 && math.Abs(vel.Y) < 0.001 && math.Abs(vel.Z) < 0.001 {
			break
		}
	}

	return trajectory
}

// FindOptimalTrajectory searches for the best pitch and power to hit a target.
// Returns the optimal pitch (degrees), power factor, and minimum error.
func FindOptimalTrajectory(pType ProjectileType, horizontalDist, verticalDist float64) (pitch, power, minError float64) {
	const (
		minPitch  = -90.0
		maxPitch  = 90.0
		pitchStep = 0.1
		minPower  = 0.1
		maxPower  = 1.0
		powerStep = 0.05
	)

	bestPitch := 0.0
	bestPower := 1.0
	minError = math.MaxFloat64

	for p := minPower; p <= maxPower; p += powerStep {
		for pitch := minPitch; pitch <= maxPitch; pitch += pitchStep {
			pitchRad := pitch * math.Pi / 180.0

			hitX, _, hit := SimulateProjectile(pType, pitchRad, p, horizontalDist, verticalDist)

			if hit {
				error := math.Abs(hitX - horizontalDist)

				if error < minError {
					minError = error
					bestPitch = pitch
					bestPower = p
				}
			}
		}
	}

	return bestPitch, bestPower, minError
}

// CalculateAiming calculates yaw, pitch, and power needed to hit a target.
// Returns yaw (degrees), pitch (degrees), and power factor (0.0-1.0).
func CalculateAiming(pType ProjectileType, origin, target V3) (yaw, pitch, power float64) {
	// Calculate deltas
	dx := target.X - origin.X
	dy := target.Y - (origin.Y + PlayerEyeHeight) // Adjust for eye height
	dz := target.Z - origin.Z

	// Calculate horizontal distance
	horizontalDist := math.Sqrt(dx*dx + dz*dz)

	// Yaw: horizontal direction
	yaw = math.Atan2(-dx, -dz) * 180 / math.Pi

	// Pitch and power: use trajectory optimization
	pitch, power, _ = FindOptimalTrajectory(pType, horizontalDist, dy)

	return yaw, pitch, power
}

// PredictLandingPosition predicts where a projectile will land given launch parameters.
// Useful for calculating where a thrown ender pearl will teleport the player.
func PredictLandingPosition(pType ProjectileType, origin V3, yaw, pitch, power float64) V3 {
	// Convert angles to radians
	yawRad := yaw * math.Pi / 180
	pitchRad := pitch * math.Pi / 180

	phys := GetProjectilePhysics(pType)
	initialSpeed := phys.InitialSpeed * power

	// Calculate initial velocity components
	velY := math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed
	velX := math.Sin(yawRad) * velXZ
	velZ := -math.Cos(yawRad) * velXZ

	pos := origin
	vel := V3{X: velX, Y: velY, Z: velZ}

	// Simulate until projectile hits ground
	for tick := 0; tick < 400; tick++ {
		pos.X += vel.X
		pos.Y += vel.Y
		pos.Z += vel.Z

		vel.Y -= phys.Gravity
		vel.X *= phys.Drag
		vel.Y *= phys.Drag
		vel.Z *= phys.Drag

		// Simple ground check (Y <= starting Y means landed)
		if pos.Y <= origin.Y && vel.Y < 0 {
			break
		}
	}

	return pos
}
