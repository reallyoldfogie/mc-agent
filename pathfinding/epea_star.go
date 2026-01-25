package pathfinding

import (
	"container/heap"
	"fmt"
	"log"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Enhanced Partial Expansion A* (EPEA*): https://www.aaai.org/Papers/ICAPS/2007/ICAPS07-013.pdf
// EPEA* reduces node expansions by only generating successors that could improve the current f-value.
// Instead of generating all successors at once, it generates them incrementally based on the
// current best f-value in the OPEN list.

// epeaStarPathFinder implements PathFinder using EPEA*
type epeaStarPathFinder struct {
	world             World
	shapeMgr          BlockShapeManager
	movementValidator *MovementValidator
	goalRadius        float64
}

// NewEPEAStarPathFinder creates a new EPEA* pathfinder
func NewEPEAStarPathFinder(w World, shapeMgr BlockShapeManager) PathFinder {
	return NewEPEAStarPathFinderWithConfig(w, shapeMgr, PathfinderConfig{})
}

// NewEPEAStarPathFinderWithConfig creates a new EPEA* pathfinder with custom settings.
func NewEPEAStarPathFinderWithConfig(w World, shapeMgr BlockShapeManager, cfg PathfinderConfig) PathFinder {
	goalRadius := normalizeGoalRadius(cfg.GoalRadius)
	return &epeaStarPathFinder{
		world:             w,
		shapeMgr:          shapeMgr,
		movementValidator: NewMovementValidator(w, shapeMgr),
		goalRadius:        goalRadius,
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
	// EPEA* specific: tracks which successors have been generated for this f-value threshold
	generatedSuccessors map[int]bool
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
func (pf *epeaStarPathFinder) FindPath(start, goal models.V3, maxSteps int) (*Path, error) {
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
	log.Printf("[EPEA*] Start position (%f,%f,%f) has %d possible moves (filtered toward goal)",
		start.X, start.Y, start.Z, len(startMoves))

	if len(startMoves) > 0 {
		log.Printf("[EPEA*] First possible moves from (%f,%f,%f):\n", start.X, start.Y, start.Z)
		for i := range startMoves {
			log.Printf("  - Move %d: %s to (%f,%f,%f) cost=%.2f",
				i+1, startMoves[i].Movement, startMoves[i].Position.X,
				startMoves[i].Position.Y, startMoves[i].Position.Z, startMoves[i].Cost)
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
		pos:                 start,
		parent:              nil,
		movement:            Traverse,
		gCost:               0,
		hCost:               heuristic(start, goal),
		generatedSuccessors: make(map[int]bool),
		lastFThreshold:      0,
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

		// Check step limit
		if maxSteps > 0 && stepsProcessed > maxSteps {
			log.Printf("[EPEA*] Stats: steps=%d, successors generated=%d, skipped=%d (%.1f%% reduction)",
				stepsProcessed, successorsGenerated, successorsSkipped,
				100.0*float64(successorsSkipped)/float64(successorsGenerated+successorsSkipped))
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
			log.Printf("[EPEA*] %s", path.LogSummary())
			log.Printf("[EPEA*] Path details:\n%s", path.LogDetails(true))
			log.Printf("[EPEA*] Stats: steps=%d, successors generated=%d, skipped=%d (%.1f%% reduction)",
				stepsProcessed, successorsGenerated, successorsSkipped,
				100.0*float64(successorsSkipped)/float64(successorsGenerated+successorsSkipped))

			return path, nil
		}

		// Add to closed set
		closedSet[current.pos] = true

		// EPEA* partial expansion: only generate successors that could improve f-value
		// Get all possible moves from current position
		allNeighbors := pf.movementValidator.GetPossibleMoves(current.pos, goal, prune)

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

			// EPEA* optimization: skip generating this successor if its f-cost
			// would be worse than the next best node in OPEN
			// This is safe because we'll reconsider this node later if needed
			if openSet.Len() > 0 && tentativeFCost > (*openSet)[0].fCost {
				// Check if we've already generated this successor at this f-threshold
				if !current.generatedSuccessors[neighborIdx] {
					successorsSkipped++
					continue
				}
			}

			// Mark this successor as generated for this node
			current.generatedSuccessors[neighborIdx] = true
			successorsGenerated++

			// Check if this is a better path
			existingGCost, exists := gScores[neighborPos]
			if !exists || tentativeGCost < existingGCost {
				// This is a better path, update or add neighbor
				gScores[neighborPos] = tentativeGCost

				neighborNode := &epeaNode{
					pos:                 neighborPos,
					parent:              current,
					movement:            neighborStep.Movement,
					gCost:               tentativeGCost,
					hCost:               tentativeHCost,
					fCost:               tentativeFCost,
					generatedSuccessors: make(map[int]bool),
					lastFThreshold:      currentFThreshold,
				}

				heap.Push(openSet, neighborNode)
				nodeMap[neighborPos] = neighborNode
			}
		}

		// EPEA* re-expansion: if this node still has unexpanded successors and
		// could be useful later, add it back to OPEN with updated f-threshold
		if len(current.generatedSuccessors) < len(allNeighbors) && openSet.Len() > 0 {
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
	log.Printf("[EPEA*] Pathfinding failed: openSet exhausted after %d steps, closedSet size=%d",
		stepsProcessed, len(closedSet))
	log.Printf("[EPEA*] Stats: successors generated=%d, skipped=%d (%.1f%% reduction)",
		successorsGenerated, successorsSkipped,
		100.0*float64(successorsSkipped)/float64(successorsGenerated+successorsSkipped))
	return &Path{
		Found:      false,
		StartPos:   start,
		GoalPos:    goal,
		SearchTime: float64(time.Since(startTime).Milliseconds()),
	}, fmt.Errorf("path not found: no valid path exists")
}

// FindGroundBelow delegates to the movement validator to find valid ground
func (pf *epeaStarPathFinder) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return pf.movementValidator.FindGroundBelow(x, z, startY, maxSearchDepth)
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
