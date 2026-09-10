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

// bidirAStarPathFinder implements PathFinder using bidirectional A*
// It searches from both start and goal simultaneously, meeting in the middle.
// This typically explores 40-80% fewer nodes than standard A* for long paths.
type bidirAStarPathFinder struct {
	world             models.World
	shapeMgr          models.BlockShapeManager
	movementValidator *MovementValidator
	goalRadius        float64
	contextCheckFreq  int
	logger            *slog.Logger
}

// NewBidirAStarPathFinder creates a new bidirectional A* pathfinder
func NewBidirAStarPathFinder(w models.World, shapeMgr models.BlockShapeManager, logger *slog.Logger) models.PathFinder {
	return NewBidirAStarPathFinderWithConfig(w, shapeMgr, PathfinderConfig{}, logger)
}

// NewBidirAStarPathFinderWithConfig creates a new bidirectional A* pathfinder with custom settings.
func NewBidirAStarPathFinderWithConfig(w models.World, shapeMgr models.BlockShapeManager, cfg PathfinderConfig, logger *slog.Logger) models.PathFinder {
	goalRadius := normalizeGoalRadius(cfg.GoalRadius)
	contextCheckFreq := normalizeContextCheckFreq(cfg.ContextCheckFreq)
	logger = utils.SafeLogger(logger)
	return &bidirAStarPathFinder{
		world:             w,
		shapeMgr:          shapeMgr,
		movementValidator: NewMovementValidator(w, shapeMgr, logger),
		goalRadius:        goalRadius,
		contextCheckFreq:  contextCheckFreq,
		logger:            logger,
	}
}

// bidirNode represents a node in bidirectional A* search
type bidirNode struct {
	pos      models.V3
	parent   *bidirNode
	movement MovementType
	gCost    float64 // Cost from search origin to this node
	hCost    float64 // Heuristic cost to target
	fCost    float64 // gCost + hCost
	index    int     // Index in the priority queue
}

// bidirNodeHeap implements heap.Interface for bidirectional A* priority queue
type bidirNodeHeap []*bidirNode

func (h bidirNodeHeap) Len() int           { return len(h) }
func (h bidirNodeHeap) Less(i, j int) bool { return h[i].fCost < h[j].fCost }
func (h bidirNodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *bidirNodeHeap) Push(x any) {
	n := len(*h)
	item := x.(*bidirNode)
	item.index = n
	*h = append(*h, item)
}

