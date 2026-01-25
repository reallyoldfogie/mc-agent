package pathfinding

import (
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// CollisionChecker provides methods for checking path collisions
type CollisionChecker struct {
	world        World
	shapeManager BlockShapeManager
	playerWidth  float64
	playerHeight float64
}

// NewCollisionChecker creates a collision checker
func NewCollisionChecker(world World, shapeManager BlockShapeManager) *CollisionChecker {
	return &CollisionChecker{
		world:        world,
		shapeManager: shapeManager,
		playerWidth:  0.6, // Standard player width
		playerHeight: 1.8, // Standard player height
	}
}

// CheckPathClear verifies if a straight-line path is clear of collisions.
// Returns true if the path is clear, false if blocked.
func (cc *CollisionChecker) CheckPathClear(from, to models.V3) bool {
	// Calculate path direction
	dx := to.X - from.X
	dy := to.Y - from.Y
	dz := to.Z - from.Z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	if distance < 0.01 {
		return true // Already at destination
	}

	// Normalize direction
	dx /= distance
	dy /= distance
	dz /= distance

	// Check collision at intervals along the path
	steps := int(math.Ceil(distance / 0.5)) // Check every 0.5 blocks
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		checkPos := models.V3{
			X: from.X + dx*distance*t,
			Y: from.Y + dy*distance*t,
			Z: from.Z + dz*distance*t,
		}

		if cc.wouldCollideAt(checkPos) {
			return false
		}
	}

	return true
}

// wouldCollideAt checks if the player would collide at a given position
func (cc *CollisionChecker) wouldCollideAt(pos models.V3) bool {
	// Check blocks that player hitbox would overlap
	minX := int(math.Floor(pos.X - cc.playerWidth/2))
	maxX := int(math.Ceil(pos.X + cc.playerWidth/2))
	minY := int(math.Floor(pos.Y))
	maxY := int(math.Ceil(pos.Y + cc.playerHeight))
	minZ := int(math.Floor(pos.Z - cc.playerWidth/2))
	maxZ := int(math.Ceil(pos.Z + cc.playerWidth/2))

	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			for z := minZ; z < maxZ; z++ {
				blockStateID, loaded := cc.world.GetBlockAt(float64(x), float64(y), float64(z))
				if !loaded {
					continue
				}

				// Check if block is solid (not passable)
				if !cc.shapeManager.IsPassable(blockStateID) {
					return true // Collision detected
				}
			}
		}
	}

	return false
}

// PathCollisionCost calculates additional cost for risky paths.
// Paths near walls or tight spaces get higher cost.
func (cc *CollisionChecker) PathCollisionCost(from, to models.V3) float64 {
	// Base cost is distance
	dx := to.X - from.X
	dy := to.Y - from.Y
	dz := to.Z - from.Z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Check if path is clear
	if !cc.CheckPathClear(from, to) {
		// Path is blocked - return very high cost
		return distance * 1000.0
	}

	// Check clearance (distance to nearest wall)
	clearance := cc.calculateClearance(from, to)

	// Tight paths get penalty
	if clearance < 0.7 { // Less than player width + margin
		tightnessPenalty := (0.7 - clearance) * 2.0
		return distance * (1.0 + tightnessPenalty)
	}

	return distance
}

// calculateClearance estimates minimum clearance along path
func (cc *CollisionChecker) calculateClearance(from, to models.V3) float64 {
	minClearance := 10.0 // Start with large value

	// Sample a few points along the path
	steps := 5
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		pos := models.V3{
			X: from.X + (to.X-from.X)*t,
			Y: from.Y + (to.Y-from.Y)*t,
			Z: from.Z + (to.Z-from.Z)*t,
		}

		clearance := cc.getClearanceAt(pos)
		if clearance < minClearance {
			minClearance = clearance
		}
	}

	return minClearance
}

// getClearanceAt gets clearance (distance to nearest solid block) at a position
func (cc *CollisionChecker) getClearanceAt(pos models.V3) float64 {
	minDistance := 5.0 // Check within 5 blocks

	// Check nearby blocks
	for dy := -2; dy <= 2; dy++ {
		for dx := -3; dx <= 3; dx++ {
			for dz := -3; dz <= 3; dz++ {
				blockX := int(math.Floor(pos.X)) + dx
				blockY := int(math.Floor(pos.Y)) + dy
				blockZ := int(math.Floor(pos.Z)) + dz

				blockStateID, loaded := cc.world.GetBlockAt(float64(blockX), float64(blockY), float64(blockZ))
				if !loaded {
					continue
				}

				if !cc.shapeManager.IsPassable(blockStateID) {
					// Calculate distance to this solid block
					blockCenter := models.V3{
						X: float64(blockX) + 0.5,
						Y: float64(blockY) + 0.5,
						Z: float64(blockZ) + 0.5,
					}
					dist := math.Sqrt(
						math.Pow(pos.X-blockCenter.X, 2) +
							math.Pow(pos.Y-blockCenter.Y, 2) +
							math.Pow(pos.Z-blockCenter.Z, 2),
					)

					if dist < minDistance {
						minDistance = dist
					}
				}
			}
		}
	}

	return minDistance
}

// IsMovementPossible checks if a specific movement type is physically possible.
// This is a simplified check - would be enhanced with full movement validation.
func (cc *CollisionChecker) IsMovementPossible(movementType MovementType, from, to models.V3) bool {
	// Check if destination is clear
	if cc.wouldCollideAt(to) {
		return false
	}

	// Check if path is clear (for traverse movements)
	if movementType == Traverse || movementType == Sprint || movementType == Sneak ||
		movementType == DiagonalTraverse ||
		movementType == TraverseNorthEast || movementType == TraverseNorthWest ||
		movementType == TraverseSouthEast || movementType == TraverseSouthWest {
		return cc.CheckPathClear(from, to)
	}

	// For jumps and climbs, just check destination
	// More sophisticated checks would verify the arc/climb path
	return true
}
