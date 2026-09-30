package combat

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func MaceSupportedVersion(version string) bool {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 4)
	if len(parts) < 2 {
		return false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	if errMajor != nil || errMinor != nil {
		return false
	}
	return major > 1 || (major == 1 && minor >= 21)
}

const (
	MaceMinReach = 1.0
	// Vanilla's PLAYER_ENTITY_INTERACTION_RANGE is 3.0 blocks in 1.21.1.
	// The Mace changes fall damage, not entity interaction reach.
	MaceMaxReach             = 3.0
	MaceAdditionalDamageFall = 1.5
	MaceHeavySmashFall       = 5.0
	MaceKnockbackRange       = 3.5
	MaceKnockbackPower       = 0.7
)

type MaceAttackRequest struct {
	TargetID     int32
	ItemName     string
	Distance     float64
	MinReach     float64
	MaxReach     float64
	FallDistance float64
	Gliding      bool
}

func (r MaceAttackRequest) Validate() error {
	if r.TargetID == 0 {
		return fmt.Errorf("mace attack: missing target entity")
	}
	if ClassifyMeleeItem(r.ItemName) != MaceMeleeWeapon {
		return fmt.Errorf("mace attack: %q is not a mace", r.ItemName)
	}
	if r.MinReach < 0 || r.MaxReach <= 0 || r.MinReach > r.MaxReach {
		return fmt.Errorf("mace attack: invalid reach %.2f..%.2f", r.MinReach, r.MaxReach)
	}
	if r.Distance < r.MinReach || r.Distance > r.MaxReach {
		return fmt.Errorf("mace attack: target distance %.2f outside %.2f..%.2f", r.Distance, r.MinReach, r.MaxReach)
	}
	if math.IsNaN(r.FallDistance) || math.IsInf(r.FallDistance, 0) || r.FallDistance < 0 {
		return fmt.Errorf("mace attack: invalid fall distance %v", r.FallDistance)
	}
	return nil
}

func (r MaceAttackRequest) ShouldDealAdditionalDamage() bool {
	return r.FallDistance > MaceAdditionalDamageFall && !r.Gliding
}

func (r MaceAttackRequest) SmashBonusDamage() float64 {
	if !r.ShouldDealAdditionalDamage() {
		return 0
	}
	fall := r.FallDistance
	if fall <= 3 {
		return 4 * fall
	}
	if fall <= 8 {
		return 12 + 2*(fall-3)
	}
	return 22 + fall - 8
}

func MaceKnockbackStrength(distance, fallDistance float64) float64 {
	if distance < 0 || distance > MaceKnockbackRange {
		return 0
	}
	strength := (MaceKnockbackRange - distance) * MaceKnockbackPower
	if fallDistance > MaceHeavySmashFall {
		strength *= 2
	}
	return strength
}
