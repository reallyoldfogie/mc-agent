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
