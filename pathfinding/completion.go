package pathfinding

// IsComplete checks if a movement step has been completed based on current position.
// Returns true when the bot is "close enough" to the target position.
//
// Different movement types have different completion thresholds:
//   - Horizontal movements: Within 0.18 blocks horizontally, -0.065 to +0.08 vertically
//   - Vertical movements (ladders, drops): Within 0.2 blocks horizontally, Y threshold varies
//   - Jump movements: Slightly tighter horizontal tolerance
//
// Based on phys archive bot/path/path.go:193-211 (Tile.IsComplete() logic).
func IsComplete(currentPos V3, targetStep PathStep) bool {
	// Calculate delta from current position to target
	deltaPos := V3Sub(targetStep.Position, currentPos)

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

	case Climb, DescendLadderNorth, DescendLadderSouth, DescendLadderEast, DescendLadderWest:
		// Ladder movements (descent or directional descent)
		// Similar to regular descent but for ladder movement
		horizontalDist2 := deltaPos.X*deltaPos.X + deltaPos.Z*deltaPos.Z
		return horizontalDist2 < (2*0.2*0.25) && deltaPos.Y <= 0.05

	case SwimUp:
		// Ladder/vine ascent or swimming upward
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
	currentPos V3,
	currentVel V3,
	targetStep PathStep,
	runTime int, // Ticks spent on this step
	estimatedTicks int, // Estimated ticks to complete
) bool {
	// If we haven't exceeded estimated time, we're not stuck yet
	if runTime < estimatedTicks*2 {
		return false
	}

	// Check if velocity is near zero (not moving)
	velocityMagnitude := currentVel.DistanceTo(V3{X: 0, Y: 0, Z: 0})
	if velocityMagnitude > 0.05 {
		// Still moving, not stuck
		return false
	}

	// Check if we're making progress
	// If we're very close to target, we're probably just settling into position
	deltaPos := V3Sub(targetStep.Position, currentPos)
	dist := deltaPos.DistanceTo(V3{X: 0, Y: 0, Z: 0})
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
func EstimateProgress(startPos, currentPos, targetPos V3) float64 {
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
