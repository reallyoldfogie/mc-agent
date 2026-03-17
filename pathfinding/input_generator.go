package pathfinding

import (
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// PhysicsState is an alias to models.PhysicsState for convenience.
type PhysicsState = models.PhysicsState

// Inputs is an alias to models.Inputs for convenience.
type Inputs = models.Inputs

// InputGenerator converts path steps to physics inputs.
// This is the bridge between high-level pathfinding (discrete blocks)
// and low-level physics simulation (continuous movement with inputs).
// type InputGenerator interface {
// 	// GenerateInputs converts a path step to player inputs for physics simulation.
// 	// currentState: Current physics state (position, velocity, etc.)
// 	// targetStep: The path step we're trying to reach
// 	// runTime: How long we've been executing this step
// 	GenerateInputs(currentState PhysicsState, targetStep PathStep, runTime time.Duration) Inputs

// 	// EstimateTicksRequired estimates how many ticks to complete a movement step.
// 	// This is used for path execution planning.
// 	EstimateTicksRequired(step PathStep, currentState PhysicsState) int
// }

// DefaultInputGenerator is the standard implementation of InputGenerator.
// It uses vanilla Minecraft movement physics and timing.
type DefaultInputGenerator struct {
	// No configuration needed for now
}

// NewInputGenerator creates a new default input generator.
func NewInputGenerator() models.InputGenerator {
	return &DefaultInputGenerator{}
}

// GenerateInputs implements the InputGenerator interface.
// Based on phys archive bot/path/path.go:107-191 (Tile.Inputs() logic).
func (ig *DefaultInputGenerator) GenerateInputs(
	currentState PhysicsState,
	targetStep PathStep,
	runTime time.Duration,
) Inputs {
	// Get current position, rotation, and velocity
	pos, _, pitch, _ := currentState.GetPosition()
	vel := currentState.GetVelocity()

	// Convert target to block center for precise movement types
	// This ensures the agent aims for the center of the target block, not the corner
	targetPos := targetStep.Position
	switch targetStep.Movement {
	case AscendJump, DiagonalAscend, JumpToClimb, Jump2ToClimb:
		// These movements need precise landing - aim for block center
		targetPos = models.V3{
			X: math.Floor(targetStep.Position.X) + 0.5,
			Y: targetStep.Position.Y,
			Z: math.Floor(targetStep.Position.Z) + 0.5,
		}
	}

	// Calculate delta from current position to target
	deltaPos := targetPos.Sub(pos)

	// Calculate desired yaw based on movement direction
	// NOTE: Trajectory is simulated in local space with Z=forward, X=0
	// Yaw formula must convert from world delta to firing direction
	// atan2(-dx, dz) accounts for the coordinate system rotation
	desiredYaw := math.Atan2(-deltaPos.X, deltaPos.Z) * 180.0 / math.Pi

	// Calculate throttle as normalized direction vector (simpler and more direct than yaw conversion)
	// IMPORTANT: Throttle represents a world-space direction vector, NOT player-relative WASD controls
	// The physics engine will apply this as world-absolute velocity, not rotate it by yaw
	horizontalDist := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)
	throttleX := 0.0
	throttleZ := 0.0
	if horizontalDist > 0.01 {
		throttleX = deltaPos.X / horizontalDist // Normalized X component
		throttleZ = deltaPos.Z / horizontalDist // Normalized Z component
	}

	out := Inputs{
		ThrottleX:      throttleX,
		ThrottleZ:      throttleZ,
		Yaw:            desiredYaw, // Face toward target (for visual appearance)
		Pitch:          pitch,      // Keep pitch stable (no random variations)
		Jump:           false,
		Sprint:         false,
		Sneak:          false,
		ClimbDirection: 0.0, // No climbing by default
	}

	// Movement-specific input adjustments
	switch targetStep.Movement {
	case Sprint:
		// Sprint on same level
		out.Sprint = true

	case Traverse, DiagonalTraverse:
		// Simple horizontal movement - default inputs are fine
		// No special logic needed

	case EnterClimb:
		// Enter climbable block (vine/ladder) from the side
		// CRITICAL: Activate climbing once in/near the climbable hitbox to prevent falling
		// This is especially important when entering from above (e.g., vine at Y=5, feet at Y=6)

		// Aim for center of the climbable block
		climbCenterX := targetStep.Position.X + 0.5
		climbCenterZ := targetStep.Position.Z + 0.5

		deltaX := climbCenterX - pos.X
		deltaZ := climbCenterZ - pos.Z
		dist2 := deltaX*deltaX + deltaZ*deltaZ

		// Once close to the climbable block center, engage climbing
		// This prevents falling through when entering from above
		if dist2 < 0.5*0.5 {
			// Very close - engage climbing to grab the vine/ladder
			// ClimbDirection = 0.5 holds position (neither ascending nor descending)
			// This engages climbing mode and prevents falling
			out.ClimbDirection = 0.5
		}

		// Adjust throttle to center on the block
		if dist2 > 0.01 {
			dist := math.Sqrt(dist2)
			out.ThrottleX = deltaX / dist
			out.ThrottleZ = deltaZ / dist
			out.Yaw = math.Atan2(-deltaX, deltaZ) * 180.0 / math.Pi
		}

	case ExitClimb:
		// Exit from ladder onto adjacent platform
		// CRITICAL: Must maintain ClimbDirection to prevent falling until OFF the ladder
		// Strategy:
		// 1. Keep ClimbDirection = 1.0 (hold position) while still on ladder
		// 2. Move horizontally toward target platform
		// 3. Once off the ladder, ClimbDirection = 0.0 is safe

		// Check if we're still on a climbable block (ladder)
		// The ladder is at the 'from' position, target is adjacent
		// If we haven't moved horizontally much yet, we're still on the ladder
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		if dist2 > 0.5 {
			// We've moved significantly toward target - likely off the ladder now
			out.ClimbDirection = 0.0 // Safe to stop climbing
		} else {
			// Still on or very close to ladder - maintain position
			out.ClimbDirection = 1.0 // Hold position on ladder while moving horizontally
		}

		out.Sprint = true // Sprint to move quickly off the ladder

		// If close horizontally but need to step up, jump to help
		if dist2 < 0.8 && deltaPos.Y > 0.3 {
			out.Jump = true
		}

	case AscendStairs, DescendStairs:
		// Natural stair traversal - no jumping needed
		// Walk normally and let the physics handle the stair step
		// No special logic needed - default inputs work

	case AscendJump, DiagonalAscend:
		// Jump up one block onto a platform
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// Jump when we're close to the target and ABOVE us
		// dist2 < 1.75: Within jump range
		// deltaPos.Y > 0.2: Target is above us (need to jump up)
		out.Jump = dist2 < 1.75 && deltaPos.Y > 0.2

		// Turn off throttle if we're stuck (close, target above us, no vertical velocity)
		if dist2 < 1 && deltaPos.Y > 0 && vel.Y == 0 {
			out.ThrottleX, out.ThrottleZ = 0, 0
		}

	case JumpToClimb:
		// Jump up to access elevated climbable (ladder/vine 1 block above ground)
		// Common Minecraft pattern: ladder placed 1 block up to prevent mobs from climbing
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// Jump when close to target (within 1.75 blocks)
		// The target is 1 block up (Y+1) where the ladder starts
		out.Jump = dist2 < 1.75 && deltaPos.Y > 0.2

		// Once we're rising/jumping, start engaging climb to grab the ladder
		if vel.Y > 0 || (dist2 < 0.5 && deltaPos.Y > 0) {
			out.ClimbDirection = 1.0 // Engage climbing to grab ladder
		}

		// If we're at the same height as target, we've grabbed the ladder - just climb
		if deltaPos.Y <= 0.5 && deltaPos.Y >= -0.1 {
			out.ClimbDirection = 1.0 // Hold on the ladder
		}

	case Jump2ToClimb:
		// Sprint jump 2 blocks to land on/grab climbable (vine/ladder across gap)
		// Similar to Jump2 but engages climbing once near target
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// Sprint jump - need sprint for the 2-block distance
		out.Sprint = true

		// Jump window: 1.5 to 1.78 blocks from target
		out.Jump = dist2 > 1.5 && dist2 < 1.78

		// Once we're past the midpoint, start engaging climb to grab vines
		if dist2 < 1.0 {
			out.ClimbDirection = 1.0 // Engage climbing to grab ladder/vines
		}

		// If rising and close, definitely grab
		if vel.Y > 0 && dist2 < 0.5 {
			out.ClimbDirection = 1.0
		}

	case Descend:
		// Drop down (1-3 blocks)
		// Just walk forward and let gravity do the work
		// Default inputs are fine

	case Jump2:
		// Jump across 2-block gap
		// Timing is critical: jump when at specific distance
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// Jump window: 1.5 to 1.78 blocks from target
		// This gives the optimal jump trajectory
		out.Jump = dist2 > 1.5 && dist2 < 1.78
		out.Sprint = true // 2-block gaps require sprint jump in vanilla

	case Climb:
		// Climb ladder/vine
		dist2 := math.Sqrt(deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z)

		// Set climb direction based on target position
		// IMPORTANT: The target position is the ladder BLOCK (e.g., Y=6),
		// but when climbing, the player's feet should end up ON TOP of that block (Y=7).
		// So we calculate deltaY based on the adjusted target (block Y + 1.0).
		adjustedTargetY := targetStep.Position.Y + 1.0
		deltaY := adjustedTargetY - pos.Y

		if deltaY > 0.1 {
			// Still need to climb up to reach top of block
			out.ClimbDirection = 1.0
		} else if deltaY < -0.1 {
			// Too high, need to descend
			// NOTE: Do NOT sneak while descending - sneaking prevents descent on ladders
			out.ClimbDirection = -1.0
		} else {
			// At target height (on top of block) - hold position
			// In vanilla Minecraft, if no key pressed, player falls down ladder
			// Enable sneaking to hold position and prevent falling past target
			out.ClimbDirection = 0.0
			out.Sneak = true
		}

		// If far from ladder and below target, approach the ladder first
		if dist2 > (0.8*0.8) && deltaPos.Y < 0 {
			// Approach phase: aim for the ladder block center
			// No special throttle adjustment needed
		} else {
			// Climbing phase: center on ladder
			// Aim for the center of the target block
			ladderCenterX := targetStep.Position.X + 0.5
			ladderCenterZ := targetStep.Position.Z + 0.5

			deltaX := ladderCenterX - pos.X
			deltaZ := ladderCenterZ - pos.Z
			dist := math.Sqrt(deltaX*deltaX + deltaZ*deltaZ)
			if dist > 0.01 {
				out.ThrottleX = deltaX / dist
				out.ThrottleZ = deltaZ / dist
			}
			// Calculate yaw for horizontal rotation
			// NOTE: Trajectory is simulated in local space with Z=forward, X=0
			// Yaw formula must convert from world delta to firing direction
			// atan2(-dx, dz) accounts for the coordinate system rotation
			ladderYaw := math.Atan2(-deltaX, deltaZ) * 180.0 / math.Pi
			out.Yaw = ladderYaw
		}

	case Swim:
		// Swimming in water (horizontal)
		// Similar to traverse but slower
		// Default inputs work, physics will handle water resistance

	case SwimUp:
		// Swimming up in water
		out.Jump = true // Jump button makes you swim up

	case SwimDown:
		// Swimming down in water
		// Sneak button makes you swim down, but sneaking on land prevents the transition from solid to water.
		if currentState.IsInWater() {
			out.Sneak = true
		}

	case ExitWater:
		// Exiting water onto adjacent solid ground
		// Must jump to break water surface and transition to standing on ground
		out.Jump = true

	// 2-block drops
	case Drop2North, Drop2South, Drop2East, Drop2West:
		// 2-block drop with direction
		// Same as Descend - walk forward and let gravity do the work
		// Default inputs are fine

	// True diagonal traverses
	case TraverseNorthEast, TraverseNorthWest, TraverseSouthEast, TraverseSouthWest:
		// Diagonal movement
		// Default inputs already handle this (atan2 calculates correct angle)
		// No special logic needed

	// Sneaking movements
	case Sneak, SneakThrough, SneakTraverse:
		// Sneaking movements (1.5 block gaps, edge safety)
		out.Sneak = true // Enable sneaking for reduced hitbox and slow speed
		// Default throttle toward target is fine
	}

	return out
}

