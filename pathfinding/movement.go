package pathfinding

import (
	"log"

	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
)

// MovementValidator validates whether specific movements are possible
type MovementValidator struct {
	world            World
	shapeMgr         BlockShapeManager
	blockMgr         mc_versions.BlockMgr
	statePropsLoader *StatePropertyLoader
	playerEyeHeight  float64 // Usually 1.62 blocks
	playerWidth      float64 // Player collision box width (usually 0.6)
	debugCheckCount  int     // Counter for debug logging
}

// NewMovementValidator creates a new movement validator
func NewMovementValidator(w World, shapeMgr BlockShapeManager, blockMgr mc_versions.BlockMgr, statePropsLoader *StatePropertyLoader) *MovementValidator {
	return &MovementValidator{
		world:            w,
		shapeMgr:         shapeMgr,
		blockMgr:         blockMgr,
		statePropsLoader: statePropsLoader,
		playerEyeHeight:  1.62,
		playerWidth:      0.6,
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
	return mv.hasGroundSupport(to)
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

	// Debug ascend checks for start position (use integer equality since V3 uses float64 for block coords)
	if int(from.X) == 255 && int(from.Y) == 72 && int(from.Z) == 82 && int(to.X) == 256 {
		log.Printf("[DEBUG CanAscend] from=(%.0f,%.0f,%.0f) to=(%.0f,%.0f,%.0f)",
			from.X, from.Y, from.Z, to.X, to.Y, to.Z)
	}

	// Check if destination is passable
	if !mv.isPositionPassable(to) {
		if int(from.X) == 255 && int(from.Y) == 72 && int(from.Z) == 82 && int(to.X) == 256 {
			log.Printf("[DEBUG CanAscend] FAILED: destination not passable")
		}
		return false
	}

	// Check if there's ground to stand on
	if !mv.hasGroundSupport(to) {
		if int(from.X) == 255 && int(from.Y) == 72 && int(from.Z) == 82 && int(to.X) == 256 {
			log.Printf("[DEBUG CanAscend] FAILED: no ground support")
		}
		return false
	}

	// Check if there's headroom to jump
	jumpSpace := from.Add(0, 2, 0)
	if !mv.isBlockPassable(jumpSpace) {
		if int(from.X) == 255 && int(from.Y) == 72 && int(from.Z) == 82 && int(to.X) == 256 {
			log.Printf("[DEBUG CanAscend] FAILED: no headroom at (%.0f,%.0f,%.0f)",
				jumpSpace.X, jumpSpace.Y, jumpSpace.Z)
		}
		return false
	}
	
	if int(from.X) == 255 && int(from.Y) == 72 && int(from.Z) == 82 && int(to.X) == 256 {
		log.Printf("[DEBUG CanAscend] SUCCESS")
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
	return mv.isBlockPassable(head)
}

// isBlockPassable checks if a single block is passable
func (mv *MovementValidator) isBlockPassable(pos V3) bool {
	// Get block state ID from world
	stateID := mv.world.GetBlockAt(pos.X, pos.Y, pos.Z)

	// State ID 0 is always air (passable)
	if stateID == 0 {
		return true
	}

	// Convert state ID to block + properties (when available)
	blockID := mv.blockMgr.BlockIDByStateID(stateID)
	block := mv.blockMgr.GetByID(blockID)

	// Try to extract properties for this specific state ID
	props := mv.propsFromStateID(stateID)

	// Use the block name + extracted properties to check if passable
	passable := mv.shapeMgr.IsPassable(block.Name, props)

	// Debug logging (first 5 checks)
	if mv.debugCheckCount < 5 {
		log.Printf("[DEBUG isBlockPassable] pos=(%f,%f,%f) stateID=%d blockID=%d blockName=%s props=%v passable=%t",
			pos.X, pos.Y, pos.Z, stateID, blockID, block.Name, props, passable)
		mv.debugCheckCount++
	}

	return passable
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
		// TODO: Parse block state properties to check "open" state
		// Currently we pass nil for props because we don't extract properties from state IDs
		// Need to:
		//   1. Add method to BlockMgr to get properties from state ID
		//   2. Parse the state ID into block ID + property map
		//   3. Check props["open"] == "true" to give lower penalty for open doors
		// For now, assume doors may be closed and apply penalty
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
	// Check block directly below feet
	groundPos := pos.Add(0, -1, 0)
	stateID := mv.world.GetBlockAt(groundPos.X, groundPos.Y, groundPos.Z)

	// State ID 0 is air - check for partial blocks (stairs, slabs)
	if stateID == 0 {
		// Check if we're standing on a partial block (stair, slab)
		// Allow up to 0.5 block tolerance by checking 2 blocks below
		altGroundPos := pos.Add(0, -2, 0)
		altStateID := mv.world.GetBlockAt(altGroundPos.X, altGroundPos.Y, altGroundPos.Z)

		if altStateID != 0 {
			// Check if it's a partial block with surface height
			blockID := mv.blockMgr.BlockIDByStateID(altStateID)
			block := mv.blockMgr.GetByID(blockID)
			props := mv.propsFromStateID(altStateID)
			surfaceHeight := mv.shapeMgr.GetStandingSurfaceHeight(block.Name, props)

			if surfaceHeight > 0 && surfaceHeight < 1.0 {
				// Standing on partial block - this is valid ground support
				log.Printf("[DEBUG hasGroundSupport] Standing on partial block at Y-2: %s (surface height %.2f)",
					block.Name, surfaceHeight)
				return true
			}
		}

		log.Printf("[DEBUG hasGroundSupport] No ground at (%f, %f, %f) below pos (%f, %f, %f) - got air (stateID 0)",
			groundPos.X, groundPos.Y, groundPos.Z, pos.X, pos.Y, pos.Z)
		return false
	}

	// Convert state ID to block ID and get block info
	blockID := mv.blockMgr.BlockIDByStateID(stateID)
	block := mv.blockMgr.GetByID(blockID)

	// Try to extract properties for this specific state ID
	props := mv.propsFromStateID(stateID)

	// Use the block name + extracted properties to determine solidity
	isSolid := mv.shapeMgr.IsSolid(block.Name, props)

	log.Printf("[DEBUG hasGroundSupport] Ground check at (%f, %f, %f): stateID=%d, blockID=%d, blockName=%s, props=%v, isSolid=%v",
		groundPos.X, groundPos.Y, groundPos.Z, stateID, blockID, block.Name, props, isSolid)

	// Check if block is solid (provides standing surface)
	return isSolid
}

// propsFromStateID attempts to extract the block state properties map for a given global state ID.
//
// Uses the StatePropertyLoader to dynamically load properties from JSON at runtime.
func (mv *MovementValidator) propsFromStateID(stateID uint32) map[string]string {
	if mv.statePropsLoader == nil {
		return make(map[string]string)
	}
	return mv.statePropsLoader.GetProperties(stateID)
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
	return mv.isBlockPassable(jumpSpace)
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

	// Check if there's a climbable block at destination
	stateID := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if stateID == 0 {
		return false // Air, not climbable
	}

	blockID := mv.blockMgr.BlockIDByStateID(stateID)
	block := mv.blockMgr.GetByID(blockID)

	return mv.shapeMgr.IsClimbable(block.Name, nil)
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

	// Check if destination is water
	toStateID := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if toStateID == 0 {
		return false
	}
	toBlockID := mv.blockMgr.BlockIDByStateID(toStateID)
	toBlock := mv.blockMgr.GetByID(toBlockID)
	if !mv.shapeMgr.IsWater(toBlock.Name, nil) {
		return false
	}

	// Check if from position is also water (must already be swimming)
	fromStateID := mv.world.GetBlockAt(from.X, from.Y, from.Z)
	if fromStateID == 0 {
		return false
	}
	fromBlockID := mv.blockMgr.BlockIDByStateID(fromStateID)
	fromBlock := mv.blockMgr.GetByID(fromBlockID)

	return mv.shapeMgr.IsWater(fromBlock.Name, nil)
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

	// Check if destination is water
	toStateID := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if toStateID == 0 {
		return false
	}
	toBlockID := mv.blockMgr.BlockIDByStateID(toStateID)
	toBlock := mv.blockMgr.GetByID(toBlockID)
	if !mv.shapeMgr.IsWater(toBlock.Name, nil) {
		return false
	}

	// Check if from position is also water (must already be swimming)
	fromStateID := mv.world.GetBlockAt(from.X, from.Y, from.Z)
	if fromStateID == 0 {
		return false
	}
	fromBlockID := mv.blockMgr.BlockIDByStateID(fromStateID)
	fromBlock := mv.blockMgr.GetByID(fromBlockID)

	return mv.shapeMgr.IsWater(fromBlock.Name, nil)
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

	// Check if destination is water
	toStateID := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if toStateID == 0 {
		return false
	}
	toBlockID := mv.blockMgr.BlockIDByStateID(toStateID)
	toBlock := mv.blockMgr.GetByID(toBlockID)
	if !mv.shapeMgr.IsWater(toBlock.Name, nil) {
		return false
	}

	// Check if from position is also water (must already be swimming)
	fromStateID := mv.world.GetBlockAt(from.X, from.Y, from.Z)
	if fromStateID == 0 {
		return false
	}
	fromBlockID := mv.blockMgr.BlockIDByStateID(fromStateID)
	fromBlock := mv.blockMgr.GetByID(fromBlockID)

	return mv.shapeMgr.IsWater(fromBlock.Name, nil)
}

// FindGroundBelow finds the nearest valid ground level below the given position
// Returns the Y coordinate where the entity's feet should be (standing on solid ground)
// Returns -1 if no valid ground found
func (mv *MovementValidator) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	log.Printf("[FindGroundBelow] Searching for ground at (%f, %f) from Y=%f", x, z, startY)

	// Check current position first
	currentStateID := mv.world.GetBlockAt(x, startY, z)

	// If current position is inside a solid block, search UPWARD to find surface
	if currentStateID != 0 {
		currentBlockID := mv.blockMgr.BlockIDByStateID(currentStateID)
		currentBlock := mv.blockMgr.GetByID(currentBlockID)
		isSolid := mv.shapeMgr.IsSolid(currentBlock.Name, nil)

		if isSolid {
			log.Printf("[FindGroundBelow] Bot is inside solid block at Y=%f (%s), searching upward for surface",
				startY, currentBlock.Name)

			// Search upward to find the top of the solid terrain
			for y := startY; y <= startY+maxSearchDepth && y <= 320; y++ {
				pos := V3{X: x, Y: y, Z: z}

				// Check if this is a valid standing position (air with solid below)
				if mv.isPositionPassable(pos) && mv.hasGroundSupport(pos) {
					log.Printf("[FindGroundBelow] Found surface at Y=%f for position (%f, %f) (searched upward from Y=%f)",
						y, x, z, startY)
					return y
				}
			}
			log.Printf("[FindGroundBelow] No surface found searching upward from Y=%f", startY)
		}
	}

	// If not in solid block, search downward for valid ground
	log.Printf("[FindGroundBelow] Searching downward from Y=%f for valid standing position", startY)

	for y := startY; y >= startY-maxSearchDepth && y >= -64; y-- {
		pos := V3{X: x, Y: y, Z: z}

		// Check if this position is valid:
		// 1. Feet and head blocks must be passable (air or passable blocks)
		// 2. Block below feet must be solid (ground support)
		if mv.isPositionPassable(pos) && mv.hasGroundSupport(pos) {
			log.Printf("[FindGroundBelow] Found valid ground at Y=%f for position (%f, %f) (searched downward from Y=%f)",
				y, x, z, startY)
			return y
		}
	}

	log.Printf("[FindGroundBelow] No valid ground found for position (%f, %f) after searching up/down from Y=%f", x, z, startY)
	return -1 // No valid ground found
}

// logTerrainAround logs all blocks around a position for debugging
func (mv *MovementValidator) logTerrainAround(from V3) {
	log.Printf("[TERRAIN] === Terrain around (%.0f, %.0f, %.0f) ===", from.X, from.Y, from.Z)
	
	// All 8 directions plus center
	directions := []struct{
		name string
		dx, dz float64
	}{
		{"CENTER", 0, 0},
		{"NORTH", 0, -1},
		{"SOUTH", 0, 1},
		{"EAST", 1, 0},
		{"WEST", -1, 0},
		{"NE", 1, -1},
		{"SE", 1, 1},
		{"SW", -1, 1},
		{"NW", -1, -1},
	}
	
	// For each direction, log blocks from Y-1 to Y+3
	for _, dir := range directions {
		x := from.X + dir.dx
		z := from.Z + dir.dz
		
		log.Printf("[TERRAIN] %s (%.0f, %.0f):", dir.name, x, z)
		for dy := float64(-1); dy <= 3; dy++ {
			y := from.Y + dy
			stateID := mv.world.GetBlockAt(x, y, z)
			
			var blockName string
			var passable bool
			var solid bool
			
			if stateID == 0 {
				blockName = "air"
				passable = true
				solid = false
			} else {
				blockID := mv.blockMgr.BlockIDByStateID(stateID)
				block := mv.blockMgr.GetByID(blockID)
				blockName = block.Name
				props := mv.propsFromStateID(stateID)
				passable = mv.shapeMgr.IsPassable(blockName, props)
				solid = mv.shapeMgr.IsSolid(blockName, props)
			}
			
			var yLabel string
			switch dy {
			case -1:
				yLabel = "Y-1(ground)"
			case 0:
				yLabel = "Y  (feet)  "
			case 1:
				yLabel = "Y+1(head)  "
			case 2:
				yLabel = "Y+2(jump)  "
			case 3:
				yLabel = "Y+3        "
			}
			
			log.Printf("  %s: %s (passable=%t, solid=%t)", yLabel, blockName, passable, solid)
		}
	}
}

// GetPossibleMoves returns all valid moves from a given position
// Optionally filters moves based on goal to reduce search space (pass zero V3 to disable filtering)
func (mv *MovementValidator) GetPossibleMoves(from V3, goal V3) []PathStep {
	moves := make([]PathStep, 0, 32) // Increased capacity for more move types
	
	// Debug: log terrain around start position once
	if mv.debugCheckCount == 0 {
		mv.logTerrainAround(from)
	}

	// Cardinal directions (N, S, E, W)
	cardinalDirs := []struct {
		dx, dz float64
	}{
		{1, 0},  // East
		{-1, 0}, // West
		{0, 1},  // South
		{0, -1}, // North
	}

	// Diagonal directions (NE, SE, SW, NW)
	diagonalDirs := []struct {
		dx, dz float64
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
		for dropHeight := float64(1); dropHeight <= 3; dropHeight++ {
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

	// Climbing (world integration complete)
	for dy := float64(-1); dy <= 1; dy++ {
		if dy == 0 {
			continue
		}
		to := from.Add(0, dy, 0)
		if mv.CanClimb(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: Climb,
				Cost:     Climb.BaseCost(),
			})
		}
	}

	// Swimming (world integration complete)
	allDirs := append(cardinalDirs, diagonalDirs...)
	for _, dir := range allDirs {
		// Horizontal swim
		to := from.Add(dir.dx, 0, dir.dz)
		if mv.CanSwim(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: Swim,
				Cost:     Swim.BaseCost(),
			})
		}

		// Swim up
		toUp := from.Add(dir.dx, 1, dir.dz)
		if mv.CanSwimUp(from, toUp) {
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: SwimUp,
				Cost:     SwimUp.BaseCost(),
			})
		}

		// Swim down
		toDown := from.Add(dir.dx, -1, dir.dz)
		if mv.CanSwimDown(from, toDown) {
			moves = append(moves, PathStep{
				Position: toDown,
				Movement: SwimDown,
				Cost:     SwimDown.BaseCost(),
			})
		}
	}

	// Prune moves that go too far from goal (if goal is provided)
	if goal.X != 0 || goal.Y != 0 || goal.Z != 0 {
		fromDist := from.DistanceTo(goal)
		filtered := make([]PathStep, 0, len(moves))
		
		// Debug: log first call to understand terrain
		if mv.debugCheckCount < 2 {
			log.Printf("[GetPossibleMoves] DEBUG from=(%.0f,%.0f,%.0f) goal=(%.0f,%.0f,%.0f) dist=%.1f",
				from.X, from.Y, from.Z, goal.X, goal.Y, goal.Z, fromDist)
			log.Printf("[GetPossibleMoves] DEBUG Generated %d moves before pruning:", len(moves))
			for i, move := range moves {
				toDist := move.Position.DistanceTo(goal)
				log.Printf("  Move %d: %s to (%.0f,%.0f,%.0f) distToGoal=%.1f",
					i+1, move.Movement, move.Position.X, move.Position.Y, move.Position.Z, toDist)
			}
		}
		
		for _, move := range moves {
			toDist := move.Position.DistanceTo(goal)
			// Allow moves that get closer, or slightly farther (for obstacle avoidance)
			// Allow up to 5 blocks detour to handle obstacles
			if toDist <= fromDist+5.0 {
				filtered = append(filtered, move)
			}
		}
		
		if mv.debugCheckCount < 2 {
			log.Printf("[GetPossibleMoves] DEBUG After pruning: %d moves (pruned %d)",
				len(filtered), len(moves)-len(filtered))
			mv.debugCheckCount++
		}
		
		return filtered
	}

	return moves
}
