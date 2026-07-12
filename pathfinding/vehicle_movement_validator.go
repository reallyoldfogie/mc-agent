package pathfinding

import (
	"github.com/reallyoldfogie/mc-agent/models"
)

// VehicleMovementValidator generates movement steps for a vehicle based on its capabilities.
type VehicleMovementValidator struct {
	caps       models.VehicleCapabilities
	world      models.World
	shapeMgr   models.BlockShapeManager
	validator  *MovementValidator // Reuse foot movement checks for land vehicles
}

// NewVehicleMovementValidator creates a new vehicle movement validator.
func NewVehicleMovementValidator(
	world models.World,
	shapeMgr models.BlockShapeManager,
	caps models.VehicleCapabilities,
) *VehicleMovementValidator {
	return &VehicleMovementValidator{
		caps:      caps,
		world:     world,
		shapeMgr:  shapeMgr,
		validator: NewMovementValidator(world, shapeMgr),
	}
}

// GetPossibleVehicleMoves returns path steps for a vehicle at 'from' heading toward 'goal'.
// Uses the vehicle's capabilities to determine which terrain types are allowed.
func (v *VehicleMovementValidator) GetPossibleVehicleMoves(
	from, goal models.V3,
	vehicleEntityID int32,
	prune *MovePruneConfig,
) []PathStep {
	moves := make([]PathStep, 0, 16)

	// Only generate moves for terrains this vehicle can traverse
	if v.caps.CanTraverseLand {
		moves = v.generateLandVehicleMoves(from, goal, vehicleEntityID, moves)
	}
	if v.caps.CanTraverseLava {
		moves = v.generateLavaVehicleMoves(from, goal, vehicleEntityID, moves)
	}
	if v.caps.CanTraverseWater && !v.caps.RequiresRails {
		moves = v.generateWaterVehicleMoves(from, goal, vehicleEntityID, moves)
	}
	if v.caps.RequiresRails {
		moves = v.generateRailVehicleMoves(from, goal, vehicleEntityID, moves)
	}
	if v.caps.CanFly3D {
		moves = v.generateFly3DVehicleMoves(from, goal, vehicleEntityID, moves)
	}

	// Apply pruning if configured
	if prune != nil {
		moves = v.pruneMovesTowardGoal(from, goal, moves, prune)
	}

	return moves
}

// generateLandVehicleMoves generates movement options for land vehicles (horses, camels, pigs, etc.)
func (v *VehicleMovementValidator) generateLandVehicleMoves(
	from, goal models.V3,
	vehicleEntityID int32,
	moves []PathStep,
) []PathStep {
	// Cardinal directions
	cardinalDirs := [][2]float64{
		{1, 0},   // East
		{-1, 0},  // West
		{0, 1},   // South
		{0, -1},  // North
	}

	for _, dir := range cardinalDirs {
		// Traverse at same level
		to := from.Add(models.V3{X: dir[0], Y: 0, Z: dir[1]})
		if v.validator.CanTraverse(from, to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleTraverse,
				Cost:            models.VehicleTraverse.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}

		// Ascend 1 block (only if vehicle can ascend)
		if v.caps.CanAscend {
			toUp := from.Add(models.V3{X: dir[0], Y: 1, Z: dir[1]})
			if v.validator.CanAscend(from, toUp) || v.validator.CanAscendStairs(from, toUp) {
				moves = append(moves, PathStep{
					Position:        toUp,
					Movement:        models.VehicleAscend,
					Cost:            models.VehicleAscend.BaseCost(),
					VehicleEntityID: vehicleEntityID,
				})
			}
		}

		// Descend 1-2 blocks
		for dropHeight := float64(1); dropHeight <= 2; dropHeight++ {
			toDown := from.Add(models.V3{X: dir[0], Y: -dropHeight, Z: dir[1]})
			if v.validator.CanDescend(from, toDown) {
				moves = append(moves, PathStep{
					Position:        toDown,
					Movement:        models.VehicleTraverse,
					Cost:            models.VehicleTraverse.BaseCost(),
					VehicleEntityID: vehicleEntityID,
				})
				break // Only take the first valid drop height
			}
		}
	}

	// Diagonal traverse (no ascend/descend for diagonals on vehicles)
	diagonalDirs := [][2]float64{
		{1, 1},   // SE
		{1, -1},  // NE
		{-1, 1},  // SW
		{-1, -1}, // NW
	}

	for _, dir := range diagonalDirs {
		to := from.Add(models.V3{X: dir[0], Y: 0, Z: dir[1]})
		if v.validator.CanDiagonalTraverse(from, to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleTraverse,
				Cost:            models.VehicleTraverse.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}
	}

	return moves
}

