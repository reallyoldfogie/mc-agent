package pathfinding

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
)

func TestClusterIDCalculation(t *testing.T) {
	clusterSize := 10
	cm := NewClusterManager(clusterSize)

	tests := []struct {
		name       string
		pos        models.V3
		expectedID ClusterID
	}{
		{
			name:       "origin",
			pos:        models.V3{X: 0, Y: 0, Z: 0},
			expectedID: ClusterID{X: 0, Y: 0, Z: 0},
		},
		{
			name:       "positive coords within first cluster",
			pos:        models.V3{X: 5, Y: 5, Z: 5},
			expectedID: ClusterID{X: 0, Y: 0, Z: 0},
		},
		{
			name:       "boundary - just inside first cluster",
			pos:        models.V3{X: 9, Y: 9, Z: 9},
			expectedID: ClusterID{X: 0, Y: 0, Z: 0},
		},
		{
			name:       "boundary - at edge of first cluster",
			pos:        models.V3{X: 10, Y: 10, Z: 10},
			expectedID: ClusterID{X: 1, Y: 1, Z: 1},
		},
		{
			name:       "negative coords",
			pos:        models.V3{X: -5, Y: -5, Z: -5},
			expectedID: ClusterID{X: -1, Y: -1, Z: -1},
		},
		{
			name:       "mixed pos/neg coords",
			pos:        models.V3{X: 15, Y: -5, Z: 25},
			expectedID: ClusterID{X: 1, Y: -1, Z: 2},
		},
		{
			name:       "problematic case - agent position",
			pos:        models.V3{X: -2.5, Y: 0, Z: -3.5},
			expectedID: ClusterID{X: -1, Y: 0, Z: -1},
		},
		{
			name:       "problematic case - goal position",
			pos:        models.V3{X: 17.5, Y: 5, Z: -3.5},
			expectedID: ClusterID{X: 1, Y: 0, Z: -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualID := cm.GetClusterID(tt.pos)
			assert.Equal(t, tt.expectedID, actualID,
				"ClusterID for position (%.1f,%.1f,%.1f) should be %v, got %v",
				tt.pos.X, tt.pos.Y, tt.pos.Z, tt.expectedID, actualID)
		})
	}
}

func TestClusterBounds(t *testing.T) {
	clusterSize := 10
	id := ClusterID{X: 0, Y: 0, Z: 0}
	cluster := NewCluster(id, clusterSize)

	tests := []struct {
		name     string
		pos      models.V3
		expected bool
	}{
		{
			name:     "inside - origin",
			pos:      models.V3{X: 0, Y: 0, Z: 0},
			expected: true,
		},
		{
			name:     "inside - center",
			pos:      models.V3{X: 5, Y: 5, Z: 5},
			expected: true,
		},
		{
			name:     "inside - near boundary",
			pos:      models.V3{X: 9, Y: 9, Z: 9},
			expected: true,
		},
		{
			name:     "outside - at max boundary",
			pos:      models.V3{X: 10, Y: 10, Z: 10},
			expected: false,
		},
		{
			name:     "outside - negative",
			pos:      models.V3{X: -1, Y: -1, Z: -1},
			expected: false,
		},
		{
			name:     "outside - one axis out",
			pos:      models.V3{X: 5, Y: 5, Z: 10},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := cluster.Bounds.Contains(tt.pos)
			assert.Equal(t, tt.expected, actual,
				"Position (%.1f,%.1f,%.1f) in cluster %s should be %v, got %v",
				tt.pos.X, tt.pos.Y, tt.pos.Z, cluster.Bounds.String(), tt.expected, actual)
		})
	}
}

func TestAdjacentClusterID(t *testing.T) {
	cm := NewClusterManager(10)
	baseID := ClusterID{X: 0, Y: 0, Z: 0}

	tests := []struct {
		name      string
		direction Direction
		expected  ClusterID
	}{
		{
			name:      "North",
			direction: North,
			expected:  ClusterID{X: 0, Y: 0, Z: -1},
		},
		{
			name:      "South",
			direction: South,
			expected:  ClusterID{X: 0, Y: 0, Z: 1},
		},
		{
			name:      "East",
			direction: East,
			expected:  ClusterID{X: 1, Y: 0, Z: 0},
		},
		{
			name:      "West",
			direction: West,
			expected:  ClusterID{X: -1, Y: 0, Z: 0},
		},
		{
			name:      "Up",
			direction: Up,
			expected:  ClusterID{X: 0, Y: 1, Z: 0},
		},
		{
			name:      "Down",
			direction: Down,
			expected:  ClusterID{X: 0, Y: -1, Z: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := cm.GetAdjacentClusterID(baseID, tt.direction)
			assert.Equal(t, tt.expected, actual,
				"Adjacent cluster in direction %s should be %v, got %v",
				tt.direction, tt.expected, actual)
		})
	}
}

func TestEntranceGetMethods(t *testing.T) {
	cluster1 := ClusterID{X: 0, Y: 0, Z: 0}
	cluster2 := ClusterID{X: 1, Y: 0, Z: 0}

	entrance := &Entrance{
		Pos1:     models.V3{X: 9, Y: 5, Z: 5},
		Pos2:     models.V3{X: 10, Y: 5, Z: 5},
		Cluster1: cluster1,
		Cluster2: cluster2,
	}

	// Test GetPosInCluster
	pos1 := entrance.GetPosInCluster(cluster1)
	assert.Equal(t, entrance.Pos1, pos1, "Should return Pos1 for Cluster1")

	pos2 := entrance.GetPosInCluster(cluster2)
	assert.Equal(t, entrance.Pos2, pos2, "Should return Pos2 for Cluster2")

	// Test GetOtherCluster
	other1 := entrance.GetOtherCluster(cluster1)
	assert.Equal(t, cluster2, other1, "Should return Cluster2 when given Cluster1")

	other2 := entrance.GetOtherCluster(cluster2)
	assert.Equal(t, cluster1, other2, "Should return Cluster1 when given Cluster2")
}
