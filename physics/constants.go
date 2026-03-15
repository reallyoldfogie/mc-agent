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

	// Gravity reduction in water
	// 0.0 = no gravity reduction, 1.0 = full gravity reduction
	WaterGravityFactor = 0.8 // Use 20% of normal gravity when in water
)
