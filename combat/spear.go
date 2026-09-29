package combat

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// SpearSupportedVersion reports whether a Java version includes spear
// semantics. Pre-release suffixes are accepted at the numeric boundary.
func SpearSupportedVersion(version string) bool {
	parts := strings.SplitN(strings.TrimSpace(version), ".", 4)
	if len(parts) < 2 {
		return false
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	patch := 0
	errPatch := error(nil)
	patchText := "0"
	if len(parts) >= 3 {
		patchText = parts[2]
	}
	if dash := strings.IndexByte(patchText, '-'); dash >= 0 {
		patchText = patchText[:dash]
	}
	patch, errPatch = strconv.Atoi(patchText)
	if errMajor != nil || errMinor != nil || errPatch != nil {
		return false
	}
	return major > 1 || (major == 1 && (minor > 21 || (minor == 21 && patch >= 11)))
}

// MeleeWeaponKind identifies the item-specific melee behavior required by the
// executor. A spear cannot be reduced to a normal AttackEntity packet because
// its charge attack is driven by held secondary-use input.
type MeleeWeaponKind uint8

const (
	UnknownMeleeWeapon MeleeWeaponKind = iota
	StandardMeleeWeapon
	SpearMeleeWeapon
)

// SpearAttackMode is the two spear actions introduced in Java 1.21.11.
type SpearAttackMode uint8

const (
	SpearJab SpearAttackMode = iota
	SpearCharge
)

// SpearChargeStage is the server-visible phase of a held spear charge.
type SpearChargeStage uint8

const (
	SpearEngaged SpearChargeStage = iota
	SpearTired
	SpearDisengaged
)

// SpearChargeProfile supplies item/version-specific stage timing. The combat
// package does not assume that every spear material has identical timings.
type SpearChargeProfile struct {
	EngagedDuration time.Duration
	TiredDuration   time.Duration
	ContactCooldown time.Duration
}

// SpearContactOutcome describes effects the server may apply at a charge
// contact stage.
type SpearContactOutcome struct {
	Damage    bool
	Knockback bool
	Dismount  bool
}

// SpearJabProfile supplies material-specific Jab timing and the maximum
// number of targets the executor may send in one Jab decision.
type SpearJabProfile struct {
	Cooldown   time.Duration
	MaxTargets int
}

func (p SpearJabProfile) CanAttack(now, lastAttack time.Time) bool {
	return p.Cooldown >= 0 && (lastAttack.IsZero() || now.Sub(lastAttack) >= p.Cooldown)
}

// SelectTargets preserves the combat rank order while applying the spear's
// multi-target limit. Range/visibility filtering has already happened in the
// target adapter.
func (p SpearJabProfile) SelectTargets(targets []Target) []Target {
	if p.MaxTargets <= 0 || len(targets) <= p.MaxTargets {
		return targets
	}
	return targets[:p.MaxTargets]
}

// StageAt returns the stage reached after holding a spear for elapsed time.
func (p SpearChargeProfile) StageAt(elapsed time.Duration) SpearChargeStage {
	if elapsed < p.EngagedDuration {
		return SpearEngaged
	}
	if elapsed < p.EngagedDuration+p.TiredDuration {
		return SpearTired
	}
	return SpearDisengaged
}

// CanContact reports whether a spear may damage the same contact again.
func (p SpearChargeProfile) CanContact(now, lastContact time.Time) bool {
	return lastContact.IsZero() || now.Sub(lastContact) >= p.ContactCooldown
}

func ContactOutcome(stage SpearChargeStage) SpearContactOutcome {
	switch stage {
	case SpearEngaged:
		return SpearContactOutcome{Damage: true, Knockback: true, Dismount: true}
	case SpearTired:
		return SpearContactOutcome{Damage: true, Knockback: true}
	default:
		return SpearContactOutcome{Damage: true}
	}
}

// SpearAttackRequest carries the item-component-derived range instead of
// baking 1.21.11 spear material values into the transport-independent layer.
type SpearAttackRequest struct {
	TargetID       int32
	ItemName       string
	Mode           SpearAttackMode
	Distance       float64
	MinReach       float64
	MaxReach       float64
	TargetPosition models.V3
	ViewAlignment  float64
	RelativeSpeed  float64
	MinSpeed       float64
	MinAlignment   float64
	HoldDuration   time.Duration
	Profile        SpearChargeProfile
}

func (r SpearAttackRequest) Validate() error {
	if r.TargetID == 0 {
		return fmt.Errorf("spear attack: missing target entity")
	}
	if ClassifyMeleeItem(r.ItemName) != SpearMeleeWeapon {
		return fmt.Errorf("spear attack: %q is not a spear", r.ItemName)
	}
	if r.Mode > SpearCharge {
		return fmt.Errorf("spear attack: unknown mode %d", r.Mode)
	}
	if r.MinReach < 0 || r.MaxReach <= 0 || r.MinReach > r.MaxReach {
		return fmt.Errorf("spear attack: invalid reach %.2f..%.2f", r.MinReach, r.MaxReach)
	}
	if r.Distance < r.MinReach || r.Distance > r.MaxReach {
		return fmt.Errorf("spear attack: target distance %.2f outside %.2f..%.2f", r.Distance, r.MinReach, r.MaxReach)
	}
	if math.IsNaN(r.ViewAlignment) || math.IsInf(r.ViewAlignment, 0) ||
		math.IsNaN(r.RelativeSpeed) || math.IsInf(r.RelativeSpeed, 0) ||
		math.IsNaN(r.MinSpeed) || math.IsInf(r.MinSpeed, 0) ||
		math.IsNaN(r.MinAlignment) || math.IsInf(r.MinAlignment, 0) {
		return fmt.Errorf("spear attack: invalid motion inputs")
	}
	if r.MinSpeed < 0 || r.MinAlignment < -1 || r.MinAlignment > 1 {
		return fmt.Errorf("spear attack: invalid charge thresholds")
	}
	if r.Mode == SpearCharge && r.HoldDuration <= 0 {
		return fmt.Errorf("spear attack: charge requires a positive hold duration")
	}
	if r.Mode == SpearCharge && (r.Profile.EngagedDuration <= 0 || r.Profile.TiredDuration <= 0 || r.Profile.ContactCooldown < 0) {
		return fmt.Errorf("spear attack: invalid charge profile")
	}
	return nil
}

// ChargeEligible applies material/component-specific thresholds supplied by a
// version-aware adapter.
func (r SpearAttackRequest) ChargeEligible() bool {
	return r.Mode == SpearCharge && r.RelativeSpeed >= r.MinSpeed && r.ViewAlignment >= r.MinAlignment
}

// ClassifyMeleeItem identifies spear item IDs without requiring a 1.21.11
// item registry in the transport-independent combat package.
func ClassifyMeleeItem(itemName string) MeleeWeaponKind {
	name := strings.ToLower(strings.TrimSpace(itemName))
	name = strings.TrimPrefix(name, "minecraft:")
	if name == "spear" || strings.HasSuffix(name, "_spear") {
		return SpearMeleeWeapon
	}
	if name == "" {
		return UnknownMeleeWeapon
	}
	return StandardMeleeWeapon
}

// SpearModeForDistance selects the safe default attack mode. Jab is the
// ordinary primary-action attack; charge requires a separate held-use
// executor and must only be selected when movement/velocity data is available.
func SpearModeForDistance(distance float64, canCharge bool) SpearAttackMode {
	if canCharge && distance > 0 {
		return SpearCharge
	}
	return SpearJab
}
