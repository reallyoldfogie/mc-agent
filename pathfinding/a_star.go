package pathfinding

import (
	"container/heap"
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// aStarPathFinder implements PathFinder
type aStarPathFinder struct {
	world             models.World
	shapeMgr          models.BlockShapeManager
	movementValidator *MovementValidator
	goalRadius        float64
	contextCheckFreq  int
	logger            *slog.Logger
}

// NewAStarPathFinder creates a new A* pathfinder
func NewAStarPathFinder(w models.World, shapeMgr models.BlockShapeManager, logger *slog.Logger) models.PathFinder {
	return NewAStarPathFinderWithConfig(w, shapeMgr, PathfinderConfig{}, logger)
}

// NewAStarPathFinderWithConfig creates a new A* pathfinder with custom settings.
func NewAStarPathFinderWithConfig(w models.World, shapeMgr models.BlockShapeManager, cfg PathfinderConfig, logger *slog.Logger) models.PathFinder {
	goalRadius := normalizeGoalRadius(cfg.GoalRadius)
	contextCheckFreq := normalizeContextCheckFreq(cfg.ContextCheckFreq)
	logger = utils.SafeLogger(logger)
	return &aStarPathFinder{
		world:             w,
		shapeMgr:          shapeMgr,
		movementValidator: NewMovementValidator(w, shapeMgr, logger),
		goalRadius:        goalRadius,
		contextCheckFreq:  contextCheckFreq,
		logger:            logger,
	}
}

// node represents a node in the A* search
type node struct {
	pos      models.V3
	parent   *node
	movement MovementType
	gCost    float64 // Cost from start to this node
	hCost    float64 // Heuristic cost from this node to goal
	fCost    float64 // gCost + hCost
	index    int     // Index in the priority queue
}

// nodeHeap implements heap.Interface for A* priority queue
type nodeHeap []*node

func (h nodeHeap) Len() int           { return len(h) }
func (h nodeHeap) Less(i, j int) bool { return h[i].fCost < h[j].fCost }
func (h nodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *nodeHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*node)
	item.index = n
	*h = append(*h, item)
}

