package pathfinding

import (
	"container/heap"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// aStarPathFinder implements PathFinder
type aStarPathFinder struct {
	world             World
	shapeMgr          BlockShapeManager
	movementValidator *MovementValidator
	goalRadius        float64
}

// NewAStarPathFinder creates a new A* pathfinder
func NewAStarPathFinder(w World, shapeMgr BlockShapeManager) PathFinder {
	return NewAStarPathFinderWithConfig(w, shapeMgr, PathfinderConfig{})
}

// NewAStarPathFinderWithConfig creates a new A* pathfinder with custom settings.
func NewAStarPathFinderWithConfig(w World, shapeMgr BlockShapeManager, cfg PathfinderConfig) PathFinder {
	goalRadius := normalizeGoalRadius(cfg.GoalRadius)
	return &aStarPathFinder{
		world:             w,
		shapeMgr:          shapeMgr,
		movementValidator: NewMovementValidator(w, shapeMgr),
		goalRadius:        goalRadius,
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
func (pf *aStarPathFinder) FindPath(start, goal models.V3, maxSteps int) (*Path, error) {
	startTime := time.Now()

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

	// Debug: Get possible moves from start to verify we can move
	prune := &MovePruneConfig{
		StartDist: start.DistanceTo(goal),
		DriftCap:  4.0,
	}
	startMoves := pf.movementValidator.GetPossibleMoves(start, goal, prune)
	log.Printf("[A*] Start position (%f,%f,%f) has %d possible moves (filtered toward goal)",
		start.X, start.Y, start.Z, len(startMoves))

	if len(startMoves) > 0 {
		log.Printf("[A*] First possible moves from (%f,%f,%f):\n", start.X, start.Y, start.Z)
		for i := range startMoves {
			log.Printf("  - Move %d: %s to (%f,%f,%f) cost=%.2f",
				i+1, startMoves[i].Movement, startMoves[i].Position.X,
				startMoves[i].Position.Y, startMoves[i].Position.Z, startMoves[i].Cost)
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

		// Check step limit
		if maxSteps > 0 && stepsProcessed > maxSteps {
			return &Path{
				Found:      false,
				StartPos:   start,
				GoalPos:    goal,
				SearchTime: float64(time.Since(startTime).Milliseconds()),
			}, fmt.Errorf("path not found: exceeded max steps (%d)", maxSteps)
		}

		// Get node with lowest f-cost
		current := heap.Pop(openSet).(*node)

		// Check if we reached the goal
		if current.pos.DistanceTo(goal) <= pf.goalRadius {
			// Reconstruct path
			path := pf.reconstructPath(current, start, goal)
			path.SearchTime = float64(time.Since(startTime).Milliseconds())

			// Log path summary and details
			log.Printf("[A*] %s", path.LogSummary())
			log.Printf("[A*] Path details:\n%s", path.LogDetails(true))

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
	log.Printf("[A*] Pathfinding failed: openSet exhausted after %d steps, closedSet size=%d",
		stepsProcessed, len(closedSet))
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
