package physics

import "math"

// This file holds the pure (dependency-free) status-effect formulas used by
// state.go's applyEnvironmentForces, split out the same way
// movement/riding_physics.go separates pure riding arithmetic from its
// handlers — these can be unit tested without a live Minecraft server or a
// physics.State instance.
//
// Both formulas are cited directly from decompiled
// net/minecraft/entity/LivingEntity.java (getEffectiveGravity/travelMidAir),
// not the wiki's prose description, since the wiki does not spell out the
// exact Slow-Falling-while-rising exemption or Levitation's lerp shape.

// EffectiveGravity mirrors Java LivingEntity.getEffectiveGravity(): Slow
// Falling caps gravity at SlowFallingMaxGravity, but only while the entity
// is not rising (velocityY <= 0) — a Slow Falling entity moving upward (e.g.
// from a jump) still decelerates at the normal rate until it starts to
// fall. Not amplifier-scaled: every level of Slow Falling uses the same cap.
func EffectiveGravity(normalGravity, velocityY float64, hasSlowFalling bool) float64 {
	if hasSlowFalling && velocityY <= 0 {
		if normalGravity < SlowFallingMaxGravity {
			return normalGravity
		}
		return SlowFallingMaxGravity
	}
	return normalGravity
}

// LevitationVerticalVelocity mirrors the Levitation branch of Java
// LivingEntity.travelMidAir(): rather than subtracting gravity, vertical
// velocity eases toward a fixed per-level target
// (LevitationBaseVelocityPerLevel * (amplifier+1)) by
// LevitationLerpFactor of the remaining distance each tick — an
// exponential approach, not an instant snap. amplifier is zero-based
// (0 = level I). currentVelocityY is the vertical velocity before this
// tick's environmental forces are applied (Java's `vec3d.y`, i.e. after
// movement input but before gravity/levitation).
func LevitationVerticalVelocity(currentVelocityY float64, amplifier int32) float64 {
	target := LevitationBaseVelocityPerLevel * float64(amplifier+1)
	return currentVelocityY + (target-currentVelocityY)*LevitationLerpFactor
}

// JumpBoostVelocityBonus mirrors Java LivingEntity.getJumpBoostVelocityModifier():
// a flat additive bonus to jump velocity, added on top of the normal
// JumpVelocity assignment rather than scaling it. amplifier is zero-based
// (0 = level I).
func JumpBoostVelocityBonus(amplifier int32) float64 {
	return JumpBoostVelocityPerLevel * float64(amplifier+1)
}

// CanSprint mirrors Java ClientPlayerEntity.canSprint()'s Blindness check:
// hasBlindnessEffect() alone (ignoring the vehicle/flying/item-use branches,
// which don't apply to the walking-player path this codebase models) is
// enough to prevent sprinting outright, whether starting a new sprint or
// continuing one already in progress — canSprint() is also the condition
// shouldStopSprinting() negates, so vanilla actively cancels an in-progress
// sprint the instant Blindness lands, not just refuses new ones.
func CanSprint(hasBlindness bool) bool {
	return !hasBlindness
}

// EffectSpeedMultiplier mirrors Java's Speed/Slowness ADD_MULTIPLIED_TOTAL
// modifiers on generic.movement_speed (StatusEffects.java): each active
// effect's amplifier-scaled amount is applied as its own separate multiply
// against the running total (EntityAttributeInstance.computeValue's
// ADD_MULTIPLIED_TOTAL stage), matching AttributeValue.Compute's stage-3
// behavior. Not implemented by calling Compute directly since the walking
// player's own Speed/Slowness state comes from GetOwnActiveEffect (the
// player's own entity never receives a live AttributeValue — see
// GetOwnActiveEffect's doc comment for why), not a wire-parsed
// AttributeValue. Clamped at 0 minimum, matching generic.movement_speed's
// ClampedEntityAttribute floor — high enough Slowness can drive the raw
// product negative, but vanilla clamps the final speed at exactly zero
// rather than reversing it.
func EffectSpeedMultiplier(hasSpeed bool, speedAmplifier int32, hasSlowness bool, slownessAmplifier int32) float64 {
	multiplier := 1.0
	if hasSpeed {
		multiplier *= 1.0 + SpeedAmountPerLevel*float64(speedAmplifier+1)
	}
	if hasSlowness {
		multiplier *= 1.0 + SlownessAmountPerLevel*float64(slownessAmplifier+1)
	}
	if multiplier < 0 {
		return 0
	}
	return multiplier
}

// HorizontalWaterDrag returns the per-tick horizontal (X/Z) velocity
// multiplier to apply while in water, mirroring Java LivingEntity.travelInWater's
// `f` (vec3d.multiply(f, 0.8F, f)): normally WaterDrag (the non-sprinting
// baseline, getBaseWaterMovementSpeedMultiplier()), but Dolphin's Grace
// overrides it to a flat DolphinsGraceWaterDragMultiplier regardless of
// sprint state or amplifier. The vertical (Y) multiplier is unaffected by
// this effect in vanilla (always a fixed 0.8F) and is not this function's
// concern — callers should keep applying WaterDrag to Y unconditionally.
func HorizontalWaterDrag(hasDolphinsGrace bool) float64 {
	if hasDolphinsGrace {
		return DolphinsGraceWaterDragMultiplier
	}
	return WaterDrag
}

// CobwebSlowdownMultiplier mirrors Java CobwebBlock.onEntityCollision /
// WebBlock.entityInside: while overlapping a cobweb block, that tick's
// attempted movement is scaled by a fixed per-axis multiplier (velocity is
// separately reset to zero afterward by the caller — see
// physics/state.go's isOverlappingCobweb doc comment). Weaving halves the
// severity rather than bypassing the slowdown; not amplifier-scaled.
func CobwebSlowdownMultiplier(hasWeaving bool) (x, y, z float64) {
	if hasWeaving {
		return WeavingCobwebSlowdownX, WeavingCobwebSlowdownY, WeavingCobwebSlowdownZ
	}
	return CobwebSlowdownX, CobwebSlowdownY, CobwebSlowdownZ
}

// PerceptionRadiusCap clamps a detection radius to the agent's own
// effective vision range while Blindness/Darkness is active, cited from
// BlindnessEffectFogModifier.java/DarknessEffectFogModifier.java (see
// BlindnessVisionRadius/DarknessVisionRadius). Neither effect is
// amplifier-scaled; presence alone determines the cap. If both are active,
// the more restrictive (smaller) cap applies. Never raises radius above
// what the caller requested.
func PerceptionRadiusCap(radius float64, hasBlindness, hasDarkness bool) float64 {
	if hasBlindness {
		radius = math.Min(radius, BlindnessVisionRadius)
	}
	if hasDarkness {
		radius = math.Min(radius, DarknessVisionRadius)
	}
	return radius
}
