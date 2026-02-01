package pathfinding

import (
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// IsComplete checks if a movement step has been completed based on current position.
// Returns true when the bot is "close enough" to the target position.
//
// Different movement types have different completion thresholds:
//   - Horizontal movements: Within 0.18 blocks horizontally, -0.065 to +0.08 vertically
//   - Vertical movements (ladders, drops): Within 0.2 blocks horizontally, Y threshold varies
//   - Jump movements: Slightly tighter horizontal tolerance
//
// Based on phys archive bot/path/path.go:193-211 (Tile.IsComplete() logic).
func IsComplete(currentPos models.V3, targetStep PathStep) bool {
	// For jump movements, adjust target to block center to match input generator behavior
	// This ensures completion checker and input generator use the same target coordinates
	targetPos := targetStep.Position
	switch targetStep.Movement {
	case AscendJump, DiagonalAscend, JumpToClimb, Jump2ToClimb, Jump2:
		// These movements need precise landing - adjust to block center
		targetPos = models.V3{
			X: math.Floor(targetStep.Position.X) + 0.5,
			Y: targetStep.Position.Y,
			Z: math.Floor(targetStep.Position.Z) + 0.5,
		}
	}

	// Calculate delta from current position to (adjusted) target
	deltaPos := V3Sub(targetPos, currentPos)

	// Movement-specific completion thresholds
	switch targetStep.Movement {
	case Descend:
		// Drop down movements (1-3 blocks)
		// Tighter horizontal tolerance, only check we're not above target
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (2*0.2*0.25) && deltaPos.Y <= 0.05

	case Drop2North, Drop2South, Drop2East, Drop2West:
		// 2-block drops (directional)
		// Same as regular descend but for 2-block falls
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (2*0.2*0.25) && deltaPos.Y <= 0.05

	case Climb:
		// Ladder climbing (both ascent and descent)
		//
		// CRITICAL: The target position is the ladder BLOCK coordinates (e.g., 108, 6, 100),
		// but when climbing, the player's FEET should be ON TOP of that block (Y = 7.0).
		// This is because in Minecraft, when you finish climbing a ladder block, you stand
		// on top of it, not inside it.
		//
		// For horizontal position, player should be centered at (108.5, 100.5).
		deltaFromCenter := models.V3{
			X: (targetStep.Position.X + 0.5) - currentPos.X,
			Y: (targetStep.Position.Y + 1.0) - currentPos.Y, // +1.0 because feet on TOP of block
			Z: (targetStep.Position.Z + 0.5) - currentPos.Z,
		}
		horizontalDist2 := deltaFromCenter.X*deltaFromCenter.X + deltaFromCenter.Z*deltaFromCenter.Z

		// Complete when horizontally centered and vertically on top of block
		// For ascending: currentPos.Y should be >= targetPos.Y + 0.9 (on top)
		// For descending: different logic (TODO if needed)
		return horizontalDist2 < (0.2*0.2) && math.Abs(deltaFromCenter.Y) <= 0.15

	case ExitClimb:
		// Exiting ladder onto adjacent platform
		// Complete when:
		// 1. Horizontally close to target (may be on platform or stepping up)
		// 2. Vertically within range (may be at same level or one block up)
		// 3. No longer on a climbable block (successfully exited)
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z

		// Allow both same level exit and step-up exit
		// Target Y is the platform level, but agent might land on top of it (Y+1)
		// So we check: -0.2 (slightly below) to +1.2 (one block above)
		return horizontalDist2 < (0.3*0.3) &&
			deltaPos.Y >= -0.2 &&
			deltaPos.Y <= 1.2

	case JumpToClimb:
		// Jump up to elevated climbable (ladder/vine 1 block above ground)
		// Target is the ladder/vine block position (e.g., Y=1 for ground level entry)
		// Complete when player is INSIDE the climbable block (not just near it)
		//
		// For climbing to work, player must actually be inside the block:
		// - Block at (X, Y, Z) covers X to X+1, Z to Z+1
		// - Player must be within the block boundaries
		// - deltaPos = target - current
		// - For player inside block: target.X <= current.X <= target.X+1
		// - This means: 0 >= deltaPos.X >= -1 (i.e., deltaPos.X in [-1, 0])
		// - Same for Z: deltaPos.Z in [-1, 0]
		//
		// Allow small tolerance (0.1) outside block edges for hitbox overlap
		insideBlockX := deltaPos.X >= -1.1 && deltaPos.X <= 0.1
		insideBlockZ := deltaPos.Z >= -1.1 && deltaPos.Z <= 0.1
		// Y should be close to target (within the block or slightly above/below)
		closeToTargetY := deltaPos.Y >= -0.5 && deltaPos.Y <= 0.5

		return insideBlockX && insideBlockZ && closeToTargetY

	case Jump2ToClimb:
		// Sprint jump 2 blocks to grab climbable (vine/ladder across gap)
		// Target is the climbable block position
		// Complete when player is inside or very close to the climbable block
		//
		// Similar to JumpToClimb but player may arrive with more momentum
		// Allow slightly larger tolerance due to sprint jump
		insideBlockX := deltaPos.X >= -1.2 && deltaPos.X <= 0.2
		insideBlockZ := deltaPos.Z >= -1.2 && deltaPos.Z <= 0.2
		// Y tolerance - player may be slightly above/below during grab
		closeToTargetY := deltaPos.Y >= -0.7 && deltaPos.Y <= 0.7

		return insideBlockX && insideBlockZ && closeToTargetY

	case SwimUp:
		// Swimming upward in water
		// NOTE: Ladder/vine ascent uses Climb movement type, not SwimUp
		// Complete when we're at or above the target Y
		// deltaPos.Y = target.Y - current.Y
		// If we're above target, deltaPos.Y is negative (current.Y > target.Y)
		// So we want deltaPos.Y <= 0
		return deltaPos.Y <= 0

	case Jump2:
		// Jump across 2-block gap
		// Slightly tighter horizontal tolerance, allow being slightly below target
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (0.22*0.22) && deltaPos.Y >= -0.065

	case SneakThrough, SneakTraverse:
		// Sneaking movements (slow, precise)
		// Tighter tolerance due to slower movement speed
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (0.15*0.15) && // Tighter horizontal (0.15 vs 0.18)
			deltaPos.Y >= -0.05 && // Tighter vertical range
			deltaPos.Y <= 0.05

	case AscendStairs, DescendStairs:
		// Stair movements require special Y tolerance
		//
		// CRITICAL: Stairs have complex collision (e.g., bottom-half stairs have surfaces
		// at both Y=0.5 and Y=1.0). Paths target the maximum Y (1.0), but agents may land
		// on intermediate surfaces (0.5) and need horizontal movement to reach the top.
		//
		// deltaPos.Y = targetY - currentY
		// - If positive: We're below target (need to go up)
		// - If negative: We're above target (need to go down)
		//
		// We must allow being up to 0.6 blocks below target for stairs, so that:
		// - Agent at Y=0.5 targeting Y=1.0 (deltaPos.Y = 0.5) considers step "complete"
		// - Agent moves to next step (with horizontal movement)
		// - Horizontal movement triggers natural stair ascent to Y=1.0
		//
		// Without this tolerance, agent gets stuck at Y=0.5 with zero throttle!
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (0.18*0.18) &&
			deltaPos.Y >= -0.6 && // Can be up to 0.6 blocks above target
			deltaPos.Y <= 0.6 // Can be up to 0.6 blocks below target

	default:
		// Default completion threshold for Traverse, Ascend, DiagonalTraverse, etc.
		//
		// Horizontal threshold: 0.18 blocks (allows some wiggle room)
		// Vertical threshold: -0.065 to +0.08 (accounts for step height and ground detection)
		//
		// The vertical range is asymmetric because:
		// - We can be slightly below target (-0.065) due to partial blocks (slabs, stairs)
		// - We can be slightly above target (+0.08) due to step-up mechanics
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z

		// Note: We don't currently track HalfBlock flag from phys archive
		// If we were on a half-block (slab), we'd adjust yLowerCutoff by -0.5
		// For now, we use the standard threshold
		yLowerCutoff := -0.065

		// TODO: Add half-block detection for more accurate completion
		// This would require querying the block shape at currentPos
		// if isHalfBlock(currentPos) {
		//     yLowerCutoff -= 0.5
		// }

		return horizontalDist2 < (0.18*0.18) &&
			deltaPos.Y >= yLowerCutoff &&
			deltaPos.Y <= 0.08
	}
}

// IsStuck checks if the bot appears to be stuck on a movement step.
// This is useful for detecting when a movement is failing and needs recovery.
//
// A bot is considered stuck if:
//   - It's been trying to complete the step for too long (based on estimated ticks)
//   - Its velocity is near zero
//   - It's not making progress toward the target
func IsStuck(
	currentPos models.V3,
	currentVel models.V3,
	targetStep PathStep,
	runTime int, // Ticks spent on this step
	estimatedTicks int, // Estimated ticks to complete
) bool {
	// If we haven't exceeded estimated time, we're not stuck yet
	if runTime < estimatedTicks*2 {
		return false
	}

	// Check if velocity is near zero (not moving)
	velocityMagnitude := currentVel.DistanceTo(models.V3{X: 0, Y: 0, Z: 0})
	if velocityMagnitude > 0.05 {
		// Still moving, not stuck
		return false
	}

	// Check if we're making progress
	// If we're very close to target, we're probably just settling into position
	deltaPos := V3Sub(targetStep.Position, currentPos)
	dist := deltaPos.DistanceTo(models.V3{X: 0, Y: 0, Z: 0})
	if dist < 0.5 {
		// Very close, probably fine
		return false
	}

	// Exceeded time, not moving, and not at target = stuck
	return true
}

// EstimateProgress returns a 0-1 value indicating how close we are to completing a step.
// 0.0 = at starting position, 1.0 = at target position, >1.0 = overshot target
//
// This is useful for:
//   - Progress visualization
//   - Detecting if movement is progressing normally
//   - Adaptive timeout calculation
//
// Note: This can return values > 1.0 if the bot has overshot the target.
func EstimateProgress(startPos, currentPos, targetPos models.V3) float64 {
	// Calculate total distance from start to target
	totalDist := startPos.DistanceTo(targetPos)
	if totalDist < 0.001 {
		// Start and target are the same (or extremely close)
		return 1.0
	}

	// Calculate distance traveled from start to current
	traveledDist := startPos.DistanceTo(currentPos)

	// Progress = distance traveled / total distance
	progress := traveledDist / totalDist

	// Note: We don't clamp to [0, 1] to allow detection of overshooting
	// Negative progress shouldn't happen (moving away from start), but we clamp it anyway
	if progress < 0 {
		return 0
	}

	return progress
}
