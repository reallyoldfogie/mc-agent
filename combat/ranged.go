package combat

import (
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

const CrossbowChargeDuration = 1250 * time.Millisecond

// ProjectileWeapon identifies the inventory/projectile family selected by the
// combat controller. It is separate from Weapon, which only describes the
// melee-versus-ranged decision.
type ProjectileWeapon uint8

const (
	Bow ProjectileWeapon = iota
	Crossbow
	Trident
)

// RangedAttackRequest is the transport-independent input to a projectile
// executor. The executor owns inventory selection, aiming, packet sequencing,
// and hit callbacks.
type RangedAttackRequest struct {
	TargetID           int32
	Weapon             ProjectileWeapon
	TargetPosition     models.V3
	TargetVelocity     models.V3
	ProjectileSpeed    float64
	FlightTime         float64
	ChargeDuration     time.Duration
	Priority           int
	CancelOnTargetLoss bool
	// ProjectileCallbacks receive the existing server-authoritative projectile
	// result, including target-position validation, after the shot is tracked.
	ProjectileCallbacks []models.ProjectileHitCallback
}

// Validate rejects requests that cannot be executed safely.
func (r RangedAttackRequest) Validate() error {
	if r.TargetID == 0 {
		return fmt.Errorf("ranged attack: missing target entity")
	}
	if r.Weapon > Trident {
		return fmt.Errorf("ranged attack: unknown weapon %d", r.Weapon)
	}
	if r.ProjectileSpeed <= 0 || math.IsNaN(r.ProjectileSpeed) || math.IsInf(r.ProjectileSpeed, 0) {
		return fmt.Errorf("ranged attack: invalid projectile speed %v", r.ProjectileSpeed)
	}
	if r.FlightTime < 0 || math.IsNaN(r.FlightTime) || math.IsInf(r.FlightTime, 0) {
		return fmt.Errorf("ranged attack: invalid flight time %v", r.FlightTime)
	}
	if r.ChargeDuration < 0 {
		return fmt.Errorf("ranged attack: invalid charge duration %v", r.ChargeDuration)
	}
	return validateFiniteVector(r.TargetPosition, "target position")
}

// LeadPosition predicts where a moving target will be when the projectile
// arrives. The executor remains responsible for ballistic trajectory solving.
func (r RangedAttackRequest) LeadPosition() (models.V3, error) {
	if err := r.Validate(); err != nil {
		return models.V3{}, err
	}
	if err := validateFiniteVector(r.TargetVelocity, "target velocity"); err != nil {
		return models.V3{}, err
	}
	return models.V3{
		X: r.TargetPosition.X + r.TargetVelocity.X*r.FlightTime,
		Y: r.TargetPosition.Y + r.TargetVelocity.Y*r.FlightTime,
		Z: r.TargetPosition.Z + r.TargetVelocity.Z*r.FlightTime,
	}, nil
}

func validateFiniteVector(v models.V3, name string) error {
	if math.IsNaN(v.X) || math.IsInf(v.X, 0) ||
		math.IsNaN(v.Y) || math.IsInf(v.Y, 0) ||
		math.IsNaN(v.Z) || math.IsInf(v.Z, 0) {
		return fmt.Errorf("ranged attack: invalid %s", name)
	}
	return nil
}
