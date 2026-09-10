package pathfinding

import (
	"container/heap"
	"context"
	"log/slog"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// RideableEntity represents a rideable vehicle entity in the world.
type RideableEntity struct {
	EntityID   int32
	EntityType models.EntityType
	Position   models.V3
}

// VehicleProvider interface allows querying for nearby rideable entities.
type VehicleProvider interface {
	// FindRideableEntitiesNear returns rideable entities within radius of
	// center. honorPerceptionEffects, when true, additionally excludes
	// candidates beyond the agent's own effective vision range under
	// Blindness/Darkness (see physics.PerceptionRadiusCap) — real
	// pathfinding decisions should behave as if the effect matters.
	FindRideableEntitiesNear(center models.V3, radius float64, honorPerceptionEffects bool) []RideableEntity
}

// VehicleAwarePathFinder wraps any PathFinder and considers vehicle paths as well as foot paths.
type VehicleAwarePathFinder struct {
	base            models.PathFinder
	vehicleProvider VehicleProvider
	world           models.World
	shapeMgr        models.BlockShapeManager
	searchRadius    float64
	logger          *slog.Logger
}

// NewVehicleAwarePathFinder creates a new vehicle-aware pathfinder.
func NewVehicleAwarePathFinder(
	base models.PathFinder,
	provider VehicleProvider,
	world models.World,
	shapeMgr models.BlockShapeManager,
	searchRadius float64,
	logger *slog.Logger,
) models.PathFinder {
	if searchRadius <= 0 {
		searchRadius = 32.0 // Default search radius
	}

	return &VehicleAwarePathFinder{
		base:            base,
		vehicleProvider: provider,
		world:           world,
		shapeMgr:        shapeMgr,
		searchRadius:    searchRadius,
		logger:          utils.SafeLogger(logger),
	}
}

// FindPath finds the best path considering both foot and vehicle options.
func (vap *VehicleAwarePathFinder) FindPath(
	ctx context.Context,
	start, goal models.V3,
	maxSteps int,
) (*Path, error) {
	// Find baseline foot path
	footPath, err := vap.base.FindPath(ctx, start, goal, maxSteps)
	if err != nil {
		return footPath, err
	}

	if !footPath.Found {
		return footPath, nil // No foot path found; return empty
	}

	// Check for nearby vehicles. Real path-planning should behave as if
	// Blindness/Darkness matters — an agent that can't see past 5-15 blocks
	// shouldn't detour to a mount outside its own vision.
	vehicles := vap.vehicleProvider.FindRideableEntitiesNear(start, vap.searchRadius, true)
	if len(vehicles) == 0 {
		return footPath, nil // No vehicles nearby; use foot path
	}

	utils.SafeLogger(vap.logger).Debug("[VehicleAware] found rideable entities", "count", len(vehicles), "x", start.X, "y", start.Y, "z", start.Z)

	// Try each vehicle and keep the best path
	bestPath := footPath
	bestCost := footPath.TotalCost

	for _, vehicle := range vehicles {
		vehiclePath := vap.buildPathWithVehicle(ctx, start, goal, vehicle, maxSteps)
		if vehiclePath != nil && vehiclePath.Found && vehiclePath.TotalCost < bestCost {
			utils.SafeLogger(vap.logger).Debug("[VehicleAware] vehicle path found", "entityType", vehicle.EntityType, "entityID", vehicle.EntityID, "cost", vehiclePath.TotalCost, "footCost", footPath.TotalCost)
			bestPath = vehiclePath
			bestCost = vehiclePath.TotalCost
		}
	}

	return bestPath, nil
}

// buildPathWithVehicle constructs a path that uses a specific vehicle:
// foot_to_vehicle + MountVehicle + vehicle_segment + DismountVehicle + foot_to_goal
func (vap *VehicleAwarePathFinder) buildPathWithVehicle(
	ctx context.Context,
	start, goal models.V3,
	vehicle RideableEntity,
	maxSteps int,
) *Path {
	// Get vehicle capabilities
	vt := models.GetVehicleType(vehicle.EntityType)
	caps := models.GetVehicleCapabilities(vt)

	if !vt.IsRideable() || !caps.CanTraverseLand && !caps.CanTraverseLava &&
		!caps.CanTraverseWater && !caps.RequiresRails && !caps.CanFly3D {
		return nil // Vehicle not viable
	}

	// Find path from start to vehicle
	pathToVehicle, err := vap.base.FindPath(ctx, start, vehicle.Position, maxSteps)
	if err != nil || !pathToVehicle.Found {
		return nil
	}

	// Find vehicle path from vehicle position toward goal
	vehiclePath := vap.buildVehiclePath(ctx, vehicle.Position, goal, vehicle.EntityID, &caps, maxSteps)
	if vehiclePath == nil || !vehiclePath.Found {
		return nil
	}

	// Find foot path from vehicle goal to final goal
	footToGoal, err := vap.base.FindPath(ctx, vehiclePath.GoalPos, goal, maxSteps)
	if err != nil || !footToGoal.Found {
		return nil
	}

	// Splice paths together
	return vap.splicePaths(pathToVehicle, vehiclePath, footToGoal, vehicle.EntityID)
}

// buildVehiclePath finds a path for a vehicle from start toward goal.
func (vap *VehicleAwarePathFinder) buildVehiclePath(
	ctx context.Context,
	start, goal models.V3,
	vehicleEntityID int32,
	caps *models.VehicleCapabilities,
	maxSteps int,
) *Path {
	validator := NewVehicleMovementValidator(vap.world, vap.shapeMgr, *caps, vap.logger)

	// Custom A* for vehicle movement using VehicleMovementValidator
	startTime := time.Now()
	openSet := &nodeHeap{}
	heap.Init(openSet)

	closedSet := make(map[models.V3]bool)
	gScores := make(map[models.V3]float64)

	startNode := &node{
		pos:      start,
		parent:   nil,
		movement: models.VehicleTraverse,
		gCost:    0,
		hCost:    heuristic(start, goal),
	}
	startNode.fCost = startNode.gCost + startNode.hCost

	heap.Push(openSet, startNode)

	prune := &MovePruneConfig{
		StartDist: start.DistanceTo(goal),
		DriftCap:  4.0,
	}

	for openSet.Len() > 0 {
		// Check context
		if ctx.Err() != nil {
			return &Path{Found: false, StartPos: start, GoalPos: goal}
		}

		currentNode := heap.Pop(openSet).(*node)

		// Check if we've reached the goal
		if currentNode.pos.DistanceTo(goal) <= 0.5 {
			return vap.reconstructVehiclePath(currentNode, start, goal, startTime, vehicleEntityID)
		}

		// Skip if already closed
		if closedSet[currentNode.pos] {
			continue
		}
		closedSet[currentNode.pos] = true

		// Get possible vehicle moves
		neighbors := validator.GetPossibleVehicleMoves(currentNode.pos, goal, vehicleEntityID, prune)

		for _, neighborStep := range neighbors {
			if closedSet[neighborStep.Position] {
				continue
			}

			tentativeGCost := currentNode.gCost + neighborStep.Cost
			prevGCost, exists := gScores[neighborStep.Position]

			if !exists || tentativeGCost < prevGCost {
				gScores[neighborStep.Position] = tentativeGCost

				neighborNode := &node{
					pos:      neighborStep.Position,
					parent:   currentNode,
					movement: neighborStep.Movement,
					gCost:    tentativeGCost,
					hCost:    heuristic(neighborStep.Position, goal),
				}
				neighborNode.fCost = neighborNode.gCost + neighborNode.hCost

				heap.Push(openSet, neighborNode)
			}
		}
	}

	// No path found
	return &Path{Found: false, StartPos: start, GoalPos: goal, SearchTime: float64(time.Since(startTime).Milliseconds())}
}

// reconstructVehiclePath reconstructs a vehicle path from goal node back to start.
func (vap *VehicleAwarePathFinder) reconstructVehiclePath(
	goalNode *node,
	start, goal models.V3,
	startTime time.Time,
	vehicleEntityID int32,
) *Path {
	steps := make([]PathStep, 0)
	current := goalNode
	totalCost := 0.0

	for current != nil {
		if current.parent != nil {
			steps = append(steps, PathStep{
				Position:        current.pos,
				Movement:        current.movement,
				Cost:            current.gCost - current.parent.gCost,
				VehicleEntityID: vehicleEntityID,
			})
			totalCost += current.gCost - current.parent.gCost
		}
		current = current.parent
	}

	// Reverse to get start->goal order
	for i := len(steps)/2 - 1; i >= 0; i-- {
		opp := len(steps) - 1 - i
		steps[i], steps[opp] = steps[opp], steps[i]
	}

	return &Path{
		Steps:      steps,
		TotalCost:  totalCost,
		StartPos:   start,
		GoalPos:    goal,
		Found:      true,
		SearchTime: float64(time.Since(startTime).Milliseconds()),
	}
}

// splicePaths combines three paths: foot -> vehicle -> foot
func (vap *VehicleAwarePathFinder) splicePaths(
	pathToVehicle, vehiclePath, pathFromVehicle *Path,
	vehicleEntityID int32,
) *Path {
	steps := make([]PathStep, 0, len(pathToVehicle.Steps)+len(vehiclePath.Steps)+len(pathFromVehicle.Steps)+2)

	// Add foot path to vehicle
	for _, step := range pathToVehicle.Steps {
		steps = append(steps, step)
	}

	// Add mount vehicle step
	mountPos := pathToVehicle.GoalPos
	steps = append(steps, PathStep{
		Position:        mountPos,
		Movement:        models.MountVehicle,
		Cost:            models.MountVehicle.BaseCost(),
		VehicleEntityID: vehicleEntityID,
	})

	// Add vehicle path
	for _, step := range vehiclePath.Steps {
		step.VehicleEntityID = vehicleEntityID
		steps = append(steps, step)
	}

	// Add dismount vehicle step
	dismountPos := vehiclePath.GoalPos
	steps = append(steps, PathStep{
		Position:        dismountPos,
		Movement:        models.DismountVehicle,
		Cost:            models.DismountVehicle.BaseCost(),
		VehicleEntityID: vehicleEntityID,
	})

	// Add foot path from vehicle to goal
	for _, step := range pathFromVehicle.Steps {
		steps = append(steps, step)
	}

	totalCost := pathToVehicle.TotalCost + models.MountVehicle.BaseCost() +
		vehiclePath.TotalCost + models.DismountVehicle.BaseCost() +
		pathFromVehicle.TotalCost

	return &Path{
		Steps:      steps,
		TotalCost:  totalCost,
		StartPos:   pathToVehicle.StartPos,
		GoalPos:    pathFromVehicle.GoalPos,
		Found:      true,
		SearchTime: pathToVehicle.SearchTime + vehiclePath.SearchTime + pathFromVehicle.SearchTime,
	}
}

// FindGroundBelow delegates to the base pathfinder.
func (vap *VehicleAwarePathFinder) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return vap.base.FindGroundBelow(x, z, startY, maxSearchDepth)
}
