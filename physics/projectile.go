package physics

import (
	"log"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TrajectoryValidator provides access to world state for trajectory validation.
// Implementations check blocks and their collision shapes during trajectory validation.
type TrajectoryValidator interface {
	// GetBlockAt returns the block state ID at the given coordinates.
	// Returns (stateID, loaded). If loaded is false, the chunk isn't available.
	GetBlockAt(x, y, z float64) (blockStateID uint32, loaded bool)

	// GetCollisionBoxes returns the collision shapes for a block at the given position.
	// Returns nil if the block has no collision or if not available.
	GetCollisionBoxes(blockStateID uint32, x, y, z int) []models.AABB

	// IsSolid returns true if the given block state is solid (has collision).
	IsSolid(blockStateID uint32) bool

	// BlockName returns the block name for a given block state ID, or empty string if unknown.
	BlockName(blockStateID uint32) string

	// FullBlockName returns the full block name with properties for a given block state ID, or empty string if unknown.
	FullBlockName(blockStateID uint32) string
}

// ProjectilePhysics defines physics constants for a projectile type
type ProjectilePhysics struct {
	Gravity      float64 // Gravitational acceleration (blocks/tick²)
	Drag         float64 // Air resistance multiplier
	InitialSpeed float64 // Base throw/shoot speed
}

// GetProjectileUpdateOrder returns the physics update order for a projectile type.
// Different projectiles apply physics in different orders based on Minecraft source code:
//   - ThrownEntity (snowballs, eggs, etc.): gravity → drag → position
//   - PersistentProjectileEntity (arrows, tridents): drag → gravity → position
func GetProjectileUpdateOrder(pType models.ProjectileType) UpdateOrder {
	switch pType {
	case models.Arrow, models.Trident:
		// PersistentProjectileEntity: drag → gravity → position
		return OrderDragGravityPosition

	case models.Snowball, models.Egg, models.EnderPearl, models.SplashPotion,
		models.ExperienceBottle, models.WindCharge:
		// ThrownEntity and similar: gravity → drag → position
		return OrderGravityDragMove

	default:
		// Default to ThrownEntity order
		return OrderGravityDragMove
	}
}

// GetProjectilePhysics returns physics constants for a projectile type
func GetProjectilePhysics(pType models.ProjectileType) ProjectilePhysics {
	switch pType {
	case models.Arrow:
		return ProjectilePhysics{
			Gravity:      ArrowGravity,      // 0.05
			Drag:         ArrowDrag,         // 0.99
			InitialSpeed: ArrowInitialSpeed, // 3.0
		}
	case models.Snowball:
		return ProjectilePhysics{
			Gravity:      SnowballGravity,      // 0.03
			Drag:         SnowballDrag,         // 0.99
			InitialSpeed: SnowballInitialSpeed, // 1.5
		}
	case models.Egg:
		return ProjectilePhysics{
			Gravity:      EggGravity,      // 0.03
			Drag:         EggDrag,         // 0.99
			InitialSpeed: EggInitialSpeed, // 1.5
		}
	case models.EnderPearl:
		return ProjectilePhysics{
			Gravity:      EnderPearlGravity,      // 0.03
			Drag:         EnderPearlDrag,         // 0.99
			InitialSpeed: EnderPearlInitialSpeed, // 1.5
		}
	case models.SplashPotion:
		return ProjectilePhysics{
			Gravity:      SplashPotionGravity,      // 0.05
			Drag:         SplashPotionDrag,         // 0.99
			InitialSpeed: SplashPotionInitialSpeed, // 0.5
		}
	case models.ExperienceBottle:
		return ProjectilePhysics{
			Gravity:      ExperienceBottleGravity,      // 0.07
			Drag:         ExperienceBottleDrag,         // 0.99
			InitialSpeed: ExperienceBottleInitialSpeed, // 0.7
		}
	case models.WindCharge:
		return ProjectilePhysics{
			Gravity:      WindChargeGravity,      // 0.02
			Drag:         WindChargeDrag,         // 0.99
			InitialSpeed: WindChargeInitialSpeed, // 1.5
		}
	case models.Trident:
		// Trident has similar physics to arrows
		return ProjectilePhysics{
			Gravity:      ArrowGravity,      // 0.05
			Drag:         ArrowDrag,         // 0.99
			InitialSpeed: ArrowInitialSpeed, // 3.0
		}
	default:
		// Default to snowball physics for unknown types
		return ProjectilePhysics{
			Gravity:      SnowballGravity,      // 0.03
			Drag:         SnowballDrag,         // 0.99
			InitialSpeed: SnowballInitialSpeed, // 1.5
		}
	}
}

// SimulateProjectile simulates projectile trajectory for a given pitch, power, and target.
// Returns the landing position and whether it hit the target area.
func SimulateProjectile(pType models.ProjectileType, pitchRad, powerFactor float64,
	targetHorizontalDist, targetVerticalDist float64) (hitX, hitY float64, hit bool) {

	phys := GetProjectilePhysics(pType)

	// Calculate initial speed based on power factor
	initialSpeed := phys.InitialSpeed * powerFactor

	// Minecraft convention: negative pitch = up, positive pitch = down
	// Therefore: vY = -sin(pitch)
	velY := -math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed

	posX, posY := 0.0, 0.0 // Simplified 2D simulation (horizontal distance, vertical position)

	// Simulate up to 400 ticks (20 seconds)
	for range 400 {
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
func SimulateProjectileTrajectory(pType models.ProjectileType, origin, velocity models.V3, maxTicks int) []models.TrajectoryPoint {
	// Get the physics model for this projectile type
	// The model encapsulates the correct physics order and handles all type-specific behavior
	model := GetProjectilePhysicsModel(pType)

	pos := origin
	vel := velocity
	trajectory := make([]models.TrajectoryPoint, 0, maxTicks)

	for tick := range maxTicks {
		// Use core physics engine (delegated through model) for consistent trajectory calculation
		pos, vel = TickProjectile(pos, vel, model)

		trajectory = append(trajectory, models.TrajectoryPoint{
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

// TrimTrajectoryToTarget trims a trajectory to only include points up to the target hit point.
// Takes the target in local space (relative to trajectory origin) to match the trajectory coordinate system.
// Uses segment-based intersection (linear interpolation) to detect hits that occur between
// discrete trajectory samples, matching the detection method used in FindOptimalAiming.
// Returns a new trajectory containing only points from start up to (and including) the hit point.
func TrimTrajectoryToTarget(trajectory []models.TrajectoryPoint, target models.V3) []models.TrajectoryPoint {
	if len(trajectory) == 0 {
		return trajectory
	}

	// Define target block bounds
	const blockRadius = 0.5
	boxMin := models.V3{X: target.X - blockRadius, Y: target.Y - blockRadius, Z: target.Z - blockRadius}
	boxMax := models.V3{X: target.X + blockRadius, Y: target.Y + blockRadius, Z: target.Z + blockRadius}

	// Find the first segment that intersects the target block
	for i := range trajectory {
		// Check if current point is in the box (endpoint hit)
		if isPointInBox(trajectory[i].Pos, boxMin, boxMax) {
			// For upward targets, skip if arrow is still rising
			if target.Y >= 0 && i > 0 && trajectory[i].Vel.Y > 0 {
				continue
			}

			// Found hit at this point, trim to here (inclusive)
			trimmed := make([]models.TrajectoryPoint, i+1)
			copy(trimmed, trajectory[:i+1])
			return trimmed
		}

		// Check if segment to next point intersects the box (segment hit)
		if i > 0 {
			if trajectorySegmentIntersectsBox(trajectory[i-1].Pos, trajectory[i].Pos, boxMin, boxMax) {
				// For upward targets, verify we're on the downward pass
				if target.Y >= 0 && trajectory[i].Vel.Y > 0 {
					continue
				}

				// Found hit on this segment, trim to current point (inclusive)
				trimmed := make([]models.TrajectoryPoint, i+1)
				copy(trimmed, trajectory[:i+1])
				return trimmed
			}
		}
	}

	// No hit found, return empty trajectory
	return []models.TrajectoryPoint{}
}

// trajectorySegmentIntersectsBox checks if a line segment between two trajectory points
// intersects the target block's bounding box. Uses linear interpolation to detect hits
// that occur between discrete trajectory samples.
func trajectorySegmentIntersectsBox(p1, p2 models.V3, boxMin, boxMax models.V3) bool {
	// Check if either endpoint is in the box
	if isPointInBox(p1, boxMin, boxMax) || isPointInBox(p2, boxMin, boxMax) {
		return true
	}

	// Check if segment intersects box by testing each axis
	// For each axis, check if the segment's range overlaps the box's range
	return segmentAxisIntersect(p1.X, p2.X, boxMin.X, boxMax.X) &&
		segmentAxisIntersect(p1.Y, p2.Y, boxMin.Y, boxMax.Y) &&
		segmentAxisIntersect(p1.Z, p2.Z, boxMin.Z, boxMax.Z)
}

// isPointInBox checks if a point is within a 3D bounding box
func isPointInBox(p, boxMin, boxMax models.V3) bool {
	return p.X >= boxMin.X && p.X <= boxMax.X &&
		p.Y >= boxMin.Y && p.Y <= boxMax.Y &&
		p.Z >= boxMin.Z && p.Z <= boxMax.Z
}

// segmentAxisIntersect checks if a 1D line segment intersects a 1D box range
func segmentAxisIntersect(p1, p2, boxMin, boxMax float64) bool {
	// Get the range of the segment
	segMin := math.Min(p1, p2)
	segMax := math.Max(p1, p2)

	// Check if segment range overlaps box range
	return segMin <= boxMax && segMax >= boxMin
}

// FindOptimalAiming searches for the best pitch with full power to hit a target.
// Accepts full 3D source and target coordinates for proper 3D validation.
// Also returns the 3D trajectory points of the selected solution (trimmed to hit point).
// Prefers low-angle shots (below 45 degrees) for reliability and realism.
// Returns optimal pitch (degrees), power (always 1.0), minimum error, and trajectory points (trimmed to target or empty if impossible).
// DEPRECATED: Use FindOptimalAiming (this one doesn't work anymore) Keeping it around for reference and testing against the new implementation.
func FindOptimalAiming_OLD(pType models.ProjectileType, origin, target models.V3) (pitch, power, minError float64, trajectory []models.TrajectoryPoint) {
	// Calculate deltas from source to target
	dx := target.X - origin.X
	dy := target.Y - origin.Y
	dz := target.Z - origin.Z
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	verticalDist := dy

	log.Printf("[FindOptimalAiming] origin=(%.2f, %.2f, %.2f), target=(%.2f, %.2f, %.2f), horizontalDist=%.2f, verticalDist=%.2f", origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z, horizontalDist, verticalDist)
	const (
		maxPower = 1.0 // Always use full power
		epsilon  = 0.5 // Tolerance for considering hits equivalent
	)

	// Determine pitch range based on target location
	// Unified Minecraft convention: negative pitch = up, positive pitch = down
	// For any target, we search across a range that includes both upward and downward angles
	var minPitch, maxPitch float64
	if verticalDist >= 0 {
		// Target at same height or above - need upward arc (negative pitch)
		// to reach the target with gravity helping to return to level
		minPitch = -60.0
		maxPitch = 20.0 // Mostly upward angles, some downward for direct shots
	} else if verticalDist < -5 {
		// Target significantly below - can use either upward arc or direct shot
		minPitch = -60.0
		maxPitch = 60.0
	} else {
		// Target moderately below (same level to -5 blocks) - both upward and downward work
		minPitch = -60.0
		maxPitch = 60.0
	}

	const pitchStep = 0.01

	bestPitch := 0.0
	bestPower := maxPower // Use full power
	minError = math.MaxFloat64
	var bestTrajectory []models.TrajectoryPoint
	var foundLowAngleSolution bool

	// Get projectile physics for calculating velocity
	projectilePhys := GetProjectilePhysics(pType)

	for pitch := minPitch; pitch <= maxPitch; pitch += pitchStep {
		pitchRad := pitch * math.Pi / 180.0

		// Calculate velocity from pitch (assuming 0 yaw for this 2D search)
		initialSpeed := projectilePhys.InitialSpeed * maxPower
		// Minecraft convention: negative pitch = up, positive pitch = down
		// Therefore: vY = -sin(pitch)
		velY := -math.Sin(pitchRad) * initialSpeed
		velXZ := math.Cos(pitchRad) * initialSpeed

		// Simulate full 3D trajectory starting from origin with calculated velocity
		// Use Z-axis as forward direction (yaw=0), X=0, Y=0 at origin
		velocity := models.V3{X: 0, Y: velY, Z: velXZ}
		traj := SimulateProjectileTrajectory(pType, models.V3{}, velocity, 400)

		// Check if this trajectory hits the target
		// Define target block bounds in local space
		const blockRadius = 0.5
		boxMin := models.V3{X: -blockRadius, Y: verticalDist - blockRadius, Z: horizontalDist - blockRadius}
		boxMax := models.V3{X: blockRadius, Y: verticalDist + blockRadius, Z: horizontalDist + blockRadius}

		hit := false
		var hitTraj []models.TrajectoryPoint

		for i, point := range traj {
			// For upward targets (or level targets), skip if arrow is still rising
			// The arrow passes through target height twice: once on the way UP (invalid),
			// and once on the way DOWN (valid). We only want the downward pass.
			if verticalDist >= 0 && i > 0 && point.Vel.Y > 0 {
				continue // Skip rising arrows for upward/level targets
			}

			// Use linear interpolation: check if segment from previous point to current point
			// intersects the target block bounds. This catches trajectories that pass through
			// the block between discrete sample points.
			if i > 0 {
				prevPoint := traj[i-1]
				// Check segment intersection using linear interpolation
				if trajectorySegmentIntersectsBox(prevPoint.Pos, point.Pos, boxMin, boxMax) {
					// Also verify this is the downward pass for upward targets
					if verticalDist >= 0 {
						// For upward targets, only accept if we're on the downward part (velocity became negative)
						if point.Vel.Y <= 0 {
							hit = true
							hitTraj = traj
							break
						}
					} else {
						// For targets below or at same level, any intersection is valid
						hit = true
						hitTraj = traj
						break
					}
				}
			}
		}

		if hit {
			// Check if this is a low-angle shot (preferred for reliability)
			isLowAngle := math.Abs(pitch) <= 45.0

			// Accept first valid hit, prefer low-angle shots
			if minError == math.MaxFloat64 {
				// First hit found
				minError = 0 // Successful hit
				bestPitch = pitch
				bestTrajectory = hitTraj
				foundLowAngleSolution = isLowAngle
			} else if isLowAngle && !foundLowAngleSolution {
				// Found a low-angle solution when we only had high-angle before, prefer it
				bestPitch = pitch
				bestTrajectory = hitTraj
				foundLowAngleSolution = true
			}
			// If we already have a low-angle solution, we're done (prefer lower angles which come first in the loop)
			if foundLowAngleSolution {
				break
			}
		}
	}

	// Check if we found a valid hit
	if minError == math.MaxFloat64 {
		// No valid hit found - target is unreachable at this distance/elevation
		// Log detailed info about the search space
		log.Printf("[FindOptimalAiming] UNREACHABLE: No valid trajectory found for horizontalDist=%.2f, verticalDist=%.2f",
			horizontalDist, verticalDist)
		log.Printf("[FindOptimalAiming] Target bounds: X=[%.2f,%.2f], Y=[%.2f,%.2f], Z=[%.2f,%.2f]",
			-0.5, 0.5, verticalDist-0.5, verticalDist+0.5, horizontalDist-0.5, horizontalDist+0.5)
		log.Printf("[FindOptimalAiming] Pitch range tested: [%.1f, %.1f] with step %.1f",
			minPitch, maxPitch, pitchStep)
		log.Printf("[FindOptimalAiming] Projectile physics: speed=%.3f, gravity=%.3f, drag=%.3f",
			projectilePhys.InitialSpeed, projectilePhys.Gravity, projectilePhys.Drag)
		return bestPitch, bestPower, minError, []models.TrajectoryPoint{} // Return empty trajectory
	}

	// Trim trajectory to only show up to the hit point (not entire 400-tick path)
	// Target in local space: origin at (0,0,0), forward direction is Z-axis
	localTarget := models.V3{X: 0, Y: verticalDist, Z: horizontalDist}
	trimmedTrajectory := TrimTrajectoryToTarget(bestTrajectory, localTarget)

	log.Printf("[FindOptimalAiming] Final: pitch=%.1f, power=%.2f, minError=%.2f, models.TrajectoryPoints=%d (trimmed from %d)",
		bestPitch, bestPower, minError, len(trimmedTrajectory), len(bestTrajectory))

	return bestPitch, bestPower, minError, trimmedTrajectory
}

// GetProjectileProps converts a ProjectileType to ProjectileProps for use with the aiming implementation.
// Note: The Type field is set for model lookup;
func GetProjectileProps(pType models.ProjectileType) ProjectileProps {
	phys := GetProjectilePhysics(pType)
	// Splash potions use different ticking order per Minecraft wiki:
	// Acceleration (gravity), Drag, Position
	// Other projectiles use: Position, Drag, Gravity (OrderMoveDragGravity)
	order := OrderMoveDragGravity
	if pType == models.SplashPotion {
		order = OrderGravityDragMove
	}
	return ProjectileProps{
		Type:     pType,
		Speed:    phys.InitialSpeed,
		Drag:     phys.Drag,
		Gravity:  phys.Gravity,
		MaxTicks: 400,
		Order:    order,
	}
}

// YawForStartTarget returns the Minecraft yaw (degrees) to face from start toward target.
// This converts from the solver's convention yaw_s = atan2(dZ, dX) to Minecraft's yaw degrees.
// Empirically, yaw_mc_deg = yaw_s_deg - 90.
func YawForStartTarget(start, target models.V3) float64 {
	d := target.Sub(start)
	yawSolver := math.Atan2(d.Z, d.X) // radians
	yawMC := rad2deg(yawSolver) - 90.0
	return yawMC
}

// FindOptimalAiming calculates and returns optimal pitch (degrees), power, minimum error, and trajectory points.
func FindOptimalAiming(pType models.ProjectileType, origin, target models.V3) (pitch, power, minError float64, trajectory []models.TrajectoryPoint) {
	props := GetProjectileProps(pType)
	dx := target.X - origin.X
	dz := target.Z - origin.Z
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	verticalDist := target.Y - origin.Y
	log.Printf("[FindOptimalAiming] SolveAim input: origin=(%.2f,%.2f,%.2f), target=(%.2f,%.2f,%.2f), horizontalDist=%.2f, verticalDist=%.2f",
		origin.X, origin.Y, origin.Z, target.X, target.Y, target.Z, horizontalDist, verticalDist)

	lowSolution, err := SolveAim(origin, target, props, false) // preferHighArc = false for low-angle preference
	if err != nil {
		// Target unreachable
		log.Printf("[FindOptimalAiming] (low) New implementation: target unreachable")
		return 0, 1.0, math.MaxFloat64, []models.TrajectoryPoint{}
	}

	highSolution, err := SolveAim(origin, target, props, true)
	if err != nil {
		// Target unreachable
		log.Printf("[FindOptimalAiming] (high) New implementation: target unreachable")
		return 0, 1.0, math.MaxFloat64, []models.TrajectoryPoint{}
	}

	// Evaluate both arc solutions - both are physically valid
	// Prefer low arc for reliability (more predictable, less likely to be blocked)
	var highTraj, lowTraj []models.TrajectoryPoint
	var highPitch, highErr, lowPitch, lowErr float64

	// Evaluate high arc
	highPitch = rad2deg(highSolution.PitchRad)
	highErr = highSolution.ErrorY
	adjustedV0 := highSolution.V0
	highTraj = SimulateProjectileTrajectory(pType, origin, adjustedV0, props.MaxTicks)
	if len(highTraj) > 0 && highSolution.Tick >= 0 && highSolution.Tick < len(highTraj) {
		endIndex := min(highSolution.Tick+5, len(highTraj))
		trimmed := make([]models.TrajectoryPoint, endIndex)
		copy(trimmed, highTraj[:endIndex])
		highTraj = trimmed
	}
	log.Printf("[FindOptimalAiming] High arc: pitch=%.2f°, errorY=%.4f, points=%d", highPitch, highErr, len(highTraj))

	// Evaluate low arc
	lowPitch = rad2deg(lowSolution.PitchRad)
	lowErr = lowSolution.ErrorY
	adjustedV0 = lowSolution.V0
	lowTraj = SimulateProjectileTrajectory(pType, origin, adjustedV0, props.MaxTicks)
	if len(lowTraj) > 0 && lowSolution.Tick >= 0 && lowSolution.Tick < len(lowTraj) {
		endIndex := min(lowSolution.Tick+5, len(lowTraj))
		trimmed := make([]models.TrajectoryPoint, endIndex)
		copy(trimmed, lowTraj[:endIndex])
		lowTraj = trimmed
	}
	log.Printf("[FindOptimalAiming] Low arc: pitch=%.2f°, errorY=%.4f, points=%d", lowPitch, lowErr, len(lowTraj))

	// Prefer low arc solution (more reliable)
	pitch = lowPitch
	power = 1.0
	minError = lowErr
	trajectory = lowTraj
	log.Printf("[FindOptimalAiming] Selected LOW arc: pitch=%.2f°, errorY=%.4f", pitch, minError)
	return pitch, power, minError, trajectory
}

// CalculateAiming calculates yaw, pitch, and power needed to hit a target.
// Returns yaw (degrees), pitch (degrees), and power factor (0.0-1.0).
// DEPRECATED: Use SolvAim. Keeping around for reference.
func CalculateAiming_OLD(pType models.ProjectileType, origin, target models.V3) (yaw, pitch, power float64) {
	// Calculate deltas
	dx := target.X - origin.X
	dz := target.Z - origin.Z

	// Create adjusted origin for arrow spawn point (eye height)
	adjustedOrigin := models.V3{X: origin.X, Y: origin.Y + PlayerEyeHeight, Z: origin.Z}
	adjustedTarget := models.V3{X: target.X, Y: target.Y, Z: target.Z}

	// Calculate yaw for horizontal rotation
	// NOTE: Trajectory is simulated in local space with Z=forward, X=0
	// Yaw formula must convert from world delta to firing direction
	// atan2(-dx, -dz) accounts for the coordinate system rotation from world to Minecraft convention
	yaw = math.Atan2(-dx, -dz) * 180 / math.Pi

	// Pitch and power: use trajectory optimization
	pitch, power, _, _ = FindOptimalAiming(pType, adjustedOrigin, adjustedTarget)

	return yaw, pitch, power
}

// PredictLandingPosition predicts where a projectile will land given launch parameters.
// Useful for calculating where a thrown ender pearl will teleport the player.
func PredictLandingPosition(pType models.ProjectileType, origin models.V3, yaw, pitch, power float64) models.V3 {
	// Convert angles to radians
	yawRad := yaw * math.Pi / 180
	pitchRad := pitch * math.Pi / 180

	phys := GetProjectilePhysics(pType)
	initialSpeed := phys.InitialSpeed * power

	// Calculate initial velocity components
	// Minecraft convention: negative pitch = up, positive pitch = down
	// Therefore: vY = -sin(pitch)
	velY := -math.Sin(pitchRad) * initialSpeed
	velXZ := math.Cos(pitchRad) * initialSpeed
	velX := math.Sin(yawRad) * velXZ
	velZ := -math.Cos(yawRad) * velXZ

	pos := origin
	vel := models.V3{X: velX, Y: velY, Z: velZ}

	// Simulate until projectile hits ground
	for range 400 {
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

// ValidateTrajectory checks if a trajectory path is clear of solid blocks.
// Validates each point against actual collision shapes for accuracy.
// Returns (clear, hitPosition). If clear is true, path is unobstructed or hits target.
// If clear is false, hitPosition points to the first collision encountered.
//
// Accounts for:
// - Block collision shapes (not all blocks are full 1x1x1)
// - Slabs, stairs, trapdoors, fences, etc.
// - Small projectile radius (~0.25 blocks)
// - If a target is provided, validates only up to hitting the target block
// - Once the trajectory hits the target block, it's considered valid
func ValidateTrajectory(trajectory []models.TrajectoryPoint, validator TrajectoryValidator, targetOpt ...models.V3) (clear bool, hitPos *models.V3, blockName string) {
	if validator == nil || len(trajectory) == 0 {
		return true, nil, "" // Can't validate without validator or trajectory
	}

	var target *models.V3
	var targetBlockX, targetBlockY, targetBlockZ int
	if len(targetOpt) > 0 {
		target = &targetOpt[0]
		targetBlockX = int(math.Floor(target.X))
		targetBlockY = int(math.Floor(target.Y))
		targetBlockZ = int(math.Floor(target.Z))
	}

	const (
		sampleRate       = 1    // Check every N ticks
		projectileRadius = 0.25 // Approximate projectile collision radius
	)

	// Projectile collision box (simplified as sphere -> AABB)
	projectileAABB := func(pos models.V3) models.AABB {
		return models.NewAABB(
			pos.X-projectileRadius, pos.Y-projectileRadius, pos.Z-projectileRadius,
			pos.X+projectileRadius, pos.Y+projectileRadius, pos.Z+projectileRadius,
		)
	}

	// Check each trajectory point for collisions
	for i := 0; i < len(trajectory); i += sampleRate {
		point := trajectory[i]
		projBox := projectileAABB(point.Pos)

		// Get block coordinates
		blockX := int(math.Floor(point.Pos.X))
		blockY := int(math.Floor(point.Pos.Y))
		blockZ := int(math.Floor(point.Pos.Z))

		// Check nearby blocks (projectile might overlap multiple blocks)
		// Check 3x3x3 cube around projectile
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for dz := -1; dz <= 1; dz++ {
					checkX := blockX + dx
					checkY := blockY + dy
					checkZ := blockZ + dz

					// Get block at this position
					blockPosX := float64(checkX)
					blockPosY := float64(checkY)
					blockPosZ := float64(checkZ)
					stateID, loaded := validator.GetBlockAt(blockPosX, blockPosY, blockPosZ)

					// If chunk not loaded, assume clear (can't determine)
					if !loaded {
						continue
					}

					// Air blocks (stateID 0) are always clear
					if stateID == 0 {
						continue
					}

					// Get collision boxes for this block
					collisionBoxes := validator.GetCollisionBoxes(stateID, checkX, checkY, checkZ)
					if len(collisionBoxes) == 0 {
						// No collision shapes (block is passable or non-solid)
						continue
					}

					blockName := validator.FullBlockName(stateID)

					// If this block is at the target location, trajectory is valid!
					if target != nil && checkX == targetBlockX && checkY == targetBlockY && checkZ == targetBlockZ {
						// Hit the target block successfully
						log.Printf("[ValidateTrajectory] Trajectory hits target at (%.2f, %.2f, %.2f), tick %d",
							point.Pos.X, point.Pos.Y, point.Pos.Z, point.Tick)
						return true, nil, "" // Success - trajectory hits target
					}

					// Check if projectile overlaps any collision box
					for _, collisionBox := range collisionBoxes {
						if projBox.Intersects(collisionBox) {
							// Collision detected with non-target block
							pos := point.Pos
							log.Printf("[ValidateTrajectory] Trajectory blocked at (%.2f, %.2f, %.2f), tick %d, block at (%d, %d, %d), stateID %d",
								pos.X, pos.Y, pos.Z, point.Tick, checkX, checkY, checkZ, stateID)
							return false, &pos, blockName
						}
					}
				}
			}
		}
	}

	// No obstacles found - trajectory is clear
	return true, nil, ""
}

// TrajectoryPassesThroughRadius returns true if any segment of trajectory comes within
// radius blocks of target. Uses closest-point-on-segment for accuracy.
func TrajectoryPassesThroughRadius(trajectory []models.TrajectoryPoint, target models.V3, radius float64) bool {
	radiusSq := radius * radius
	for i := 1; i < len(trajectory); i++ {
		if closestDistSqToSegment(trajectory[i-1].Pos, trajectory[i].Pos, target) <= radiusSq {
			return true
		}
	}
	return false
}

// closestDistSqToSegment returns the squared distance from point q to the closest point on
// the line segment from segStart to segEnd.
func closestDistSqToSegment(segStart, segEnd, point models.V3) float64 {
	// Vector from segment start to end
	segDeltaX := segEnd.X - segStart.X
	segDeltaY := segEnd.Y - segStart.Y
	segDeltaZ := segEnd.Z - segStart.Z
	segLengthSq := segDeltaX*segDeltaX + segDeltaY*segDeltaY + segDeltaZ*segDeltaZ

	if segLengthSq == 0 {
		// Segment start and end are the same point
		diff := point.Sub(segStart)
		return diff.X*diff.X + diff.Y*diff.Y + diff.Z*diff.Z
	}

	// Project point onto the line containing the segment
	projectionParam := ((point.X-segStart.X)*segDeltaX + (point.Y-segStart.Y)*segDeltaY + (point.Z-segStart.Z)*segDeltaZ) / segLengthSq
	// Clamp parameter to [0, 1] to stay on the segment
	if projectionParam < 0 {
		projectionParam = 0
	} else if projectionParam > 1 {
		projectionParam = 1
	}

	// Compute closest point on segment
	closestX := segStart.X + projectionParam*segDeltaX
	closestY := segStart.Y + projectionParam*segDeltaY
	closestZ := segStart.Z + projectionParam*segDeltaZ

	// Return squared distance from point to closest point
	distX := point.X - closestX
	distY := point.Y - closestY
	distZ := point.Z - closestZ
	return distX*distX + distY*distY + distZ*distZ
}