func (h *nodeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// heuristic calculates the heuristic cost from pos to goal
func heuristic(pos, goal models.V3) float64 {
	return chebyshevHeuristic(pos, goal)
}

// Uses 3D Euclidean distance with Y-axis weight adjustment
// This is an admissible heuristic (never overestimates) which ensures A* optimality
func euclideanHeuristic(pos, goal models.V3) float64 {
	dx := goal.X - pos.X
	dy := (goal.Y - pos.Y) * 1.5 // Weight Y-axis since vertical movement is more expensive
	dz := goal.Z - pos.Z

	// Return actual Euclidean distance (not squared) for admissible heuristic
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// Uses Chebyshev distance for environments allowing diagonal movement
func chebyshevHeuristic(pos, goal models.V3) float64 {
	dx := math.Abs(goal.X - pos.X)
	dy := math.Abs(goal.Y - pos.Y)
	dz := math.Abs(goal.Z - pos.Z)

	// Chebyshev distance considers the maximum axis distance
	// This is admissible since diagonal moves are not cheaper than straight moves
	return math.Max(math.Max(dx, dz), dy)
}

// Uses Manhattan distance for better guidance on grid-based movement
func manhattanHeuristic(pos, goal models.V3) float64 {
	// Calculate horizontal Manhattan distance
	dx := math.Abs(goal.X - pos.X)
	dz := math.Abs(goal.Z - pos.Z)
	horizontalDist := dx + dz

	// Calculate vertical distance
	dy := goal.Y - pos.Y

	// Base cost: Manhattan distance (traverse cost = 1.0)
	// This assumes we can traverse horizontally at cost 1.0 per block
	cost := horizontalDist

	// Add vertical cost
	if dy > 0 {
		// Going up: use AscendStairs cost (1.0) as lower bound
		// This is admissible because AscendStairs is the cheapest upward movement
		cost += dy * 1.0
	} else if dy < 0 {
		// Going down: use Descend cost (1.2) as lower bound
		cost += math.Abs(dy) * 1.2
	}

	// Diagonal optimization: if we can move diagonally, reduce the estimate slightly
	// Diagonal moves cover sqrt(2) distance but only cost 1.4, saving 0.014 per diagonal
	// This makes the heuristic more accurate without making it inadmissible
	diagonalBlocks := math.Min(dx, dz)
	diagonalSavings := diagonalBlocks * 0.4 // 2.0 straight moves vs 1.4 diagonal
	cost -= diagonalSavings

	return cost
}

// (Pathfinding from)|(A\*)|(Pathfinding straight line distance)|(\(\d+,?\s?74+,?\s?\d+\))

// FindPath finds a path from start to goal using A* algorithm
func (pf *aStarPathFinder) FindPath(ctx context.Context, start, goal models.V3, maxSteps int) (*Path, error) {
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
	// docs/bugs/hpa-star-slowness: a real caller did exactly this (passed a
	// crafting table's own coordinates as the MoveTo target) and every call
	// burned 100s-900s before finally reporting failure.
	if pf.goalUnreachable(goal) {
		return &Path{
			Found:      false,
			StartPos:   start,
			GoalPos:    goal,
			SearchTime: float64(time.Since(startTime).Milliseconds()),
		}, fmt.Errorf("path not found: goal (%.1f,%.1f,%.1f) has no walkable cell within goal radius %.2f (likely inside a solid block or missing ground support)",
			goal.X, goal.Y, goal.Z, pf.goalRadius)
	}

	// Debug: Get possible moves from start to verify we can move
	prune := &MovePruneConfig{
		StartDist: start.DistanceTo(goal),
		DriftCap:  4.0,
	}
	startMoves := pf.movementValidator.GetPossibleMoves(start, goal, prune)
	utils.SafeLogger(pf.logger).Debug("[A*] start position possible moves", "x", start.X, "y", start.Y, "z", start.Z, "count", len(startMoves))

	if len(startMoves) > 0 && utils.DebugVerboseEnabled(pf.logger) {
		for i := range startMoves {
			utils.DebugVerbose(pf.logger, "[A*] possible move from start",
				"index", i+1, "movement", startMoves[i].Movement,
				"x", startMoves[i].Position.X, "y", startMoves[i].Position.Y, "z", startMoves[i].Position.Z,
				"cost", startMoves[i].Cost)
		}
	}

	// Initialize open set (priority queue) and closed set
	openSet := &nodeHeap{}
	heap.Init(openSet)

	closedSet := make(map[models.V3]bool)
	gScores := make(map[models.V3]float64)

	// Add start node
	startNode := &node{
		pos:      start,
		parent:   nil,
		movement: Traverse,
		gCost:    0,
		hCost:    heuristic(start, goal),
	}
	startNode.fCost = startNode.gCost + startNode.hCost
	heap.Push(openSet, startNode)
	gScores[start] = 0

	stepsProcessed := 0

	// A* main loop
	for openSet.Len() > 0 {
		stepsProcessed++

		// Check context cancellation periodically to reduce overhead
		if stepsProcessed%pf.contextCheckFreq == 0 {
			select {
			case <-ctx.Done():
				utils.SafeLogger(pf.logger).Warn("[A*] context deadline exceeded", "steps", stepsProcessed, "elapsed", time.Since(startTime), "open", openSet.Len(), "closed", len(closedSet))
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
			utils.SafeLogger(pf.logger).Warn("[A*] exceeded max steps", "steps", stepsProcessed, "elapsed", time.Since(startTime), "open", openSet.Len(), "closed", len(closedSet))
			return &Path{
				Found:      false,
				StartPos:   start,
				GoalPos:    goal,
				SearchTime: float64(time.Since(startTime).Milliseconds()),
			}, fmt.Errorf("path not found: exceeded max steps (%d)", maxSteps)
		}

		// Get node with lowest f-cost
		current := heap.Pop(openSet).(*node)

		// Skip stale heap entries: this position was already expanded via a
		// better (or equal) path, or a cheaper entry for it is still pending.
		// The heap has no decrease-key, so re-discovering a position pushes a
		// new *node instead of updating the existing one - without this check
		// every superseded duplicate gets fully re-expanded (recomputing
		// GetPossibleMoves) when it eventually reaches the front of the heap.
		if closedSet[current.pos] {
			continue
		}
		if bestKnown, ok := gScores[current.pos]; ok && current.gCost > bestKnown {
			continue
		}

		// Progress logging: a_star.go's main loop previously had zero
		// visibility between the "possible moves from start" dump and
		// success/failure, making a genuinely slow/failing search
		// indistinguishable from a hung one. See docs/bugs/hpa-star-slowness.
		if stepsProcessed%500 == 0 {
			utils.SafeLogger(pf.logger).Debug("[A*] progress", "steps", stepsProcessed, "elapsed", time.Since(startTime),
				"open", openSet.Len(), "closed", len(closedSet),
				"x", current.pos.X, "y", current.pos.Y, "z", current.pos.Z,
				"distToGoal", current.pos.DistanceTo(goal), "fCost", current.fCost)
		}

		// Check if we reached the goal
		if current.pos.DistanceTo(goal) <= pf.goalRadius {
			// Reconstruct path
			path := pf.reconstructPath(current, start, goal)
			path.SearchTime = float64(time.Since(startTime).Milliseconds())

			// Log path summary and details
			utils.SafeLogger(pf.logger).Info("[A*] " + path.LogSummary())
			if utils.DebugVerboseEnabled(pf.logger) {
				utils.DebugVerbose(pf.logger, "[A*] path details\n"+path.LogDetails(true))
			}

			return path, nil
		}

		// Add to closed set
		closedSet[current.pos] = true

		// Get all possible moves from current position (filtered toward goal)
		neighbors := pf.movementValidator.GetPossibleMoves(current.pos, goal, prune)

		for _, neighborStep := range neighbors {
			neighborPos := neighborStep.Position

			// Skip if in closed set
			if closedSet[neighborPos] {
				continue
			}

			// Calculate tentative g-cost
			tentativeGCost := current.gCost + neighborStep.Cost

			// Check if this is a better path
			existingGCost, exists := gScores[neighborPos]
			if !exists || tentativeGCost < existingGCost {
				// This is a better path, update or add neighbor
				gScores[neighborPos] = tentativeGCost

				neighborNode := &node{
					pos:      neighborPos,
					parent:   current,
					movement: neighborStep.Movement,
					gCost:    tentativeGCost,
					hCost:    heuristic(neighborPos, goal),
				}
				neighborNode.fCost = neighborNode.gCost + neighborNode.hCost

				heap.Push(openSet, neighborNode)
			}
		}
	}

	// No path found - openSet is empty
	utils.SafeLogger(pf.logger).Warn("[A*] pathfinding failed: openSet exhausted", "steps", stepsProcessed, "closedSetSize", len(closedSet))
	return &Path{
		Found:      false,
		StartPos:   start,
		GoalPos:    goal,
		SearchTime: float64(time.Since(startTime).Milliseconds()),
	}, fmt.Errorf("path not found: no valid path exists")
}

// FindGroundBelow delegates to the movement validator to find valid ground
func (pf *aStarPathFinder) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return pf.movementValidator.FindGroundBelow(x, z, startY, maxSearchDepth)
}

// goalUnreachable reports whether goal is *definitely* unreachable: no
// walkable cell (passable feet+head, solid ground support) exists anywhere
// within goalRadius of it. Cells in an unloaded chunk are treated as
// "unknown" rather than unwalkable, so this never produces a false
// negative that blocks a legitimately pending world - it only fires when
// every candidate cell's data is available and none of them qualify.
func (pf *aStarPathFinder) goalUnreachable(goal models.V3) bool {
	r := int(math.Ceil(pf.goalRadius))
	for dx := -r; dx <= r; dx++ {
		for dy := -r; dy <= r; dy++ {
			for dz := -r; dz <= r; dz++ {
				candidate := models.V3{X: goal.X + float64(dx), Y: goal.Y + float64(dy), Z: goal.Z + float64(dz)}
				if candidate.DistanceTo(goal) > pf.goalRadius {
					continue
				}
				feetID, feetLoaded := pf.world.GetBlockAt(candidate.X, candidate.Y, candidate.Z)
				headID, headLoaded := pf.world.GetBlockAt(candidate.X, candidate.Y+1, candidate.Z)
				groundID, groundLoaded := pf.world.GetBlockAt(candidate.X, candidate.Y-1, candidate.Z)
				if !feetLoaded || !headLoaded || !groundLoaded {
					return false // unknown - don't block, let the real search decide
				}
				if pf.shapeMgr.IsPassable(feetID) && pf.shapeMgr.IsPassable(headID) && !pf.shapeMgr.IsPassable(groundID) {
					return false // found a walkable cell within goalRadius
				}
			}
		}
	}
	return true
}

// reconstructPath builds the path from the goal node back to the start
func (pf *aStarPathFinder) reconstructPath(goalNode *node, start, goal models.V3) *Path {
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
