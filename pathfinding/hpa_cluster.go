package pathfinding

import (
	"fmt"
	"math"

	"github.com/reallyoldfogie/mc-agent/models"
)

// ClusterID uniquely identifies a cluster in 3D space
// Coordinates represent cluster indices, not block coordinates
type ClusterID struct {
	X, Y, Z int
}

// String returns a string representation of the cluster ID
func (id ClusterID) String() string {
	return fmt.Sprintf("Cluster(%d,%d,%d)", id.X, id.Y, id.Z)
}

// Entrance represents a connection point between two adjacent clusters
// It stores positions on both sides of the cluster boundary
type Entrance struct {
	// Pos1 is the position in Cluster1
	Pos1 models.V3
	// Pos2 is the position in Cluster2 (adjacent to Pos1)
	Pos2 models.V3
	// Cluster1 and Cluster2 are the two clusters this entrance connects
	Cluster1 ClusterID
	Cluster2 ClusterID
	// GroupedPositions stores all positions that were collapsed into this entrance
	// during entrance grouping. Useful for finding the closest position to path to.
	// Index 0 = positions in Cluster1, Index 1 = positions in Cluster2
	GroupedPositions [2][]models.V3
}

// String returns a string representation of the entrance
func (e *Entrance) String() string {
	return fmt.Sprintf("Entrance[(%0.f,%0.f,%0.f)-(%0.f,%0.f,%0.f) %s<->%s]",
		e.Pos1.X, e.Pos1.Y, e.Pos1.Z,
		e.Pos2.X, e.Pos2.Y, e.Pos2.Z,
		e.Cluster1.String(), e.Cluster2.String())
}

// GetPosInCluster returns the position of this entrance in the specified cluster
func (e *Entrance) GetPosInCluster(clusterID ClusterID) models.V3 {
	if e.Cluster1 == clusterID {
		return e.Pos1
	}
	if e.Cluster2 == clusterID {
		return e.Pos2
	}
	// Default to Pos1 if cluster not found
	return e.Pos1
}

// GetOtherCluster returns the cluster ID on the other side of this entrance
func (e *Entrance) GetOtherCluster(clusterID ClusterID) ClusterID {
	if e.Cluster1 == clusterID {
		return e.Cluster2
	}
	return e.Cluster1
}

// Matches checks if two entrances represent the same connection point
// Two entrances match if they share the same positions (in either order)
func (e *Entrance) Matches(other *Entrance) bool {
	if e == other {
		return true
	}
	// Check if positions match (same or reversed)
	return (e.Pos1 == other.Pos1 && e.Pos2 == other.Pos2) ||
		(e.Pos1 == other.Pos2 && e.Pos2 == other.Pos1)
}

// EntranceKey uniquely identifies a directed path between two entrances
type EntranceKey struct {
	From *Entrance
	To   *Entrance
}

// String returns a string representation of the entrance key
func (k EntranceKey) String() string {
	return fmt.Sprintf("Path[%s->%s]", k.From.String(), k.To.String())
}

// ClusterBounds defines the spatial boundaries of a cluster
type ClusterBounds struct {
	MinX, MinY, MinZ float64
	MaxX, MaxY, MaxZ float64
}

// Contains checks if a position is within this cluster's bounds
func (b ClusterBounds) Contains(pos models.V3) bool {
	return pos.X >= b.MinX && pos.X < b.MaxX &&
		pos.Y >= b.MinY && pos.Y < b.MaxY &&
		pos.Z >= b.MinZ && pos.Z < b.MaxZ
}

// String returns a string representation of the bounds
func (b ClusterBounds) String() string {
	return fmt.Sprintf("Bounds[(%.0f,%.0f,%.0f)-(%.0f,%.0f,%.0f)]",
		b.MinX, b.MinY, b.MinZ, b.MaxX, b.MaxY, b.MaxZ)
}

// Cluster represents a subdivision of the world for hierarchical pathfinding
type Cluster struct {
	ID       ClusterID
	Bounds   ClusterBounds
	Entrances []*Entrance
	// InternalPaths stores precomputed paths between entrances within this cluster
	// Key is the entrance pair, value is the cached path
	InternalPaths map[EntranceKey]*Path
	// Dirty indicates whether this cluster needs to be rebuilt
	Dirty bool
}

// NewCluster creates a new cluster with the given ID and size
func NewCluster(id ClusterID, clusterSize int) *Cluster {
	return &Cluster{
		ID: id,
		Bounds: ClusterBounds{
			MinX: float64(id.X * clusterSize),
			MinY: float64(id.Y * clusterSize),
			MinZ: float64(id.Z * clusterSize),
			MaxX: float64((id.X + 1) * clusterSize),
			MaxY: float64((id.Y + 1) * clusterSize),
			MaxZ: float64((id.Z + 1) * clusterSize),
		},
		Entrances:     make([]*Entrance, 0),
		InternalPaths: make(map[EntranceKey]*Path),
		Dirty:         true, // New clusters need to be built
	}
}

