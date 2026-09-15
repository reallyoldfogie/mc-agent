package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// fakeMultiSlotResolver returns a different item per hotbar slot index,
// unlike help_and_screen_test.go's fakeSlotResolver (one fixed item for
// every slot) - needed here to test picking the *right* slot out of several.
type fakeMultiSlotResolver struct {
	itemBySlot map[int16]int32
}

func (f fakeMultiSlotResolver) ResolveSlot(_ int, index int16) (int32, int, bool) {
	itemID, ok := f.itemBySlot[index]
	if !ok {
		return 0, 0, false
	}
	return itemID, 1, true
}

type fakeMultiItemMgr struct {
	nameByID map[int32]string
}

func (f fakeMultiItemMgr) GetItemNameByID(id int32) string {
	return f.nameByID[id]
}

// TestFindHotbarSlotMatching covers the hotbar-scanning half of
// WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8: given a resolved set of wanted item names, the
// right hotbar slot (0-indexed, not the raw protocol slot number) must be picked out from among
// other, non-matching items.
func TestFindHotbarSlotMatching(t *testing.T) {
	// Hotbar slots 36-44 (protocol numbering); slot 38 (index 2) holds the boat.
	a := &agent{
		slots: fakeMultiSlotResolver{itemBySlot: map[int16]int32{
			36: 1, // minecraft:dirt
			37: 2, // minecraft:cobblestone
			38: 3, // minecraft:oak_boat
			39: 4, // minecraft:stone
		}},
		itemMgr: fakeMultiItemMgr{nameByID: map[int32]string{
			1: "minecraft:dirt",
			2: "minecraft:cobblestone",
			3: "minecraft:oak_boat",
			4: "minecraft:stone",
		}},
	}

	wanted := map[string]bool{"minecraft:oak_boat": true, "minecraft:spruce_boat": true}

	slot, name := a.findHotbarSlotMatching(wanted)
	if slot != 2 {
		t.Errorf("expected 0-indexed hotbar slot 2, got %d", slot)
	}
	if name != "minecraft:oak_boat" {
		t.Errorf("expected minecraft:oak_boat, got %q", name)
	}
}

func TestFindHotbarSlotMatching_NoMatch(t *testing.T) {
	a := &agent{
		slots: fakeMultiSlotResolver{itemBySlot: map[int16]int32{
			36: 1,
		}},
		itemMgr: fakeMultiItemMgr{nameByID: map[int32]string{
			1: "minecraft:dirt",
		}},
	}

	slot, _ := a.findHotbarSlotMatching(map[string]bool{"minecraft:oak_boat": true})
	if slot >= 0 {
		t.Errorf("expected no match, got slot %d", slot)
	}
}

func TestFindHotbarSlotMatching_NoSlotsOrItemMgr(t *testing.T) {
	a := &agent{}
	slot, _ := a.findHotbarSlotMatching(map[string]bool{"minecraft:oak_boat": true})
	if slot >= 0 {
		t.Errorf("expected no match when slots/itemMgr are unset, got slot %d", slot)
	}
}

// TestResolveTag_RealBoatsTagShape mirrors the actual
// data/minecraft/tags/item/boats.json shape (a flat list of wood-type boats plus a nested
// "#minecraft:chest_boats" reference) to confirm resolveTag - the same mechanism craft.go already
// uses for recipe ingredients - correctly flattens it into concrete item names. This is the
// authoritative data source WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8 uses instead of a
// hardcoded name-suffix guess.
func TestResolveTag_RealBoatsTagShape(t *testing.T) {
	dataDir := t.TempDir()
	tagDir := filepath.Join(dataDir, "minecraft", "tags", "item")
	if err := os.MkdirAll(tagDir, 0755); err != nil {
		t.Fatal(err)
	}

	boatsJSON := `{
		"values": [
			"minecraft:oak_boat",
			"minecraft:spruce_boat",
			"minecraft:bamboo_raft",
			"#minecraft:chest_boats"
		]
	}`
	chestBoatsJSON := `{
		"values": [
			"minecraft:oak_chest_boat",
			"minecraft:bamboo_chest_raft"
		]
	}`

	if err := os.WriteFile(filepath.Join(tagDir, "boats.json"), []byte(boatsJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tagDir, "chest_boats.json"), []byte(chestBoatsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	names, err := resolveTag(dataDir, "#minecraft:boats", map[string][]string{}, 0)
	if err != nil {
		t.Fatalf("resolveTag: %v", err)
	}

	want := map[string]bool{
		"minecraft:oak_boat":          true,
		"minecraft:spruce_boat":       true,
		"minecraft:bamboo_raft":       true,
		"minecraft:oak_chest_boat":    true,
		"minecraft:bamboo_chest_raft": true,
	}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %v", len(want), len(names), names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected name %q in resolved boat tag", n)
		}
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

// TestHasPlaceableBoat_NoSlotsConfigured confirms HasPlaceableBoat degrades to false rather than
// panicking when slots/itemMgr aren't wired up (e.g. before Init has run).
func TestHasPlaceableBoat_NoSlotsConfigured(t *testing.T) {
	a := &agent{boatItemNamesCache: map[string]bool{"minecraft:oak_boat": true}}
	if a.HasPlaceableBoat() {
		t.Error("expected HasPlaceableBoat to be false with no slots/itemMgr configured")
	}
}
