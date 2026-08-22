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
