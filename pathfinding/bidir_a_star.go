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
	world            models.World
	shapeMgr         models.BlockShapeManager
	goalRadius       float64
	contextCheckFreq int
	logger           *slog.Logger
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
		world:            w,
		shapeMgr:         shapeMgr,
		goalRadius:       goalRadius,
		contextCheckFreq: contextCheckFreq,
		logger:           logger,
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

	// Snap goal onto the same grid start's own moves can actually reach —
	// see snapGoalToReachableGrid's own doc comment for the live-confirmed
	// bug this closes. In particular, the backward search below seeds
	// its own root node directly from goal (see goalNode below), so an
	// unreachable raw goal would otherwise poison that search's own
	// starting point, not just the forward search's termination check.
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

	utils.SafeLogger(pf.logger).Debug("[Bidir-A*] start", "startX", start.X, "startY", start.Y, "startZ", start.Z, "goalX", goal.X, "goalY", goal.Y, "goalZ", goal.Z)

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
	// memoization via ResetBlockCache). The forward and backward
	// searches below probe heavily overlapping territory near the
	// meeting point, so this call's own memoization is doubly valuable
	// here.
	movementValidator := NewMovementValidator(pf.world, pf.shapeMgr, pf.logger)
	movementValidator.ResetBlockCache()

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
			neighbors := movementValidator.GetPossibleMoves(current.pos, goal, forwardPrune)
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
			neighbors := pf.getReverseMoves(movementValidator, current.pos, start, backwardPrune)
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
		path := pf.reconstructBidirPath(movementValidator, bestMeetingNode, bestMeetingNodeBackward, start, goal)
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
func (pf *bidirAStarPathFinder) getReverseMoves(mv *MovementValidator, to models.V3, searchTarget models.V3, prune *MovePruneConfig) []PathStep {
	moves := make([]PathStep, 0, 32)

	// cardinalDirs/diagonalDirs/allDirs are package-level (see movement.go)

	// For each direction, check what moves could have arrived here
	for _, dir := range cardinalDirs {
		// If we can traverse FROM neighbor TO here, then neighbor is a valid reverse move
		from := to.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: Traverse,
				Cost:     Traverse.BaseCost(),
			})
		}

		// If we can ascend FROM neighbor (1 below) TO here, then that neighbor is valid
		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if mv.CanAscend(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: AscendJump,
				Cost:     AscendJump.BaseCost(),
			})
		}
		if mv.CanAscendStairs(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: AscendStairs,
				Cost:     AscendStairs.BaseCost(),
			})
		}

		// If we can descend FROM neighbor (1-3 above) TO here, then that neighbor
		// is valid. Unlike forward generation (movement.go), which can stop at
		// the first valid drop height because 'from' is a fixed, real position
		// and gravity would stop the fall there, CanDescend itself never
		// validates 'from' - so here, where we're guessing candidate 'from'
		// positions at every height, a shallower drop height can spuriously
		// satisfy CanDescend from a position that was never actually reachable
		// (e.g. mid-air under an elevated platform). Breaking after the first
		// match discarded the genuine deeper drop (e.g. straight off a 3-block
		// platform edge) whenever that happened, silently making the real
		// source unreachable from backward search. Emit every valid height
		// instead and let normal graph connectivity discard the bogus ones.
		for dropHeight := float64(1); dropHeight <= 3; dropHeight++ {
			fromAbove := to.Add(models.V3{X: dir.dx, Y: dropHeight, Z: dir.dz})
			if mv.CanDescend(fromAbove, to) {
				moves = append(moves, PathStep{
					Position: fromAbove,
					Movement: Descend,
					Cost:     Descend.BaseCost(),
				})
			}
		}

		// Descend stairs reverse
		fromAbove := to.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if mv.CanDescendStairs(fromAbove, to) {
			moves = append(moves, PathStep{
				Position: fromAbove,
				Movement: DescendStairs,
				Cost:     DescendStairs.BaseCost(),
			})
		}

		// Jump2 reverse - could have jumped 2 blocks to get here
		fromJump := to.Add(models.V3{X: dir.dx * 2, Y: 0, Z: dir.dz * 2})
		if mv.CanJump2(fromJump, to) {
			moves = append(moves, PathStep{
				Position: fromJump,
				Movement: Jump2,
				Cost:     Jump2.BaseCost(),
			})
		}

		// Jump2 up reverse - could have jumped 2 blocks and up 1 to get here
		fromJumpBelow := to.Add(models.V3{X: dir.dx * 2, Y: -1, Z: dir.dz * 2})
		if mv.CanJump2(fromJumpBelow, to) {
			moves = append(moves, PathStep{
				Position: fromJumpBelow,
				Movement: Jump2,
				Cost:     Jump2.BaseCost() + 0.5,
			})
		}

		// WadeWater reverse - could have waded into this water cell from an
		// adjacent dry or water cell. Forward GetPossibleMoves generates this
		// for cardinal directions (movement.go); it was previously missing
		// here entirely, which silently made any water crossing unreachable
		// from the backward (goal-side) search.
		if mv.CanWadeWater(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: WadeWater,
				Cost:     WadeWater.BaseCost(),
			})
		}

		// ExitWater reverse - could have exited water onto this dry position,
		// either at the same level or via a one-block step-up. Also previously
		// missing, so backward search could never leave a body of water.
		if mv.CanExitWater(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: ExitWater,
				Cost:     ExitWater.BaseCost(),
			})
		}
		if mv.CanExitWater(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: ExitWater,
				Cost:     ExitWater.BaseCost() + 0.5,
			})
		}
	}

	// Diagonal movements
	for _, dir := range diagonalDirs {
		from := to.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanDiagonalTraverse(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: DiagonalTraverse,
				Cost:     DiagonalTraverse.BaseCost(),
			})
		}
		if mv.CanDiagonalWadeWater(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: WadeWater,
				Cost:     WadeWater.BaseCost(),
			})
		}

		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if mv.CanDiagonalAscend(fromBelow, to) {
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
		if mv.CanClimb(from, to) {
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
		if mv.CanEnterClimb(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}
		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if mv.CanEnterClimb(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: JumpToClimb,
				Cost:     JumpToClimb.BaseCost(),
			})
		}
		fromAbove := to.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if mv.CanEnterClimb(fromAbove, to) {
			moves = append(moves, PathStep{
				Position: fromAbove,
				Movement: EnterClimb,
				Cost:     EnterClimb.BaseCost(),
			})
		}

		// ExitClimb reverse - could have exited a ladder/vine onto this
		// position, either at the same level or via a one-block step-up (see
		// MovementValidator.CanExitClimb / getExitClimbTargetY). Also
		// previously missing, so backward search could never leave a
		// ladder-gated platform - the exact case that made the goal
		// unreachable from a course where the only way up is a ladder.
		sameLevelTo := models.V3{X: to.X, Y: to.Y, Z: to.Z}
		if mv.CanExitClimb(from, sameLevelTo) &&
			mv.getExitClimbTargetY(from, sameLevelTo) == to.Y {
			moves = append(moves, PathStep{
				Position: from,
				Movement: ExitClimb,
				Cost:     ExitClimb.BaseCost(),
			})
		}
		stepUpTo := models.V3{X: to.X, Y: to.Y - 1, Z: to.Z}
		if mv.CanExitClimb(fromBelow, stepUpTo) &&
			mv.getExitClimbTargetY(fromBelow, stepUpTo) == to.Y {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: ExitClimb,
				Cost:     ExitClimb.BaseCost(),
			})
		}
	}

	// Swimming reverse
	for _, dir := range allDirs {
		from := to.Add(models.V3{X: dir.dx, Y: 0, Z: dir.dz})
		if mv.CanSwim(from, to) {
			moves = append(moves, PathStep{
				Position: from,
				Movement: Swim,
				Cost:     Swim.BaseCost(),
			})
		}
		fromBelow := to.Add(models.V3{X: dir.dx, Y: -1, Z: dir.dz})
		if mv.CanSwimUp(fromBelow, to) {
			moves = append(moves, PathStep{
				Position: fromBelow,
				Movement: SwimUp,
				Cost:     SwimUp.BaseCost(),
			})
		}
		fromAbove := to.Add(models.V3{X: dir.dx, Y: 1, Z: dir.dz})
		if mv.CanSwimDown(fromAbove, to) {
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
func (pf *bidirAStarPathFinder) reconstructBidirPath(mv *MovementValidator, forwardNode, backwardNode *bidirNode, start, goal models.V3) *Path {
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
		moves := mv.GetPossibleMoves(current.pos, parentPos, nil)

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
	return NewMovementValidator(pf.world, pf.shapeMgr, pf.logger).FindGroundBelow(x, z, startY, maxSearchDepth)
}
