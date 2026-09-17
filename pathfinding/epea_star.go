package pathfinding

import (
	"container/heap"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// Enhanced Partial Expansion A* (EPEA*): https://www.aaai.org/Papers/ICAPS/2007/ICAPS07-013.pdf
// EPEA* reduces node expansions by only generating successors that could improve the current f-value.
// Instead of generating all successors at once, it generates them incrementally based on the
// current best f-value in the OPEN list.

// epeaStarPathFinder implements PathFinder using EPEA*
type epeaStarPathFinder struct {
	world            models.World
	shapeMgr         models.BlockShapeManager
	goalRadius       float64
	contextCheckFreq int
	logger           *slog.Logger
}

// NewEPEAStarPathFinder creates a new EPEA* pathfinder
func NewEPEAStarPathFinder(w models.World, shapeMgr models.BlockShapeManager, logger *slog.Logger) models.PathFinder {
	return NewEPEAStarPathFinderWithConfig(w, shapeMgr, PathfinderConfig{}, logger)
}

// NewEPEAStarPathFinderWithConfig creates a new EPEA* pathfinder with custom settings.
func NewEPEAStarPathFinderWithConfig(w models.World, shapeMgr models.BlockShapeManager, cfg PathfinderConfig, logger *slog.Logger) models.PathFinder {
	goalRadius := normalizeGoalRadius(cfg.GoalRadius)
	contextCheckFreq := normalizeContextCheckFreq(cfg.ContextCheckFreq)
	logger = utils.SafeLogger(logger)
	return &epeaStarPathFinder{
		world:            w,
		shapeMgr:         shapeMgr,
		goalRadius:       goalRadius,
		contextCheckFreq: contextCheckFreq,
		logger:           logger,
	}
}

// epeaNode represents a node in EPEA* search
type epeaNode struct {
	pos      models.V3
	parent   *epeaNode
	movement MovementType
	gCost    float64
	hCost    float64
	fCost    float64
	index    int
	// neighbors caches this node's GetPossibleMoves result, computed lazily
	// the first time the node is expanded. EPEA* re-expands a node (pushes it
	// back onto OPEN) whenever it still has ungenerated successors; without
	// this cache, every re-expansion re-ran the full ~20-check move-generation
	// pass for the same position from scratch, which made EPEA* slower than
	// plain A* despite generating fewer successors per pass - see
	// docs/PATHFINDING_BENCHMARKS.md.
	neighbors []PathStep
	// generatedSuccessors tracks which of neighbors (by index) have been
	// generated so far across this node's re-expansion passes. A []bool
	// indexed by neighborIdx instead of a map[int]bool avoids a map
	// allocation per node - neighbor counts are small (well under 64) and
	// indices are dense from 0..len(neighbors), so a slice is a strict
	// improvement with identical lookup semantics.
	generatedSuccessors []bool
	generatedCount      int // count of true entries in generatedSuccessors, i.e. len(map) in the old map-based version
	lastFThreshold      float64
}

// epeaNodeHeap implements heap.Interface for EPEA* priority queue
type epeaNodeHeap []*epeaNode

func (h epeaNodeHeap) Len() int           { return len(h) }
func (h epeaNodeHeap) Less(i, j int) bool { return h[i].fCost < h[j].fCost }
func (h epeaNodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *epeaNodeHeap) Push(x any) {
	n := len(*h)
	item := x.(*epeaNode)
	item.index = n
	*h = append(*h, item)
}

func (h *epeaNodeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// FindPath finds a path from start to goal using EPEA* algorithm
func (pf *epeaStarPathFinder) FindPath(ctx context.Context, start, goal models.V3, maxSteps int) (*Path, error) {
	startTime := time.Now()

	// Check context before starting
	if ctx.Err() != nil {
		return &Path{
			Found:      false,
			StartPos:   start,
			GoalPos:    goal,
			SearchTime: float64(time.Since(startTime).Milliseconds()),
		}, ctx.Err()
	}

	// Snap goal onto the same grid start's own moves can actually reach —
	// see snapGoalToReachableGrid's own doc comment for the live-confirmed
	// bug this closes. Every goalRadius check below, and the main search
	// loop's own termination check, now operate on an achievable goal.
	goal = snapGoalToReachableGrid(start, goal)

	// Validate start and goal positions
	if start.DistanceTo(goal) <= pf.goalRadius {
		return &Path{
			Steps:      []PathStep{},
			TotalCost:  0,
			StartPos:   start,
			GoalPos:    goal,
			Found:      true,
			SearchTime: 0,
		}, nil
	}

	// Cheap upfront reachability check: if nothing within goalRadius of the
	// goal is even walkable (e.g. the goal is a solid block's own
	// coordinates), the search below is guaranteed to exhaust its entire
	// step budget without success, every time, regardless of maxSteps or
	// context deadline - because the termination condition can never be
	// satisfied. Fail fast instead of proving that the slow way. See
	// docs/bugs/hpa-star-slowness.
	if goalUnreachable(pf.world, pf.shapeMgr, pf.goalRadius, goal) {
		return &Path{
			Found:      false,
			StartPos:   start,
			GoalPos:    goal,
			SearchTime: float64(time.Since(startTime).Milliseconds()),
		}, fmt.Errorf("path not found: goal (%.1f,%.1f,%.1f) has no walkable cell within goal radius %.2f (likely inside a solid block or missing ground support)",
			goal.X, goal.Y, goal.Z, pf.goalRadius)
	}

	// A fresh MovementValidator per call, not a shared field - see
	// a_star.go's FindPath for why (concurrent callers on the same
	// pathfinder instance would otherwise invalidate each other's
	// memoization via ResetBlockCache).
	movementValidator := NewMovementValidator(pf.world, pf.shapeMgr, pf.logger)
	movementValidator.ResetBlockCache()

	// Debug: Get possible moves from start to verify we can move
	prune := &MovePruneConfig{
		Start:     start,
		StartDist: start.DistanceTo(goal),
		DriftCap:  4.0,
	}
	startMoves := movementValidator.GetPossibleMoves(start, goal, prune)
	utils.SafeLogger(pf.logger).Debug("[EPEA*] start position possible moves", "x", start.X, "y", start.Y, "z", start.Z, "count", len(startMoves))

	if len(startMoves) > 0 && utils.DebugVerboseEnabled(pf.logger) {
		for i := range startMoves {
			utils.DebugVerbose(pf.logger, "[EPEA*] possible move from start",
				"index", i+1, "movement", startMoves[i].Movement,
				"x", startMoves[i].Position.X, "y", startMoves[i].Position.Y, "z", startMoves[i].Position.Z,
				"cost", startMoves[i].Cost)
		}
	}

	// Initialize open set (priority queue) and closed set
	openSet := &epeaNodeHeap{}
	heap.Init(openSet)

	closedSet := make(map[models.V3]bool)
	gScores := make(map[models.V3]float64)
	nodeMap := make(map[models.V3]*epeaNode) // Track nodes for EPEA* partial expansion

	// Add start node
	startNode := &epeaNode{
		pos:            start,
		parent:         nil,
		movement:       Traverse,
		gCost:          0,
		hCost:          heuristic(start, goal),
		lastFThreshold: 0,
	}
	startNode.fCost = startNode.gCost + startNode.hCost
	heap.Push(openSet, startNode)
	gScores[start] = 0
	nodeMap[start] = startNode

	stepsProcessed := 0
	successorsGenerated := 0
	successorsSkipped := 0

	// EPEA* main loop
	for openSet.Len() > 0 {
		stepsProcessed++

		// Check context cancellation periodically to reduce overhead
		if stepsProcessed%pf.contextCheckFreq == 0 {
			select {
			case <-ctx.Done():
				utils.SafeLogger(pf.logger).Warn("[EPEA*] context deadline exceeded", "steps", stepsProcessed,
					"successorsGenerated", successorsGenerated, "successorsSkipped", successorsSkipped,
					"reductionPct", 100.0*float64(successorsSkipped)/float64(successorsGenerated+successorsSkipped))
				return &Path{
					Found:      false,
					StartPos:   start,
					GoalPos:    goal,
					SearchTime: float64(time.Since(startTime).Milliseconds()),
				}, ctx.Err()
			default:
			}
		}

		// Check step limit
		if maxSteps > 0 && stepsProcessed > maxSteps {
			utils.SafeLogger(pf.logger).Warn("[EPEA*] exceeded max steps", "steps", stepsProcessed,
				"successorsGenerated", successorsGenerated, "successorsSkipped", successorsSkipped,
				"reductionPct", 100.0*float64(successorsSkipped)/float64(successorsGenerated+successorsSkipped))
			return &Path{
				Found:      false,
				StartPos:   start,
				GoalPos:    goal,
				SearchTime: float64(time.Since(startTime).Milliseconds()),
			}, fmt.Errorf("path not found: exceeded max steps (%d)", maxSteps)
		}

		// Get node with lowest f-cost (this is the current f-threshold)
		current := heap.Pop(openSet).(*epeaNode)
		currentFThreshold := current.fCost

		// Check if we reached the goal
		if current.pos.DistanceTo(goal) <= pf.goalRadius {
			// Reconstruct path
			path := pf.reconstructPath(current, start, goal)
			path.SearchTime = float64(time.Since(startTime).Milliseconds())

			// Log path summary and stats
			utils.SafeLogger(pf.logger).Info("[EPEA*] " + path.LogSummary())
			if utils.DebugVerboseEnabled(pf.logger) {
				utils.DebugVerbose(pf.logger, "[EPEA*] path details\n"+path.LogDetails(true))
			}
			utils.SafeLogger(pf.logger).Debug("[EPEA*] stats", "steps", stepsProcessed,
				"successorsGenerated", successorsGenerated, "successorsSkipped", successorsSkipped,
				"reductionPct", 100.0*float64(successorsSkipped)/float64(successorsGenerated+successorsSkipped))

			return path, nil
		}

		// Add to closed set
		closedSet[current.pos] = true

		// EPEA* partial expansion: only generate successors that could improve f-value.
		// Get all possible moves from current position - computed once and cached on
		// the node (see epeaNode.neighbors), since re-expansion below can pop this
		// same position again and the world hasn't changed.
		if current.neighbors == nil {
			current.neighbors = movementValidator.GetPossibleMoves(current.pos, goal, prune)
			current.generatedSuccessors = make([]bool, len(current.neighbors))
		}
		allNeighbors := current.neighbors

		// For each neighbor, check if it should be generated based on f-threshold
		for neighborIdx, neighborStep := range allNeighbors {
			neighborPos := neighborStep.Position

			// Skip if in closed set
			if closedSet[neighborPos] {
				successorsSkipped++
				continue
			}

			// Calculate tentative g-cost and f-cost for this successor
			tentativeGCost := current.gCost + neighborStep.Cost
			tentativeHCost := heuristic(neighborPos, goal)
			tentativeFCost := tentativeGCost + tentativeHCost

			// EPEA* optimization: defer generating successors with significantly worse f-cost
			// Use a threshold margin to avoid being too aggressive with pruning
			// This allows paths through terrain with elevation changes (stairs, etc.)
			// where the heuristic may underestimate actual costs
			const fCostMargin = 2.0 // Allow successors within 2.0 of best f-cost
			if openSet.Len() > 0 && tentativeFCost > (*openSet)[0].fCost+fCostMargin {
				// Check if we've already generated this successor at this f-threshold
				if !current.generatedSuccessors[neighborIdx] {
					successorsSkipped++
					continue
				}
			}

			// Mark this successor as generated for this node
			if !current.generatedSuccessors[neighborIdx] {
				current.generatedSuccessors[neighborIdx] = true
				current.generatedCount++
			}
			successorsGenerated++

			// Check if this is a better path
			existingGCost, exists := gScores[neighborPos]
			if !exists || tentativeGCost < existingGCost {
				// This is a better path, update or add neighbor
				gScores[neighborPos] = tentativeGCost

				neighborNode := &epeaNode{
					pos:            neighborPos,
					parent:         current,
					movement:       neighborStep.Movement,
					gCost:          tentativeGCost,
					hCost:          tentativeHCost,
					fCost:          tentativeFCost,
					lastFThreshold: currentFThreshold,
				}

				heap.Push(openSet, neighborNode)
				nodeMap[neighborPos] = neighborNode
			}
		}

		// EPEA* re-expansion: if this node still has unexpanded successors and
		// could be useful later, add it back to OPEN with updated f-threshold
		if current.generatedCount < len(allNeighbors) && openSet.Len() > 0 {
			nextFThreshold := (*openSet)[0].fCost
			if nextFThreshold > current.lastFThreshold {
				// Update the node's f-threshold and add it back to OPEN
				current.lastFThreshold = nextFThreshold
				// Remove from closed set so it can be re-expanded
				delete(closedSet, current.pos)
				heap.Push(openSet, current)
			}
		}
	}

	// No path found - openSet is empty
	utils.SafeLogger(pf.logger).Warn("[EPEA*] pathfinding failed: openSet exhausted", "steps", stepsProcessed, "closedSetSize", len(closedSet),
		"successorsGenerated", successorsGenerated, "successorsSkipped", successorsSkipped,
		"reductionPct", 100.0*float64(successorsSkipped)/float64(successorsGenerated+successorsSkipped))
	return &Path{
		Found:      false,
		StartPos:   start,
		GoalPos:    goal,
		SearchTime: float64(time.Since(startTime).Milliseconds()),
	}, fmt.Errorf("path not found: no valid path exists")
}

// FindGroundBelow delegates to the movement validator to find valid ground
func (pf *epeaStarPathFinder) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return NewMovementValidator(pf.world, pf.shapeMgr, pf.logger).FindGroundBelow(x, z, startY, maxSearchDepth)
}

// reconstructPath builds the path from the goal node back to the start
func (pf *epeaStarPathFinder) reconstructPath(goalNode *epeaNode, start, goal models.V3) *Path {
	// Walk back from goal to start
	steps := make([]PathStep, 0)
	totalCost := 0.0

	current := goalNode
	for current.parent != nil {
		step := PathStep{
			Position: current.pos,
			Movement: current.movement,
			Cost:     current.gCost - current.parent.gCost,
		}
		steps = append(steps, step)
		totalCost += step.Cost
		current = current.parent
	}

	// Reverse to get start -> goal order
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}

	return &Path{
		Steps:     steps,
		TotalCost: totalCost,
		StartPos:  start,
		GoalPos:   goal,
		Found:     true,
	}
}
