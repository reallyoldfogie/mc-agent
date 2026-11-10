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
	// deltaPos is always measured against the raw target corner, never the
	// input generator's centered steering aim-point - but what each case
	// below does with it differs. Most cases (default, Traverse/MineThrough/
	// etc.) use a tight radius around this same raw corner, consistent
	// with never being centered for steering either. The movement types
	// the input generator DOES center (Descend/MineDown/MineNeighborFloor/
	// Drop2*/AscendJump/DiagonalAscend - see GenerateInputs' centering
	// switch) instead check whether the bot landed anywhere inside the
	// whole target cell, since a centered aim-point and a raw corner are
	// both valid resting spots ~0.71 blocks apart - no single radius
	// around either one alone covers both without rejecting the other.
	targetPos := targetStep.Position

	// Calculate delta from current position to target
	deltaPos := targetPos.Sub(currentPos)

	// Movement-specific completion thresholds
	switch targetStep.Movement {
	case Descend, MineDown, MineDownDiagonal, MineNeighborFloor,
		Drop2North, Drop2South, Drop2East, Drop2West:
		// Drop down movements (1-3 blocks), MineDown/MineDownDiagonal/
		// MineNeighborFloor's single- or two-level drop once the
		// obstruction(s) are cleared (the physics executor never even
		// calls IsComplete while a Mine* step is still waiting on the mine
		// itself - see isMineActionStep - so by the time this runs, it's
		// an ordinary drop into now-open space), or a directional 2-block
		// drop.
		//
		// All of these are in the input generator's block-centering list
		// (GenerateInputs' centering switch, input_generator.go) - the bot
		// is steered toward the target cell's CENTER
		// (floor(X)+0.5, floor(Z)+0.5), not its raw corner, to stop
		// throttle collapsing to zero right at a drop's lip. This check
		// used to compare against the raw corner only, with a small fixed
		// radius (2*0.2*0.25, radius ~0.316) - fine for a Descend/MineDown
		// step, where the fall's own momentum usually carries the bot
		// toward (or past) the corner before Y catches up to target. But
		// found live for MineNeighborFloor: a bot that reaches the exact
		// point it was told to steer toward - the cell CENTER - sits
		// sqrt(0.5^2+0.5^2) ~= 0.71 blocks from the raw corner this check
		// compared against, over double that radius, so the step never
		// completed and stuck-recovery fired every 3 seconds with zero
		// progress on any axis (pos 1297.51,-2.00,-58.50 vs raw target
		// 1297,-2,-59 - exactly this movement's own centered aim point,
		// 1297.5,-2,-58.5).
		//
		// Checking a radius around either single point is the wrong
		// shape: a radius around the center would reject a bot that
		// legitimately overshot toward the corner (the Descend/MineDown
		// case above), and a radius around the corner is what just failed
		// for MineNeighborFloor. The correct check is "did the bot's
		// horizontal position land inside the target cell at all" - the
		// cell spans its raw corner to corner+1 in both X and Z, and
		// either a center-settled or a corner-overshot bot is inside it.
		// Same pattern JumpToClimb/Jump2ToClimb already use below
		// (insideBlockX/Z), with the same small margin for hitbox
		// overlap just outside the cell's edges.
		insideX := deltaPos.X >= -1.1 && deltaPos.X <= 0.1
		insideZ := deltaPos.Z >= -1.1 && deltaPos.Z <= 0.1
		return insideX && insideZ && deltaPos.Y >= -0.05

	case AscendJump, DiagonalAscend:
		// Jump up one block (straight or diagonal) onto a platform. Also
		// in the input generator's block-centering list (same reasoning
		// as the Descend/MineDown/MineNeighborFloor case above - these
		// get a centered steering target too), but previously had no
		// dedicated case here at all and fell through to the tight
		// default (0.18 horizontal) checked against the raw corner -
		// the same mismatch that caused MineNeighborFloor's live stuck
		// loop, just never observed for a jump: forward momentum from the
		// jump itself tends to carry the bot past the center toward (or
		// beyond) the corner before it needs to register as complete, so
		// this was a latent inconsistency rather than a confirmed one.
		// Same inside-cell check as above rather than a tight radius.
		insideX := deltaPos.X >= -1.1 && deltaPos.X <= 0.1
		insideZ := deltaPos.Z >= -1.1 && deltaPos.Z <= 0.1
		return insideX && insideZ &&
			deltaPos.Y >= -0.065 &&
			deltaPos.Y <= 0.08

	case Climb:
		// Ladder climbing (both ascent and descent)
		//
		// The target position is the ladder BLOCK coordinates (e.g., 108, 6, 100).
		// Completion can happen when:
		// 1. Player is horizontally close to the ladder block
		// 2. Player is vertically at or near the target height
		//
		// We use generous tolerances because:
		// - Player may not land exactly centered on the ladder
		// - Y position varies during climbing animation
		// - The goal is to be "close enough" to proceed to next step
		deltaFromTarget := models.V3{
			X: targetStep.Position.X - currentPos.X,
			Y: targetStep.Position.Y - currentPos.Y,
			Z: targetStep.Position.Z - currentPos.Z,
		}
		horizontalDist2 := deltaFromTarget.X*deltaFromTarget.X + deltaFromTarget.Z*deltaFromTarget.Z

		// Complete when within ~0.5 blocks horizontally of ladder block corner
		// and within 0.5 blocks vertically of target (allows for standing on/in ladder)
		return horizontalDist2 < (0.5*0.5) && math.Abs(deltaFromTarget.Y) <= 0.5

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
		// Allow landing within 0.25 blocks horizontally of target
		// and slightly below target Y (common when landing from a jump)
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (0.25*0.25) && deltaPos.Y >= -0.1 && deltaPos.Y <= 0.1

	case SneakThrough, SneakTraverse:
		// Sneaking movements (slow, precise)
		// Tighter tolerance due to slower movement speed
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (0.15*0.15) && // Tighter horizontal (0.15 vs 0.18)
			deltaPos.Y >= -0.05 && // Tighter vertical range
			deltaPos.Y <= 0.05

	case Traverse, DiagonalTraverse, MineThrough, WadeWater, Swim, EnterClimb:
		// Horizontal corridor movement: walking (Traverse/MineThrough once
		// its obstruction is cleared), wading/sprint-swimming through water
		// (WadeWater/Swim), or approaching a ladder/vine to climb out of a
		// tunnel (EnterClimb) - all share the same shape: continuous
		// walking/swimming, never jumping, into a cell whose path target is
		// the raw block corner.
		//
		// In a single-wide tunnel (or water channel), that target X/Z is the
		// raw integer coordinate (the corner, not the center) - unlike
		// Descend/MineDown/Drop2*, none of these movements get the input
		// generator's block-center snap; see input_generator.go's
		// GenerateInputs. With a solid wall on one side, the bot's own
		// collision hitbox (~0.3 blocks radius) physically can't press its
		// center all the way to that corner - it bottoms out ~0.3 blocks
		// short on that one axis.
		//
		// A 90-degree turn (wall on both X and Z) bottoms out on BOTH axes
		// at once - confirmed live 2026-11-09 (train.log, agents
		// RSIRecurrentCo-*): 53 of 71 sampled STUCK DETECTED events were
		// this exact dx=dz=-0.30 pattern, by far the dominant trigger, each
		// one costing a full stuckThreshold+repath cycle before the bot
		// could round the corner. A single Euclidean radius can't fix this
		// without also mis-accepting a genuine single-axis overshoot: the
		// combined two-axis distance (dx=dz=0.3, dist=0.424) is BIGGER than
		// a single-axis 0.4 overshoot the "not complete" test case below
		// requires rejecting, so any one circular threshold either still
		// rejects the corner or wrongly accepts the overshoot. Checking
		// each axis independently avoids that: a wall can only ever block
		// one axis per side, so what matters is whether EACH axis
		// individually is at (or past) its own hitbox-rest limit, not how
		// far away the combined diagonal distance is.
		horizontalTolerance := 0.35 // ~0.3 hitbox-rest offset + margin, still below the 0.4 single-axis "still approaching" case
		return math.Abs(deltaPos.X) < horizontalTolerance &&
			math.Abs(deltaPos.Z) < horizontalTolerance &&
			deltaPos.Y >= -0.065 &&
			deltaPos.Y <= 0.08

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
		// Default completion threshold for Ascend and other movement types
		// not given their own case above.
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
	deltaPos := targetStep.Position.Sub(currentPos)
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
