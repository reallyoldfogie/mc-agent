package pathfinding

import (
	"math"
	"slices"

	"github.com/reallyoldfogie/mc-agent/models"
)

// EntranceGroup represents a group of contiguous walkable positions on a cluster boundary
type EntranceGroup struct {
	// Representative position (center or first position)
	RepPos models.V3
	// All positions in this group
	Positions []models.V3
	// Cluster IDs this entrance connects
	Cluster1, Cluster2 ClusterID
}

// groupEntrances groups contiguous entrance positions into single entrances
// This reduces the number of entrances significantly (e.g. 40 -> 10)
func groupEntrances(entrances []*Entrance) []*Entrance {
	if len(entrances) == 0 {
		return entrances
	}

	type entranceKey struct {
		pos1, pos2 models.V3
		c1, c2     ClusterID
	}

	deduped := make([]*Entrance, 0, len(entrances))
	seen := make(map[entranceKey]struct{}, len(entrances))
	for _, entrance := range entrances {
		key := entranceKey{
			pos1: entrance.Pos1,
			pos2: entrance.Pos2,
			c1:   entrance.Cluster1,
			c2:   entrance.Cluster2,
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, entrance)
	}

	// Group by cluster pair and direction
	type groupKey struct {
		c1, c2 ClusterID
		// Use Pos2-Pos1 as direction indicator
		dx, dy, dz float64
	}

	groups := make(map[groupKey][]*Entrance)

	for _, entrance := range deduped {
		dx := entrance.Pos2.X - entrance.Pos1.X
		dy := entrance.Pos2.Y - entrance.Pos1.Y
		dz := entrance.Pos2.Z - entrance.Pos1.Z

		key := groupKey{
			c1: entrance.Cluster1,
			c2: entrance.Cluster2,
			dx: dx, dy: dy, dz: dz,
		}
		groups[key] = append(groups[key], entrance)
	}

	// For each group, find contiguous regions
	result := make([]*Entrance, 0)
	for _, group := range groups {
		contiguousGroups := findContiguousRegions(group)
		result = append(result, contiguousGroups...)
	}

	return result
}

// unionFind implements Union-Find data structure for efficient connected component detection
type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(size int) *unionFind {
	uf := &unionFind{
		parent: make([]int, size),
		rank:   make([]int, size),
	}
	for i := range size {
		uf.parent[i] = i
	}
	return uf
}

func (uf *unionFind) find(x int) int {
	if uf.parent[x] != x {
		uf.parent[x] = uf.find(uf.parent[x]) // Path compression
	}
	return uf.parent[x]
}

func (uf *unionFind) union(x, y int) {
	rootX, rootY := uf.find(x), uf.find(y)
	if rootX == rootY {
		return
	}
	// Union by rank
	if uf.rank[rootX] < uf.rank[rootY] {
		uf.parent[rootX] = rootY
	} else if uf.rank[rootX] > uf.rank[rootY] {
		uf.parent[rootY] = rootX
	} else {
		uf.parent[rootY] = rootX
		uf.rank[rootX]++
	}
}

