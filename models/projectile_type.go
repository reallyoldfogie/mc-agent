package models

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
	ExperienceBottle
	WindCharge
)

func (pt ProjectileType) String() string {
	switch pt {
	case Arrow:
		return "Arrow"
	case Snowball:
		return "Snowball"
	case Egg:
		return "Egg"
	case EnderPearl:
		return "EnderPearl"
	case SplashPotion:
		return "SplashPotion"
	case Trident:
		return "Trident"
	case FishingBobber:
		return "FishingBobber"
	case ExperienceBottle:
		return "ExperienceBottle"
	case WindCharge:
		return "WindCharge"
	default:
		return "UnknownProjectileType"
	}
}

func (pt ProjectileType) GetID() string {
	switch pt {
	case Arrow:
		return "arrow"
	case Snowball:
		return "snowball"
	case Egg:
		return "egg"
	case EnderPearl:
		return "ender_pearl"
	case SplashPotion:
		return "splash_potion"
	case Trident:
		return "trident"
	case FishingBobber:
		return "fishing_bobber"
	case ExperienceBottle:
		return "experience_bottle"
	case WindCharge:
		return "wind_charge"
	default:
		return ""
	}
}

// IsPersistent returns true if this projectile type uses persistent projectile mechanics
// (i.e., the server sets isInGround=true on block collision rather than immediately removing)
func (pt ProjectileType) IsPersistent() bool {
	return pt == Arrow || pt == Trident
}
