package physics

import (
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Player dimension constants
const (
	PlayerWidth          = models.PlayerWidth
	PlayerHeight         = models.PlayerHeight
	PlayerEyeHeight      = models.PlayerEyeHeight
	PlayerHeightSneaking = models.PlayerHeightSneaking
)

// Movement constants
const (
	StepHeight    = 0.6   // Maximum height the player can automatically step up
	ResetVelocity = 0.003 // Velocity below this threshold is reset to zero
)

// Look/rotation constraints (degrees per tick)
const (
	MaxYawChange   = 11.0 // Maximum yaw change per tick (anti-cheat)
	MaxPitchChange = 7.0  // Maximum pitch change per tick (anti-cheat)
)

// Ladder movement constants
const (
	MinJumpTicks     = 14   // Minimum ticks between jumps (jump cooldown)
	LadderMaxSpeed   = 0.15 // Maximum velocity while on ladder
	LadderClimbSpeed = 0.2  // Upward velocity when climbing ladder
)

// Physics simulation constants (vanilla Minecraft values)
const (
	Gravity      = 0.08 // Gravitational acceleration (blocks/tick²) - downward
	Drag         = 0.98 // Air resistance multiplier (velocity *= drag each tick)
	Acceleration = 0.02 // Horizontal acceleration when moving
	Inertia      = 0.91 // Horizontal velocity multiplier (friction when on ground)
	Slipperiness = 0.6  // Base ground friction multiplier
	JumpVelocity = 0.42 // Initial upward velocity when jumping
)

// Tick rate constants
const (
	TicksPerSecond = 20                    // Minecraft runs at 20 TPS
	TickDuration   = 50 * time.Millisecond // 50ms per tick
)

// Sprint and sneak multipliers
const (
	SprintMultiplier = 1.3 // Sprint increases horizontal speed by 30%
	SneakMultiplier  = 0.3 // Sneak reduces horizontal speed to 30% (base, without Swift Sneak enchantment)
)

// Sneaking mechanics constants
// Note: Player width remains 0.6 blocks when sneaking.

// Swift Sneak enchantment multipliers (future enhancement)
// When Swift Sneak enchantment is detected on leggings, these replace SneakMultiplier
const (
	SwiftSneakI   = 0.45 // Swift Sneak I: 45% speed (1.5x faster than base sneak)
	SwiftSneakII  = 0.60 // Swift Sneak II: 60% speed (2x faster than base sneak)
	SwiftSneakIII = 0.75 // Swift Sneak III: 75% speed (2.5x faster than base sneak)
	// Implementation requires equipment tracking and NBT enchantment parsing
)

// Projectile physics constants (arrows, snowballs, etc.)
const (
	// Arrow physics (from Minecraft 1.21.8 decompiled source)
	// BowItem.onStoppedUsing passes speed = f * 3.0F where f = pull progress (0.0 to 1.0)
	// At full draw, f = 1.0, so max speed = 1.0 * 3.0 = 3.0 blocks/tick
	// Source: /work/1.21.8/extractedSrc/net/minecraft/item/BowItem.java:42
	ArrowGravity      = 0.05 // Arrow gravity - PersistentProjectileEntity.java:302
	ArrowDrag         = 0.99 // Arrow air resistance - PersistentProjectileEntity.java:247
	ArrowInitialSpeed = 3.0  // Max arrow speed (fully charged bow) - BowItem.java:42 (f * 3.0F)

	// Snowball physics
	SnowballGravity      = 0.03 // Snowball/egg gravity
	SnowballDrag         = 0.99 // Snowball/egg drag
	SnowballInitialSpeed = 1.5  // Snowball/egg throw speed

	// Egg physics
	EggGravity      = 0.03 // Snowball/egg gravity
	EggDrag         = 0.99 // Snowball/egg drag
	EggInitialSpeed = 1.5  // Snowball/egg throw speed

	// Ender Pearl physics
	EnderPearlGravity      = 0.03 // Ender pearl gravity
	EnderPearlDrag         = 0.99 // Ender pearl drag
	EnderPearlInitialSpeed = 1.5  // Ender pearl throw speed

	// Splash Potion physics
	SplashPotionGravity      = 0.05 // Splash potion gravity
	SplashPotionDrag         = 0.99 // Splash potion drag
	SplashPotionInitialSpeed = 0.5  // Splash potion throw speed (slower arc)

	// Experience Bottle physics
	ExperienceBottleGravity      = 0.07 // Experience bottle gravity (heavier than potions)
	ExperienceBottleDrag         = 0.99 // Experience bottle drag
	ExperienceBottleInitialSpeed = 0.7  // Experience bottle throw speed

	// Wind Charge physics (1.21+)
	WindChargeGravity      = 0.02 // Wind charge gravity (light, affected by wind)
	WindChargeDrag         = 0.99 // Wind charge drag
	WindChargeInitialSpeed = 1.5  // Wind charge throw speed
)

// Block-specific slipperiness values (for future use)
// Base slipperiness is 0.6, but some blocks are different:
// - Ice: 0.98
// - Slime block: 0.8
// - Most blocks: 0.6

const (
	// Specific block slipperiness constants
	IceSlipperiness        = 0.98  // Ice and similar blocks (slippery)
	BlueIceSlipperiness    = 0.989 // Blue ice (even slipperier than regular ice)
	SlimeBlockSlipperiness = 0.80  // Slime blocks (some slide but also bounce)
	DefaultSlipperiness    = 0.60  // Default for most blocks (normal friction)
	HoneyBlockSlipperiness = 0.4   // Honey blocks (sticky, very low slipperiness)
	// Note: Packed ice / blue ice may have slightly different values; add as needed.
)

// Entity collision constants
const (
	// EntitySeparationForce is the push magnitude applied when entities overlap (horizontal only).
	// Mirrors the 0.05 factor from vanilla Minecraft's Entity.pushAwayFrom().
	// Scaled by min(1/sqrt(chebyshev), 1) so deep overlaps produce a smaller force
	// and shallow overlaps (near the edge of AABB contact) produce up to 0.05 blocks/tick.
	EntitySeparationForce = 0.05

	// Player collision dimensions (in blocks)
	// Used for generating entity AABBs when entity-specific dimensions unavailable
	PlayerCollisionWidth  = 0.6 // Horizontal dimension (X/Z)
	PlayerCollisionHeight = 1.8 // Vertical dimension (Y)
)

// Knockback constants (vanilla Minecraft values)
const (
	// KnockbackHorizontalStrength is the horizontal velocity applied per knockback event.
	// Vanilla Minecraft applies 0.4 blocks/tick in the attack direction.
	KnockbackHorizontalStrength = 0.4

	// KnockbackVerticalStrength is the upward velocity applied per knockback event.
	// Vanilla Minecraft always applies a 0.4 blocks/tick upward component.
	KnockbackVerticalStrength = 0.4
)

// Water physics constants
const (
	// Water flow speed when agent is in flowing water
	// Scales with water flow level (age 1-7)
	WaterFlowSpeedBase = 0.15 // Base flow velocity (blocks/tick)

	// Water resistance/drag when submerged
	// Higher value = more drag (slower movement)
	WaterDrag = 0.8 // Velocity multiplier per tick

	// WaterGravityFactor is the fraction of normal gravity applied when in water.
	// Vanilla Minecraft uses ~0.02 blocks/tick² underwater (25% of the normal 0.08).
	WaterGravityFactor = 0.25
)

// Boat physics constants (from Minecraft 1.21.10 AbstractBoatEntity)
// Per-tick velocity multipliers and gravity for different boat conditions.
const (
	// Standard boat in water
	BoatInWaterVelocityMultiplier = 0.9   // Velocity multiplier per tick
	BoatInWaterGravity            = -0.04 // Gravitational acceleration (blocks/tick²)

	// Boat fully submerged in water
	BoatUnderWaterVelocityMultiplier = 0.45  // Significant drag underwater
	BoatUnderWaterGravity            = -0.04 // Same gravity as in water

	// Boat under flowing water (stronger current effect)
	BoatUnderFlowingWaterVelocityMultiplier = 0.9   // Same as standard water
	BoatUnderFlowingWaterGravity            = -0.0007 // Reduced gravity in flowing water

	// Boat on land surfaces (these are the block slipperiness values)
	BoatOnLandStandardVelocityMultiplier = 0.6   // Default block friction
	BoatOnLandIceVelocityMultiplier      = 0.98  // Ice and packed ice
	BoatOnLandBlueIceVelocityMultiplier  = 0.989 // Blue ice (highest slipperiness)
	BoatOnLandGravity                    = -0.04 // Standard gravity on land

	// Boat horizontal acceleration constants (from AbstractBoatEntity.java).
	// Forward and backward use different values; the server validates against these.
	BoatForwardAcceleration  = 0.04  // speed += 0.04 when pressing forward
	BoatBackwardAcceleration = 0.005 // speed -= 0.005 when pressing backward (much slower)

	// Note: When on land with player controlling boat, slipperiness is halved (AbstractBoatEntity:565)
	BoatOnLandPlayerControlHalving = 0.5 // Multiply slipperiness by this when player controls boat
)

// Swimming constants
const (
	// SwimUpVelocity is the upward velocity applied each tick while the jump input
	// is held and the player is in water. Vanilla Minecraft uses ~0.04 blocks/tick.
	SwimUpVelocity = 0.04

	// SwimDownVelocity is the downward velocity applied each tick while the sneak
	// input is held and the player is in water.
	SwimDownVelocity = 0.04
)

// Minecart physics constants
// Source: net/minecraft/entity/vehicle/DefaultMinecartController.java (1.21.2+)
//         net/minecraft/entity/vehicle/AbstractMinecartEntity.java (1.21.1)
const (
	// MinecartRailDrag is the per-tick velocity multiplier when on a rail with a passenger.
	// Java DefaultMinecartController.getSpeedRetention(): 0.997 with passengers, 0.96 empty.
	// This path is only reached while the agent is riding, so the passenger value is always used.
	MinecartRailDrag = 0.997

	// MinecartRailDragEmpty is the per-tick velocity multiplier for an unoccupied minecart.
	// Included for completeness; not used by the riding executor.
	MinecartRailDragEmpty = 0.96

	// MinecartOffRailDrag is the velocity drag per tick when not on rail and on ground.
	// Java AbstractMinecartEntity.moveOffRail: velocity *= 0.5 when isOnGround().
	MinecartOffRailDrag = 0.5

	// MinecartOffRailAirDrag is the velocity drag per tick when not on rail and airborne.
	// Java AbstractMinecartEntity.moveOffRail: velocity *= 0.95 when !isOnGround().
	MinecartOffRailAirDrag = 0.95

	// MinecartNudgeImpulse is the tiny velocity added when the player presses forward/backward
	// on a nearly-stopped minecart to break static inertia.
	// Java: this.getVelocity().add(vec3d3.x * 0.001, 0.0, vec3d3.z * 0.001)
	// This is NOT continuous thrust — it fires only once the cart is nearly stopped.
	MinecartNudgeImpulse = 0.001

	// MinecartNudgeSpeedThreshold is the horizontal speed-squared below which the nudge fires.
	// Java: m < 0.01 where m = getVelocity().horizontalLengthSquared()
	MinecartNudgeSpeedThreshold = 0.01

	// MinecartPassengerSpeedMultiplier scales the per-tick displacement when a passenger is riding.
	// Java DefaultMinecartController.moveOnRail:
	//   double s = this.minecart.hasPassengers() ? 0.75 : 1.0;
	//   move(SELF, clamp(s * vel.x, -maxSpeed, maxSpeed), 0, clamp(s * vel.z, ...))
	// Only the position delta is scaled; the stored velocity is unaffected.
	MinecartPassengerSpeedMultiplier = 0.75

	// MinecartUnpoweredBrakeThreshold is the speed below which an unpowered powered rail
	// brings the minecart to a full stop instead of halving its speed.
	// Java: if (n < 0.03) { this.setVelocity(Vec3d.ZERO); } else { velocity *= 0.5; }
	MinecartUnpoweredBrakeThreshold = 0.03

	// MinecartSlopeGravity is the speed delta per tick when traversing a slope.
	// Java uses 0.0078125 (1/128). Applied as a constant downhill force; in the
	// signed-speed model this is always subtracted from speed.
	MinecartSlopeGravity = 0.0078125

	// MinecartPoweredRailBoost is the speed added per tick when on an energized powered rail.
	// Java: vec3d6.add(vec3d6.x / v * 0.06, 0.0, vec3d6.z / v * 0.06)
	MinecartPoweredRailBoost = 0.06

	// MinecartMaxSpeed is the maximum horizontal speed (blocks/tick).
	// Java DefaultMinecartController.getMaxSpeed(): 0.4 on land, 0.2 in water.
	MinecartMaxSpeed = 0.4

	// MinecartWaterMaxSpeed is the maximum horizontal speed in water.
	// Java DefaultMinecartController.getMaxSpeed(): 0.2 when isTouchingWater().
	MinecartWaterMaxSpeed = 0.2

	// MinecartWaterSlopeGravityFactor scales slope gravity when in water.
	// Java DefaultMinecartController.moveOnRail: g *= 0.2 when isTouchingWater().
	MinecartWaterSlopeGravityFactor = 0.2

	// MinecartWaterDragMultiplier is additional drag applied after normal drag in water.
	// Java AbstractMinecartEntity.applySlowdown: velocity *= 0.95 when isTouchingWater().
	MinecartWaterDragMultiplier = 0.95

	// MinecartFallGravity is the Y-axis gravity per tick when the minecart is off rail.
	// Java AbstractMinecartEntity.getGravity(): 0.04 on land, accumulated in Y velocity.
	MinecartFallGravity = -0.04

	// MinecartWaterGravity is the Y-axis gravity per tick when in water.
	// Java AbstractMinecartEntity.getGravity(): 0.005 when isTouchingWater().
	MinecartWaterGravity = -0.005

	// MinecartRailYOffset is the Y offset for rail anchor positions.
	// Java snapPositionToRail uses j + 0.0625 as the base Y for rail anchors.
	MinecartRailYOffset = 0.0625
)

// Rideable entity (camel, horse, etc.) water physics constants
const (
	// When a rideable entity (camel, horse, donkey, llama) is in water with a rider,
	// it sinks instead of floating. These values control the sinking rate and drag.

	// RideableInWaterGravity is the downward acceleration when a ridden entity is in water.
	// Vanilla behavior: ridden entities sink with full gravity applied.
	RideableInWaterGravity = 0.08 // Full gravity - entities sink when ridden in water

	// RideableInWaterDragMultiplier reduces horizontal velocity when ridden in water.
	// Matches player swimming drag behavior.
	RideableInWaterDragMultiplier = 0.8 // Significant drag in water
)

// HorseLandFriction computes the per-tick velocity multiplier for a ridden entity
// on land, based on the block's slipperiness. Mirrors the vanilla formula from
// LivingEntity.travelControlled → Entity.applyMovementInput → friction path:
//
//	friction = Inertia * slipperiness  (0.91 * 0.6 = 0.546 for most blocks)
//
// The result is the fraction of horizontal velocity retained each tick.
func HorseLandFriction(slipperiness float64) float64 {
	return Inertia * slipperiness
}
