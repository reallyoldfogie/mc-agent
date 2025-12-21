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

// CalculateTerrainCostPenalty calculates additional cost penalties based on block properties
// This includes dangerous blocks (lava, fire), fences, doors, etc.
// Returns the penalty to add to the base movement cost
func (mv *MovementValidator) CalculateTerrainCostPenalty(pos V3, blockID string, props map[string]string) float64 {
	penalty := 0.0

	// Dangerous blocks - very high penalty to avoid
	if mv.shapeMgr.IsDangerous(blockID, props) {
		penalty += 1000.0 // Extremely high cost to avoid lava, fire, etc.
	}

	// Fences are 1.5 blocks tall - not jumpable, treat as barriers
	if mv.shapeMgr.IsFenceLike(blockID, props) {
		penalty += 500.0 // High cost - prefer to path around
	}

	// Water - moderate penalty (slower movement)
	if mv.shapeMgr.IsWater(blockID, props) {
		penalty += 1.0 // Swimming is slower than walking
	}

	// Doors/gates - slight penalty (may need to open)
	if mv.shapeMgr.IsDoorLike(blockID, props) {
		// Check if door is open
		// TODO: Parse props to check "open" state
		penalty += 2.0 // Small penalty for door interaction
	}

	// Slabs and stairs - slight bonus (easier to traverse)
	if mv.shapeMgr.IsSlab(blockID, props) || mv.shapeMgr.IsStair(blockID, props) {
		penalty -= 0.2 // Small bonus for easier terrain
	}

	return penalty
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

// GetStandingSurfaceHeight returns the height of the standing surface at a position
// For slabs and stairs, this may be 0.5 or variable height
// Returns 0.0 if not solid, 1.0 for full block, 0.0-1.0 for partial blocks
func (mv *MovementValidator) GetStandingSurfaceHeight(pos V3, blockID string, props map[string]string) float64 {
	// Use the BlockShapeManager to get precise height
	return mv.shapeMgr.GetStandingSurfaceHeight(blockID, props)
}

// CanTraversePartialBlock checks if movement is valid considering partial blocks (slabs, stairs)
// This provides more precise collision detection than the basic CanTraverse
func (mv *MovementValidator) CanTraversePartialBlock(from, to V3, fromBlockID, toBlockID string, fromProps, toProps map[string]string) bool {
	// Get the height of the standing surfaces
	fromHeight := mv.GetStandingSurfaceHeight(from.Add(0, -1, 0), fromBlockID, fromProps)
	toHeight := mv.GetStandingSurfaceHeight(to.Add(0, -1, 0), toBlockID, toProps)

	// Calculate height difference
	heightDiff := toHeight - fromHeight

	// If moving to a higher surface that's less than a full block, might not need a jump
	if heightDiff > 0 && heightDiff <= 0.5 {
		// Check if destination is passable
		if !mv.isPositionPassable(to) {
			return false
		}
		return true
	}

	// For other cases, use standard validation
	return mv.CanTraverse(from, to)
}

// IsSlabOrStair checks if a block is a slab or stair
func (mv *MovementValidator) IsSlabOrStair(blockID string, props map[string]string) bool {
	return mv.shapeMgr.IsSlab(blockID, props) || mv.shapeMgr.IsStair(blockID, props)
}

// CanDiagonalTraverse checks if the bot can walk diagonally on the same Y level
func (mv *MovementValidator) CanDiagonalTraverse(from, to V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be on same Y level
	if dy != 0 {
		return false
	}

	// Must be diagonal (both dx and dz are non-zero and magnitude 1)
	if (dx == 0 || dz == 0) || (dx*dx > 1 || dz*dz > 1) {
		return false
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check ground support
	if !mv.hasGroundSupport(to) {
		return false
	}

	// Check that the two adjacent cardinal squares are also passable (no corner cutting)
	adj1 := from.Add(dx, 0, 0)
	adj2 := from.Add(0, 0, dz)
	if !mv.isPositionPassable(adj1) || !mv.isPositionPassable(adj2) {
		return false
	}

	return true
}

// CanDiagonalAscend checks if the bot can jump up diagonally
func (mv *MovementValidator) CanDiagonalAscend(from, to V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be 1 block higher
	if dy != 1 {
		return false
	}

	// Must be diagonal
	if (dx == 0 || dz == 0) || (dx*dx > 1 || dz*dz > 1) {
		return false
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check ground support
	if !mv.hasGroundSupport(to) {
		return false
	}

	// Check headroom to jump
	jumpSpace := from.Add(0, 2, 0)
	if !mv.isBlockPassable(jumpSpace) {
		return false
	}

	// Check adjacent squares are passable (no corner cutting)
	adj1 := from.Add(dx, 0, 0)
	adj2 := from.Add(0, 0, dz)
	if !mv.isPositionPassable(adj1) || !mv.isPositionPassable(adj2) {
		return false
	}

	return true
}

// CanJump2 checks if the bot can jump across a 2-block gap
func (mv *MovementValidator) CanJump2(from, to V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be on same Y level or up to 1 block higher
	if dy < 0 || dy > 1 {
		return false
	}

	// Must be 2 blocks away in one cardinal direction
	if (dx != 0 && dz != 0) || (dx*dx+dz*dz != 4) {
		return false
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check ground support
	if !mv.hasGroundSupport(to) {
		return false
	}

	// Check headroom to jump
	jumpSpace := from.Add(0, 2, 0)
	if !mv.isBlockPassable(jumpSpace) {
		return false
	}

	return true
}

// CanClimb checks if the bot can climb (ladders, vines, etc.)
func (mv *MovementValidator) CanClimb(from, to V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be moving vertically (up or down)
	if dy == 0 {
		return false
	}

	// Must be same horizontal position or adjacent
	if dx*dx+dz*dz > 1 {
		return false
	}

	// TODO: Check if there's a climbable block at destination using shapeMgr.IsClimbable()
	// For now, return false until we integrate with world manager
	return false
}

// CanSwim checks if the bot can swim horizontally through water
func (mv *MovementValidator) CanSwim(from, to V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be on same Y level
	if dy != 0 {
		return false
	}

	// Must be adjacent (cardinal or diagonal)
	if dx*dx+dz*dz == 0 || dx*dx+dz*dz > 2 {
		return false
	}

	// TODO: Check if destination is water using shapeMgr.IsWater()
	// TODO: Check if from position is also water (must already be swimming)
	// For now, return false until we integrate with world manager
	return false
}

// CanSwimUp checks if the bot can swim upward in water
func (mv *MovementValidator) CanSwimUp(from, to V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be moving up (1 block)
	if dy != 1 {
		return false
	}

	// Must be same horizontal position or adjacent
	if dx*dx+dz*dz > 1 {
		return false
	}

	// TODO: Check if both positions are water using shapeMgr.IsWater()
	// For now, return false until we integrate with world manager
	return false
}

// CanSwimDown checks if the bot can swim downward in water
func (mv *MovementValidator) CanSwimDown(from, to V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be moving down (1-2 blocks)
	if dy >= 0 || dy < -2 {
		return false
	}

	// Must be same horizontal position or adjacent
	if dx*dx+dz*dz > 1 {
		return false
	}

	// TODO: Check if both positions are water using shapeMgr.IsWater()
	// For now, return false until we integrate with world manager
	return false
}

// GetPossibleMoves returns all valid moves from a given position
func (mv *MovementValidator) GetPossibleMoves(from V3) []PathStep {
	moves := make([]PathStep, 0, 32) // Increased capacity for more move types

	// Cardinal directions (N, S, E, W)
	cardinalDirs := []struct {
		dx, dz int
	}{
		{1, 0},  // East
		{-1, 0}, // West
		{0, 1},  // South
		{0, -1}, // North
	}

	// Diagonal directions (NE, SE, SW, NW)
	diagonalDirs := []struct {
		dx, dz int
	}{
		{1, 1},   // SE
		{1, -1},  // NE
		{-1, 1},  // SW
		{-1, -1}, // NW
	}

	// Cardinal movements
	for _, dir := range cardinalDirs {
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

		// Try jump2 (2-block gap)
		toJump := from.Add(dir.dx*2, 0, dir.dz*2)
		if mv.CanJump2(from, toJump) {
			moves = append(moves, PathStep{
				Position: toJump,
				Movement: Jump2,
				Cost:     Jump2.BaseCost(),
			})
		}

		// Try jump2 up (2-block gap, 1 block higher)
		toJumpUp := from.Add(dir.dx*2, 1, dir.dz*2)
		if mv.CanJump2(from, toJumpUp) {
			moves = append(moves, PathStep{
				Position: toJumpUp,
				Movement: Jump2,
				Cost:     Jump2.BaseCost() + 0.5, // Slightly more expensive
			})
		}
	}

	// Diagonal movements
	for _, dir := range diagonalDirs {
		// Try diagonal traverse
		to := from.Add(dir.dx, 0, dir.dz)
		if mv.CanDiagonalTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: DiagonalTraverse,
				Cost:     DiagonalTraverse.BaseCost(),
			})
		}

		// Try diagonal ascend
		toUp := from.Add(dir.dx, 1, dir.dz)
		if mv.CanDiagonalAscend(from, toUp) {
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: DiagonalAscend,
				Cost:     DiagonalAscend.BaseCost(),
			})
		}
	}

	// TODO Phase 4: Enable climbing, swimming when we integrate with world manager
	// These require checking block types via shapeMgr

	// // Climbing
	// for dy := -1; dy <= 1; dy++ {
	// 	if dy == 0 {
	// 		continue
	// 	}
	// 	to := from.Add(0, dy, 0)
	// 	if mv.CanClimb(from, to) {
	// 		moves = append(moves, PathStep{
	// 			Position: to,
	// 			Movement: Climb,
	// 			Cost:     Climb.BaseCost(),
	// 		})
	// 	}
	// }

	// // Swimming
	// for _, dir := range append(cardinalDirs, diagonalDirs...) {
	// 	// Horizontal swim
	// 	to := from.Add(dir.dx, 0, dir.dz)
	// 	if mv.CanSwim(from, to) {
	// 		moves = append(moves, PathStep{
	// 			Position: to,
	// 			Movement: Swim,
	// 			Cost:     Swim.BaseCost(),
	// 		})
	// 	}
	//
	// 	// Swim up
	// 	toUp := from.Add(dir.dx, 1, dir.dz)
	// 	if mv.CanSwimUp(from, toUp) {
	// 		moves = append(moves, PathStep{
	// 			Position: toUp,
	// 			Movement: SwimUp,
	// 			Cost:     SwimUp.BaseCost(),
	// 		})
	// 	}
	//
	// 	// Swim down
	// 	toDown := from.Add(dir.dx, -1, dir.dz)
	// 	if mv.CanSwimDown(from, toDown) {
	// 		moves = append(moves, PathStep{
	// 			Position: toDown,
	// 			Movement: SwimDown,
	// 			Cost:     SwimDown.BaseCost(),
	// 		})
	// 	}
	// }

	return moves
}
