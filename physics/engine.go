package physics

import "github.com/reallyoldfogie/mc-agent/models"

// TickProjectile applies one tick of physics to a projectile using the appropriate physics model.
// This delegates to the model's ApplyPhysics() method which handles the correct order for that projectile type.
//
// The physics simulation order varies by projectile type:
// - ThrownEntity (snowballs, eggs, etc.): gravity → drag → position
// - PersistentProjectileEntity (arrows, tridents): position → drag → gravity
// - ExplosiveProjectileEntity (wind charges): drag → position (custom acceleration)
//
// All projectile trajectory code (aiming, interpolation, visualization) uses this function.
//
// Parameters:
//   - pos: current position
//   - vel: current velocity
//   - model: physics model for this projectile type (encapsulates correct order)
//
// Returns:
//   - new position after this tick
//   - new velocity after this tick
func TickProjectile(pos, vel models.V3, model models.ProjectilePhysicsModel) (models.V3, models.V3) {
	// Delegate to the model, which applies physics in the correct order for its type
	return model.ApplyPhysics(pos, vel, false) // false = not in water (for now)
}
