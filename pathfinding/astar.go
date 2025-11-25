package pathfinding

import (
	"container/heap"
	"fmt"
	"time"

	"github.com/reallyoldfogie/mc-bot-go/bot/world"
)

// PathFinder finds paths using A* algorithm
type PathFinder interface {
	FindPath(start, goal V3, maxSteps int) (*Path, error)
}

// pathFinder implements PathFinder
type pathFinder struct {
	world          *world.World
	shapeMgr       BlockShapeManager
	movementValidator *MovementValidator
}

// NewPathFinder creates a new A* pathfinder
func NewPathFinder(w *world.World, shapeMgr BlockShapeManager) PathFinder {
	return &pathFinder{
		world:             w,
		shapeMgr:          shapeMgr,
		movementValidator: NewMovementValidator(w, shapeMgr),
	}
}

// node represents a node in the A* search
type node struct {
	pos      V3
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

func (h *nodeHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// heuristic calculates the heuristic cost from pos to goal
// Uses 3D Euclidean distance with Y-axis weight adjustment
func heuristic(pos, goal V3) float64 {
	dx := float64(goal.X - pos.X)
	dy := float64(goal.Y - pos.Y)
	dz := float64(goal.Z - pos.Z)

	// Weight Y-axis more since vertical movement is more expensive
	dy = dy * 1.5

	return (dx*dx + dy*dy + dz*dz)
}

// FindPath finds a path from start to goal using A* algorithm
func (pf *pathFinder) FindPath(start, goal V3, maxSteps int) (*Path, error) {
	startTime := time.Now()

	// Validate start and goal positions
	if start == goal {
		return &Path{
			Steps:      []PathStep{},
			TotalCost:  0,
			StartPos:   start,
			GoalPos:    goal,
			Found:      true,
			SearchTime: 0,
		}, nil
	}

	// Initialize open set (priority queue) and closed set
	openSet := &nodeHeap{}
	heap.Init(openSet)

	closedSet := make(map[V3]bool)
	gScores := make(map[V3]float64)

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
		if current.pos == goal {
			// Reconstruct path
			path := pf.reconstructPath(current, start, goal)
			path.SearchTime = float64(time.Since(startTime).Milliseconds())
			return path, nil
		}

		// Add to closed set
		closedSet[current.pos] = true

		// Get all possible moves from current position
		neighbors := pf.movementValidator.GetPossibleMoves(current.pos)

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

	// No path found
	return &Path{
		Found:      false,
		StartPos:   start,
		GoalPos:    goal,
		SearchTime: float64(time.Since(startTime).Milliseconds()),
	}, fmt.Errorf("path not found: no valid path exists")
}

// reconstructPath builds the path from the goal node back to the start
func (pf *pathFinder) reconstructPath(goalNode *node, start, goal V3) *Path {
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
