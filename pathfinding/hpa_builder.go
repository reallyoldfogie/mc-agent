package pathfinding

import (
	"context"
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// HPABuilder builds the hierarchical pathfinding structures
type HPABuilder struct {
	world              World
	shapeMgr           BlockShapeManager
	lowLevelPathfinder PathFinder
	clusterManager     *ClusterManager
	abstractGraph      *AbstractGraph
	movementValidator  *MovementValidator
	DebugViz           *HPADebugVisualizer
}

// NewHPABuilder creates a new HPA* builder
func NewHPABuilder(world World, shapeMgr BlockShapeManager, lowLevelPathfinder PathFinder, clusterSize int) *HPABuilder {
	return &HPABuilder{
		world:              world,
		shapeMgr:           shapeMgr,
		lowLevelPathfinder: lowLevelPathfinder,
		clusterManager:     NewClusterManager(clusterSize),
		abstractGraph:      NewAbstractGraph(clusterSize),
		movementValidator:  NewMovementValidator(world, shapeMgr),
	}
}

// BuildCluster builds a single cluster: finds entrances and computes internal paths
func (b *HPABuilder) BuildCluster(clusterID ClusterID) *Cluster {
	cluster := b.clusterManager.GetCluster(clusterID)

	if !cluster.Dirty {
		// Cluster is already built
		return cluster
	}

	log.Printf("[HPABuilder] Building cluster %s", cluster.String())

	// Clear old data
	cluster.Clear()

	// Phase 1: Find entrances on all 6 faces
	entrances := b.findEntrances(cluster)

	// Group contiguous entrances to reduce entrance count
	groupedEntrances := groupEntrances(entrances)
	for _, entrance := range groupedEntrances {
		cluster.AddEntrance(entrance)
	}

	log.Printf("[HPABuilder] Found %d entrances (%d grouped from %d) for %s",
		len(groupedEntrances), len(groupedEntrances), len(entrances), cluster.ID.String())

	// Log entrance positions for debugging
	for i, entrance := range groupedEntrances {
		pos1 := entrance.GetPosInCluster(cluster.ID)
		otherCluster := entrance.GetOtherCluster(cluster.ID)
		log.Printf("[HPABuilder]   Entrance %d/%d: pos=(%.0f, %.0f, %.0f) connects to %s",
			i+1, len(groupedEntrances), pos1.X, pos1.Y, pos1.Z, otherCluster.String())
	}

	// Visualize entrances
	if b.DebugViz != nil && b.DebugViz.IsEnabled() {
		b.DebugViz.VisualizeEntrances(context.Background(), groupedEntrances)
	}

	// Phase 2: Compute internal paths between entrances
	b.computeInternalPaths(cluster)

	log.Printf("[HPABuilder] Computed %d internal paths for %s", len(cluster.InternalPaths), cluster.ID.String())

	// Phase 3: Add edges to abstract graph
	b.addClusterToAbstractGraph(cluster)

	cluster.Dirty = false
	return cluster
}

// findEntrances scans all 6 faces of a cluster to find entrance points
func (b *HPABuilder) findEntrances(cluster *Cluster) []*Entrance {
	var entrances []*Entrance

	// Check all 6 directions
	directions := []Direction{North, South, East, West, Up, Down}
	for _, direction := range directions {
		faceEntrances := b.scanBoundaryFace(cluster, direction)
		entrances = append(entrances, faceEntrances...)
	}

	return entrances
}

// scanBoundaryFace scans one face of a cluster boundary for walkable connections
func (b *HPABuilder) scanBoundaryFace(cluster *Cluster, direction Direction) []*Entrance {
	var entrances []*Entrance

	// Get the adjacent cluster ID
	adjacentClusterID := b.clusterManager.GetAdjacentClusterID(cluster.ID, direction)

	// Determine which face to scan based on direction
	switch direction {
	case North: // -Z face
		entrances = b.scanZFace(cluster, adjacentClusterID, cluster.Bounds.MinZ, direction)
	case South: // +Z face
		entrances = b.scanZFace(cluster, adjacentClusterID, cluster.Bounds.MaxZ-1, direction)
	case East: // +X face
		entrances = b.scanXFace(cluster, adjacentClusterID, cluster.Bounds.MaxX-1, direction)
	case West: // -X face
		entrances = b.scanXFace(cluster, adjacentClusterID, cluster.Bounds.MinX, direction)
	case Up: // +Y face
		entrances = b.scanYFace(cluster, adjacentClusterID, cluster.Bounds.MaxY-1, direction)
	case Down: // -Y face
		entrances = b.scanYFace(cluster, adjacentClusterID, cluster.Bounds.MinY, direction)
	}

	return entrances
}

// scanXFace scans an X-aligned face (East or West)
func (b *HPABuilder) scanXFace(cluster *Cluster, adjacentClusterID ClusterID, x float64, direction Direction) []*Entrance {
	var entrances []*Entrance

	// Scan the YZ plane at this X coordinate
	for yInt := int(cluster.Bounds.MinY); yInt < int(cluster.Bounds.MaxY); yInt++ {
		for z := cluster.Bounds.MinZ; z < cluster.Bounds.MaxZ; z++ {
			// Check both sides of the boundary for standing surfaces
			// Side 1 (current cluster)
			blockPos1 := models.V3{X: x, Y: float64(yInt), Z: z}
			blockStateID1, loaded := b.world.GetBlockAt(blockPos1.X, blockPos1.Y, blockPos1.Z)
			if !loaded {
				continue
			}

			surfaceHeight1 := b.shapeMgr.GetStandingSurfaceHeight(blockStateID1)

			// Side 2 (adjacent cluster)
			var blockPos2 models.V3
			if direction == East {
				blockPos2 = models.V3{X: x + 1, Y: float64(yInt), Z: z}
			} else { // West
				blockPos2 = models.V3{X: x - 1, Y: float64(yInt), Z: z}
			}

			blockStateID2, loaded2 := b.world.GetBlockAt(blockPos2.X, blockPos1.Y, blockPos1.Z)
			if !loaded2 {
				continue
			}
			surfaceHeight2 := b.shapeMgr.GetStandingSurfaceHeight(blockStateID2)

			// Try entrance at standing height on side 1
			if surfaceHeight1 > 0 {
				standingY1 := float64(yInt) + surfaceHeight1
				pos1 := models.V3{X: x, Y: standingY1, Z: z}
				pos2 := models.V3{X: blockPos2.X, Y: standingY1, Z: z}

				if b.isConnectedAcrossBoundary(pos1, pos2) {
					entrance := &Entrance{
						Pos1:     pos1,
						Pos2:     pos2,
						Cluster1: cluster.ID,
						Cluster2: adjacentClusterID,
					}
					entrances = append(entrances, entrance)
				}
			}

			// Try entrance at standing height on side 2
			if surfaceHeight2 > 0 && surfaceHeight2 != surfaceHeight1 {
				standingY2 := float64(yInt) + surfaceHeight2
				pos1 := models.V3{X: x, Y: standingY2, Z: z}
				pos2 := models.V3{X: blockPos2.X, Y: standingY2, Z: z}

				if b.isConnectedAcrossBoundary(pos1, pos2) {
					entrance := &Entrance{
						Pos1:     pos1,
						Pos2:     pos2,
						Cluster1: cluster.ID,
						Cluster2: adjacentClusterID,
					}
					entrances = append(entrances, entrance)
				}
			}
		}
	}

	return entrances
}

// scanYFace scans a Y-aligned face (Up or Down)
func (b *HPABuilder) scanYFace(cluster *Cluster, adjacentClusterID ClusterID, y float64, direction Direction) []*Entrance {
	var entrances []*Entrance

	// Scan the XZ plane at this Y coordinate
	yInt := int(y)
	for x := cluster.Bounds.MinX; x < cluster.Bounds.MaxX; x++ {
		for z := cluster.Bounds.MinZ; z < cluster.Bounds.MaxZ; z++ {
			// Get the block at this Y level and check for standing surface
			blockPos := models.V3{X: x, Y: float64(yInt), Z: z}
			blockStateID, loaded := b.world.GetBlockAt(blockPos.X, blockPos.Y, blockPos.Z)
			if !loaded {
				continue
			}

			surfaceHeight := b.shapeMgr.GetStandingSurfaceHeight(blockStateID)

			// If there's a standing surface here, check for vertical connections
			if surfaceHeight > 0 {
				standingY := float64(yInt) + surfaceHeight
				pos1 := models.V3{X: x, Y: standingY, Z: z}

				// Calculate adjacent position (above or below)
				var pos2 models.V3
				if direction == Up {
					pos2 = models.V3{X: x, Y: standingY + 1, Z: z}
				} else { // Down
					pos2 = models.V3{X: x, Y: standingY - 1, Z: z}
				}

				// Check if the boundary positions are connected by a legal move
				if b.isConnectedAcrossBoundary(pos1, pos2) {
					entrance := &Entrance{
						Pos1:     pos1,
						Pos2:     pos2,
						Cluster1: cluster.ID,
						Cluster2: adjacentClusterID,
					}
					entrances = append(entrances, entrance)
				}
			}
		}
	}

	return entrances
}

// scanZFace scans a Z-aligned face (North or South)
func (b *HPABuilder) scanZFace(cluster *Cluster, adjacentClusterID ClusterID, z float64, direction Direction) []*Entrance {
	var entrances []*Entrance

	// Scan the XY plane at this Z coordinate
	for x := cluster.Bounds.MinX; x < cluster.Bounds.MaxX; x++ {
		for yInt := int(cluster.Bounds.MinY); yInt < int(cluster.Bounds.MaxY); yInt++ {
			// Check both sides of the boundary for standing surfaces
			// Side 1 (current cluster)
			blockPos1 := models.V3{X: x, Y: float64(yInt), Z: z}
			blockStateID1, loaded := b.world.GetBlockAt(blockPos1.X, blockPos1.Y, blockPos1.Z)
			if !loaded {
				continue
			}

			surfaceHeight1 := b.shapeMgr.GetStandingSurfaceHeight(blockStateID1)

			// Side 2 (adjacent cluster)
			var blockPos2 models.V3
			if direction == South {
				blockPos2 = models.V3{X: x, Y: float64(yInt), Z: z + 1}
			} else { // North
				blockPos2 = models.V3{X: x, Y: float64(yInt), Z: z - 1}
			}
			blockStateID2, loaded2 := b.world.GetBlockAt(blockPos2.X, blockPos1.Y, blockPos1.Z)
			if !loaded2 {
				continue
			}

			surfaceHeight2 := b.shapeMgr.GetStandingSurfaceHeight(blockStateID2)

			// Try entrance at standing height on side 1
			if surfaceHeight1 > 0 {
				standingY1 := float64(yInt) + surfaceHeight1
				pos1 := models.V3{X: x, Y: standingY1, Z: z}
				pos2 := models.V3{X: x, Y: standingY1, Z: blockPos2.Z}

				if b.isConnectedAcrossBoundary(pos1, pos2) {
					entrance := &Entrance{
						Pos1:     pos1,
						Pos2:     pos2,
						Cluster1: cluster.ID,
						Cluster2: adjacentClusterID,
					}
					entrances = append(entrances, entrance)
				}
			}

			// Try entrance at standing height on side 2
			if surfaceHeight2 > 0 && surfaceHeight2 != surfaceHeight1 {
				standingY2 := float64(yInt) + surfaceHeight2
				pos1 := models.V3{X: x, Y: standingY2, Z: z}
				pos2 := models.V3{X: x, Y: standingY2, Z: blockPos2.Z}

				if b.isConnectedAcrossBoundary(pos1, pos2) {
					entrance := &Entrance{
						Pos1:     pos1,
						Pos2:     pos2,
						Cluster1: cluster.ID,
						Cluster2: adjacentClusterID,
					}
					entrances = append(entrances, entrance)
				}
			}
		}
	}

	return entrances
}

// isWalkable checks if a position is walkable (has ground support and passable space)
func (b *HPABuilder) isWalkable(pos models.V3) bool {
	// Check if position is passable (air or similar)
	if !b.movementValidator.isPositionPassable(pos) {
		return false
	}

	// Check if there's ground support
	if !b.movementValidator.hasGroundSupport(pos) {
		return false
	}

	return true
}

// isConnectedAcrossBoundary checks if a legal move exists between the two boundary positions.
// This captures stair/slab ascents that aren't both "walkable" at the same Y.
func (b *HPABuilder) isConnectedAcrossBoundary(pos1, pos2 models.V3) bool {
	if b.movementValidator == nil {
		return false
	}
	if b.hasLegalMove(pos1, pos2) {
		return true
	}
	return b.hasLegalMove(pos2, pos1)
}

func (b *HPABuilder) hasLegalMove(from, to models.V3) bool {
	// Use zero goal to avoid distance pruning.
	moves := b.movementValidator.GetPossibleMoves(from, models.V3{}, nil)
	for _, move := range moves {
		if move.Position == to {
			return true
		}
	}
	return false
}

// computeInternalPaths is now a no-op - paths are computed lazily on-demand
func (b *HPABuilder) computeInternalPaths(cluster *Cluster) {
	// Skip precomputation - paths will be computed when needed during refinePath
	// This reduces cluster build time from 30-40s to ~100ms
	log.Printf("[HPABuilder] Skipping precomputation of internal paths (lazy evaluation)")
}

// getOrComputeInternalPath returns a cached path or computes it on-demand
func (b *HPABuilder) getOrComputeInternalPath(cluster *Cluster, from, to *Entrance) *Path {
	// Check cache first
	if path := cluster.GetInternalPath(from, to); path != nil {
		return path
	}

	// Compute on-demand
	start := from.Pos1
	goal := to.Pos1

	path, err := b.lowLevelPathfinder.FindPath(start, goal, 500)
	if err != nil || !path.Found {
		return nil
	}

	// Cache bidirectional paths for future use
	reversePath := b.reversePath(path)
	cluster.SetInternalPath(from, to, path)
	cluster.SetInternalPath(to, from, reversePath)

	return path
}

// reversePath creates a reversed copy of a path
func (b *HPABuilder) reversePath(path *Path) *Path {
	if path == nil {
		return nil
	}

	reversedSteps := make([]PathStep, len(path.Steps))
	for i, step := range path.Steps {
		reversedSteps[len(path.Steps)-1-i] = step
	}

	return &Path{
		Steps:      reversedSteps,
		TotalCost:  path.TotalCost,
		StartPos:   path.GoalPos,
		GoalPos:    path.StartPos,
		Found:      path.Found,
		SearchTime: path.SearchTime,
	}
}

// pathWithinCluster checks if all steps of a path are within the cluster bounds
func (b *HPABuilder) pathWithinCluster(path *Path, cluster *Cluster) bool {
	for _, step := range path.Steps {
		if !cluster.Bounds.Contains(step.Position) {
			return false
		}
	}
	return true
}

// addClusterToAbstractGraph adds a cluster's entrances to the abstract graph
func (b *HPABuilder) addClusterToAbstractGraph(cluster *Cluster) {
	// Add edges between all entrance pairs (with nil paths for lazy computation)
	// Cost estimate: straight-line distance
	for i, from := range cluster.Entrances {
		for j := i + 1; j < len(cluster.Entrances); j++ {
			to := cluster.Entrances[j]

			// Estimate cost as Euclidean distance
			dist := from.Pos1.DistanceTo(to.Pos1)

			// Add bidirectional edges with nil path (computed lazily)
			b.abstractGraph.AddEdge(from, to, dist, nil)
			b.abstractGraph.AddEdge(to, from, dist, nil)
		}
	}

	// Add inter-cluster edges (between adjacent clusters)
	// For each entrance in this cluster
	for _, entrance := range cluster.Entrances {
		// Check if the entrance connects to an adjacent cluster
		if entrance.Cluster1 != cluster.ID && entrance.Cluster2 != cluster.ID {
			continue // This entrance doesn't belong to this cluster
		}

		// The entrance connects this cluster to an adjacent cluster
		// Build both clusters if needed
		adjacentClusterID := entrance.Cluster1
		if adjacentClusterID == cluster.ID {
			adjacentClusterID = entrance.Cluster2
		}

		// Build adjacent cluster if dirty
		adjacentCluster := b.clusterManager.GetCluster(adjacentClusterID)
		if adjacentCluster.Dirty {
			// Don't build now - will be built when needed
			continue
		}

		// Find matching entrance in adjacent cluster
		for _, adjEntrance := range adjacentCluster.Entrances {
			// Check if this is the same entrance (shared between clusters)
			if entrance.Matches(adjEntrance) {
				// Compute actual path between the two entrance positions
				// (handles stairs, ladders, etc.)
				forwardPath, err := b.lowLevelPathfinder.FindPath(entrance.Pos1, entrance.Pos2, 50)
				if err == nil && forwardPath.Found {
					// Add bidirectional edges with actual paths
					reversePath := b.reversePath(forwardPath)

					b.abstractGraph.AddEdge(entrance, adjEntrance, forwardPath.TotalCost, forwardPath)
					b.abstractGraph.AddEdge(adjEntrance, entrance, reversePath.TotalCost, reversePath)
				}
				// Note: If pathfinding fails, we skip this inter-cluster connection
				// This is correct behavior - it means the entrance isn't actually traversable
				break
			}
		}
	}
}

// GetClusterManager returns the cluster manager
func (b *HPABuilder) GetClusterManager() *ClusterManager {
	return b.clusterManager
}

// GetAbstractGraph returns the abstract graph
func (b *HPABuilder) GetAbstractGraph() *AbstractGraph {
	return b.abstractGraph
}
