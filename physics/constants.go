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

// Flying (creative/spectator) constants. Cited from decompiled
// PlayerEntity.travel()/ClientPlayerEntity.tickMovement() - see
// PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4.
const (
	// FlyingVerticalDecay replaces gravity/drag entirely while flying:
	// vertical velocity is unconditionally overwritten to this fraction of
	// its pre-tick value (which already includes this tick's ascend/
	// descend impulse - see FlyingVerticalImpulseScale) every tick,
	// discarding whatever gravity would otherwise have computed. Matches
	// PlayerEntity.travel()'s `velocity.y = d * 0.6`.
	FlyingVerticalDecay = 0.6
	// FlyingVerticalImpulseScale multiplies PlayerAbilities.FlySpeed
	// (vanilla default 0.05) to get the per-tick ascend/descend impulse
	// added while jump/sneak is held: matches
	// ClientPlayerEntity.tickMovement()'s `i * flySpeed * 3.0F`.
	FlyingVerticalImpulseScale = 3.0
)

// Status effect constants (Phase 4a). Cited directly from decompiled
// LivingEntity.java (getEffectiveGravity/travelMidAir) rather than the
// wiki's amplifier-scaled approximation for Slow Falling — vanilla does not
// scale the gravity cap by effect level.
const (
	// SlowFallingMaxGravity is the gravity cap while Slow Falling is active
	// and the entity is not rising (Vel.Y <= 0): getEffectiveGravity()
	// returns Math.min(getFinalGravity(), SlowFallingMaxGravity) rather than
	// the normal Gravity constant. Not amplifier-scaled — every level of
	// Slow Falling caps gravity at exactly this value.
	SlowFallingMaxGravity = 0.01

	// LevitationBaseVelocityPerLevel is the target vertical velocity per
	// effect level (0-indexed amplifier + 1) that Levitation eases the
	// entity's Y velocity toward, replacing gravity entirely while active:
	// travelMidAir's `d += (LevitationBaseVelocityPerLevel * (amplifier+1) - d) * LevitationLerpFactor`.
	LevitationBaseVelocityPerLevel = 0.05

	// LevitationLerpFactor is the fraction of the distance to the target
	// velocity closed each tick (an exponential approach, not an instant
	// snap or a constant acceleration).
	LevitationLerpFactor = 0.2

	// BlindnessVisionRadius is the flat visible-range cap while Blindness is
	// active, cited from BlindnessEffectFogModifier.java: `environmentalEnd`
	// (the distance fog becomes fully opaque) ramps to exactly 5 blocks once
	// fully faded in. Not amplifier-scaled.
	BlindnessVisionRadius = 5.0

	// DarknessVisionRadius is the flat visible-range cap at Darkness's
	// darkest point, cited from DarknessEffectFogModifier.java: pulses via
	// getFadeFactor rather than holding a constant radius, but 15 blocks is
	// the cap once fully faded in. Not amplifier-scaled. Used as a flat cap
	// for the whole active duration rather than modeling the pulse.
	DarknessVisionRadius = 15.0

	// JumpBoostVelocityPerLevel is the flat additive bonus to jump velocity
	// per effect level, cited from LivingEntity.java's
	// getJumpBoostVelocityModifier(): 0.1F * (amplifier + 1.0F). This is a
	// direct addition bolted onto jump velocity in jump(), not a generic
	// attribute modifier — StatusEffects.java only registers a
	// SAFE_FALL_DISTANCE modifier for Jump Boost (fall-damage safety),
	// unrelated to jump height.
	JumpBoostVelocityPerLevel = 0.1

	// SpeedAmountPerLevel is Speed's ADD_MULTIPLIED_TOTAL modifier amount on
	// generic.movement_speed per effect level, cited from
	// StatusEffects.java's SPEED registration (+0.2F) and
	// StatusEffect.EffectAttributeModifierCreator.createAttributeModifier's
	// amplifier scaling (baseValue * (amplifier+1)).
	SpeedAmountPerLevel = 0.2

	// SlownessAmountPerLevel is Slowness's ADD_MULTIPLIED_TOTAL modifier
	// amount on generic.movement_speed per effect level, cited from
	// StatusEffects.java's SLOWNESS registration (-0.15F). Negative: high
	// enough levels can drive the multiplier negative, but
	// generic.movement_speed is a ClampedEntityAttribute with a 0.0 floor
	// (EntityAttributes.java's MOVEMENT_SPEED registration), so the final
	// speed clamps at exactly zero rather than reversing.
	SlownessAmountPerLevel = -0.15
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
	// Cited from Java LivingEntity.getBaseWaterMovementSpeedMultiplier()
	// (returns 0.8F), the non-sprinting, non-Dolphin's-Grace horizontal drag
	// baseline used by travelInWater. Vanilla applies this value to the
	// horizontal axes only (vec3d.multiply(f, 0.8F, f)) and a separate,
	// always-0.8F multiplier to the vertical axis regardless of f, which
	// happens to be numerically identical to this constant — so applying
	// WaterDrag uniformly to X/Y/Z (as this codebase already did before
	// Dolphin's Grace existed) was already correct for Y; only the
	// horizontal (X/Z) multiplier needs to become effect-dependent.
	WaterDrag = 0.8 // Velocity multiplier per tick

	// DolphinsGraceWaterDragMultiplier overrides the horizontal-only water
	// drag multiplier (see WaterDrag) while Dolphin's Grace is active, cited
	// from Java LivingEntity.travelInWater's `if (hasStatusEffect(DOLPHINS_GRACE)) { f = 0.96F; }`.
	// Applies regardless of sprint state or amplifier; vertical drag is
	// unaffected (vanilla's fixed 0.8F for Y is untouched by this branch).
	DolphinsGraceWaterDragMultiplier = 0.96

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
	BoatUnderFlowingWaterVelocityMultiplier = 0.9     // Same as standard water
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

// Cobweb slowdown constants, cited from Java CobwebBlock.onEntityCollision
// (Yarn, pre-26.1) / WebBlock.entityInside (Mojang, 26.1+) — identical
// numeric values, confirmed against both mapping sets. Entity.slowMovement/
// makeStuckInBlock scales *that tick's* attempted movement by these
// per-axis multipliers, then resets velocity to zero entirely (not a
// continuous per-tick drag the way water/gravity effects work — see
// physics/state.go's isOverlappingCobweb for how this is applied).
const (
	// CobwebSlowdownX/Y/Z are the normal (no Weaving) per-axis multipliers.
	CobwebSlowdownX = 0.25
	CobwebSlowdownY = 0.05
	CobwebSlowdownZ = 0.25

	// WeavingCobwebSlowdownX/Y/Z apply instead of the above while Weaving is
	// active: half the severity, not a full-speed bypass (correcting
	// minecraft_movement_effects.md's "restores full walking speed" claim —
	// vanilla's own multiplier is still well below 1.0 on every axis).
	WeavingCobwebSlowdownX = 0.5
	WeavingCobwebSlowdownY = 0.25
	WeavingCobwebSlowdownZ = 0.5
)

// Elytra gliding constants, cited from Java LivingEntity.calcGlidingVelocity
// (Yarn, decompiled 1.21.11) — see physics/elytra.go for the full formula.
const (
	// GlideHorizontalDrag/GlideVerticalDrag are the final per-tick velocity
	// multipliers applied after calcGlidingVelocity's other adjustments
	// (Java: `oldVelocity.multiply(0.99F, 0.98F, 0.99F)`), replacing the
	// normal air Drag/inertia constants entirely while gliding.
	GlideHorizontalDrag = 0.99
	GlideVerticalDrag   = 0.98

	// GlideDiveFactor is the coefficient in calcGlidingVelocity's
	// "diving accelerates you" term (Java: `oldVelocity.y * -0.1 * h`).
	GlideDiveFactor = -0.1

	// GlideDiveGravityBlend is calcGlidingVelocity's gravity blend factor
	// (Java: `g * (-1.0 + h * 0.75)`) — at pitch 0 (looking level, h=1) this
	// reduces effective gravity to -0.25g; looking straight up/down (h=0)
	// applies the full -1.0g.
	GlideDiveGravityBlend = 0.75

	// GlideClimbFactor is the coefficient in calcGlidingVelocity's
	// looking-upward vertical boost term (Java: `e * -sin(f) * 0.04`).
	GlideClimbFactor = 0.04

	// GlideClimbVerticalMultiplier scales GlideClimbFactor's vertical
	// component specifically (Java: `i * 3.2`) — the upward push from
	// pitching up is more than 3x the horizontal pull-back it costs.
	GlideClimbVerticalMultiplier = 3.2

	// GlideHorizontalEaseFactor is calcGlidingVelocity's final per-tick ease
	// of horizontal velocity toward the look direction (Java: `... * 0.1`).
	GlideHorizontalEaseFactor = 0.1
)

// Firework rocket boost constants, cited from Java
// FireworkRocketEntity.tick()'s shooter-velocity nudge (Yarn, decompiled
// 1.21.11) — applied every tick a firework used while gliding is alive and
// attached to the shooter. See physics/elytra.go's FireworkBoostVelocity.
const (
	// FireworkBoostBlend is the flat per-tick nudge toward the look
	// direction (Java: `vec3d.x * 0.1`, applied per axis including Y).
	FireworkBoostBlend = 0.1

	// FireworkBoostTarget is the target speed (in the look direction) the
	// ease term pulls velocity toward (Java: `vec3d.x * 1.5`).
	FireworkBoostTarget = 1.5

	// FireworkBoostEase is the ease factor applied to the gap between
	// current velocity and FireworkBoostTarget (Java: `... * 0.5`) — a much
	// stronger per-tick pull than gliding's own 0.1 horizontal ease.
	FireworkBoostEase = 0.5
)

// Minecart physics constants
// Source: net/minecraft/entity/vehicle/DefaultMinecartController.java (1.21.2+)
//
//	net/minecraft/entity/vehicle/AbstractMinecartEntity.java (1.21.1)
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

// Pig riding constants
const (
	// PigBaseMovementSpeed is the default movement speed for a ridden pig.
	// Vanilla generic.movement_speed for pigs (PigEntity.createPigAttributes: 0.25).
	PigBaseMovementSpeed = 0.25

	// PigSaddledSpeedMultiplier scales the movement_speed attribute for a ridden pig.
	// Java PigEntity.getSaddledSpeed():
	//
	//	return (float)(getAttributeValue(MOVEMENT_SPEED) * 0.225 * saddledComponent.getMovementSpeedMultiplier());
	//
	// Vanilla multiplies the raw attribute by 0.225 before applying the sinusoidal
	// boost. Without this factor, the ridden pig used the full attribute value and
	// moved far too fast.
	PigSaddledSpeedMultiplier = 0.225

	// PigBoostSinAmplitude is the amplitude of the sinusoidal speed boost from
	// using a carrot_on_a_stick, matching Java SaddledComponent.getMovementSpeedMultiplier():
	//
	//	1.0F + 1.15F * sin(boostedTime / boostTime * PI)
	//
	// The boost is triggered by "using" (right-click) the carrot_on_a_stick,
	// NOT by merely holding it.
	PigBoostSinAmplitude = 1.15

	// PigInWaterBaseSpeed is the base horizontal movement speed acceleration
	// applied to a ridden pig in water. Mirrors Java LivingEntity.travelInWater
	// where g = 0.02F (the fallback when WATER_MOVEMENT_EFFICIENCY is 0, as it
	// is for pigs which have no water efficiency attribute).
	PigInWaterBaseSpeed = 0.02
)

// Horse riding constants (Java AbstractHorseEntity, 1.21.11).
// Horse jump motion is client-authoritative in vanilla: the client calls
// setJumpStrength (arming jumpStrength), tickControlled fires jump() on the
// next on-ground tick, and the resulting Y velocity is shipped via VehicleMove.
// The START_RIDING_JUMP packet only drives animation/anger server-side.
const (
	// HorseBaseJumpStrength is the default JUMP_STRENGTH attribute for horses.
	// Java AbstractHorseEntity.createBaseHorseAttributes():
	//
	//	.add(EntityAttributes.JUMP_STRENGTH, 0.7)
	HorseBaseJumpStrength = 0.7

	// HorseJumpForwardBoost scales the forward velocity added when jumping while
	// moving forward. Java AbstractHorseEntity.jump():
	//
	//	this.setVelocity(this.getVelocity().add(-0.4F * sin(yaw) * strength,
	//	                                       0.0,
	//	                                        0.4F * cos(yaw) * strength));
	// (only when movementInput.z > 0). The 0.4 factor is applied per-axis after
	// the yaw rotation, so the horizontal boost magnitude is 0.4 * strength.
	HorseJumpForwardBoost = 0.4
)

// RidingAirborneAcceleration is the horizontal movement-input acceleration
// applied to a ridden entity (horse, camel, etc.) while airborne. Vanilla gives
// only limited air control (~0.02) versus the full ground movement speed, so a
// falling mount keeps its horizontal momentum but does not accelerate to many
// times its ground speed and "fly" forward. Ground movement is unaffected.
const RidingAirborneAcceleration = 0.02

// Strider riding constants
const (
	// StriderBaseMovementSpeed is the default movement speed for a ridden strider.
	// Java StriderEntity.createStriderAttributes(): EntityAttributes.MOVEMENT_SPEED = 0.175.
	StriderBaseMovementSpeed = 0.175

	// StriderHurtByWater mirrors Java StriderEntity.hurtByWater() which returns true.
	// Striders take damage from water, rain, and splash water bottles.
	// This constant is a stub for future damage-system integration; the movement
	// executor does not currently apply water damage.
	StriderHurtByWater = true

	// StriderWarmSpeedMultiplier scales saddled speed when the strider is warm (on lava).
	// Java StriderEntity.getSaddledSpeed(): this.isCold() ? 0.35F : 0.55F.
	StriderWarmSpeedMultiplier = 0.55

	// StriderColdSpeedMultiplier scales saddled speed when the strider is cold (off lava).
	// Java StriderEntity.getSaddledSpeed(): this.isCold() ? 0.35F : 0.55F.
	StriderColdSpeedMultiplier = 0.35

	// StriderSuffocatingModifier is the ADD_MULTIPLIED_BASE modifier applied to the
	// movement_speed attribute when the strider is cold. Effective speed = base * (1 + modifier).
	// Java StriderEntity.SUFFOCATING_MODIFIER: -0.34F, Operation.ADD_MULTIPLIED_BASE.
	StriderSuffocatingModifier = -0.34

	// StriderOnLavaGravity is the gravitational acceleration when strider is on lava surface.
	// Striders float on lava without sinking (updateFloating sets onGround=true).
	StriderOnLavaGravity = 0.0

	// StriderOffLavaGravity is the gravitational acceleration when strider is off lava.
	// Java: standard entity gravity = 0.08 blocks/tick².
	StriderOffLavaGravity = -0.08

	// StriderLavaBobUpVelocity is the upward velocity added when a strider is submerged
	// in lava (not floating on the surface).
	// Java StriderEntity.updateFloating(): velocity.multiply(0.5).add(0, 0.05, 0).
	StriderLavaBobUpVelocity = 0.05

	// StriderLavaBobDrag is the velocity multiplier applied when a strider is submerged
	// in lava (bobbing up).
	// Java StriderEntity.updateFloating(): velocity.multiply(0.5).
	StriderLavaBobDrag = 0.5

	// StriderBoostSinAmplitude is the amplitude of the sinusoidal speed boost from
	// using warped_fungus_on_a_stick.
	// Java SaddledComponent.getMovementSpeedMultiplier(): 1.0F + 1.15F * sin(...).
	StriderBoostSinAmplitude = 1.15

	// StriderBoostMinDuration is the minimum boost duration in ticks.
	// Java SaddledComponent.boost(): random.nextInt(841) + 140, so minimum is 140.
	// But getBoostTime checks boostedTime > boostTime, so effective min = 141.
	StriderBoostMinDuration = 140

	// StriderBoostRandomRange is the random range added to min duration.
	// Java SaddledComponent.boost(): random.nextInt(841) + 140.
	StriderBoostRandomRange = 841
)

// Nautilus riding constants (Java AbstractNautilusEntity, 1.21.11).
// Per-type base movement_speed and dash constants live in models/nautilus.go
// alongside NautilusState; these are the medium speed/drag factors used by the
// ridden-movement predictor.
const (
	// NautilusInWaterSpeedMultiplier is the saddled-speed multiplier in water.
	// Java getSaddledSpeed(): 0.0325 * MOVEMENT_SPEED when isTouchingWater().
	NautilusInWaterSpeedMultiplier = 0.0325

	// NautilusOnLandSpeedMultiplier is the saddled-speed multiplier on land.
	// Java getSaddledSpeed(): 0.02 * MOVEMENT_SPEED when not touching water.
	NautilusOnLandSpeedMultiplier = 0.02

	// NautilusWaterDrag is the per-tick velocity multiplier in water.
	// Java AbstractNautilusEntity.travelInWater(): velocity.multiply(0.9) on all axes.
	NautilusWaterDrag = 0.9
)

// Happy ghast riding constants (Java HappyGhastEntity, 1.21.6+). Source:
// net/minecraft/entity/passive/HappyGhastEntity.java, decompiled and read
// directly (mc-data-gen/extractedSrc/<version>/...) rather than inferred —
// see docs/plans/physics_and_movement_engine_enhancement/PHASE_6_PLAN.md §3.
const (
	// HappyGhastControlledMovementMultiplier is getControlledMovementInput's
	// fixed scale factor applied to the raw (sideways, vertical, forward)
	// input vector, alongside the flying_speed attribute. Note this input is
	// NOT clamped to unit length before this multiply — the clamp happens
	// later, inside movementInputToVelocity, on the already-scaled vector.
	// Java: return new Vec3d(f, h, g).multiply(3.9F * getAttributeValue(FLYING_SPEED));
	HappyGhastControlledMovementMultiplier = 3.9

	// HappyGhastJumpVerticalBoost is added to the vertical input component
	// while the pilot holds jump, before the 3.9*flying_speed scale — not a
	// separate "ascend" branch, just an offset stacked on whatever the
	// pitch-driven vertical component already was.
	// Java: if (controllingPlayer.isJumping()) { h += 0.5F; }
	HappyGhastJumpVerticalBoost = 0.5

	// HappyGhastTravelSpeedFactor converts the flying_speed attribute into
	// the speed argument travel() passes to travelFlying.
	// Java travel(): float f = flying_speed * 5.0F / 3.0F;
	HappyGhastTravelSpeedFactor = 5.0 / 3.0

	// HappyGhastFlightDrag is travelFlying's per-tick velocity multiplier in
	// the "not touching water or lava" branch — the only branch that ever
	// applies here, since the happy ghast dismounts its rider on submersion
	// (server-side, EntityTypeTags.DISMOUNTS_UNDERWATER + LivingEntity's
	// tickWaterBreathing dismount check) rather than the ridden ghast itself
	// ever running the water/lava travel math.
	// Java: this.setVelocity(this.getVelocity().multiply(0.91F));
	//
	// Notably absent from this branch: any gravity term at all. Flight is
	// genuinely zero-gravity, not "reduced gravity" — confirmed by its
	// absence in travelFlying's source, not inferred from behavior.
	HappyGhastFlightDrag = 0.91

	// HappyGhastDefaultFlyingSpeed is the vanilla flying_speed attribute
	// default, used as the fallback-of-fallback when neither the live server
	// value nor Phase 7's data-driven default is available.
	// Java createHappyGhastAttributes(): .add(EntityAttributes.FLYING_SPEED, 0.05)
	HappyGhastDefaultFlyingSpeed = 0.05

	// HappyGhastYawEaseFactor is how far the ghast's own yaw closes the gap
	// toward the pilot's look yaw each tick — much slower than the
	// equivalent nautilus factor (0.5).
	// Java tickControlled: f += MathHelper.wrapDegrees(pilotYaw - f) * 0.08F;
	HappyGhastYawEaseFactor = 0.08

	// HappyGhastPitchFactor halves the pilot's look pitch for the ghast's
	// own pitch (used only in the VehicleMove packet, like nautilus).
	// Java getGhastRotation(): new Vec2f(controllingEntity.getPitch() * 0.5F, ...)
	HappyGhastPitchFactor = 0.5

	// HappyGhastWidth and HappyGhastHeight are the adult hitbox dimensions.
	// Java: default_dimensions width=4, height=4 (also confirmed directly
	// against mc-data-gen's extracted entity JSON).
	HappyGhastWidth  = 4.0
	HappyGhastHeight = 4.0
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
