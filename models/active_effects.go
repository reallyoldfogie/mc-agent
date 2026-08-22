package models

// ActiveEffect is the last-known state of one status effect on a tracked
// entity, as reported by ClientboundEntityEffect (aliased
// ClientboundUpdateMobEffect). Held per-entity in the same shape as
// Attributes/Equipment: last-known value, updated or removed by later
// packets, not locally decremented — vanilla sends an explicit removal
// packet rather than relying on the client to count down duration itself.
type ActiveEffect struct {
	// Amplifier is zero-based (0 = level I).
	Amplifier int32
	// DurationTicks is the remaining duration in ticks as last reported by
	// the server, not adjusted locally after receipt.
	DurationTicks int32
	Ambient       bool
	ShowParticles bool
	ShowIcon      bool
}

// ActiveEffects is the small, physics-relevant subset of the walking
// player's own active status effects, computed from the full per-entity
// ActiveEffect map (via GetOwnActiveEffect) once per tick and handed to
// PhysicsState.SetActiveEffects. Kept minimal and boolean/amplifier-shaped
// rather than exposing the full effect-name map to the physics package,
// which has no registry access and shouldn't need one.
type ActiveEffects struct {
	// HasSlowFalling mirrors Java LivingEntity.getEffectiveGravity(): caps
	// gravity at SlowFallingMaxGravity while falling (Vel.Y <= 0), and
	// (like Levitation) negates fall damage. Not amplifier-scaled in
	// vanilla — level has no effect on the cap.
	HasSlowFalling bool

	// HasLevitation and LevitationAmplifier mirror Java
	// LivingEntity.travelMidAir's Levitation branch: vertical velocity
	// eases toward LevitationBaseVelocityPerLevel*(amplifier+1) instead of
	// gravity being applied at all.
	HasLevitation       bool
	LevitationAmplifier int32
}
