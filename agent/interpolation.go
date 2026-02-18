package agent

import (
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// InterpolateProjectilePosition calculates the position of a projectile at a given time
// using discrete physics simulation with sub-tick interpolation (matching Minecraft's tick-based physics).
// This is used when velocity-based prediction is needed (before server position updates).
//
// Sub-tick interpolation ensures accurate positions even when render loop runs mid-tick.
// For example, at 44ms elapsed (0.88 ticks), we run 0 full ticks and then interpolate
// 0.88 of a tick linearly using velocity.
func (a *agent) InterpolateProjectilePosition(
	projInfo *activeProjectileInfo,
	timeElapsedSeconds float64) models.V3 {

	// Get physics model for this projectile type
	// The model encapsulates the correct physics order and handles all type-specific behavior
	model := physics.GetProjectilePhysicsModel(projInfo.projectileType)

	// Start from spawn position and velocity
	pos := projInfo.spawnPos
	vel := projInfo.spawnVelocity

	// Convert time to ticks (20 ticks per second in Minecraft)
	// This gives us the number of complete ticks and the fractional remainder
	totalTicks := timeElapsedSeconds * 20.0
	targetTick := int(totalTicks)
	subTickFraction := totalTicks - float64(targetTick)

	// Cap at 600 ticks (30 seconds) to avoid excessive computation
	if targetTick > 600 {
		targetTick = 600
		subTickFraction = 0 // Don't interpolate beyond cap
	}

	// Simulate complete ticks, matching Minecraft's discrete physics
	for t := 0; t < targetTick; t++ {
		// Use core physics engine (delegated through model) for consistent simulation
		pos, vel = physics.TickProjectile(pos, vel, model)

		// Check for block collision (early exit optimization)
		if a.checkBlockCollision(pos) {
			// Hit a block, stop simulation
			return pos
		}

		// Early exit if velocity is negligible
		if vel.X*vel.X+vel.Y*vel.Y+vel.Z*vel.Z < 1e-8 {
			break
		}
	}

	// Handle fractional tick using linear interpolation
	// This is the key fix: when less than one full tick has passed, we still move the projectile
	// linearly by its current velocity times the fractional tick time.
	// This ensures smooth motion and prevents projectiles from remaining at spawn position.
	if subTickFraction > 0 && subTickFraction < 1.0 {
		// Apply velocity to position for the fractional tick portion
		pos.X += vel.X * subTickFraction
		pos.Y += vel.Y * subTickFraction
		pos.Z += vel.Z * subTickFraction
	}

	return pos
}
