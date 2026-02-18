package models

import "time"

type ProjectileHitType int

const (
	ProjectileHitBlock   ProjectileHitType = iota // arrow stuck in block (isInGround=true)
	ProjectileHitEntity                           // projectile removed while still in flight
	ProjectileHitUnknown                          // non-persistent projectile; can't distinguish
	ProjectileHitTimeout                          // projectile left loaded area / grace expired
)

func (p ProjectileHitType) String() string {
	switch p {
	case ProjectileHitBlock:
		return "HitBlock"
	case ProjectileHitEntity:
		return "HitEntity"
	case ProjectileHitUnknown:
		return "HitUnknown"
	case ProjectileHitTimeout:
		return "HitTimeout"
	default:
		return "Unknown"
	}
}

type ProjectileHitEvent struct {
	ProjectileEntityID int32
	ProjectileType     ProjectileType
	HitType            ProjectileHitType
	Position           V3 // last known server-reported position
	FiredAt            time.Time
	HitAt              time.Time
}

type ProjectileHitCallback func(event ProjectileHitEvent)
