package pathfinding_test

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// TestHPABuilder_WaterOnlyBoundaryStillProducesEntrances is
// WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 4: before this fix, GetStandingSurfaceHeight is always
// 0 for water (no collision), so a cluster boundary face that's entirely water produced zero
// entrances - a real hole in the abstract graph forcing every cross-cluster route through it back to
// the slow, un-hierarchical low-level pathfinder. This test builds two clusters connected ONLY by a
// water strip (no dry land connection anywhere along the shared boundary) and confirms entrances are
// still found.
func TestHPABuilder_WaterOnlyBoundaryStillProducesEntrances(t *testing.T) {
	registry := mctesting.NewSimpleBlockRegistry()
	stoneID := registry.GetStateID("minecraft:stone", nil)
	waterID := registry.GetStateID("minecraft:water", nil)
	const clusterSize = 16

	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(0, 0, 31, 15, 63, stoneID). // Solid floor under both clusters and the boundary
		WaterDirect(14, 64, 0, 17, 64, 15, waterID). // Water strip straddling the X=15/16 cluster boundary, full Z range
		Build()

	shapeMgr := mctesting.NewMockShapeManager()
	lowLevel := pathfinding.NewAStarPathFinder(world, shapeMgr, nil)
	builder := pathfinding.NewHPABuilder(world, shapeMgr, lowLevel, clusterSize, nil)

	westClusterID := builder.GetClusterManager().GetClusterID(models.V3{X: 5, Y: 64, Z: 5})
	eastClusterID := builder.GetClusterManager().GetClusterID(models.V3{X: 25, Y: 64, Z: 5})
	if westClusterID == eastClusterID {
		t.Fatalf("test setup bug: expected west/east positions to fall in different clusters, both got %s", westClusterID)
	}

	cluster := builder.BuildCluster(westClusterID)

	if len(cluster.Entrances) == 0 {
		t.Fatal("expected at least one entrance across a water-only cluster boundary, got none")
	}

	foundCrossClusterWaterEntrance := false
	for _, entrance := range cluster.Entrances {
		other := entrance.GetOtherCluster(westClusterID)
		if other == eastClusterID && entrance.Pos1.Y == 64 && entrance.Pos2.Y == 64 {
			foundCrossClusterWaterEntrance = true
			break
		}
	}
	if !foundCrossClusterWaterEntrance {
		t.Errorf("expected an entrance connecting %s <-> %s at water level (Y=64), got: %v",
			westClusterID, eastClusterID, cluster.Entrances)
	}
}