// generateLavaVehicleMoves generates movement options for lava vehicles (strider).
func (v *VehicleMovementValidator) generateLavaVehicleMoves(
	from, goal models.V3,
	vehicleEntityID int32,
	moves []PathStep,
) []PathStep {
	// Cardinal directions only for lava travel
	cardinalDirs := [][2]float64{
		{1, 0},
		{-1, 0},
		{0, 1},
		{0, -1},
	}

	for _, dir := range cardinalDirs {
		// Traverse horizontally on lava
		to := from.Add(models.V3{X: dir[0], Y: 0, Z: dir[1]})
		if v.isOnLava(to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleLavaTraverse,
				Cost:            models.VehicleLavaTraverse.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}

		// Lava ascend (stepping up on lava)
		toUp := from.Add(models.V3{X: dir[0], Y: 1, Z: dir[1]})
		if v.isOnLava(toUp) {
			moves = append(moves, PathStep{
				Position:        toUp,
				Movement:        models.VehicleLavaTraverse,
				Cost:            models.VehicleLavaTraverse.BaseCost() + 0.5,
				VehicleEntityID: vehicleEntityID,
			})
		}

		// Lava descend
		for dropHeight := float64(1); dropHeight <= 2; dropHeight++ {
			toDown := from.Add(models.V3{X: dir[0], Y: -dropHeight, Z: dir[1]})
			if v.isOnLava(toDown) {
				moves = append(moves, PathStep{
					Position:        toDown,
					Movement:        models.VehicleLavaTraverse,
					Cost:            models.VehicleLavaTraverse.BaseCost(),
					VehicleEntityID: vehicleEntityID,
				})
				break
			}
		}
	}

	return moves
}

// generateWaterVehicleMoves generates movement options for water vehicles (boats).
func (v *VehicleMovementValidator) generateWaterVehicleMoves(
	from, goal models.V3,
	vehicleEntityID int32,
	moves []PathStep,
) []PathStep {
	// All 8 directions for boat travel
	allDirs := [][2]float64{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},     // Cardinal
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1}, // Diagonal
	}

	for _, dir := range allDirs {
		to := from.Add(models.V3{X: dir[0], Y: 0, Z: dir[1]})
		if v.isOnWater(to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleSwim,
				Cost:            models.VehicleSwim.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}

		// Ascending on water (boats can climb out of water)
		toUp := from.Add(models.V3{X: dir[0], Y: 1, Z: dir[1]})
		if v.isOnWater(toUp) || v.validator.CanTraverse(from, toUp) {
			moves = append(moves, PathStep{
				Position:        toUp,
				Movement:        models.VehicleSwim,
				Cost:            models.VehicleSwim.BaseCost() + 0.5,
				VehicleEntityID: vehicleEntityID,
			})
		}
	}

	return moves
}

