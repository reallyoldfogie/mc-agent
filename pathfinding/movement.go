package pathfinding

import (
	"fmt"
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// MovePruneConfig controls distance-based pruning for move generation.
type MovePruneConfig struct {
	StartDist float64
	DriftCap  float64
}

// MovementValidator validates whether specific movements are possible
type MovementValidator struct {
	world           models.World
	shapeMgr        models.BlockShapeManager
	playerEyeHeight float64 // Usually 1.62 blocks
	playerWidth     float64 // Player collision box width (usually 0.6)
	debugCheckCount int     // Counter for debug logging
	climbDebugCount int     // Counter for climb debug logging
}

// NewMovementValidator creates a new movement validator
func NewMovementValidator(w models.World, shapeMgr models.BlockShapeManager) *MovementValidator {
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
func (mv *MovementValidator) CanTraverse(from, to models.V3) bool {
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
func (mv *MovementValidator) CanAscend(from, to models.V3) bool {
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

	// Debug ascend checks for start position (use integer equality since models.V3 uses float64 for block coords)
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

// CanAscendStairs checks if the bot can walk up stairs from 'from' to 'to' (1 block higher)
// This is similar to CanAscend but specifically for stair blocks where no jump is needed.
// Requires:
// - to.Y == from.Y + 1
// - Ground block at target (to.Y - 1) is a stair
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanAscendStairs(from, to models.V3) bool {
	// Check destination is 1 block higher
	dy := to.Y - from.Y
	if dy != 1 {
		return false
	}

	// Check horizontal distance (cardinal direction only)
	dx := to.X - from.X
	dz := to.Z - from.Z
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if the ground block at target is a stair
	groundPos := to.Add(0, -1, 0)
	groundStateID, loaded := mv.world.GetBlockAt(groundPos.X, groundPos.Y, groundPos.Z)
	if !loaded || groundStateID == 0 {
		return false
	}
	if !mv.shapeMgr.IsStair(groundStateID) {
		return false // Ground is not a stair
	}

	// Check if destination is passable (feet and head)
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if there's ground support (stair provides this)
	if !mv.hasGroundSupport(to) {
		return false
	}

	// No headroom check needed for stairs - natural walking, no jump
	return true
}

// CanDescendStairs checks if the bot can walk down stairs from 'from' to 'to' (1 block lower)
// This is for walking down stair blocks naturally without falling.
// Requires:
// - to.Y == from.Y - 1
// - Ground block at current position (from.Y - 1) is a stair
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanDescendStairs(from, to models.V3) bool {
	// Check destination is 1 block lower
	dy := from.Y - to.Y
	if dy != 1 {
		return false
	}

	// Check horizontal distance (cardinal direction only)
	dx := to.X - from.X
	dz := to.Z - from.Z
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Check if the ground block at current position is a stair
	// (we're descending FROM a stair)
	groundPos := from.Add(0, -1, 0)
	groundStateID, loaded := mv.world.GetBlockAt(groundPos.X, groundPos.Y, groundPos.Z)
	if !loaded || groundStateID == 0 {
		return false
	}
	if !mv.shapeMgr.IsStair(groundStateID) {
		return false // Current ground is not a stair
	}

	// Check if destination is passable (feet and head)
	if !mv.isPositionPassable(to) {
		return false
	}

	// Check if there's ground support at destination
	if !mv.hasGroundSupport(to) {
		return false
	}

	return true
}

// CanDescend checks if the bot can drop down from 'from' to 'to' (1-3 blocks lower)
// Requires:
// - to.Y < from.Y (and to.Y >= from.Y - 3 for safety)
// - Destination is passable
// - Has ground support
func (mv *MovementValidator) CanDescend(from, to models.V3) bool {
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
func (mv *MovementValidator) isPositionPassable(pos models.V3) bool {
	// Check feet level
	if !mv.isBlockPassable(pos) {
		return false
	}

	// Check head level (1 block above feet)
	head := pos.Add(0, 1, 0)
	return mv.isBlockPassable(head)
}

// isBlockPassable checks if a single block is passable
func (mv *MovementValidator) isBlockPassable(pos models.V3) bool {
	// Get block state ID from world
	stateID, loaded := mv.world.GetBlockAt(pos.X, pos.Y, pos.Z)

	if !loaded {
		return false // Chunk not loaded, not passable
	}

	// State ID 0 is always air (passable)
	if stateID == 0 {
		return true
	}

	// Use shapeMgr to check if passable
	return mv.shapeMgr.IsPassable(stateID)
}

// hasGroundSupport checks if there's solid ground below the position
func (mv *MovementValidator) hasGroundSupport(pos models.V3) bool {
	// Check block directly below feet
	groundPos := pos.Add(0, -1, 0)
	stateID, loaded := mv.world.GetBlockAt(groundPos.X, groundPos.Y, groundPos.Z)

	if !loaded {
		return false // Chunk not loaded
	}

	// State ID 0 is air - no ground support
	if stateID == 0 {
		return false
	}

	// Check if block is solid (provides standing surface)
	return mv.shapeMgr.IsSolid(stateID)
}

// CanDiagonalTraverse checks if the bot can walk diagonally on the same Y level
func (mv *MovementValidator) CanDiagonalTraverse(from, to models.V3) bool {
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
func (mv *MovementValidator) CanDiagonalAscend(from, to models.V3) bool {
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
func (mv *MovementValidator) CanJump2(from, to models.V3) bool {
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
func (mv *MovementValidator) CanClimb(from, to models.V3) bool {
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
	stateID, loaded := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if !loaded || stateID == 0 {
		return false // Chunk not loaded or air, not climbable
	}

	return mv.shapeMgr.IsClimbable(stateID)
}

// CanSwim checks if the bot can swim horizontally through water
func (mv *MovementValidator) CanSwim(from, to models.V3) bool {
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
	toStateID, toLoaded := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if !toLoaded || toStateID == 0 {
		return false
	}
	if !mv.shapeMgr.IsWater(toStateID) {
		return false
	}

	// Check if from position is also water (must already be swimming)
	fromStateID, fromLoaded := mv.world.GetBlockAt(from.X, from.Y, from.Z)
	if !fromLoaded || fromStateID == 0 {
		return false
	}

	return mv.shapeMgr.IsWater(fromStateID)
}

// CanSwimUp checks if the bot can swim upward in water
func (mv *MovementValidator) CanSwimUp(from, to models.V3) bool {
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
	toStateID, toLoaded := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if !toLoaded || toStateID == 0 {
		return false
	}
	if !mv.shapeMgr.IsWater(toStateID) {
		return false
	}

	// Check if from position is also water (must already be swimming)
	fromStateID, fromLoaded := mv.world.GetBlockAt(from.X, from.Y, from.Z)
	if !fromLoaded || fromStateID == 0 {
		return false
	}

	return mv.shapeMgr.IsWater(fromStateID)
}

// CanSwimDown checks if the bot can swim downward in water
func (mv *MovementValidator) CanSwimDown(from, to models.V3) bool {
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
	toStateID, toLoaded := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if !toLoaded || toStateID == 0 {
		return false
	}
	if !mv.shapeMgr.IsWater(toStateID) {
		return false
	}

	// Check if from position is also water (must already be swimming)
	fromStateID, fromLoaded := mv.world.GetBlockAt(from.X, from.Y, from.Z)
	if !fromLoaded || fromStateID == 0 {
		return false
	}

	return mv.shapeMgr.IsWater(fromStateID)
}

// isOnClimbable checks if the given position has a climbable block (ladder/vine)
func (mv *MovementValidator) isOnClimbable(pos models.V3) bool {
	stateID, loaded := mv.world.GetBlockAt(pos.X, pos.Y, pos.Z)
	if !loaded || stateID == 0 {
		return false
	}
	return mv.shapeMgr.IsClimbable(stateID)
}

// CanEnterClimb checks if the bot can enter a climbable block from an adjacent position
// Requires:
// - Target position has a climbable block (ladder/vine)
// - From and to are adjacent (cardinal direction, same Y or ±1)
func (mv *MovementValidator) CanEnterClimb(from, to models.V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be adjacent horizontally (cardinal direction only)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Must be same Y or ±1
	if dy < -1 || dy > 1 {
		return false
	}

	// Check if target has a climbable block
	return mv.isOnClimbable(to)
}

// CanExitClimb checks if the bot can exit from a climbable block onto an adjacent platform
// Handles two scenarios:
// - Scenario 1: Same-level exit (to block is air with ground support)
// - Scenario 2: Step-up exit (to block is solid, to+1 is air - stepping onto platform)
func (mv *MovementValidator) CanExitClimb(from, to models.V3) bool {
	dx := to.X - from.X
	dz := to.Z - from.Z
	dy := to.Y - from.Y

	// Must be adjacent horizontally (cardinal direction only)
	if (dx != 0 && dz != 0) || (dx == 0 && dz == 0) {
		return false // Diagonal or same position
	}
	if dx*dx+dz*dz > 1 {
		return false // Too far
	}

	// Must be same Y level for the check (we adjust Y later for step-up)
	if dy != 0 {
		return false
	}

	// Must be on a climbable block
	if !mv.isOnClimbable(from) {
		return false
	}

	// Get the block at target position
	toStateID, toLoaded := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if !toLoaded {
		return false
	}

	toPassable := toStateID == 0 || mv.shapeMgr.IsPassable(toStateID)

	// Scenario 1: Same-level exit (to is passable with ground support)
	if toPassable {
		// Check if there's ground to stand on and head clearance
		if mv.hasGroundSupport(to) && mv.isPositionPassable(to) {
			log.Printf("[CanExitClimb] from=(%.0f,%.0f,%.0f) to=(%.0f,%.0f,%.0f) SUCCESS: same-level exit",
				from.X, from.Y, from.Z, to.X, to.Y, to.Z)
			return true
		}
	}

	// Scenario 2: Step-up exit (to is solid floor, step up onto it)
	// Check if to+1 is passable (air above the floor)
	toUp := to.Add(0, 1, 0)
	toUpStateID, toUpLoaded := mv.world.GetBlockAt(toUp.X, toUp.Y, toUp.Z)
	if !toUpLoaded {
		return false
	}

	toUpPassable := toUpStateID == 0 || mv.shapeMgr.IsPassable(toUpStateID)

	if toUpPassable {
		// Check if 'to' block is solid (the floor we step onto)
		toSolid := toStateID != 0 && mv.shapeMgr.IsSolid(toStateID)
		if toSolid {
			// Check head clearance at to+2
			toUp2 := to.Add(0, 2, 0)
			if mv.isBlockPassable(toUp2) {
				log.Printf("[CanExitClimb] from=(%.0f,%.0f,%.0f) to=(%.0f,%.0f,%.0f) SUCCESS: step-up exit to (%.0f,%.0f,%.0f)",
					from.X, from.Y, from.Z, to.X, to.Y, to.Z, toUp.X, toUp.Y, toUp.Z)
				return true
			}
		}
	}

	return false
}

// getExitClimbTargetY determines the correct Y coordinate for an ExitClimb move
// For same-level exits: returns to.Y
// For step-up exits: returns to.Y + 1
func (mv *MovementValidator) getExitClimbTargetY(from, to models.V3) float64 {
	// Check if this is a same-level exit
	toStateID, toLoaded := mv.world.GetBlockAt(to.X, to.Y, to.Z)
	if !toLoaded {
		return to.Y
	}

	toPassable := toStateID == 0 || mv.shapeMgr.IsPassable(toStateID)

	// If to is passable and has ground support, it's a same-level exit
	if toPassable && mv.hasGroundSupport(to) {
		return to.Y
	}

	// Otherwise it's a step-up exit - target is one block higher
	return to.Y + 1
}

// FindGroundBelow finds the nearest valid ground level below the given position
// Returns the Y coordinate where the entity's feet should be (standing on solid ground)
// Returns -1 if no valid ground found
func (mv *MovementValidator) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	log.Printf("[FindGroundBelow] Searching for ground at (%f, %f) from Y=%f", x, z, startY)

	// Check current position first
	currentStateID, _ := mv.world.GetBlockAt(x, startY, z)

	// If current position is inside a solid block, search UPWARD to find surface
	if currentStateID != 0 {
		isSolid := mv.shapeMgr.IsSolid(currentStateID)

		if isSolid {
			log.Printf("[FindGroundBelow] Bot is inside solid block at Y=%f (stateID=%d), searching upward for surface",
				startY, currentStateID)

			// Search upward to find the top of the solid terrain
			for y := startY; y <= startY+maxSearchDepth && y <= 320; y++ {
				pos := models.V3{X: x, Y: y, Z: z}

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
		pos := models.V3{X: x, Y: y, Z: z}

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
func (mv *MovementValidator) logTerrainAround(from models.V3) {
	log.Printf("[TERRAIN] === Terrain around (%.0f, %.0f, %.0f) ===", from.X, from.Y, from.Z)

	// All 8 directions plus center
	directions := []struct {
		name   string
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
			stateID, _ := mv.world.GetBlockAt(x, y, z)

			var blockName string
			var passable bool
			var solid bool

			if stateID == 0 {
				blockName = "air"
				passable = true
				solid = false
			} else {
				blockName = fmt.Sprintf("stateID=%d", stateID)
				passable = mv.shapeMgr.IsPassable(stateID)
				solid = mv.shapeMgr.IsSolid(stateID)
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
// Optionally filters moves based on goal to reduce search space (pass zero models.V3 to disable filtering)
func (mv *MovementValidator) GetPossibleMoves(from models.V3, goal models.V3, prune *MovePruneConfig) []PathStep {
	moves := make([]PathStep, 0, 32) // Increased capacity for more move types

	// Debug: log terrain around start position once
	if mv.debugCheckCount == 0 {
		mv.logTerrainAround(from)
	}

	// Check if we're on a climbable (affects which moves are valid)
	onClimbable := mv.isOnClimbable(from)
	atTopOfClimbable := onClimbable && !mv.CanClimb(from, from.Add(0, 1, 0))

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
		// IMPORTANT: Skip when at top of climbable - use ExitClimb instead
		toUp := from.Add(dir.dx, 1, dir.dz)
		if !atTopOfClimbable {
			// First try AscendStairs (walking up stairs naturally, no jump needed)
			if mv.CanAscendStairs(from, toUp) {
				moves = append(moves, PathStep{
					Position: toUp,
					Movement: AscendStairs,
					Cost:     AscendStairs.BaseCost(),
				})
			} else if mv.CanAscend(from, toUp) {
				// Fall back to AscendJump (jumping up a full block)
				moves = append(moves, PathStep{
					Position: toUp,
					Movement: AscendJump,
					Cost:     AscendJump.BaseCost(),
				})
			}
		}

		// Try descend (1 block down first for stairs, then 1-3 blocks for drops)
		toDown := from.Add(dir.dx, -1, dir.dz)
		// First try DescendStairs (walking down stairs naturally)
		if mv.CanDescendStairs(from, toDown) {
			moves = append(moves, PathStep{
				Position: toDown,
				Movement: DescendStairs,
				Cost:     DescendStairs.BaseCost(),
			})
		} else {
			// Fall back to Descend (dropping 1-3 blocks)
			for dropHeight := float64(1); dropHeight <= 3; dropHeight++ {
				toDropDown := from.Add(dir.dx, -dropHeight, dir.dz)
				if mv.CanDescend(from, toDropDown) {
					moves = append(moves, PathStep{
						Position: toDropDown,
						Movement: Descend,
						Cost:     Descend.BaseCost(),
					})
					break // Only take the first valid drop height
				}
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
		// IMPORTANT: Skip when at top of climbable - use ExitClimb instead
		toUp := from.Add(dir.dx, 1, dir.dz)
		if !atTopOfClimbable && mv.CanDiagonalAscend(from, toUp) {
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

	// EnterClimb - entering a climbable block from adjacent position
	for _, dir := range cardinalDirs {
		// Same level enter
		to := from.Add(dir.dx, 0, dir.dz)
		if mv.CanEnterClimb(from, to) {
			moves = append(moves, PathStep{
				Position: to,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}

		// Enter from below (jumping to grab ladder)
		toUp := from.Add(dir.dx, 1, dir.dz)
		if mv.CanEnterClimb(from, toUp) {
			moves = append(moves, PathStep{
				Position: toUp,
				Movement: JumpToClimb,
				Cost:     JumpToClimb.BaseCost(),
			})
		}

		// Enter from above (dropping onto ladder)
		toDown := from.Add(dir.dx, -1, dir.dz)
		if mv.CanEnterClimb(from, toDown) {
			moves = append(moves, PathStep{
				Position: toDown,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}
	}

	// ExitClimb - exiting from a climbable block onto adjacent platform
	// Only check if we're currently on a climbable and can't climb further up
	isOnClimb := mv.isOnClimbable(from)
	canClimbUp := mv.CanClimb(from, from.Add(0, 1, 0))

	if isOnClimb && !canClimbUp {
		// At top of ladder/vine - check for exits in cardinal directions
		for _, dir := range cardinalDirs {
			to := from.Add(dir.dx, 0, dir.dz)
			if mv.CanExitClimb(from, to) {
				// Determine correct Y coordinate for the exit
				targetY := mv.getExitClimbTargetY(from, to)
				targetPos := models.V3{X: to.X, Y: targetY, Z: to.Z}

				moves = append(moves, PathStep{
					Position: targetPos,
					Movement: ExitClimb,
					Cost:     ExitClimb.BaseCost(),
				})
			}
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

	// Prune moves that go too far from goal (if pruning config provided)
	if prune != nil && (goal.X != 0 || goal.Y != 0 || goal.Z != 0) {
		fromDist := from.DistanceTo(goal)
		filtered := make([]PathStep, 0, len(moves))

		// Debug: log first call to understand terrain
		if mv.debugCheckCount < 2 {
			log.Printf("[GetPossibleMoves] from=(%.0f,%.0f,%.0f) goal=(%.0f,%.0f,%.0f) dist=%.1f",
				from.X, from.Y, from.Z, goal.X, goal.Y, goal.Z, fromDist)
			log.Printf("[GetPossibleMoves] Generated %d moves (AscendStairs=%d)", len(moves), 0)
		}

		pruned := 0
		for _, move := range moves {
			toDist := move.Position.DistanceTo(goal)
			// Allow moves that get closer, or slightly farther (for obstacle avoidance)
			// Use DriftCap from prune config
			if toDist <= fromDist+prune.DriftCap {
				filtered = append(filtered, move)
			} else {
				pruned++
			}
		}

		if mv.debugCheckCount < 2 {
			log.Printf("[GetPossibleMoves] After distance pruning: %d moves (pruned %d)",
				len(filtered), pruned)
			mv.debugCheckCount++
		}

		// Remove duplicates
		seen := make(map[models.V3]bool)
		unique := make([]PathStep, 0, len(filtered))
		dups := 0
		for _, move := range filtered {
			if !seen[move.Position] {
				seen[move.Position] = true
				unique = append(unique, move)
			} else {
				dups++
			}
		}

		if mv.debugCheckCount < 2 {
			log.Printf("[GetPossibleMoves] After deduplication: %d moves (removed %d duplicates)",
				len(unique), dups)
		}

		return unique
	}

	return moves
}
