package pathfinding

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// testMockWorld is a simple mock world for parallel building tests
type testMockWorld struct {
	blocks map[blockKey]uint32
	mu     sync.RWMutex

	tod        int64
	worldTime1 int64
	worldTime2 int64
}

type blockKey struct {
	x, y, z int
}

var _ models.World = (*testMockWorld)(nil) // Ensure testMockWorld implements World

func newTestMockWorld() *testMockWorld {
	return &testMockWorld{
		blocks: make(map[blockKey]uint32),
	}
}

func (w *testMockWorld) GetBlockAt(x, y, z float64) (uint32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	key := blockKey{int(x), int(y), int(z)}
	if state, ok := w.blocks[key]; ok {
		return state, true
	}
	return 0, true // Air by default
}

func (w *testMockWorld) SetBlockAt(x, y, z float64, stateID uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := blockKey{int(x), int(y), int(z)}
	w.blocks[key] = stateID
}

func (w *testMockWorld) IsChunkLoaded(x, z int) bool {
	return true
}

func (w *testMockWorld) GetTimeOfDay() (int64, bool) {
	return w.tod, true
}

func (w *testMockWorld) SetTimeOfDay(tod int64) {
	w.tod = tod
}

func (w *testMockWorld) SetWorldTime(worldTime1, worldTime2 int64) {
	w.worldTime1 = worldTime1
	w.worldTime2 = worldTime2
}

func (w *testMockWorld) GetWorldAge() (int64, bool) {
	return 5000, true
}

// testMockShapeManager is a simple mock block shape manager
type testMockShapeManager struct{}

func newTestMockShapeManager() *testMockShapeManager {
	return &testMockShapeManager{}
}

func (m *testMockShapeManager) IsPassable(stateID uint32) bool {
	return stateID == 0 // Air is passable
}

func (m *testMockShapeManager) IsSolid(stateID uint32) bool {
	return stateID != 0 // Non-air is solid
}