func (h *bidirNodeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

// FindPath finds a path from start to goal using bidirectional A* algorithm
func (pf *bidirAStarPathFinder) FindPath(ctx context.Context, start, goal models.V3, maxSteps int) (*Path, error) {
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

	utils.SafeLogger(pf.logger).Debug("[Bidir-A*] start", "startX", start.X, "startY", start.Y, "startZ", start.Z, "goalX", goal.X, "goalY", goal.Y, "goalZ", goal.Z)

	// Initialize forward search (from start)
	forwardOpen := &bidirNodeHeap{}
	heap.Init(forwardOpen)
	forwardClosed := make(map[models.V3]*bidirNode)
	forwardGScores := make(map[models.V3]float64)

	startNode := &bidirNode{
		pos:      start,
		parent:   nil,
		movement: Traverse,
		gCost:    0,
		hCost:    heuristic(start, goal),
	}
	startNode.fCost = startNode.gCost + startNode.hCost
	heap.Push(forwardOpen, startNode)
	forwardGScores[start] = 0

	// Initialize backward search (from goal)
	backwardOpen := &bidirNodeHeap{}
	heap.Init(backwardOpen)
	backwardClosed := make(map[models.V3]*bidirNode)
	backwardGScores := make(map[models.V3]float64)

	goalNode := &bidirNode{
		pos:      goal,
		parent:   nil,
		movement: Traverse,
		gCost:    0,
		hCost:    heuristic(goal, start),
	}
	goalNode.fCost = goalNode.gCost + goalNode.hCost
	heap.Push(backwardOpen, goalNode)
	backwardGScores[goal] = 0

	stepsProcessed := 0
	var bestMeetingNode *bidirNode
	var bestMeetingNodeBackward *bidirNode
	bestPathCost := float64(1e18) // Very large number

	// Prune config for move generation
	forwardPrune := &MovePruneConfig{
		StartDist: start.DistanceTo(goal),
		DriftCap:  8.0, // More generous for bidirectional
	}
	backwardPrune := &MovePruneConfig{
		StartDist: start.DistanceTo(goal),
		DriftCap:  8.0,
	}

	// Bidirectional A* main loop - alternate between forward and backward
	for forwardOpen.Len() > 0 || backwardOpen.Len() > 0 {
		stepsProcessed++

		// Check context cancellation periodically
		if stepsProcessed%pf.contextCheckFreq == 0 {
			select {
			case <-ctx.Done():
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
			return &Path{
				Found:      false,
				StartPos:   start,
				GoalPos:    goal,
				SearchTime: float64(time.Since(startTime).Milliseconds()),
			}, fmt.Errorf("path not found: exceeded max steps (%d)", maxSteps)
		}

		// Termination check: if the minimum f-cost in both open sets exceeds best path cost, we're done
		forwardMinF, backwardMinF := 1e18, 1e18
		if forwardOpen.Len() > 0 {
			forwardMinF = (*forwardOpen)[0].fCost
		}
		if backwardOpen.Len() > 0 {
			backwardMinF = (*backwardOpen)[0].fCost
		}

		if forwardMinF+backwardMinF >= bestPathCost {
			// We've found the optimal path
			break
		}

		// Expand forward search
		if forwardOpen.Len() > 0 && (backwardOpen.Len() == 0 || forwardOpen.Len() <= backwardOpen.Len()) {
			current := heap.Pop(forwardOpen).(*bidirNode)
			forwardClosed[current.pos] = current

			// Check if this node was reached by backward search
			if backwardNode, found := backwardClosed[current.pos]; found {
				pathCost := current.gCost + backwardNode.gCost
				if pathCost < bestPathCost {
					bestPathCost = pathCost
					bestMeetingNode = current
					bestMeetingNodeBackward = backwardNode
				}
			}

			// Expand forward neighbors
			neighbors := pf.movementValidator.GetPossibleMoves(current.pos, goal, forwardPrune)
			for _, neighborStep := range neighbors {
				neighborPos := neighborStep.Position

				if _, inClosed := forwardClosed[neighborPos]; inClosed {
					continue
				}

				tentativeGCost := current.gCost + neighborStep.Cost

				existingGCost, exists := forwardGScores[neighborPos]
				if !exists || tentativeGCost < existingGCost {
					forwardGScores[neighborPos] = tentativeGCost

					neighborNode := &bidirNode{
						pos:      neighborPos,
						parent:   current,
						movement: neighborStep.Movement,
						gCost:    tentativeGCost,
						hCost:    heuristic(neighborPos, goal),
					}
					neighborNode.fCost = neighborNode.gCost + neighborNode.hCost
					heap.Push(forwardOpen, neighborNode)

					// Check if backward search already reached this node
					if backwardNode, found := backwardClosed[neighborPos]; found {
						pathCost := tentativeGCost + backwardNode.gCost
						if pathCost < bestPathCost {
							bestPathCost = pathCost
							bestMeetingNode = neighborNode
							bestMeetingNodeBackward = backwardNode
						}
					}
				}
			}
		}

		// Expand backward search
		if backwardOpen.Len() > 0 && (forwardOpen.Len() == 0 || backwardOpen.Len() < forwardOpen.Len()) {
			current := heap.Pop(backwardOpen).(*bidirNode)
			backwardClosed[current.pos] = current

			// Check if this node was reached by forward search
			if forwardNode, found := forwardClosed[current.pos]; found {
				pathCost := forwardNode.gCost + current.gCost
				if pathCost < bestPathCost {
					bestPathCost = pathCost
					bestMeetingNode = forwardNode
					bestMeetingNodeBackward = current
				}
			}

			// Expand backward neighbors (using reverse moves)
			neighbors := pf.getReverseMoves(current.pos, start, backwardPrune)
			for _, neighborStep := range neighbors {
				neighborPos := neighborStep.Position

				if _, inClosed := backwardClosed[neighborPos]; inClosed {
					continue
				}

				tentativeGCost := current.gCost + neighborStep.Cost

				existingGCost, exists := backwardGScores[neighborPos]
				if !exists || tentativeGCost < existingGCost {
					backwardGScores[neighborPos] = tentativeGCost

					neighborNode := &bidirNode{
						pos:      neighborPos,
						parent:   current,
						movement: neighborStep.Movement,
						gCost:    tentativeGCost,
						hCost:    heuristic(neighborPos, start),
					}
					neighborNode.fCost = neighborNode.gCost + neighborNode.hCost
					heap.Push(backwardOpen, neighborNode)

					// Check if forward search already reached this node
					if forwardNode, found := forwardClosed[neighborPos]; found {
						pathCost := forwardNode.gCost + tentativeGCost
						if pathCost < bestPathCost {
							bestPathCost = pathCost
							bestMeetingNode = forwardNode
							bestMeetingNodeBackward = neighborNode
						}
					}
				}
			}
		}
	}

	// Reconstruct path if found
	if bestMeetingNode != nil && bestMeetingNodeBackward != nil {
		path := pf.reconstructBidirPath(bestMeetingNode, bestMeetingNodeBackward, start, goal)
		path.SearchTime = float64(time.Since(startTime).Milliseconds())

		utils.SafeLogger(pf.logger).Info("[Bidir-A*] " + path.LogSummary())
		utils.SafeLogger(pf.logger).Debug("[Bidir-A*] explored", "steps", stepsProcessed, "forwardClosed", len(forwardClosed), "backwardClosed", len(backwardClosed))

		return path, nil
	}

	// No path found
	utils.SafeLogger(pf.logger).Warn("[Bidir-A*] pathfinding failed", "steps", stepsProcessed, "forwardClosed", len(forwardClosed), "backwardClosed", len(backwardClosed))
	return &Path{
		Found:      false,
		StartPos:   start,
		GoalPos:    goal,
		SearchTime: float64(time.Since(startTime).Milliseconds()),
	}, fmt.Errorf("path not found: no valid path exists")
}

// getReverseMoves returns moves that could ARRIVE at a position
// This is the inverse of GetPossibleMoves - what moves could have led TO this position
func (pf *bidirAStarPathFinder) getReverseMoves(to models.V3, searchTarget models.V3, prune *MovePruneConfig) []PathStep {
	moves := make([]PathStep, 0, 32)

	// Cardinal directions
	cardinalDirs := []struct {
		dx, dz float64
	}{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	}

	// Diagonal directions
	diagonalDirs := []struct {
		dx, dz float64
	}{
		{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
	}

	// For each direction, check what moves could have arrived here
	for _, dir := range cardinalDirs {
		// If we can traverse FROM neighbor TO here, then neighbor is a valid reverse move
		from := to.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if pf.movementValidator.CanTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: Traverse,
				Cost:     Traverse.BaseCost(),
			})
		}

		// If we can ascend FROM neighbor (1 below) TO here, then that neighbor is valid
		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if pf.movementValidator.CanAscend(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: AscendJump,
				Cost:     AscendJump.BaseCost(),
			})
		}
		if pf.movementValidator.CanAscendStairs(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: AscendStairs,
				Cost:     AscendStairs.BaseCost(),
			})
		}

		// If we can descend FROM neighbor (1-3 above) TO here, then that neighbor is valid
		for dropHeight := float64(1); dropHeight <= 3; dropHeight++ {
			fromAbove := to.Add(models.V3{X: dir.dx, Y: dropHeight, Z: dir.dz})
			if pf.movementValidator.CanDescend(fromAbove, to) {
				moves = append(moves, PathStep{
					Position: fromAbove,
					Movement: Descend,
					Cost:     Descend.BaseCost(),
				})
				break // Only take first valid
			}
		}

		// Descend stairs reverse
		fromAbove := to.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if pf.movementValidator.CanDescendStairs(fromAbove, to) {
			moves = append(moves, PathStep{
				Position: fromAbove,
				Movement: DescendStairs,
				Cost:     DescendStairs.BaseCost(),
			})
		}

		// Jump2 reverse - could have jumped 2 blocks to get here
		fromJump := to.Add(models.V3{X: dir.dx * 2, Y: 0, Z: dir.dz * 2})
		if pf.movementValidator.CanJump2(fromJump, to) {
			moves = append(moves, PathStep{
				Position: fromJump,
				Movement: Jump2,
				Cost:     Jump2.BaseCost(),
			})
		}

		// Jump2 up reverse - could have jumped 2 blocks and up 1 to get here
		fromJumpBelow := to.Add(models.V3{X: dir.dx * 2, Y: -1, Z: dir.dz * 2})
		if pf.movementValidator.CanJump2(fromJumpBelow, to) {
			moves = append(moves, PathStep{
				Position: fromJumpBelow,
				Movement: Jump2,
				Cost:     Jump2.BaseCost() + 0.5,
			})
		}
	}

	// Diagonal movements
	for _, dir := range diagonalDirs {
		from := to.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if pf.movementValidator.CanDiagonalTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: DiagonalTraverse,
				Cost:     DiagonalTraverse.BaseCost(),
			})
		}

		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if pf.movementValidator.CanDiagonalAscend(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: DiagonalAscend,
				Cost:     DiagonalAscend.BaseCost(),
			})
		}
	}

	// Climbing reverse - could have climbed up or down to get here
	for dy := float64(-1); dy <= 1; dy++ {
		if dy == 0 {
			continue
		}
		from := to.Add(models.V3{X: 0, Y: dy, Z: 0})
		if pf.movementValidator.CanClimb(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: Climb,
				Cost:     Climb.BaseCost(),
			})
		}
	}

	// EnterClimb reverse - could have entered climb from adjacent
	for _, dir := range cardinalDirs {
		from := to.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if pf.movementValidator.CanEnterClimb(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}
		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if pf.movementValidator.CanEnterClimb(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: JumpToClimb,
				Cost:     JumpToClimb.BaseCost(),
			})
		}
		fromAbove := to.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if pf.movementValidator.CanEnterClimb(fromAbove, to) {
			moves = append(moves, PathStep{
				Position: fromAbove,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}
	}

	// Swimming reverse
	allDirs := append(cardinalDirs, diagonalDirs...)
	for _, dir := range allDirs {
		from := to.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if pf.movementValidator.CanSwim(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: Swim,
				Cost:     Swim.BaseCost(),
			})
		}
		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if pf.movementValidator.CanSwimUp(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: SwimUp,
				Cost:     SwimUp.BaseCost(),
			})
		}
		fromAbove := to.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if pf.movementValidator.CanSwimDown(fromAbove, to) {
			moves = append(moves, PathStep{
				Position: fromAbove,
				Movement: SwimDown,
				Cost:     SwimDown.BaseCost(),
			})
		}
	}

	// Apply pruning if configured
	if prune != nil && (searchTarget.X != 0 || searchTarget.Y != 0 || searchTarget.Z != 0) {
		toDist := to.DistanceTo(searchTarget)
		filtered := make([]PathStep, 0, len(moves))

		for _, move := range moves {
			fromDist := move.Position.DistanceTo(searchTarget)
			// For backward search, we want moves that get us closer to start
			if fromDist <= toDist+prune.DriftCap {
				filtered = append(filtered, move)
			}
		}

		// Remove duplicates
		seen := make(map[models.V3]bool)
		unique := make([]PathStep, 0, len(filtered))
		for _, move := range filtered {
			if !seen[move.Position] {
				seen[move.Position] = true
				unique = append(unique, move)
			}
		}

		return unique
	}

	return moves
}

