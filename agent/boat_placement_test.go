package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestIsBoatItemName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"minecraft:oak_boat", true},
		{"minecraft:dark_oak_boat", true},
		{"minecraft:bamboo_chest_raft", true},
		{"minecraft:bamboo_raft", true},
		{"minecraft:boat", true},
		{"minecraft:oak_chest_boat", true},
		{"minecraft:oak_planks", false},
		{"minecraft:water_bucket", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBoatItemName(tt.name); got != tt.want {
				t.Errorf("isBoatItemName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestFindNewNearbyBoat covers WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8: the boat-placement
// detection step must find a newly-appeared boat entity near the placement target, while ignoring
// entities that already existed before placement and entities that aren't boats at all.
func TestFindNewNearbyBoat(t *testing.T) {
	waterPos := models.V3{X: 10, Y: 64, Z: 10}

	reg := models.NewEntityRegistry()
	reg.RegisterEntity(1, models.EntityTypeBoat)
	reg.RegisterEntity(2, models.EntityTypeHorse)
	reg.RegisterEntity(3, models.EntityTypeChestBoat)

	a := &agent{
		entityRegistry: reg,
		entities: map[int32]*trackedEntity{
			1: {EntityID: 1, X: 10, Y: 64, Z: 10.5}, // New boat, close - should match
			2: {EntityID: 2, X: 10, Y: 64, Z: 10.5}, // New but not a boat - should be ignored
			3: {EntityID: 3, X: 50, Y: 64, Z: 50},   // New boat but far away - should be ignored
		},
	}

	t.Run("matches a new nearby boat", func(t *testing.T) {
		before := map[int32]bool{2: true, 3: true} // Entity 1 is the only "new" one
		id, found := a.findNewNearbyBoat(waterPos, before)
		if !found {
			t.Fatal("expected to find the new nearby boat entity")
		}
		if id != 1 {
			t.Errorf("expected entity 1, got %d", id)
		}
	})

	t.Run("ignores entities that already existed before placement", func(t *testing.T) {
		before := map[int32]bool{1: true, 2: true, 3: true} // All pre-existing
		_, found := a.findNewNearbyBoat(waterPos, before)
		if found {
			t.Error("expected no match when the boat entity already existed before placement")
		}
	})

	t.Run("ignores a new boat that's too far away", func(t *testing.T) {
		before := map[int32]bool{1: true, 2: true} // Only entity 3 (far away) is "new"
		_, found := a.findNewNearbyBoat(waterPos, before)
		if found {
			t.Error("expected no match for a new boat far outside the detection radius")
		}
	})

	t.Run("ignores a new non-boat entity", func(t *testing.T) {
		before := map[int32]bool{1: true, 3: true} // Only entity 2 (horse) is "new"
		_, found := a.findNewNearbyBoat(waterPos, before)
		if found {
			t.Error("expected no match for a newly-appeared non-boat entity")
		}
	})
}

func TestSnapshotEntityIDs(t *testing.T) {
	a := &agent{
		entities: map[int32]*trackedEntity{
			1: {EntityID: 1},
			2: {EntityID: 2},
		},
	}

	ids := a.snapshotEntityIDs()
	if len(ids) != 2 || !ids[1] || !ids[2] {
		t.Errorf("expected snapshot {1, 2}, got %v", ids)
	}
}
