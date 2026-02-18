package models

// ProjectilePhysicsModel defines how a specific projectile type applies physics
// Each projectile type (snowballs, arrows, wind charges, etc.) has different physics:
// - Different update orders (gravity→drag→position vs position→drag→gravity)
// - Different gravity values
// - Different drag coefficients
// - Custom acceleration for some types
//
// Implementations must handle the complete physics simulation for one tick,
// including both vertical and horizontal movement.
type ProjectilePhysicsModel interface {
	// Type returns the projectile type this model represents
	Type() ProjectileType

	// Gravity returns the gravitational acceleration (blocks/tick²)
	// Returns 0 for projectiles that don't use standard gravity (e.g., wind charges)
	Gravity() float64

	// Drag returns the air drag coefficient (multiplicative per tick)
	// Typical values: 0.99 for most projectiles, 0.95 for fireballs/wind charges
	Drag() float64

	// WaterDrag returns the water drag coefficient (different from air)
	// Typical values: 0.8 for most, 0.99 for arrows, 0.6 for persistent projectiles
	WaterDrag() float64

	// InitialSpeed returns the default throw/shoot speed (blocks/tick)
	// Examples: 1.5 for snowballs, 3.0 for arrows, 0.7 for experience bottles
	InitialSpeed() float64

	// UpdateOrder returns how physics should be applied each tick
	// Different projectile types apply gravity, drag, and position in different orders
	UpdateOrder() UpdateOrder

	// ApplyPhysics applies one complete tick of physics to the projectile
	// This encapsulates the correct order of operations for this projectile type
	//
	// Parameters:
	//   pos: current position
	//   vel: current velocity
	//   inWater: whether projectile is in water (affects drag)
	//
	// Returns:
	//   new position after physics
	//   new velocity after physics
	ApplyPhysics(pos, vel V3, inWater bool) (V3, V3)

	// ApplyVerticalForce applies gravity or custom vertical adjustment
	// Allows customization for projectiles that don't use standard gravity
	//
	// Standard implementation: vel.Y -= gravity
	// Wind charges: vel.Y -= 0.02 (custom adjustment instead of gravity)
	ApplyVerticalForce(vel V3) V3
}

// UpdateOrder specifies the order in which physics is applied per tick
type UpdateOrder int

const (
	// OrderGravityDragPosition: gravity → drag → position
	// Used by: ThrownEntity variants (snowballs, eggs, enderpearls, potions)
	// Source: ThrownEntity.java:48-67 in Minecraft 1.21.8
	OrderGravityDragPosition UpdateOrder = iota

	// OrderCollisionDragGravity: collision/position → drag → gravity
	// Used by: PersistentProjectileEntity variants (arrows, tridents)
	// Source: PersistentProjectileEntity.java:163-256 in Minecraft 1.21.8
	// Note: Position update is handled via collision detection in real Minecraft,
	// but for trajectory simulation we treat it as position update
	OrderCollisionDragGravity

	// OrderDragPositionCustomAccel: drag → position (with custom acceleration)
	// Used by: ExplosiveProjectileEntity variants (wind charges)
	// Source: ExplosiveProjectileEntity.java:66-94 in Minecraft 1.21.8
	// Note: Does NOT use standard gravity, uses custom vertical adjustment instead
	OrderDragPositionCustomAccel

	// OrderMoveDragGravity: position → drag → gravity (DEPRECATED - incorrect)
	// This was the original incorrect order in engine.go
	// DO NOT USE for new code - kept for backwards compatibility
	OrderMoveDragGravity
)
