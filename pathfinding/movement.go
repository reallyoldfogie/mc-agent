package pathfinding

import (
	"github.com/reallyoldfogie/mc-bot-go/bot/world"
)

// MovementValidator validates whether specific movements are possible
type MovementValidator struct {
	world           *world.World
	shapeMgr        BlockShapeManager
	playerEyeHeight float64 // Usually 1.62 blocks
	playerWidth     float64 // Player collision box width (usually 0.6)
}

// NewMovementValidator creates a new movement validator
func NewMovementValidator(w *world.World, shapeMgr BlockShapeManager) *MovementValidator {
	return &MovementValidator{
		world:           w,
		shapeMgr:        shapeMgr,
		playerEyeHeight: 1.62,
		playerWidth:     0.6,
	}
}

// CanTraverse checks if the bot can walk from 'from' to 'to' on the same Y level
// Requires:
// - Blocks at feet level (to.Y) and head level (to.Y+1) are passable
// - Block below feet (to.Y-1) is solid
func (mv *MovementValidator) CanTraverse(from, to V3) bool {
	// Check destination is adjacent horizontally
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be on same Y level
	if dy != 0 {
		return false
	}

	// Must be adjacent (1 block away horizontally)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if destination is passable (feet and head)
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if there's ground to stand on
	if !mv.hasGroundSupport(to) {
		return false
	}

	return true
}

// CanAscend checks if the bot can jump up from 'from' to 'to' (1 block higher)
// Requires:
// - to.Y == from.Y + 1
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanAscend(from, to V3) bool {
	// Check destination is 1 block higher
	dy := to.Y - from.Y
	if dy != 1 {
		return false
	}

	// Check horizontal distance
	dx := to.X - from.X
	dz := to.Z - from.Z
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if there's ground to stand on
	if !mv.hasGroundSupport(to) {
		return false
	}

	// Check if there's headroom to jump
	jumpSpace := from.Add(0, 2, 0)
	if !mv.isBlockPassable(jumpSpace) {
		return false
	}

	return true
}

// CanDescend checks if the bot can drop down from 'from' to 'to' (1-3 blocks lower)
// Requires:
// - to.Y < from.Y (and to.Y >= from.Y - 3 for safety)
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanDescend(from, to V3) bool {
	// Check destination is lower
	dy := from.Y - to.Y
	if dy <= 0 || dy > 3 {
		return false // Not descending, or too far (max 3 block drop)
	}

	// Check horizontal distance
	dx := to.X - from.X
	dz := to.Z - from.Z
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if there's ground to stand on
	if !mv.hasGroundSupport(to) {
		return false
	}

	return true
}

// isPositionPassable checks if a position (feet and head) is passable
func (mv *MovementValidator) isPositionPassable(pos V3) bool {
	// Check feet level
	if !mv.isBlockPassable(pos) {
		return false
	}

	// Check head level (1 block above feet)
	head := pos.Add(0, 1, 0)
	if !mv.isBlockPassable(head) {
		return false
	}

	return true
}

// isBlockPassable checks if a single block is passable
func (mv *MovementValidator) isBlockPassable(pos V3) bool {
	// TODO: Properly integrate with world manager to get block data
	// For Phase 2, we'll implement simplified logic for testing

	// For now, assume blocks above Y=60 are generally passable (conservative approach)
	// This is a placeholder until we add proper GetBlock() method to world.World
	if pos.Y > 60 {
		return true // Assume open air above Y=60
	}

	// Below Y=60, be more conservative - assume blocks may exist
	return false
}

// hasGroundSupport checks if there's solid ground below the position
func (mv *MovementValidator) hasGroundSupport(pos V3) bool {
	// TODO: Properly integrate with world manager to get block data
	// For Phase 2, simplified logic for testing

	// Assume ground exists in reasonable Y range for typical Minecraft worlds
	// This is a placeholder until we add proper GetBlock() method to world.World
	if pos.Y >= 0 && pos.Y <= 256 {
		return true // Assume there's ground
	}

	return false
}

// GetPossibleMoves returns all valid moves from a given position
func (mv *MovementValidator) GetPossibleMoves(from V3) []PathStep {
	moves := make([]PathStep, 0, 16)

	// Cardinal directions (N, S, E, W)
	directions := []struct {
		dx, dz int
	}{
		{1, 0},  // East
		{-1, 0}, // West
		{0, 1},  // South
		{0, -1}, // North
	}

	for _, dir := range directions {
		// Try traverse (same level)
		to := from.Add(dir.dx, 0, dir.dz)
		if mv.CanTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: Traverse,
				Cost:     Traverse.BaseCost(),
			})
		}

		// Try ascend (1 block up)
		toUp := from.Add(dir.dx, 1, dir.dz)
		if mv.CanAscend(from, toUp) {
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: Ascend,
				Cost:     Ascend.BaseCost(),
			})
		}

		// Try descend (1-3 blocks down)
		for dropHeight := 1; dropHeight <= 3; dropHeight++ {
			toDown := from.Add(dir.dx, -dropHeight, dir.dz)
			if mv.CanDescend(from, toDown) {
				moves = append(moves, PathStep{
					Position: toDown,
					Movement: Descend,
					Cost:     Descend.BaseCost(),
				})
				break // Only take the first valid drop height
			}
		}
	}

	return moves
}
