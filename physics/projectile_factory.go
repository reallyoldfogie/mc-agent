package physics

import "github.com/reallyoldfogie/mc-agent/models"

// GetProjectilePhysicsModel returns the appropriate physics model for a projectile type
// This single factory function is the only place where projectile types are switched on
// All physics logic is encapsulated in the model implementations
func GetProjectilePhysicsModel(pType models.ProjectileType) models.ProjectilePhysicsModel {
	switch pType {
	// ThrownEntity variants (gravity → drag → position)
	case models.Snowball:
		return SnowballModel
	case models.Egg:
		return EggModel
	case models.EnderPearl:
		return EnderPearlModel
	case models.SplashPotion:
		return SplashPotionModel
	case models.ExperienceBottle:
		return ExperienceBottleModel

	// PersistentProjectileEntity variants (position → drag → gravity)
	case models.Arrow:
		return ArrowModel
	case models.Trident:
		return TridentModel

	// ExplosiveProjectileEntity variants (drag → position with custom accel)
	case models.WindCharge:
		return WindChargeModelInstance

	// Default to snowball physics for unknown types
	default:
		return SnowballModel
	}
}
