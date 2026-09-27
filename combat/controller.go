package combat

import "time"

// State is the high-level combat mode selected from a single observation.
type State uint8

const (
	Idle State = iota
	Engaging
	Evading
	Retreating
)

// Weapon is the combat range class, not a specific inventory item.
type Weapon uint8

const (
	NoWeapon Weapon = iota
	MeleeWeapon
	RangedWeapon
)

// Observation is deliberately transport-independent. An agent adapter can
// build it from tracked entities, health, inventory, and movement state.
type Observation struct {
	Health         float32
	MaxHealth      float32
	EnemyCount     int
	HasTarget      bool
	TargetVisible  bool
	TargetDistance float64
	// TargetDirection is the normalized horizontal direction from the agent to
	// the selected target. It is used only for movement intents.
	TargetDirectionX float64
	TargetDirectionZ float64
	CurrentWeapon    Weapon
}

// Decision is the controller's next high-level action. Execution is left to
// an adapter so this package can be tested without a Minecraft connection.
type Decision struct {
	State  State
	Weapon Weapon
	Attack bool
}

const (
	meleeRange          = 5.0
	rangedRange         = 8.0
	criticalHealthRatio = 0.20
	evadeHealthRatio    = 0.40
)

// HealthRatio returns health/max-health, clamped to [0, 1]. Unknown maximum
// health is treated as full health rather than triggering an unsafe retreat.
func HealthRatio(health, maxHealth float32) float32 {
	if maxHealth <= 0 {
		return 1
	}
	ratio := health / maxHealth
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

// ChooseWeapon applies the prompt's 5/8-block hysteresis. A currently-held
// weapon is retained in the transition band to prevent oscillating slots.
func ChooseWeapon(distance float64, current Weapon) Weapon {
	if current == RangedWeapon && distance > meleeRange {
		return RangedWeapon
	}
	if current == MeleeWeapon && distance < rangedRange {
		return MeleeWeapon
	}
	if distance <= meleeRange {
		return MeleeWeapon
	}
	return RangedWeapon
}

// Decide selects the next state, weapon class, and whether an attack is
// allowed in principle. Cooldown timing is applied separately by ReadyToAttack.
func Decide(obs Observation) Decision {
	if !obs.HasTarget {
		return Decision{State: Idle, Weapon: obs.CurrentWeapon}
	}
	ratio := HealthRatio(obs.Health, obs.MaxHealth)
	state := Engaging
	if ratio <= criticalHealthRatio || obs.EnemyCount >= 4 {
		state = Retreating
	} else if ratio < evadeHealthRatio || !obs.TargetVisible {
		state = Evading
	}
	weapon := ChooseWeapon(obs.TargetDistance, obs.CurrentWeapon)
	return Decision{State: state, Weapon: weapon, Attack: state == Engaging && obs.TargetVisible}
}

// AttackInterval returns conservative range-class intervals. Specific item
// attack speeds can refine this in the inventory adapter.
func AttackInterval(weapon Weapon) time.Duration {
	if weapon == RangedWeapon {
		return time.Second
	}
	if weapon == MeleeWeapon {
		return 500 * time.Millisecond
	}
	return 0
}

// ReadyToAttack reports whether an attack may be scheduled at now.
func ReadyToAttack(now, last time.Time, weapon Weapon) bool {
	interval := AttackInterval(weapon)
	return weapon != NoWeapon && (last.IsZero() || now.Sub(last) >= interval)
}

// MovementAction describes the movement policy selected for this decision.
type MovementAction uint8

const (
	HoldPosition MovementAction = iota
	ApproachTarget
	StrafeTarget
	EvadeTarget
	RetreatFromTarget
)

// MovementIntent is a normalized horizontal movement request. It is an
// instruction for an adapter, not a packet or a promise that terrain permits
// the movement.
type MovementIntent struct {
	Action    MovementAction
	ThrottleX float64
	ThrottleZ float64
	Sprint    bool
	Jump      bool
}

// MovementEnvironment contains the world facts that can make an otherwise
// sensible combat movement unsafe. It is supplied by the world adapter.
type MovementEnvironment struct {
	GroundStable   bool
	InWater        bool
	FallRisk       bool
	CanJump        bool
	HasCover       bool
	TargetIsHazard bool
}

// FilterMovement applies conservative environmental safety rules before an
// intent reaches the movement executor. Unknown/unsafe ground stops movement;
// water removes sprinting; and a hazardous exposed target favors evasive
// lateral motion over approach.
func FilterMovement(intent MovementIntent, env MovementEnvironment) MovementIntent {
	if !env.GroundStable || env.FallRisk {
		return MovementIntent{Action: HoldPosition}
	}
	if env.InWater {
		intent.Sprint = false
		intent.Jump = true
	}
	if !env.CanJump {
		intent.Jump = false
	}
	if env.TargetIsHazard && !env.HasCover && (intent.Action == ApproachTarget || intent.Action == StrafeTarget) {
		intent.Action = EvadeTarget
		intent.Sprint = true
	}
	return intent
}

// Controller combines the pure policies into a tick-oriented decision maker.
// It does not move or attack directly. Call CommitAttack only after the
// execution adapter successfully performs the returned attack.
type Controller struct {
	lastAttack time.Time
	clockwise  bool
}

// NewController creates a controller with no attack history.
func NewController() *Controller {
	return &Controller{}
}

// Next evaluates one observation and returns the decision plus movement intent
// for that tick. The caller must call CommitAttack after a successful attack.
func (c *Controller) Next(now time.Time, obs Observation) (Decision, MovementIntent) {
	decision := Decide(obs)
	if decision.Attack && !ReadyToAttack(now, c.lastAttack, decision.Weapon) {
		decision.Attack = false
	}
	intent := MovementFor(obs, decision, c.clockwise)
	if intent.Action == StrafeTarget || intent.Action == EvadeTarget {
		c.clockwise = !c.clockwise
	}
	return decision, intent
}

// CommitAttack records a successfully executed attack for cooldown purposes.
func (c *Controller) CommitAttack(at time.Time) {
	c.lastAttack = at
}

// MovementFor converts a combat decision into a deterministic movement
// intent. clockwise selects one of the two strafe directions and should be
// varied by the controller over time to avoid a fixed pattern.
func MovementFor(obs Observation, decision Decision, clockwise bool) MovementIntent {
	if !obs.HasTarget {
		return MovementIntent{Action: HoldPosition}
	}
	dx, dz := obs.TargetDirectionX, obs.TargetDirectionZ
	if dx == 0 && dz == 0 {
		return MovementIntent{Action: HoldPosition}
	}
	intent := MovementIntent{}
	switch decision.State {
	case Retreating:
		intent.Action = RetreatFromTarget
		intent.ThrottleX, intent.ThrottleZ = -dx, -dz
		intent.Sprint, intent.Jump = true, true
	case Evading:
		intent.Action = EvadeTarget
		if clockwise {
			intent.ThrottleX, intent.ThrottleZ = dz, -dx
		} else {
			intent.ThrottleX, intent.ThrottleZ = -dz, dx
		}
		intent.Sprint, intent.Jump = true, true
	case Engaging:
		if decision.Weapon == MeleeWeapon && obs.TargetDistance > meleeRange {
			intent.Action = ApproachTarget
			intent.ThrottleX, intent.ThrottleZ = dx, dz
		} else {
			intent.Action = StrafeTarget
			if clockwise {
				intent.ThrottleX, intent.ThrottleZ = dz, -dx
			} else {
				intent.ThrottleX, intent.ThrottleZ = -dz, dx
			}
		}
	}
	return intent
}
