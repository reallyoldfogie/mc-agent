package models

// PhysicsState is an interface for physics state needed by input generators and movement executors.
type PhysicsState interface {
	Position

	Position() V3
	Velocity() V3
	Yaw() float64
	Pitch() float64
	OnGround() bool
	IsSneaking() bool
	IsSwimming() bool
	IsInWater() bool
	FallDistance() float64
	GetDimensions() (width, height, eyeHeight float64)
	GetAABB() AABB
	SetPosition(pos V3, yaw, pitch float64, onGround bool)
	SetPositionSimple(pos V3)
	SetYaw(yaw float64)
	SetPitch(pitch float64)
	SetVelocity(vel V3)
	SetOnGround(onGround bool)
	SetSneaking(sneaking bool)
	SetFallDistance(distance float64)
	// SetActiveEffects updates the walking player's own status-effect state
	// consulted by Tick() (Slow Falling gravity cap, Levitation). Callers
	// should call this once per tick, before Tick(), the same way other
	// externally-sourced per-tick state is synced in. See ActiveEffects.
	SetActiveEffects(effects ActiveEffects)
	// SetElytraEquipped updates whether the player's chest slot currently
	// holds a glide-capable item, consulted by Tick() to gate elytra
	// gliding (Java canGlide()'s equipment check). Callers should call this
	// once per tick, before Tick(), the same way SetActiveEffects is synced
	// in — physics.State has no inventory access of its own.
	SetElytraEquipped(equipped bool)
	// IsGliding reports whether elytra-gliding physics are currently
	// active. Callers compare this before/after Tick() to detect a
	// start-gliding transition and send the corresponding
	// start_elytra_flying EntityAction packet — physics.State has no
	// packet access of its own.
	IsGliding() bool
	// SetFireworkBoosting updates whether a firework rocket used while
	// gliding is currently attached and boosting velocity, consulted by
	// Tick() to apply FireworkBoostVelocity. Callers should call this once
	// per tick, before Tick(), the same way SetElytraEquipped is synced in —
	// physics.State has no entity-tracking access of its own.
	SetFireworkBoosting(boosting bool)
	// SetFlying updates whether creative/spectator-style flying physics are
	// currently active, and the vertical ascend/descend impulse scale to
	// use while so (PlayerAbilities.FlySpeed). Callers should call this
	// once per tick, before Tick(), the same way SetElytraEquipped is
	// synced in — physics.State has no ability-tracking access of its own.
	SetFlying(flying bool, flySpeed float64)
	// SetNoClip updates whether collision resolution should be bypassed
	// entirely this tick (spectator mode - see Entity.noClip). Callers
	// should call this once per tick, before Tick(), the same way
	// SetElytraEquipped is synced in — physics.State has no game-mode
	// access of its own.
	SetNoClip(noClip bool)
	GetVelocity() V3
	Tick(input Inputs, w PhysicsWorld) error
	PredictMovement(inputs []Inputs, maxTicks int, w PhysicsWorld) []PhysicsState
	PredictPosition(vel V3, ticks int, w PhysicsWorld) V3
	WillCollide(targetPos V3, w PhysicsWorld) bool
	HasGroundSupportAt(pos V3, w PhysicsWorld) bool
	IsLookingAtTarget(targetYaw, targetPitch float64) bool
	GetSurroundingBoxes(queryBB AABB, w PhysicsWorld) []AABB

	// ResolveCollision performs collision detection and resolution for an arbitrary
	// AABB moving with the given velocity through the world. Returns the corrected
	// AABB, corrected velocity, and whether horizontal/vertical collisions occurred.
	// This is used by riding handlers to apply collision detection for ridden entities
	// whose dimensions differ from the player's (e.g., strider 0.9×1.7, horse 1.4×1.6).
	ResolveCollision(entityBB AABB, vel V3, w PhysicsWorld) (AABB, V3, bool, bool)
}
