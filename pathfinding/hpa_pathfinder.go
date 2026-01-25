package pathfinding

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// hpaPathFinder implements PathFinder using hierarchical pathfinding
type hpaPathFinder struct {
	world              models.World
	shapeMgr           models.BlockShapeManager
	lowLevelPathfinder models.PathFinder
	builder            *HPABuilder
	clusterSize        int
	debugViz           *HPADebugVisualizer
	entranceMaxCount   int
	entranceMaxCost    float64
}

// NewHPAPathFinder creates a new HPA* pathfinder
func NewHPAPathFinder(world models.World, shapeMgr models.BlockShapeManager, lowLevelPathfinder models.PathFinder, clusterSize int) models.PathFinder {
	builder := NewHPABuilder(world, shapeMgr, lowLevelPathfinder, clusterSize)

	return &hpaPathFinder{
		world:              world,
		shapeMgr:           shapeMgr,
		lowLevelPathfinder: lowLevelPathfinder,
		builder:            builder,
		clusterSize:        clusterSize,
		entranceMaxCount:   4,
		entranceMaxCost:    10,
	}
}

// FindPath finds a path using hierarchical pathfinding
func (hpa *hpaPathFinder) FindPath(start, goal models.V3, maxSteps int) (resultPath *Path, err error) {
	defer func() {
		// Visualize final path
		if hpa.debugViz != nil && hpa.debugViz.IsEnabled() && resultPath != nil {
			hpa.debugViz.VisualizePath(context.Background(), resultPath)
		}
	}()

	startTime := time.Now()

	log.Printf("[HPA*] Finding path from (%.0f,%.0f,%.0f) to (%.0f,%.0f,%.0f)",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z)

	// Check if start == goal
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

	// For short distances, the low-level pathfinder is more reliable than HPA* entrances.
	// This avoids detours to cluster boundaries for nearby goals.
	dist := start.DistanceTo(goal)
	if dist <= float64(hpa.clusterSize) {
		log.Printf("[HPA*] Short distance %.1f (<= clusterSize %d), using low-level pathfinding",
			dist, hpa.clusterSize)
		return hpa.lowLevelPathfinder.FindPath(start, goal, maxSteps)
	}

	// Step 1: Build clusters containing start and goal
	startClusterID := hpa.builder.GetClusterManager().GetClusterID(start)
	goalClusterID := hpa.builder.GetClusterManager().GetClusterID(goal)

	log.Printf("[HPA*] Start cluster: %s, Goal cluster: %s", startClusterID.String(), goalClusterID.String())

	hpa.buildClusterRegion(startClusterID, goalClusterID)

	// Ensure clusters are built
	hpa.builder.BuildCluster(startClusterID)
	hpa.builder.BuildCluster(goalClusterID)

	// If same cluster, just use low-level pathfinding
	if startClusterID == goalClusterID {
		log.Printf("[HPA*] Same cluster, using low-level pathfinding")
		return hpa.lowLevelPathfinder.FindPath(start, goal, maxSteps)
	}
	if clustersAdjacent(startClusterID, goalClusterID) {
		log.Printf("[HPA*] Adjacent clusters, using low-level pathfinding")
		return hpa.lowLevelPathfinder.FindPath(start, goal, maxSteps)
	}

	// Step 2: Insert start and goal into abstract graph
	startNodes, startCleanup := hpa.insertNode(start)
	goalNodes, goalCleanup := hpa.insertNode(goal)

	defer startCleanup()
	defer goalCleanup()

	if len(startNodes) == 0 {
		log.Printf("[HPA*] Cannot connect start to abstract graph, falling back to low-level pathfinding")
		return hpa.lowLevelPathfinder.FindPath(start, goal, maxSteps)
	}
	if len(goalNodes) == 0 {
		log.Printf("[HPA*] Cannot connect goal to abstract graph, falling back to low-level pathfinding")
		return hpa.lowLevelPathfinder.FindPath(start, goal, maxSteps)
	}

	filterStartEdges := filterTempNodeEdges(startNodes, "start")

	// goalNodes contains the temporary node(s) inserted for the goal
	// Extract the actual entrance nodes that the goal connects to
	goalEntrances := make([]*AbstractNode, 0)
	for _, goalTempNode := range goalNodes {
		for _, edge := range goalTempNode.Edges {
			goalEntrances = append(goalEntrances, edge.To)
		}
	}

	log.Printf("[HPA*] Inserted start (%d connections) and goal (%d connections to entrances)",
		len(startNodes), len(goalEntrances))

	if len(goalEntrances) == 0 {
		return nil, fmt.Errorf("goal position has no reachable entrances")
	}

	filteredGoalEntrances := filterEntranceNodes(goalEntrances)
	if len(filteredGoalEntrances) > 0 {
		goalEntrances = filteredGoalEntrances
	}

	if filterStartEdges || len(filteredGoalEntrances) > 0 {
		log.Printf("[HPA*] Filtered entrance candidates: startFiltered=%t goalFiltered=%t",
			filterStartEdges, len(filteredGoalEntrances) > 0)
	}

	// Sort goal entrances by distance to actual goal (closest first)
	// This ensures we try entrances nearest to the goal first
	for i := 0; i < len(goalEntrances); i++ {
		for j := i + 1; j < len(goalEntrances); j++ {
			dist1 := goalEntrances[i].GetPosition().DistanceTo(goal)
			dist2 := goalEntrances[j].GetPosition().DistanceTo(goal)
			if dist2 < dist1 {
				goalEntrances[i], goalEntrances[j] = goalEntrances[j], goalEntrances[i]
			}
		}
	}

	// Step 3: Try each goal entrance until we find a refinable path
	var refinedPath *Path
	for i, goalEntrance := range goalEntrances {
		// Search abstract graph to this specific goal entrance
		abstractPath := hpa.builder.GetAbstractGraph().SearchAbstractGraph(startNodes, []*AbstractNode{goalEntrance})
		if abstractPath == nil {
			log.Printf("[HPA*] No abstract path to goal entrance %d/%d at (%.0f,%.0f,%.0f)",
				i+1, len(goalEntrances), goalEntrance.GetPosition().X, goalEntrance.GetPosition().Y, goalEntrance.GetPosition().Z)
			continue
		}

		log.Printf("[HPA*] Found abstract path to goal entrance %d/%d with %d edges", i+1, len(goalEntrances), len(abstractPath))
		hpa.logAbstractPath(abstractPath)

		// Visualize abstract path
		if hpa.debugViz != nil && hpa.debugViz.IsEnabled() {
			hpa.debugViz.VisualizeAbstractPath(context.Background(), abstractPath, start, goal)
		}

		// Step 4: Try to refine this abstract path + final segment to goal
		refinedPath = hpa.refinePathThroughEntrance(abstractPath, start, goal, goalNodes[0])
		if refinedPath != nil {
			log.Printf("[HPA*] Successfully refined path through goal entrance %d/%d", i+1, len(goalEntrances))
			break // Success!
		}

		log.Printf("[HPA*] Failed to refine path through goal entrance %d/%d, trying next entrance", i+1, len(goalEntrances))
	}

	if refinedPath == nil {
		return nil, fmt.Errorf("failed to refine abstract path through any goal entrance (%d entrances tried)", len(goalEntrances))
	}

	refinedPath.SearchTime = float64(time.Since(startTime).Milliseconds())

	log.Printf("[HPA*] Refined path: %d steps, cost=%.2f, time=%.0fms",
		len(refinedPath.Steps), refinedPath.TotalCost, refinedPath.SearchTime)

	// Log first 10 steps to diagnose direction issues
	log.Printf("[HPA*] Refined path first 10 steps:")
	for i := 0; i < len(refinedPath.Steps) && i < 10; i++ {
		step := refinedPath.Steps[i]
		log.Printf("[HPA*]   Step %d: %s to (%.1f, %.2f, %.1f)",
			i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
	}

	normalized, err := hpa.normalizePath(refinedPath)
	if err != nil {
		return nil, fmt.Errorf("refined path normalization failed: %w", err)
	}

	if err := hpa.validatePath(normalized); err != nil {
		log.Printf("[HPA*] Refined path validation failed: %v", err)
		return nil, fmt.Errorf("refined path invalid: %w", err)
	}

	return normalized, nil
}

// SetEntranceLimits configures how many entrances are considered per start/goal insert
// and the maximum allowed low-level path cost to an entrance (<=0 disables the limit).
func (hpa *hpaPathFinder) SetEntranceLimits(maxCount int, maxCost float64) {
	hpa.entranceMaxCount = maxCount
	hpa.entranceMaxCost = maxCost
}

// insertNode inserts a temporary node into the abstract graph
// Returns the created node (as single-element slice) and a cleanup function
func (hpa *hpaPathFinder) insertNode(pos models.V3) ([]*AbstractNode, func()) {
	clusterID := hpa.builder.GetClusterManager().GetClusterID(pos)
	cluster := hpa.builder.GetClusterManager().GetCluster(clusterID)

	// Ensure cluster is built
	if cluster.Dirty {
		hpa.builder.BuildCluster(clusterID)
	}

	abstractGraph := hpa.builder.GetAbstractGraph()

	// Create a single temporary node
	tempNode := &AbstractNode{
		Entrance:     nil,
		Edges:        make([]*AbstractEdge, 0),
		IsTemporary:  true,
		TempPosition: pos,
	}

	// Track edges added to entrance nodes (for cleanup)
	var addedEdges []*AbstractNode

	// Try to connect to entrances in this cluster with bidirectional edges.
	// Limit to the nearest entrances by low-level path cost to avoid detours.
	type entranceCandidate struct {
		entrance     *Entrance
		path         *Path
		cost         float64
		actualPos    models.V3 // The actual grouped position we pathed to (may differ from entrance.Pos1/Pos2)
		actualIsPos1 bool      // True if actualPos is on Pos1 side, false if Pos2 side
	}
	candidates := make([]entranceCandidate, 0, len(cluster.Entrances))

	for _, entrance := range cluster.Entrances {
		// Determine which cluster we're coming from (0=Cluster1, 1=Cluster2)
		clusterIndex := 0
		if entrance.Cluster2 == clusterID {
			clusterIndex = 1
		}

		// Try all grouped positions for this entrance, not just the representative
		positionsToTry := entrance.GroupedPositions[clusterIndex]
		if len(positionsToTry) == 0 {
			// Fallback: no grouped positions, use the main position
			positionsToTry = []models.V3{entrance.GetPosInCluster(clusterID)}
		}

		// Try each position and keep the best path
		var bestPath *Path
		var bestCost float64 = math.MaxFloat64
		var bestPos models.V3

		for _, entrancePos := range positionsToTry {
			// Find path from pos to this entrance position
			dist := pos.DistanceTo(entrancePos)
			maxSteps := int(dist * 100) // 100 steps per block of distance
			if maxSteps < 1000 {
				maxSteps = 1000 // Increased minimum from 500 to 1000
			}

			path, err := hpa.lowLevelPathfinder.FindPath(pos, entrancePos, maxSteps)
			if err != nil || !path.Found {
				continue // Try next position
			}
			if hpa.entranceMaxCost > 0 && path.TotalCost > hpa.entranceMaxCost {
				continue // Too expensive
			}

			// Keep the best path to any position in this entrance group
			if path.TotalCost < bestCost {
				bestPath = path
				bestCost = path.TotalCost
				bestPos = entrancePos
			}
		}

		if bestPath != nil {
			log.Printf("[HPA*] Best path to entrance: pos=(%.0f,%.0f,%.0f), cost=%.2f (tried %d positions)",
				bestPos.X, bestPos.Y, bestPos.Z, bestCost, len(positionsToTry))
			candidates = append(candidates, entranceCandidate{
				entrance:     entrance,
				path:         bestPath,
				cost:         bestCost,
				actualPos:    bestPos,
				actualIsPos1: (clusterIndex == 0), // 0=Cluster1/Pos1, 1=Cluster2/Pos2
			})
		} else {
			log.Printf("[HPA*] Failed to find path to any position in entrance group (tried %d positions)", len(positionsToTry))
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].cost < candidates[j].cost
	})

	log.Printf("[HPA*] insertNode at (%.0f,%.0f,%.0f): %d entrances, %d candidates after cost limit",
		pos.X, pos.Y, pos.Z, len(cluster.Entrances), len(candidates))
	if hpa.entranceMaxCount > 0 && len(candidates) > hpa.entranceMaxCount {
		candidates = candidates[:hpa.entranceMaxCount]
	}

	for i, candidate := range candidates {
		posInCluster := candidate.entrance.GetPosInCluster(clusterID)
		log.Printf("[HPA*]   candidate %d: entrance=(%.0f,%.0f,%.0f) cost=%.2f",
			i+1, posInCluster.X, posInCluster.Y, posInCluster.Z, candidate.cost)
	}

	for _, candidate := range candidates {
		// Use the ORIGINAL entrance object to connect to existing graph nodes
		// The path already goes to the correct actualPos, so no modification needed
		// This ensures temporary nodes connect to the same nodes used by inter-cluster edges
		entranceNode := abstractGraph.GetOrCreateNode(candidate.entrance)

		// Add forward edge: temp -> entrance
		// The path goes to actualPos, which handles the position properly
		forwardEdge := &AbstractEdge{
			To:   entranceNode,
			Cost: candidate.cost,
			Path: candidate.path,
		}
		tempNode.Edges = append(tempNode.Edges, forwardEdge)

		// Add reverse edge: entrance -> temp (for goal nodes to be reachable)
		reverseEdge := &AbstractEdge{
			To:   tempNode,
			Cost: candidate.cost,
			Path: candidate.path,
		}
		entranceNode.Edges = append(entranceNode.Edges, reverseEdge)
		addedEdges = append(addedEdges, entranceNode)
	}

	// Cleanup function to remove temporary node and reverse edges
	cleanup := func() {
		// Remove reverse edges from entrance nodes
		for _, entranceNode := range addedEdges {
			newEdges := make([]*AbstractEdge, 0)
			for _, edge := range entranceNode.Edges {
				if edge.To != tempNode {
					newEdges = append(newEdges, edge)
				}
			}
			entranceNode.Edges = newEdges
		}
	}

	if len(tempNode.Edges) == 0 {
		log.Printf("[HPA*] Warning: insertNode at (%.0f,%.0f,%.0f) found NO reachable entrances in cluster %s",
			pos.X, pos.Y, pos.Z, clusterID.String())
		return nil, cleanup
	}

	log.Printf("[HPA*] insertNode at (%.0f,%.0f,%.0f): connected to %d entrances",
		pos.X, pos.Y, pos.Z, len(tempNode.Edges))

	return []*AbstractNode{tempNode}, cleanup
}