// findContiguousRegions splits entrances into contiguous groups and returns representative entrances
func findContiguousRegions(entrances []*Entrance) []*Entrance {
	if len(entrances) == 0 {
		return nil
	}
	if len(entrances) == 1 {
		return entrances
	}

	// Sort entrances to make grouped entrances deterministic (map iteration is non-deterministic, causing inconsistent entrance grouping)
	slices.SortFunc(entrances, func(a, b *Entrance) int {
		if a.Pos1.X != b.Pos1.X {
			return int(a.Pos1.X - b.Pos1.X)
		}
		if a.Pos1.Y != b.Pos1.Y {
			return int(a.Pos1.Y - b.Pos1.Y)
		}
		if a.Pos1.Z != b.Pos1.Z {
			return int(a.Pos1.Z - b.Pos1.Z)
		}
		return 0
	})

	// Build spatial grid for O(1) neighbor lookups
	spatialGrid := make(map[models.V3]int) // pos -> entrance index
	for i, entrance := range entrances {
		spatialGrid[entrance.Pos1] = i
	}

	// Use Union-Find to build connected components
	uf := newUnionFind(len(entrances))

	// Determine boundary type from first entrance to optimize adjacency checks
	var isHorizontalBoundary bool
	if len(entrances) > 0 {
		e := entrances[0]
		dirX := math.Abs(e.Pos2.X - e.Pos1.X)
		dirY := math.Abs(e.Pos2.Y - e.Pos1.Y)
		dirZ := math.Abs(e.Pos2.Z - e.Pos1.Z)
		// Horizontal boundary crosses in X or Z direction (not Y)
		isHorizontalBoundary = (dirX > 0.5 || dirZ > 0.5) && dirY < 0.5
	}

	// Connect adjacent entrances
	for i, entrance := range entrances {
		// Check 6 cardinal directions
		adjacents := []models.V3{
			{X: 1, Y: 0, Z: 0}, {X: -1, Y: 0, Z: 0},
			{X: 0, Y: 1, Z: 0}, {X: 0, Y: -1, Z: 0},
			{X: 0, Y: 0, Z: 1}, {X: 0, Y: 0, Z: -1},
		}

		for _, offset := range adjacents {
			adjacentPos := models.V3{
				X: entrance.Pos1.X + offset.X,
				Y: entrance.Pos1.Y + offset.Y,
				Z: entrance.Pos1.Z + offset.Z,
			}

			if j, exists := spatialGrid[adjacentPos]; exists {
				uf.union(i, j)
				continue
			}

			// For horizontal boundaries, also check with Y offset up to ±1.5
			// This handles stairs/slabs at different heights
			if isHorizontalBoundary && math.Abs(offset.Y) < 0.01 {
				// Check same XZ but different Y heights
				for yOffset := -1.5; yOffset <= 1.5; yOffset += 0.5 {
					if math.Abs(yOffset) < 0.01 {
						continue // Already checked exact Y
					}
					testPos := models.V3{
						X: entrance.Pos1.X + offset.X,
						Y: entrance.Pos1.Y + yOffset,
						Z: entrance.Pos1.Z + offset.Z,
					}
					if j, exists := spatialGrid[testPos]; exists {
						uf.union(i, j)
					}
				}
			}
		}
	}

	// Group entrances by their root component
	components := make(map[int][]*Entrance)
	for i, entrance := range entrances {
		root := uf.find(i)
		components[root] = append(components[root], entrance)
	}

	// Create representative entrance for each component
	var result []*Entrance
	for _, component := range components {
		// Calculate centroid of the component
		var sumX, sumY, sumZ float64
		for _, e := range component {
			sumX += e.Pos1.X
			sumY += e.Pos1.Y
			sumZ += e.Pos1.Z
		}
		centroid := models.V3{
			X: sumX / float64(len(component)),
			Y: sumY / float64(len(component)),
			Z: sumZ / float64(len(component)),
		}

		// Find entrance closest to centroid as representative
		var representative *Entrance
		minDist := math.MaxFloat64
		for _, e := range component {
			dist := e.Pos1.DistanceTo(centroid)
			if dist < minDist {
				minDist = dist
				representative = e
			}
		}

		// Store all grouped positions for both clusters
		// This allows insertNode to try all positions and find the closest one
		pos1List := make([]models.V3, 0, len(component))
		pos2List := make([]models.V3, 0, len(component))
		for _, e := range component {
			pos1List = append(pos1List, e.Pos1)
			pos2List = append(pos2List, e.Pos2)
		}
		representative.GroupedPositions = [2][]models.V3{pos1List, pos2List}

		result = append(result, representative)
	}

	return result
}

// areAdjacent checks if two positions are adjacent (share an edge, NOT diagonals)
func areAdjacent(p1, p2 models.V3) bool {
	dx := abs(p1.X - p2.X)
	dy := abs(p1.Y - p2.Y)
	dz := abs(p1.Z - p2.Z)

	// Only group if they share an edge (Manhattan distance = 1)
	// This prevents over-aggressive grouping that loses vertical connections
	return (dx + dy + dz) == 1
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
