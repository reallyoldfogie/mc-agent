package rlenv

import "math"

// Reward shaping constants. Named and documented per RL_POLICY_INTEGRATION_PLAN.md
// item 4 ("Reward function: mc-agent-specific... define reward as a
// function of state deltas"). Not configurable via Config: these are a
// property of *this* task (reach a target), not a generic knob — a
// different task type would define its own reward function entirely
// (see doc.go and pkg/hierarchical's precedent in cRL-go for why reward
// shaping lives with the environment, not a pluggable strategy object,
// until a second task type actually exists to justify one).
const (
	// distanceRewardScale converts blocks of progress toward the target
	// into reward per step: +1 reward per block closer, -1 per block
	// farther.
	distanceRewardScale float32 = 1.0
	// timePenalty is subtracted every step, independent of outcome, so an
	// episode that never reaches the target scores worse the longer it
	// takes (encourages efficiency, matches chatgpt-training.md's
	// suggested "time_cost" reward component).
	timePenalty float32 = 0.01
	// damagePenaltyScale converts health lost this step into reward: -1
	// per point of health lost.
	damagePenaltyScale float32 = 1.0
	// arrivalBonus is added once, the step the target is reached.
	arrivalBonus float32 = 10.0
	// mineRewardBonus is added once, the step Config.MineTargetBlock's
	// nearest visible instance is observed to change (see
	// Environment.Step's before/after BlockNameAt comparison) — same
	// magnitude as arrivalBonus, since both mark "this episode's task was
	// accomplished." Not gated on the dispatched action having been
	// ActionMine specifically, matching this file's existing philosophy of
	// judging outcomes from actual world-state deltas rather than from
	// which action was chosen (see TestStepReflectsExternalPositionChange
	// in environment_test.go for the movement-side precedent).
	mineRewardBonus float32 = 10.0
	// deathPenalty is added (as a negative) the step health is observed to
	// reach zero.
	deathPenalty float32 = -20.0
)

// stepOutcome bundles what computeReward needs from a Step call: the
// distance to target before and after this step's action, and the
// health/food state before and after (health deltas drive the damage
// penalty; healthKnownBefore/After gate it — see observation.go's
// healthKnown feature for why "unknown" must be handled explicitly rather
// than treated as zero).
type stepOutcome struct {
	prevDistance, newDistance           float64
	prevHealth, newHealth               float32
	healthKnownBefore, healthKnownAfter bool
}

// computeReward implements the progress/time/damage/death portion of the
// reward function described above, and reports whether death ends the
// episode. It doesn't know about arrival (Config.ArrivalThreshold isn't
// part of stepOutcome) — Environment.Step adds arrivalBonus and sets
// done=true on arrival itself, after calling this.
func computeReward(o stepOutcome) (reward float32, diedThisStep bool) {
	reward = distanceRewardScale*float32(o.prevDistance-o.newDistance) - timePenalty

	if o.healthKnownBefore && o.healthKnownAfter {
		if damage := o.prevHealth - o.newHealth; damage > 0 {
			reward -= damagePenaltyScale * damage
		}
	}

	if o.healthKnownAfter && o.newHealth <= 0 {
		reward += deathPenalty
		diedThisStep = true
	}

	return reward, diedThisStep
}

// distance3 is the Euclidean distance between two points.
func distance3(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx, dy, dz := x2-x1, y2-y1, z2-z1
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