// generateRailVehicleMoves generates movement options for rail vehicles (minecarts).
// Minecarts can only move forward/backward on rails.
func (v *VehicleMovementValidator) generateRailVehicleMoves(
	from, goal models.V3,
	vehicleEntityID int32,
	moves []PathStep,
) []PathStep {
	// Cardinal directions only (minecarts don't turn in place on rails)
	cardinalDirs := [][2]float64{
		{1, 0},   // East
		{-1, 0},  // West
		{0, 1},   // South
		{0, -1},  // North
	}

	for _, dir := range cardinalDirs {
		// Forward on rail
		to := from.Add(models.V3{X: dir[0], Y: 0, Z: dir[1]})
		if v.isOnRail(to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleRailTraverse,
				Cost:            models.VehicleRailTraverse.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}

		// Ascend rail (upward slope)
		toUp := from.Add(models.V3{X: dir[0], Y: 1, Z: dir[1]})
		if v.isOnRail(toUp) {
			moves = append(moves, PathStep{
				Position:        toUp,
				Movement:        models.VehicleRailTraverse,
				Cost:            models.VehicleRailTraverse.BaseCost() + 0.5,
				VehicleEntityID: vehicleEntityID,
			})
		}

		// Descend rail (downward slope)
		toDown := from.Add(models.V3{X: dir[0], Y: -1, Z: dir[1]})
		if v.isOnRail(toDown) {
			moves = append(moves, PathStep{
				Position:        toDown,
				Movement:        models.VehicleRailTraverse,
				Cost:            models.VehicleRailTraverse.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}
	}

	return moves
}

// generateFly3DVehicleMoves generates movement options for 3D flying vehicles (nautilus).
// Nautilus can move in all 6 directions (±X, ±Y, ±Z) in water.
func (v *VehicleMovementValidator) generateFly3DVehicleMoves(
	from, goal models.V3,
	vehicleEntityID int32,
	moves []PathStep,
) []PathStep {
	// All 6 directions (up, down, all 4 cardinal)
	directions := [][2]float64{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1}, // Cardinal
	}

	for _, dir := range directions {
		to := from.Add(models.V3{X: dir[0], Y: 0, Z: dir[1]})
		if v.isFullyInWater(to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleFly3D,
				Cost:            models.VehicleFly3D.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}
	}

	// Up and down in water
	for _, dy := range []float64{1, -1} {
		to := from.Add(models.V3{X: 0, Y: dy, Z: 0})
		if v.isFullyInWater(to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleFly3D,
				Cost:            models.VehicleFly3D.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}
	}

	// Diagonals at the same Y level
	diagonalDirs := [][2]float64{
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
	}

	for _, dir := range diagonalDirs {
		to := from.Add(models.V3{X: dir[0], Y: 0, Z: dir[1]})
		if v.isFullyInWater(to) {
			moves = append(moves, PathStep{
				Position:        to,
				Movement:        models.VehicleFly3D,
				Cost:            models.VehicleFly3D.BaseCost(),
				VehicleEntityID: vehicleEntityID,
			})
		}
	}

	return moves
}

// isOnLava checks if a position is on a lava surface.
func (v *VehicleMovementValidator) isOnLava(pos models.V3) bool {
	// Check block at position
	blockStateID, loaded := v.world.GetBlockAt(pos.X, pos.Y, pos.Z)
	if !loaded || blockStateID == 0 {
		return false
	}

	return v.shapeMgr.IsLava(blockStateID)
}

// isOnWater checks if a position is on a water surface.
func (v *VehicleMovementValidator) isOnWater(pos models.V3) bool {
	// Check if position is in water or just above water
	blockStateID, loaded := v.world.GetBlockAt(pos.X, pos.Y, pos.Z)
	if !loaded {
		return false
	}

	if blockStateID == 0 {
		// Check if water is directly above
		blockAbove, loadedAbove := v.world.GetBlockAt(pos.X, pos.Y+1, pos.Z)
		return loadedAbove && v.shapeMgr.IsWater(blockAbove)
	}

	return v.shapeMgr.IsWater(blockStateID)
}

// isFullyInWater checks if a position is fully submerged in water (nautilus).
func (v *VehicleMovementValidator) isFullyInWater(pos models.V3) bool {
	blockStateID, loaded := v.world.GetBlockAt(pos.X, pos.Y, pos.Z)
	if !loaded || blockStateID == 0 {
		return false
	}

	return v.shapeMgr.IsWater(blockStateID)
}

// isOnRail checks if a position has a rail block below it.
func (v *VehicleMovementValidator) isOnRail(pos models.V3) bool {
	// Check block below position
	blockBelow, loaded := v.world.GetBlockAt(pos.X, pos.Y-1, pos.Z)
	if !loaded || blockBelow == 0 {
		return false
	}

	// Check if block is a rail type
	blockName := v.shapeMgr.BlockName(blockBelow)
	return blockName == "rail" || blockName == "powered_rail" ||
		blockName == "detector_rail" || blockName == "activator_rail"
}

// pruneMovesTowardGoal filters moves that drift too far from the goal direction.
func (v *VehicleMovementValidator) pruneMovesTowardGoal(
	from, goal models.V3,
	moves []PathStep,
	prune *MovePruneConfig,
) []PathStep {
	if prune == nil {
		return moves
	}

	fromDist := from.DistanceTo(goal)
	filtered := make([]PathStep, 0, len(moves))

	for _, step := range moves {
		toDist := step.Position.DistanceTo(goal)
		// Allow moves that get closer to the goal or don't drift more than the cap
		if toDist <= fromDist || (toDist-fromDist) <= prune.DriftCap {
			filtered = append(filtered, step)
		}
	}

	return filtered
}