// EstimateTicksRequired estimates ticks needed to complete a movement step.
// This is a rough estimate used for planning and timeout detection.
func (ig *DefaultInputGenerator) EstimateTicksRequired(
	step PathStep,
	currentState PhysicsState,
) int {
	// Get current position
	pos, _, _, _ := currentState.GetPosition()

	// Calculate distance to target
	dist := pos.DistanceTo(step.Position)

	// Estimate based on movement type
	// These are rough estimates based on vanilla Minecraft speeds
	switch step.Movement {
	case Traverse, EnterClimb:
		// Walking speed: ~4.3 blocks/second = ~0.215 blocks/tick (at 20 TPS)
		// Add buffer for acceleration
		return int(dist/0.20) + 10

	case Sprint:
		// Sprint speed: ~5.6 blocks/second = ~0.28 blocks/tick
		return int(dist/0.28) + 8

	case DiagonalTraverse:
		// Diagonal is sqrt(2) longer but same speed
		return int(dist/0.20) + 10

	case AscendStairs, DescendStairs:
		// Natural stair traversal - same speed as walking
		// Just horizontal distance, no jump time needed
		return int(dist / 0.20)

	case AscendJump, DiagonalAscend:
		// Jump up takes ~10 ticks (0.5 seconds)
		// Plus horizontal travel time
		horizontalDist := math.Sqrt(
			(step.Position.X-pos.X)*(step.Position.X-pos.X) +
				(step.Position.Z-pos.Z)*(step.Position.Z-pos.Z),
		)
		return int(horizontalDist/0.20) + 15

	case Descend:
		// Falling: ~0.08 blocks/tick² (gravity)
		// For 1 block: ~5 ticks, 2 blocks: ~7 ticks, 3 blocks: ~9 ticks
		verticalDist := pos.Y - step.Position.Y
		fallTime := int(math.Sqrt(verticalDist/0.04)) + 5

		horizontalDist := math.Sqrt(
			(step.Position.X-pos.X)*(step.Position.X-pos.X) +
				(step.Position.Z-pos.Z)*(step.Position.Z-pos.Z),
		)
		horizontalTime := int(horizontalDist / 0.20)

		// Return max of fall time and horizontal time
		if fallTime > horizontalTime {
			return fallTime + 5
		}
		return horizontalTime + 5

	case Jump2:
		// 2-block gap jump takes ~20 ticks
		return 25

	case Climb:
		// Ladder climbing: ~0.15 blocks/tick (slower than walking)
		return int(dist/0.12) + 20

	case JumpToClimb:
		// Jump up to elevated ladder - similar to AscendJump
		// Takes ~10 ticks for jump + a bit extra for ladder grab
		horizontalDist := math.Sqrt(
			(step.Position.X-pos.X)*(step.Position.X-pos.X) +
				(step.Position.Z-pos.Z)*(step.Position.Z-pos.Z),
		)
		return int(horizontalDist/0.20) + 20

	case Jump2ToClimb:
		// Sprint jump 2 blocks to grab climbable - similar to Jump2
		// Takes ~25 ticks (sprint jump distance + grab time)
		return 30

	case Swim:
		// Swimming: ~0.12 blocks/tick (even slower)
		return int(dist/0.10) + 15

	case SwimUp:
		// Swimming up: ~0.1 blocks/tick
		return int(dist/0.08) + 20

	case SwimDown:
		// Swimming down: ~0.15 blocks/tick (faster with sneak)
		return int(dist/0.12) + 15

	case ExitWater:
		// Exiting water with jump: ~15 ticks for jump + transition
		// Quick exit movement
		return 15

	// Exiting climb movements
	case ExitClimb:
		// Same as Climb - ladder speed is ~0.15 blocks/tick
		return int(dist/0.12) + 20

	// 2-block drops
	case Drop2North, Drop2South, Drop2East, Drop2West:
		// 2-block fall - similar to Descend but longer
		verticalDist := pos.Y - step.Position.Y
		if verticalDist < 0.1 {
			verticalDist = 2.0 // Assume 2 blocks if calculation fails
		}
		fallTime := int(math.Sqrt(verticalDist/0.04)) + 5

		horizontalDist := math.Sqrt(
			(step.Position.X-pos.X)*(step.Position.X-pos.X) +
				(step.Position.Z-pos.Z)*(step.Position.Z-pos.Z),
		)
		horizontalTime := int(horizontalDist / 0.20)

		if fallTime > horizontalTime {
			return fallTime + 5
		}
		return horizontalTime + 5

	// True diagonals
	case TraverseNorthEast, TraverseNorthWest, TraverseSouthEast, TraverseSouthWest:
		// Diagonal traverse - same speed as normal traverse
		return int(dist/0.20) + 10

	// Sneaking movements
	case Sneak, SneakThrough, SneakTraverse:
		// Sneaking is 30% speed = ~0.065 blocks/tick
		// Much slower, so increase estimate
		return int(dist/0.06) + 20

	default:
		// Unknown movement type, use conservative estimate
		return int(dist/0.15) + 20
	}
}