// String returns a string representation of the cluster
func (c *Cluster) String() string {
	return fmt.Sprintf("%s %s [%d entrances, %d paths]",
		c.ID.String(), c.Bounds.String(),
		len(c.Entrances), len(c.InternalPaths))
}

// AddEntrance adds an entrance to this cluster
func (c *Cluster) AddEntrance(entrance *Entrance) {
	c.Entrances = append(c.Entrances, entrance)
}

// GetInternalPath retrieves a cached path between two entrances
func (c *Cluster) GetInternalPath(from, to *Entrance) *Path {
	key := EntranceKey{From: from, To: to}
	return c.InternalPaths[key]
}

// SetInternalPath caches a path between two entrances
func (c *Cluster) SetInternalPath(from, to *Entrance, path *Path) {
	key := EntranceKey{From: from, To: to}
	c.InternalPaths[key] = path
}

// MarkDirty marks this cluster as needing rebuilding
func (c *Cluster) MarkDirty() {
	c.Dirty = true
}

// Clear removes all entrances and cached paths from this cluster
func (c *Cluster) Clear() {
	c.Entrances = make([]*Entrance, 0)
	c.InternalPaths = make(map[EntranceKey]*Path)
	c.Dirty = true
}

// ClusterManager manages the collection of clusters
type ClusterManager struct {
	clusters    map[ClusterID]*Cluster
	clusterSize int
}

// NewClusterManager creates a new cluster manager
func NewClusterManager(clusterSize int) *ClusterManager {
	return &ClusterManager{
		clusters:    make(map[ClusterID]*Cluster),
		clusterSize: clusterSize,
	}
}

// GetClusterID returns the cluster ID for a given world position
func (cm *ClusterManager) GetClusterID(pos models.V3) ClusterID {
	return ClusterID{
		X: int(math.Floor(pos.X / float64(cm.clusterSize))),
		Y: int(math.Floor(pos.Y / float64(cm.clusterSize))),
		Z: int(math.Floor(pos.Z / float64(cm.clusterSize))),
	}
}

// GetCluster retrieves or creates a cluster for the given ID
func (cm *ClusterManager) GetCluster(id ClusterID) *Cluster {
	if cluster, exists := cm.clusters[id]; exists {
		return cluster
	}
	// Create new cluster on demand
	cluster := NewCluster(id, cm.clusterSize)
	cm.clusters[id] = cluster
	return cluster
}

// GetClusterForPos retrieves or creates a cluster for the given position
func (cm *ClusterManager) GetClusterForPos(pos models.V3) *Cluster {
	id := cm.GetClusterID(pos)
	return cm.GetCluster(id)
}

// GetAdjacentClusterID returns the cluster ID adjacent to the given cluster in the specified direction
func (cm *ClusterManager) GetAdjacentClusterID(id ClusterID, direction Direction) ClusterID {
	switch direction {
	case North:
		return ClusterID{X: id.X, Y: id.Y, Z: id.Z - 1}
	case South:
		return ClusterID{X: id.X, Y: id.Y, Z: id.Z + 1}
	case East:
		return ClusterID{X: id.X + 1, Y: id.Y, Z: id.Z}
	case West:
		return ClusterID{X: id.X - 1, Y: id.Y, Z: id.Z}
	case Up:
		return ClusterID{X: id.X, Y: id.Y + 1, Z: id.Z}
	case Down:
		return ClusterID{X: id.X, Y: id.Y - 1, Z: id.Z}
	default:
		return id
	}
}

// ClusterExists checks if a cluster exists for the given ID
func (cm *ClusterManager) ClusterExists(id ClusterID) bool {
	_, exists := cm.clusters[id]
	return exists
}

// MarkClusterDirty marks a cluster as needing rebuilding
func (cm *ClusterManager) MarkClusterDirty(id ClusterID) {
	if cluster, exists := cm.clusters[id]; exists {
		cluster.MarkDirty()
	}
}

// MarkPositionDirty marks the cluster containing the position as dirty
func (cm *ClusterManager) MarkPositionDirty(pos models.V3) {
	id := cm.GetClusterID(pos)
	cm.MarkClusterDirty(id)
}

// Direction represents the six possible directions in 3D space
type Direction int

const (
	North Direction = iota // -Z
	South                  // +Z
	East                   // +X
	West                   // -X
	Up                     // +Y
	Down                   // -Y
)

// String returns a string representation of the direction
func (d Direction) String() string {
	switch d {
	case North:
		return "North"
	case South:
		return "South"
	case East:
		return "East"
	case West:
		return "West"
	case Up:
		return "Up"
	case Down:
		return "Down"
	default:
		return "Unknown"
	}
}
