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
	// craftRewardBonus is added once, the step Config.CraftTargetItem's held
	// inventory count is observed to increase (see Environment.Step's
	// before/after InventoryCount comparison) — same magnitude as
	// arrivalBonus/mineRewardBonus, marking "this episode's task was
	// accomplished." Not gated on the dispatched action having been
	// ActionCraft specifically, matching mineRewardBonus's philosophy of
	// judging outcomes from actual world-state (here, inventory) deltas
	// rather than which action was chosen — see
	// docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1d.
	craftRewardBonus float32 = 10.0
	// equipRewardBonus is added once, the step Config.EquipTargetItem is
	// observed to become equipped (see Environment.Step's before/after
	// IsEquipped comparison) — same magnitude as mineRewardBonus/
	// craftRewardBonus, marking "this episode's task was accomplished."
	// ActionUseItem deliberately has no reward component of its own — see
	// docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 1.
	equipRewardBonus float32 = 10.0
	// pickupRewardBonus is added once, the step Config.PickupTargetItem's
	// held inventory count is observed to increase (see Environment.Step's
	// before/after InventoryCount comparison) — same magnitude/philosophy
	// as craftRewardBonus, not mineRewardBonus: there's no position whose
	// content to diff (the item entity disappears entirely once collected),
	// so this is judged the same way craftRewardBonus is.
	pickupRewardBonus float32 = 10.0
	// flyRewardBonus is added once, the step the bot's horizontal (X/Z)
	// distance to Environment.flyTargetX/Z is observed to fall within
	// Config.FlyArrivalRadius — same magnitude as arrivalBonus, and the
	// same one-shot ("episode ends the instant it's true") shape, not a
	// transition check: unlike equip/pickup, which need "before vs after"
	// detection because the condition can stay true for many steps before
	// the episode notices, arrival ends the episode on the very first
	// step it's observed, so there's no later step to re-pay it on.
	flyRewardBonus float32 = 10.0
	// rideRewardBonus is added once, the step LiveAgent.IsMounted() is
	// observed to become true (see Environment.Step's before/after
	// comparison) — same magnitude/philosophy as equipRewardBonus: a
	// one-shot transition check, ending the episode immediately like
	// every other task bonus. Mounting alone (not riding anywhere) is a
	// complete, testable task for a first pass — see docs/plans/
	// RL_ACTION_SPACE_EXPANSION_II.md Phase 4's own note on why "ride
	// somewhere" is left as a future composite task, not built here.
	rideRewardBonus float32 = 10.0
	// combatRewardBonus is added once, the step Environment's resolved
	// combat target (the specific entity ID e.combatEntityID held at the
	// start of the step — see Step's own before/after
	// LiveAgent.GetEntityHealth comparison) is observed to die — either
	// no longer tracked at all, or tracked with health <= 0 (the server
	// doesn't guarantee a final 0-health update before removing a killed
	// entity, so either signal counts). Same magnitude/philosophy as
	// mineRewardBonus: a world-state-delta check, not "was ActionCombat
	// the dispatched action." The bot's own health loss/death from the
	// fight needs no special handling here — computeReward's
	// damagePenaltyScale/deathPenalty already apply generically to every
	// task, combat included (see Config.CombatTargetMobType's own doc
	// comment).
	combatRewardBonus float32 = 10.0
	// placeRewardBonus is added once, the step Config.PlaceTargetItem's
	// held inventory count is observed to decrease (placing consumes one
	// unit) during a standalone (non-chain) placement episode — same
	// magnitude/philosophy as craftRewardBonus, just the opposite
	// direction (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 2).
	placeRewardBonus float32 = 10.0
	// followRewardBonus is added once, the step CommandAgent.IsFollowing()
	// is observed to transition false->true — same magnitude/philosophy
	// as rideRewardBonus: a one-shot start-transition, not continuous
	// proximity-shaping. Following/manager.go's own control loop
	// (sprint/sneak distance thresholds, stuck recovery, continuous path
	// recalculation) is an open-ended background behavior with no
	// natural "done" state, structurally unlike every other task in this
	// package — rather than extending computeReward with a second,
	// independent distance-shaping stream for one task, this mirrors
	// ActionMount's own shape instead. Sustained "follow well" as a
	// richer, continuous reward is a legitimate future enhancement this
	// doesn't foreclose, just doesn't build speculatively (docs/plans/
	// RL_ACTION_SPACE_EXPANSION_III.md item 4).
	followRewardBonus float32 = 10.0
	// creeperGunpowderRewardBonus is added once, the step
	// "minecraft:gunpowder"'s held inventory count is observed to
	// increase during a Config.CreeperGunpowderActive episode — same
	// magnitude/philosophy as craftRewardBonus/pickupRewardBonus. Faithful
	// to the real encounter's uncertain drop rate: a kill with no drop (a
	// valid, documented outcome of agent.KillCreeperForGunpowder) earns
	// nothing, matching how a real player farming creepers experiences it
	// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5).
	creeperGunpowderRewardBonus float32 = 10.0
	// spearChargeRewardBonus is added once, the step Environment's
	// resolved spear-charge target (the specific entity ID
	// e.spearEntityID held at the start of the step) is observed to die —
	// same magnitude/philosophy as combatRewardBonus, a world-state-delta
	// check, not "was ActionSpearChargeWithApproach the dispatched
	// action" (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 6).
	spearChargeRewardBonus float32 = 10.0
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
	// timePenalty overrides the default per-step penalty when > 0 (see
	// Config.TimePenalty).
	timePenalty float32

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
	diedThisStep = o.healthKnownAfter && o.newHealth <= 0

	// A death step's position is the respawn point, not somewhere the
	// action moved the bot to: with a target thousands of blocks from world
	// spawn (shared-server working areas), the "progress" of respawning is
	// thousands of reward points of pure teleport artifact - found live as
	// an epoch average return of -360 against a normal ~11. Death is
	// already scored by deathPenalty and the damage term.
	if !diedThisStep {
		reward = distanceRewardScale * float32(o.prevDistance-o.newDistance)
	}
	penalty := o.timePenalty
	if penalty <= 0 {
		penalty = timePenalty
	}
	reward -= penalty

	if o.healthKnownBefore && o.healthKnownAfter {
		if damage := o.prevHealth - o.newHealth; damage > 0 {
			reward -= damagePenaltyScale * damage
		}
	}

	if diedThisStep {
		reward += deathPenalty
	}

	return reward, diedThisStep
}

// distance3 is the Euclidean distance between two points.
func distance3(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx, dy, dz := x2-x1, y2-y1, z2-z1
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// distanceXZ is the horizontal (X/Z only) Euclidean distance between two
// points — Flight's own arrival check (Config.FlyArrivalRadius), matching
// agent.FlyTo's own landing behavior of never targeting Y exactly (see
// Config.FlyTargetOffset's own doc comment).
func distanceXZ(x1, z1, x2, z2 float64) float64 {
	dx, dz := x2-x1, z2-z1
	return math.Sqrt(dx*dx + dz*dz)
}
