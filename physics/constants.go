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
	// Arrow physics (from agent/bowCommands.go)
	ArrowGravity      = 0.05 // Arrow gravity (different from player)
	ArrowDrag         = 0.99 // Arrow air resistance
	ArrowInitialSpeed = 3.25 // Max arrow speed (fully charged bow) - empirically determined for Minecraft 1.21.8

	// Snowball/Egg physics
	SnowballGravity      = 0.03 // Snowball/egg gravity
	SnowballDrag         = 0.99 // Snowball/egg drag
	SnowballInitialSpeed = 1.5  // Snowball/egg throw speed

	// Ender Pearl physics
	EnderPearlGravity      = 0.03 // Ender pearl gravity
	EnderPearlDrag         = 0.99 // Ender pearl drag
	EnderPearlInitialSpeed = 1.5  // Ender pearl throw speed

	// Splash Potion physics
	SplashPotionGravity      = 0.05 // Splash potion gravity
	SplashPotionDrag         = 0.99 // Splash potion drag
	SplashPotionInitialSpeed = 0.5  // Splash potion throw speed (slower arc)
)

// Block-specific slipperiness values (for future use)
// Base slipperiness is 0.6, but some blocks are different:
// - Ice: 0.98
// - Slime block: 0.8
// - Most blocks: 0.6
