package models

import "time"

// ProjectileHitType describes what the projectile collided with
type ProjectileHitType int

const (
	ProjectileHitBlock   ProjectileHitType = iota // arrow stuck in block (isInGround=true)
	ProjectileHitEntity                           // projectile collided with entity
	ProjectileHitUnknown                          // non-persistent projectile; can't distinguish block vs entity
)

func (p ProjectileHitType) String() string {
	switch p {
	case ProjectileHitBlock:
		return "HitBlock"
	case ProjectileHitEntity:
		return "HitEntity"
	case ProjectileHitUnknown:
		return "HitUnknown"
	default:
		return "Unknown"
	}
}

// ProjectileHitResult describes how the hit notification was triggered
type ProjectileHitResult int

const (
	ProjectileResultBlock   ProjectileHitResult = iota // hit was confirmed by server (block hit)
	ProjectileResultEntity                              // hit confirmed to be entity hit
	ProjectileResultTimeout                             // hit notification fired due to callback timeout
)

func (p ProjectileHitResult) String() string {
	switch p {
	case ProjectileResultBlock:
		return "Block"
	case ProjectileResultEntity:
		return "Entity"
	case ProjectileResultTimeout:
		return "Timeout"
	default:
		return "Unknown"
	}
}

type ProjectileHitEvent struct {
	ProjectileEntityID int32              // ID of the projectile entity
	ProjectileType     ProjectileType     // Type of projectile (arrow, snowball, etc.)
	HitType            ProjectileHitType  // What was hit (block vs entity)
	Position           V3                 // Last known server-reported position
	FiredAt            time.Time          // When the projectile was fired
	HitAt              time.Time          // When the hit occurred
	HitResult          ProjectileHitResult // How the hit was determined (Block/Entity/Timeout)
	HitEntityID        int32              // ID of entity hit (if HitResult is Entity), 0 otherwise
}

type ProjectileHitCallback func(event ProjectileHitEvent)