// refinePathThroughEntrance refines an abstract path to an entrance, then adds the final segment to goal
func (hpa *hpaPathFinder) refinePathThroughEntrance(abstractPath []*AbstractEdge, start, goal models.V3, goalTempNode *AbstractNode) *Path {
	// First refine the abstract path to the entrance
	if len(abstractPath) == 0 {
		// No abstract path, just need to go from start to goal directly through temp node's edge
		// This happens when start and goal are in the same cluster
		if len(goalTempNode.Edges) > 0 {
			// Use the cached path from the temp node's first edge
			if goalTempNode.Edges[0].Path != nil {
				return goalTempNode.Edges[0].Path
			}
		}
		return nil
	}

	allSteps := make([]PathStep, 0)
	totalCost := 0.0
	currentPos := start

	// Refine the abstract path (start -> ... -> entrance)
	for i, edge := range abstractPath {
		nextPos := edge.To.GetPosition()

		if edge.Path != nil {
			// Validate that cached path connects to currentPos
			if len(edge.Path.Steps) > 0 {
				firstStep := edge.Path.Steps[0].Position
				if !positionsEqual(currentPos, firstStep) {
					log.Printf("[HPA*] Gap detected in abstract edge %d/%d: currentPos=(%.0f,%.0f,%.0f) but cached path starts at (%.0f,%.0f,%.0f)",
						i+1, len(abstractPath), currentPos.X, currentPos.Y, currentPos.Z, firstStep.X, firstStep.Y, firstStep.Z)

					// Create bridge path to connect the gap
					bridgeDist := currentPos.DistanceTo(firstStep)
					bridgeMaxSteps := int(bridgeDist * 50)
					if bridgeMaxSteps < 100 {
						bridgeMaxSteps = 100
					}

					bridgePath, err := hpa.lowLevelPathfinder.FindPath(currentPos, firstStep, bridgeMaxSteps)
					if err != nil || !bridgePath.Found {
						log.Printf("[HPA*] Failed to bridge gap in abstract edge %d/%d from (%.0f,%.0f,%.0f) to (%.0f,%.0f,%.0f): %v",
							i+1, len(abstractPath), currentPos.X, currentPos.Y, currentPos.Z, firstStep.X, firstStep.Y, firstStep.Z, err)
						return nil
					}

					log.Printf("[HPA*] Created bridge path for abstract edge %d/%d with %d steps, cost=%.2f",
						i+1, len(abstractPath), len(bridgePath.Steps), bridgePath.TotalCost)

					// Append bridge steps
					for _, step := range bridgePath.Steps {
						allSteps = append(allSteps, step)
					}
					totalCost += bridgePath.TotalCost
				}
			}

			// Append cached path steps
			for _, step := range edge.Path.Steps {
				allSteps = append(allSteps, step)
			}
			totalCost += edge.Path.TotalCost
			// IMPORTANT: Use the path's actual endpoint, not the entrance representative position
			// This ensures continuity when the path goes to a grouped position different from the representative
			if len(edge.Path.Steps) > 0 {
				currentPos = edge.Path.Steps[len(edge.Path.Steps)-1].Position
			} else {
				currentPos = nextPos // Fallback if path has no steps
			}
			log.Printf("[HPA*] After abstract edge %d/%d (cached): currentPos=(%.0f,%.0f,%.0f), entrancePos=(%.0f,%.0f,%.0f)",
				i+1, len(abstractPath), currentPos.X, currentPos.Y, currentPos.Z, nextPos.X, nextPos.Y, nextPos.Z)
		} else {
			// Calculate dynamic maxSteps based on distance
			// Vertical navigation requires many more steps than horizontal
			dist := currentPos.DistanceTo(nextPos)
			maxSteps := int(dist * 100) // 100 steps per block of Euclidean distance
			if maxSteps < 1000 {
				maxSteps = 1000 // Minimum 1000 steps for complex terrain
			}

			path, err := hpa.lowLevelPathfinder.FindPath(currentPos, nextPos, maxSteps)
			if err != nil || !path.Found {
				log.Printf("[HPA*] Failed to refine abstract edge %d/%d from (%.0f,%.0f,%.0f) to (%.0f,%.0f,%.0f) with maxSteps=%d",
					i+1, len(abstractPath), currentPos.X, currentPos.Y, currentPos.Z, nextPos.X, nextPos.Y, nextPos.Z, maxSteps)
				return nil
			}
			edge.Path = path
			for _, step := range path.Steps {
				allSteps = append(allSteps, step)
			}
			totalCost += path.TotalCost
			// Use the path's actual endpoint for continuity
			if len(path.Steps) > 0 {
				currentPos = path.Steps[len(path.Steps)-1].Position
			} else {
				currentPos = nextPos // Fallback if path has no steps
			}
			log.Printf("[HPA*] After abstract edge %d/%d (computed): currentPos=(%.0f,%.0f,%.0f), entrancePos=(%.0f,%.0f,%.0f)",
				i+1, len(abstractPath), currentPos.X, currentPos.Y, currentPos.Z, nextPos.X, nextPos.Y, nextPos.Z)
		}
	}

	// Now add final segment: entrance -> goal
	// Find the edge from the goal temp node that goes to the entrance we reached
	entrancePos := abstractPath[len(abstractPath)-1].To.GetPosition()
	var finalSegment *Path
	for _, edge := range goalTempNode.Edges {
		if edge.To.GetPosition() == entrancePos && edge.Path != nil {
			// Found the cached path from goal to this entrance
			// Log original cached path details
			log.Printf("[HPA*] Found cached path from goal to entrance:")
			log.Printf("[HPA*]   Cached path: StartPos=(%.0f,%.0f,%.0f) GoalPos=(%.0f,%.0f,%.0f) Steps=%d",
				edge.Path.StartPos.X, edge.Path.StartPos.Y, edge.Path.StartPos.Z,
				edge.Path.GoalPos.X, edge.Path.GoalPos.Y, edge.Path.GoalPos.Z,
				len(edge.Path.Steps))
			if len(edge.Path.Steps) > 0 {
				log.Printf("[HPA*]   First step: to (%.0f,%.0f,%.0f), Last step: to (%.0f,%.0f,%.0f)",
					edge.Path.Steps[0].Position.X, edge.Path.Steps[0].Position.Y, edge.Path.Steps[0].Position.Z,
					edge.Path.Steps[len(edge.Path.Steps)-1].Position.X,
					edge.Path.Steps[len(edge.Path.Steps)-1].Position.Y,
					edge.Path.Steps[len(edge.Path.Steps)-1].Position.Z)
			}
			// We need to reverse it (it goes goal->entrance, we want entrance->goal)
			finalSegment = reversePath(edge.Path, goal)
			break
		}
	}

	if finalSegment == nil {
		// No cached path, compute it
		log.Printf("[HPA*] No cached final segment, computing from (%.0f,%.0f,%.0f) to goal (%.0f,%.0f,%.0f)",
			currentPos.X, currentPos.Y, currentPos.Z, goal.X, goal.Y, goal.Z)
		dist := currentPos.DistanceTo(goal)
		maxSteps := int(dist * 50)
		if maxSteps < 500 {
			maxSteps = 500
		}
		var err error
		finalSegment, err = hpa.lowLevelPathfinder.FindPath(currentPos, goal, maxSteps)
		if err != nil || !finalSegment.Found {
			log.Printf("[HPA*] Failed to find final segment from entrance (%.0f,%.0f,%.0f) to goal (%.0f,%.0f,%.0f)",
				currentPos.X, currentPos.Y, currentPos.Z, goal.X, goal.Y, goal.Z)
			return nil
		}
	} else {
		log.Printf("[HPA*] Using cached (reversed) final segment: %d steps", len(finalSegment.Steps))
		if len(finalSegment.Steps) > 0 {
			log.Printf("[HPA*]   First step: to (%.0f,%.0f,%.0f), Last step: to (%.0f,%.0f,%.0f)",
				finalSegment.Steps[0].Position.X, finalSegment.Steps[0].Position.Y, finalSegment.Steps[0].Position.Z,
				finalSegment.Steps[len(finalSegment.Steps)-1].Position.X,
				finalSegment.Steps[len(finalSegment.Steps)-1].Position.Y,
				finalSegment.Steps[len(finalSegment.Steps)-1].Position.Z)
		}
	}

	// Fix 1: Validate that final segment connects to currentPos
	// If there's a gap, create a bridge path
	if finalSegment != nil && len(finalSegment.Steps) > 0 {
		firstStep := finalSegment.Steps[0].Position
		if !positionsEqual(currentPos, firstStep) {
			log.Printf("[HPA*] Gap detected: currentPos=(%.0f,%.0f,%.0f) but finalSegment starts at (%.0f,%.0f,%.0f)",
				currentPos.X, currentPos.Y, currentPos.Z, firstStep.X, firstStep.Y, firstStep.Z)

			// Try to create a bridge path
			bridgeDist := currentPos.DistanceTo(firstStep)
			bridgeMaxSteps := int(bridgeDist * 50)
			if bridgeMaxSteps < 100 {
				bridgeMaxSteps = 100
			}

			bridgePath, err := hpa.lowLevelPathfinder.FindPath(currentPos, firstStep, bridgeMaxSteps)
			if err != nil || !bridgePath.Found {
				log.Printf("[HPA*] Failed to bridge gap from (%.0f,%.0f,%.0f) to (%.0f,%.0f,%.0f): %v",
					currentPos.X, currentPos.Y, currentPos.Z, firstStep.X, firstStep.Y, firstStep.Z, err)
				return nil
			}

			log.Printf("[HPA*] Created bridge path with %d steps, cost=%.2f",
				len(bridgePath.Steps), bridgePath.TotalCost)

			// Append bridge steps
			for _, step := range bridgePath.Steps {
				allSteps = append(allSteps, step)
			}
			totalCost += bridgePath.TotalCost
			currentPos = firstStep
		}

		// Now append the final segment
		for _, step := range finalSegment.Steps {
			allSteps = append(allSteps, step)
		}
		totalCost += finalSegment.TotalCost
	}

	raw := &Path{
		Steps:     allSteps,
		TotalCost: totalCost,
		StartPos:  start,
		GoalPos:   goal,
		Found:     len(allSteps) > 0,
	}

	// Log first and last 10 steps of raw path before normalization
	log.Printf("[HPA*] Raw path before normalization: %d steps", len(allSteps))
	log.Printf("[HPA*] First 10 steps:")
	for i := 0; i < len(allSteps) && i < 10; i++ {
		step := allSteps[i]
		log.Printf("[HPA*]   Step %d: %s to (%.1f, %.2f, %.1f)",
			i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
	}
	if len(allSteps) > 10 {
		log.Printf("[HPA*] Last 10 steps:")
		for i := len(allSteps) - 10; i < len(allSteps); i++ {
			step := allSteps[i]
			log.Printf("[HPA*]   Step %d: %s to (%.1f, %.2f, %.1f)",
				i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
		}
	}

	normalized, err := hpa.normalizePath(raw)
	if err != nil {
		log.Printf("[HPA*] RefinePath normalization failed: %v", err)
		return nil
	}
	if err := hpa.validatePath(normalized); err != nil {
		log.Printf("[HPA*] RefinePath validation failed: %v", err)
		return nil
	}
	return normalized
}

// reversePath reverses a path (swaps start and goal, reverses steps)
func reversePath(p *Path, newGoal models.V3) *Path {
	if p == nil || len(p.Steps) == 0 {
		return nil
	}

	reversedFull := make([]PathStep, len(p.Steps))
	for i := range p.Steps {
		reversedFull[i] = p.Steps[len(p.Steps)-1-i]
	}

	// Drop the first step ONLY if it equals the new start (old goal).
	// Path steps should not include the starting position.
	// But if the first step doesn't match, keep it to avoid gaps.
	reversed := reversedFull
	if len(reversedFull) > 0 && positionsEqual(reversedFull[0].Position, p.GoalPos) {
		reversed = reversedFull[1:]
	}

	return &Path{
		Steps:     reversed,
		TotalCost: p.TotalCost,
		StartPos:  p.GoalPos,
		GoalPos:   newGoal,
		Found:     true,
	}
}

func (hpa *hpaPathFinder) validatePath(path *Path) error {
	if path == nil || len(path.Steps) == 0 {
		return fmt.Errorf("empty path")
	}
	if hpa.builder == nil || hpa.builder.movementValidator == nil {
		return fmt.Errorf("movement validator unavailable")
	}

	log.Printf("[hpaPathFinder.validatePath] validating path: %s", path.LogDetails(true))
	prev := path.StartPos
	for i, step := range path.Steps {
		if !hpa.isStepReachable(prev, step.Position) {
			return fmt.Errorf("invalid step %d: from (%.0f,%.0f,%.0f) to (%.0f,%.0f,%.0f)",
				i+1, prev.X, prev.Y, prev.Z, step.Position.X, step.Position.Y, step.Position.Z)
		}
		prev = step.Position
	}
	return nil
}

func (hpa *hpaPathFinder) isStepReachable(from, to models.V3) bool {
	// Use zero goal to avoid distance pruning for validation.
	moves := hpa.builder.movementValidator.GetPossibleMoves(from, models.V3{}, nil)
	for _, move := range moves {
		if positionsEqual(move.Position, to) {
			return true
		}
	}
	return false
}

func positionsEqual(a, b models.V3) bool {
	const eps = 0.01
	return math.Abs(a.X-b.X) < eps && math.Abs(a.Y-b.Y) < eps && math.Abs(a.Z-b.Z) < eps
}

func clustersAdjacent(a, b ClusterID) bool {
	return intAbs(a.X-b.X) <= 1 && intAbs(a.Y-b.Y) <= 1 && intAbs(a.Z-b.Z) <= 1
}

func intAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (hpa *hpaPathFinder) buildClusterRegion(startClusterID, goalClusterID ClusterID) {
	minX, maxX := orderedInts(startClusterID.X, goalClusterID.X)
	minY, maxY := orderedInts(startClusterID.Y, goalClusterID.Y)
	minZ, maxZ := orderedInts(startClusterID.Z, goalClusterID.Z)

	clusterCount := (maxX - minX + 1) * (maxY - minY + 1) * (maxZ - minZ + 1)
	const maxRegionClusters = 512
	if clusterCount > maxRegionClusters {
		log.Printf("[HPA*] Skipping region build for %d clusters (max %d); abstract graph may be incomplete",
			clusterCount, maxRegionClusters)
		return
	}

	minPos := models.V3{
		X: float64(minX * hpa.clusterSize),
		Y: float64(minY * hpa.clusterSize),
		Z: float64(minZ * hpa.clusterSize),
	}
	maxPos := models.V3{
		X: float64(maxX * hpa.clusterSize),
		Y: float64(maxY * hpa.clusterSize),
		Z: float64(maxZ * hpa.clusterSize),
	}
	hpa.BuildClustersInRegion(minPos, maxPos)
}

func orderedInts(a, b int) (int, int) {
	if a <= b {
		return a, b
	}
	return b, a
}

func filterTempNodeEdges(nodes []*AbstractNode, label string) bool {
	changed := false
	for _, node := range nodes {
		if node == nil || len(node.Edges) == 0 {
			continue
		}
		kept := make([]*AbstractEdge, 0, len(node.Edges))
		for _, edge := range node.Edges {
			if edge == nil || edge.To == nil || edge.To.Entrance == nil {
				kept = append(kept, edge)
				continue
			}
			if entranceHasInterClusterEdge(edge.To) {
				kept = append(kept, edge)
			}
		}
		if len(kept) > 0 && len(kept) < len(node.Edges) {
			node.Edges = kept
			changed = true
		}
	}
	if changed {
		log.Printf("[HPA*] Filtered temp node edges for %s node(s)", label)
	}
	return changed
}

func filterEntranceNodes(nodes []*AbstractNode) []*AbstractNode {
	if len(nodes) == 0 {
		return nil
	}
	kept := make([]*AbstractNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || node.Entrance == nil {
			continue
		}
		if entranceHasInterClusterEdge(node) {
			kept = append(kept, node)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

func entranceHasInterClusterEdge(node *AbstractNode) bool {
	if node == nil || node.Entrance == nil {
		return false
	}
	for _, edge := range node.Edges {
		if edge == nil || edge.To == nil || edge.To.Entrance == nil {
			continue
		}
		if node.Entrance.Matches(edge.To.Entrance) {
			return true
		}
	}
	return false
}

func (hpa *hpaPathFinder) logAbstractPath(abstractPath []*AbstractEdge) {
	if len(abstractPath) == 0 {
		log.Printf("[HPA*] Abstract path: <empty>")
		return
	}
	log.Printf("[HPA*] Abstract path edges:")
	for i, edge := range abstractPath {
		pos := edge.To.GetPosition()
		log.Printf("[HPA*]   edge %d: to (%.0f,%.0f,%.0f) cost=%.2f",
			i+1, pos.X, pos.Y, pos.Z, edge.Cost)
	}
}

func (hpa *hpaPathFinder) normalizePath(path *Path) (*Path, error) {
	if path == nil || len(path.Steps) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	if hpa.builder == nil || hpa.builder.movementValidator == nil {
		return nil, fmt.Errorf("movement validator unavailable")
	}

	steps := make([]PathStep, 0, len(path.Steps)*2) // Allocate extra space for micro-paths
	totalCost := 0.0
	prev := path.StartPos

	for i, step := range path.Steps {
		if positionsEqual(prev, step.Position) {
			continue
		}

		// Try direct move first (optimization for adjacent steps)
		move, ok := hpa.findMove(prev, step.Position)
		if ok {
			// Direct move exists - use it
			steps = append(steps, PathStep{
				Position: step.Position,
				Movement: move.Movement,
				Cost:     move.Cost,
			})
			totalCost += move.Cost
			prev = step.Position
			continue
		}

		// No direct move - need to pathfind between waypoints
		// This handles long segments from abstract path refinement
		microPath, err := hpa.lowLevelPathfinder.FindPath(prev, step.Position, 1000)
		if err != nil || !microPath.Found {
			return nil, fmt.Errorf("cannot pathfind between waypoints %d: from (%.0f,%.0f,%.0f) to (%.0f,%.0f,%.0f): %v",
				i+1, prev.X, prev.Y, prev.Z, step.Position.X, step.Position.Y, step.Position.Z, err)
		}

		// Insert all steps from micro-path
		for _, microStep := range microPath.Steps {
			steps = append(steps, microStep)
			totalCost += microStep.Cost
		}

		// Update prev to the end of micro-path
		if len(microPath.Steps) > 0 {
			prev = microPath.Steps[len(microPath.Steps)-1].Position
		} else {
			prev = step.Position
		}
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("empty path after normalization")
	}

	return &Path{
		Steps:      steps,
		TotalCost:  totalCost,
		StartPos:   path.StartPos,
		GoalPos:    path.GoalPos,
		Found:      true,
		SearchTime: path.SearchTime,
	}, nil
}

func (hpa *hpaPathFinder) findMove(from, to models.V3) (PathStep, bool) {
	moves := hpa.builder.movementValidator.GetPossibleMoves(from, models.V3{}, nil)
	for _, move := range moves {
		if positionsEqual(move.Position, to) {
			return move, true
		}
	}
	return PathStep{}, false
}

// refinePath refines an abstract path into a low-level path
func (hpa *hpaPathFinder) refinePath(abstractPath []*AbstractEdge, start, goal models.V3) *Path {
	if len(abstractPath) == 0 {
		return nil
	}

	allSteps := make([]PathStep, 0)
	totalCost := 0.0

	currentPos := start

	// For each edge in the abstract path
	for i, edge := range abstractPath {
		nextPos := edge.To.GetPosition()

		// Check if we already have a cached path
		if edge.Path != nil {
			// Use cached path
			for _, step := range edge.Path.Steps {
				allSteps = append(allSteps, step)
			}
			totalCost += edge.Path.TotalCost
			if len(edge.Path.Steps) > 0 {
				currentPos = edge.Path.Steps[len(edge.Path.Steps)-1].Position
			}
		} else {
			// No cached path - compute on-demand
			path, err := hpa.lowLevelPathfinder.FindPath(currentPos, nextPos, 500)
			if err != nil || !path.Found {
				log.Printf("[HPA*] Warning: Failed to refine edge %d/%d from (%.0f,%.0f,%.0f) to (%.0f,%.0f,%.0f)",
					i+1, len(abstractPath), currentPos.X, currentPos.Y, currentPos.Z, nextPos.X, nextPos.Y, nextPos.Z)
				continue
			}

			// Cache the path for future use
			edge.Path = path

			for _, step := range path.Steps {
				allSteps = append(allSteps, step)
			}
			totalCost += path.TotalCost
			currentPos = nextPos
		}
	}

	// Final segment: current position to goal
	if currentPos != goal {
		// Use larger step limit for final segment (may need to navigate around obstacles)
		dist := currentPos.DistanceTo(goal)
		maxSteps := int(dist * 50) // Allow 50 steps per block of distance
		if maxSteps < 500 {
			maxSteps = 500 // Minimum 500 steps
		}
		finalPath, err := hpa.lowLevelPathfinder.FindPath(currentPos, goal, maxSteps)
		if err != nil || !finalPath.Found {
			log.Printf("[HPA*] Failed to find path from last entrance (%.0f,%.0f,%.0f) to goal (%.0f,%.0f,%.0f): %v",
				currentPos.X, currentPos.Y, currentPos.Z, goal.X, goal.Y, goal.Z, err)
			return nil
		}

		for _, step := range finalPath.Steps {
			allSteps = append(allSteps, step)
		}
		totalCost += finalPath.TotalCost
	}

	return &Path{
		Steps:     allSteps,
		TotalCost: totalCost,
		StartPos:  start,
		GoalPos:   goal,
		Found:     len(allSteps) > 0,
	}
}

// FindGroundBelow delegates to the low-level pathfinder
func (hpa *hpaPathFinder) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return hpa.lowLevelPathfinder.FindGroundBelow(x, z, startY, maxSearchDepth)
}

// BuildClustersInRegion builds all clusters in a rectangular region
// This can be called proactively to preprocess a known area
func (hpa *hpaPathFinder) BuildClustersInRegion(minPos, maxPos models.V3) {
	minClusterID := hpa.builder.GetClusterManager().GetClusterID(minPos)
	maxClusterID := hpa.builder.GetClusterManager().GetClusterID(maxPos)

	log.Printf("[HPA*] Building clusters in region %s to %s",
		minClusterID.String(), maxClusterID.String())

	clustersBuilt := 0
	for x := minClusterID.X; x <= maxClusterID.X; x++ {
		for y := minClusterID.Y; y <= maxClusterID.Y; y++ {
			for z := minClusterID.Z; z <= maxClusterID.Z; z++ {
				clusterID := ClusterID{X: x, Y: y, Z: z}
				hpa.builder.BuildCluster(clusterID)
				clustersBuilt++
			}
		}
	}

	log.Printf("[HPA*] Built %d clusters", clustersBuilt)
}

// GetBuilder returns the HPA builder for external access
func (hpa *hpaPathFinder) GetBuilder() *HPABuilder {
	return hpa.builder
}

// SetDebugVisualizer sets the debug visualizer for this pathfinder
func (hpa *hpaPathFinder) SetDebugVisualizer(viz *HPADebugVisualizer) {
	hpa.debugViz = viz
}