// reconstructBidirPath builds the final path from the meeting point
func (pf *bidirAStarPathFinder) reconstructBidirPath(forwardNode, backwardNode *bidirNode, start, goal models.V3) *Path {
	// Build forward path (start -> meeting point)
	forwardSteps := make([]PathStep, 0)
	totalCost := 0.0

	current := forwardNode
	for current.parent != nil {
		step := PathStep{
			Position: current.pos,
			Movement: current.movement,
			Cost:     current.gCost - current.parent.gCost,
		}
		forwardSteps = append(forwardSteps, step)
		totalCost += step.Cost
		current = current.parent
	}

	// Reverse forward steps to get start -> meeting order
	for i, j := 0, len(forwardSteps)-1; i < j; i, j = i+1, j-1 {
		forwardSteps[i], forwardSteps[j] = forwardSteps[j], forwardSteps[i]
	}

	// Build backward path (meeting point -> goal)
	// The backward search went from goal toward start, so we traverse parent links
	// but the moves are already in the correct direction (toward goal)
	current = backwardNode
	for current.parent != nil {
		// The move stored in current is how we got FROM current TO current.parent in backward search
		// But for the final path, we need the move FROM current.parent TO current
		// We need to look up what move would take us from current to parent
		parentPos := current.parent.pos
		moves := pf.movementValidator.GetPossibleMoves(current.pos, parentPos, nil)

		// Find the move that goes to parent
		var moveToParent PathStep
		found := false
		for _, m := range moves {
			if m.Position == parentPos {
				moveToParent = m
				found = true
				break
			}
		}

		if found {
			step := PathStep{
				Position: parentPos,
				Movement: moveToParent.Movement,
				Cost:     moveToParent.Cost,
			}
			forwardSteps = append(forwardSteps, step)
			totalCost += step.Cost
		} else {
			// Fallback: use the stored movement type with estimated cost
			step := PathStep{
				Position: parentPos,
				Movement: current.movement,
				Cost:     current.gCost - current.parent.gCost,
			}
			forwardSteps = append(forwardSteps, step)
			totalCost += step.Cost
		}

		current = current.parent
	}

	return &Path{
		Steps:     forwardSteps,
		TotalCost: totalCost,
		StartPos:  start,
		GoalPos:   goal,
		Found:     true,
	}
}

// FindGroundBelow delegates to the movement validator
func (pf *bidirAStarPathFinder) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return pf.movementValidator.FindGroundBelow(x, z, startY, maxSearchDepth)
}
