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

// BoatInventoryChecker reports whether the agent currently carries an item
// that could be placed as a boat and mounted (see agent.PlaceAndMountBoat).
// VehicleAwarePathFinder calls this once per FindPath call, not once per
// move-generation step — inventory doesn't change mid-search, so there's no
// need to re-check it on every candidate move. See
// docs/plans/WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8.
type BoatInventoryChecker interface {
	HasPlaceableBoat() bool
}

// VehicleAwarePathFinder wraps any PathFinder and considers vehicle paths as well as foot paths.
type VehicleAwarePathFinder struct {
	base            models.PathFinder
	vehicleProvider VehicleProvider
	world           models.World
	shapeMgr        models.BlockShapeManager
	searchRadius    float64
	logger          *slog.Logger
	// boatChecker is optional (nil-safe) — set via SetBoatInventoryChecker.
	// When nil, boat-placement candidates are never considered, matching the
	// pre-Item-8 behavior exactly.
	boatChecker BoatInventoryChecker
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

// SetBoatInventoryChecker wires in the optional carried-boat check (see
// BoatInventoryChecker's doc comment). Not required — a VehicleAwarePathFinder
// with no checker set behaves exactly as before Item 8, only ever considering
// vehicles that already exist in the world.
func (vap *VehicleAwarePathFinder) SetBoatInventoryChecker(checker BoatInventoryChecker) {
	vap.boatChecker = checker
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

	bestPath := footPath
	bestCost := footPath.TotalCost

	// Check for nearby vehicles. Real path-planning should behave as if
	// Blindness/Darkness matters — an agent that can't see past 5-15 blocks
	// shouldn't detour to a mount outside its own vision.
	vehicles := vap.vehicleProvider.FindRideableEntitiesNear(start, vap.searchRadius, true)
	if len(vehicles) > 0 {
		utils.SafeLogger(vap.logger).Debug("[VehicleAware] found rideable entities", "count", len(vehicles), "x", start.X, "y", start.Y, "z", start.Z)

		for _, vehicle := range vehicles {
			vehiclePath := vap.buildPathWithVehicle(ctx, start, goal, vehicle, maxSteps)
			if vehiclePath != nil && vehiclePath.Found && vehiclePath.TotalCost < bestCost {
				utils.SafeLogger(vap.logger).Debug("[VehicleAware] vehicle path found", "entityType", vehicle.EntityType, "entityID", vehicle.EntityID, "cost", vehiclePath.TotalCost, "footCost", footPath.TotalCost)
				bestPath = vehiclePath
				bestCost = vehiclePath.TotalCost
			}
		}
	}

	// Boat placement: checked once here, not once per move-generation step —
	// inventory doesn't change mid-search. Considered independently of
	// whether any in-world vehicle exists.
	if vap.boatChecker != nil && vap.boatChecker.HasPlaceableBoat() {
		placementPath := vap.buildPathWithBoatPlacement(ctx, footPath, goal, maxSteps)
		if placementPath != nil && placementPath.Found && placementPath.TotalCost < bestCost {
			utils.SafeLogger(vap.logger).Debug("[VehicleAware] boat placement path found", "cost", placementPath.TotalCost, "footCost", footPath.TotalCost)
			bestPath = placementPath
			bestCost = placementPath.TotalCost
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

// buildPathWithBoatPlacement constructs a path that places a carried boat at
// the water's edge and rides it toward goal:
// foot_to_water_edge + PlaceVehicle + vehicle_segment + DismountVehicle + foot_to_goal.
//
// The placement point is derived from footPath's own first WadeWater/Swim
// step, rather than an independent "nearest water" search — this directly
// covers the scenario this whole plan is about (a foot path that already
// crosses water, per pathfinding/movement.go's WadeWater/Swim generation),
// letting a boat compete as a cheaper alternative for that same crossing.
// Known scope limit: if the base pathfinder avoided water entirely (chose a
// dry detour because swimming wasn't worth it), footPath contains no
// water-touching step at all, so this returns nil even in cases where
// placing a boat might have been worth a detour of its own — extending this
// to search for a water crossing independent of the already-chosen foot path
// is real, separate follow-up work, not attempted here. See
// docs/plans/WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8.
func (vap *VehicleAwarePathFinder) buildPathWithBoatPlacement(
	ctx context.Context,
	footPath *Path,
	goal models.V3,
	maxSteps int,
) *Path {
	waterStepIdx := -1
	for i, step := range footPath.Steps {
		if step.Movement == models.WadeWater || step.Movement == models.Swim {
			waterStepIdx = i
			break
		}
	}
	if waterStepIdx < 0 {
		return nil // Foot path never touches water; nothing to place a boat onto
	}

	// The vehicle segment targets the FAR shore (the last step of this
	// contiguous water crossing in footPath), not the overall goal directly.
	// A boat-only search (caps.CanTraverseLand=false) can never satisfy
	// buildVehiclePath's "within 0.5 blocks of goal" completion condition if
	// goal is inland past the crossing - it would just exhaust its open set
	// and report not-found. Deriving both shore points from footPath (which
	// already knows exactly where the water starts and ends) sidesteps that
	// entirely.
	farShoreStepIdx := waterStepIdx
	for farShoreStepIdx+1 < len(footPath.Steps) {
		next := footPath.Steps[farShoreStepIdx+1]
		if next.Movement != models.WadeWater && next.Movement != models.Swim {
			break
		}
		farShoreStepIdx++
	}

	waterPos := footPath.Steps[waterStepIdx].Position
	farShorePos := footPath.Steps[farShoreStepIdx].Position

	landSteps := footPath.Steps[:waterStepIdx]
	pathToWater := &Path{
		Steps:    append([]PathStep{}, landSteps...),
		StartPos: footPath.StartPos,
		GoalPos:  waterPos,
		Found:    true,
	}
	for _, s := range pathToWater.Steps {
		pathToWater.TotalCost += s.Cost
	}

	caps := models.GetVehicleCapabilities(models.VehicleTypeBoat)
	vehiclePath := vap.buildVehiclePath(ctx, waterPos, farShorePos, 0, &caps, maxSteps)
	if vehiclePath == nil || !vehiclePath.Found {
		return nil
	}

	footToGoal, err := vap.base.FindPath(ctx, vehiclePath.GoalPos, goal, maxSteps)
	if err != nil || !footToGoal.Found {
		return nil
	}

	return vap.splicePathsWithPlacement(pathToWater, vehiclePath, footToGoal, waterPos)
}

// splicePathsWithPlacement is splicePaths' counterpart for a placed (rather
// than pre-existing) vehicle: a single PlaceVehicle step stands in for
// MountVehicle, since the real entity ID doesn't exist until execution
// reaches that step — agent.PlaceAndMountBoat resolves it at runtime and
// mounts directly, so nothing downstream needs to know it in advance (the
// following VehicleSwim/DismountVehicle steps' VehicleEntityID is unused —
// see movement/physics_executor.go's handleDismountStep, which checks
// pe.mountedEntityID, never step.VehicleEntityID).
func (vap *VehicleAwarePathFinder) splicePathsWithPlacement(
	pathToWater, vehiclePath, pathFromVehicle *Path,
	waterPos models.V3,
) *Path {
	steps := make([]PathStep, 0, len(pathToWater.Steps)+len(vehiclePath.Steps)+len(pathFromVehicle.Steps)+2)

	steps = append(steps, pathToWater.Steps...)

	steps = append(steps, PathStep{
		Position: waterPos,
		Movement: models.PlaceVehicle,
		Cost:     models.PlaceVehicle.BaseCost(),
	})

	steps = append(steps, vehiclePath.Steps...)

	dismountPos := vehiclePath.GoalPos
	steps = append(steps, PathStep{
		Position: dismountPos,
		Movement: models.DismountVehicle,
		Cost:     models.DismountVehicle.BaseCost(),
	})

	steps = append(steps, pathFromVehicle.Steps...)

	totalCost := pathToWater.TotalCost + models.PlaceVehicle.BaseCost() +
		vehiclePath.TotalCost + models.DismountVehicle.BaseCost() +
		pathFromVehicle.TotalCost

	return &Path{
		Steps:      steps,
		TotalCost:  totalCost,
		StartPos:   pathToWater.StartPos,
		GoalPos:    pathFromVehicle.GoalPos,
		Found:      true,
		SearchTime: pathToWater.SearchTime + vehiclePath.SearchTime + pathFromVehicle.SearchTime,
	}
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
		Start:     start,
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