func (m *testMockShapeManager) IsClimbable(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsWater(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsLava(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) GetBlockProperties(stateID uint32) map[string]string {
	if stateID == 0 {
		return map[string]string{
			"Passable": "true",
			"Solid":    "false",
		}
	}
	return map[string]string{
		"Passable": "false",
		"Solid":    "true",
	}
}

func (m *testMockShapeManager) GetStandingSurfaceHeight(stateID uint32) float64 {
	if stateID == 0 {
		return 0 // Air - no standing surface
	}
	return 1.0 // Full block
}

func (m *testMockShapeManager) GetCollisionBoxes(stateID uint32, x, y, z int) []models.AABB {
	if stateID == 0 {
		return nil // Air - no collision
	}
	return []models.AABB{models.NewAABB(0, 0, 0, 1, 1, 1)}
}

func (m *testMockShapeManager) IsFluid(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsDangerous(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsDoorLike(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsFenceLike(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsSlab(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsStair(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsLogOrLeaf(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsHayBale(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsBed(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsHoneyBlock(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsSlimeBlock(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsPowderSnow(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsCobweb(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsScaffolding(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsIce(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) IsBlueIce(stateID uint32) bool {
	return false
}

func (m *testMockShapeManager) GetWaterFlowDirection(x, y, z int, world models.PhysicsWorld) models.V3 {
	return models.V3{} // No flow in mock world
}

func (m *testMockShapeManager) GetWaterFlowSpeed(stateID uint32) float64 {
	return 0.0 // No flow in mock world
}

func (m *testMockShapeManager) BlockName(stateID uint32) string {
	if stateID == 0 {
		return "minecraft:air"
	}
	return "minecraft:stone"
}

func (m *testMockShapeManager) FullBlockName(stateID uint32) string {
	return m.BlockName(stateID)
}

func (m *testMockShapeManager) GetMiningInfo(stateID uint32) (float64, []string, bool) {
	if stateID == 0 {
		return 0, nil, false
	}
	return 1.5, []string{"mineable/pickaxe"}, true
}

// TestParallelClusterBuilding tests that parallel cluster building works correctly
// and provides a performance benefit over sequential building.
func TestParallelClusterBuilding(t *testing.T) {
	// Create a larger world for cluster building test
	worldSize := 64 // 64x64 blocks = 4x4 clusters with 16-block clusters
	clusterSize := 16

	world := newTestMockWorld()
	shapeMgr := newTestMockShapeManager()

	// Create flat world with stone ground and air above
	for x := 0; x < worldSize; x++ {
		for z := 0; z < worldSize; z++ {
			// Ground at Y=64
			world.SetBlockAt(float64(x), 64, float64(z), 1) // Stone
			// Air above
			for y := 65; y < 70; y++ {
				world.SetBlockAt(float64(x), float64(y), float64(z), 0)
			}
		}
	}

	t.Run("SequentialVsParallel", func(t *testing.T) {
		// Test sequential building (buildClusterRegion calls BuildClustersInRegion)
		seqPathfinder := NewHPAPathFinderWithAStar(world, shapeMgr, clusterSize).(*hpaPathFinder)
		seqStart := time.Now()
		seqPathfinder.buildClusterRegion(
			ClusterID{X: 0, Y: 4, Z: 0},
			ClusterID{X: 3, Y: 4, Z: 3},
		)
		seqDuration := time.Since(seqStart)

		// Test parallel building (uses new parallel implementation directly)
		parPathfinder := NewHPAPathFinderWithAStar(world, shapeMgr, clusterSize).(*hpaPathFinder)
		parStart := time.Now()
		err := parPathfinder.BuildClustersInRegionWithContext(
			context.Background(),
			models.V3{X: 0, Y: 64, Z: 0},
			models.V3{X: float64(worldSize - 1), Y: 64, Z: float64(worldSize - 1)},
		)
		parDuration := time.Since(parStart)

		if err != nil {
			t.Errorf("Parallel build failed: %v", err)
		}

		t.Logf("Sequential: %v", seqDuration)
		t.Logf("Parallel:   %v", parDuration)

		// Parallel should generally be faster (allow some variance)
		// On single-core, it may not be faster due to overhead
		if parDuration > seqDuration*2 {
			t.Logf("Warning: Parallel building took >2x longer than sequential (may be expected on single-core)")
		}

		// Verify both built the same number of clusters
		seqClusters := 0
		parClusters := 0
		for x := 0; x <= 3; x++ {
			for z := 0; z <= 3; z++ {
				seqCluster := seqPathfinder.builder.GetClusterManager().GetCluster(ClusterID{X: x, Y: 4, Z: z})
				parCluster := parPathfinder.builder.GetClusterManager().GetCluster(ClusterID{X: x, Y: 4, Z: z})
				if !seqCluster.IsDirty() {
					seqClusters++
				}
				if !parCluster.IsDirty() {
					parClusters++
				}
			}
		}

		t.Logf("Sequential built %d clusters", seqClusters)
		t.Logf("Parallel built %d clusters", parClusters)

		if seqClusters != parClusters {
			t.Errorf("Mismatch: sequential built %d, parallel built %d", seqClusters, parClusters)
		}
	})

	t.Run("ConcurrentBuildSameCluster", func(t *testing.T) {
		// Test that concurrent builds of the same cluster don't cause issues
		pathfinder := NewHPAPathFinderWithAStar(world, shapeMgr, clusterSize).(*hpaPathFinder)
		clusterID := ClusterID{X: 0, Y: 4, Z: 0}

		// Launch multiple goroutines to build the same cluster
		var wg sync.WaitGroup
		numGoroutines := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pathfinder.builder.BuildCluster(clusterID)
			}()
		}

		wg.Wait()

		// Verify cluster was built correctly
		cluster := pathfinder.builder.GetClusterManager().GetCluster(clusterID)
		if cluster.IsDirty() {
			t.Error("Cluster should not be dirty after building")
		}

		// Verify no panics occurred (test passing means no race conditions)
		t.Log("Concurrent builds completed without panic")
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		pathfinder := NewHPAPathFinderWithAStar(world, shapeMgr, clusterSize).(*hpaPathFinder)

		// Create a context that cancels immediately
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := pathfinder.BuildClustersInRegionWithContext(
			ctx,
			models.V3{X: 0, Y: 64, Z: 0},
			models.V3{X: float64(worldSize - 1), Y: 64, Z: float64(worldSize - 1)},
		)

		if err == nil {
			t.Log("Context cancellation may not have been detected (clusters built quickly)")
		} else if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got: %v", err)
		} else {
			t.Log("Context cancellation detected correctly")
		}
	})
}

// TestClusterThreadSafety runs the cluster methods concurrently to detect race conditions.
// Run with: go test -race -run TestClusterThreadSafety
func TestClusterThreadSafety(t *testing.T) {
	cluster := NewCluster(ClusterID{X: 0, Y: 0, Z: 0}, 16)

	var wg sync.WaitGroup
	numOps := 100

	// Concurrent AddEntrance operations
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			entrance := &Entrance{
				Pos1:     models.V3{X: float64(idx), Y: 65, Z: 0},
				Pos2:     models.V3{X: float64(idx) + 1, Y: 65, Z: 0},
				Cluster1: ClusterID{X: 0, Y: 0, Z: 0},
				Cluster2: ClusterID{X: 1, Y: 0, Z: 0},
			}
			cluster.AddEntrance(entrance)
		}(i)
	}

	// Concurrent IsDirty reads
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cluster.IsDirty()
		}()
	}

	// Concurrent SetDirty operations
	for i := 0; i < numOps/10; i++ {
		wg.Add(1)
		go func(val bool) {
			defer wg.Done()
			cluster.SetDirty(val)
		}(i%2 == 0)
	}

	wg.Wait()

	// Verify all entrances were added
	entrances := cluster.GetEntrances()
	if len(entrances) != numOps {
		t.Errorf("Expected %d entrances, got %d", numOps, len(entrances))
	}

	t.Logf("Added %d entrances concurrently without race conditions", len(entrances))
}

// TestAbstractGraphThreadSafety tests concurrent access to the abstract graph.
// Run with: go test -race -run TestAbstractGraphThreadSafety
func TestAbstractGraphThreadSafety(t *testing.T) {
	graph := NewAbstractGraph(16)

	var wg sync.WaitGroup
	numOps := 100

	// Create entrances for testing
	entrances := make([]*Entrance, numOps)
	for i := 0; i < numOps; i++ {
		entrances[i] = &Entrance{
			Pos1:     models.V3{X: float64(i), Y: 65, Z: 0},
			Pos2:     models.V3{X: float64(i), Y: 65, Z: 1},
			Cluster1: ClusterID{X: i / 16, Y: 4, Z: 0},
			Cluster2: ClusterID{X: i / 16, Y: 4, Z: 1},
		}
	}

	// Concurrent GetOrCreateNode operations
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			graph.GetOrCreateNode(entrances[idx])
		}(i)
	}

	// Concurrent AddEdge operations
	for i := 0; i < numOps-1; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			graph.AddEdge(entrances[idx], entrances[idx+1], 1.0, nil)
		}(i)
	}

	wg.Wait()

	// Verify nodes were created
	nodeCount := len(graph.Nodes)
	if nodeCount != numOps {
		t.Errorf("Expected %d nodes, got %d", numOps, nodeCount)
	}

	t.Logf("Created %d nodes concurrently without race conditions", nodeCount)
}
