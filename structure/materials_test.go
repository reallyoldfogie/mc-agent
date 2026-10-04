package structure

import (
	"path/filepath"
	"testing"
)

func TestComputeMaterialList_CountsAndSortsByName(t *testing.T) {
	s := &Structure{
		Palette: []PaletteEntry{
			{Name: "minecraft:stone"},
			{Name: "minecraft:oak_planks"},
			{Name: "minecraft:air"},
		},
		Blocks: []BlockEntry{
			{Pos: Pos{0, 0, 0}, PaletteIndex: 0},
			{Pos: Pos{1, 0, 0}, PaletteIndex: 0},
			{Pos: Pos{2, 0, 0}, PaletteIndex: 1},
			{Pos: Pos{3, 0, 0}, PaletteIndex: 2}, // air - must not appear
		},
	}

	ml := ComputeMaterialList(s)

	want := []MaterialEntry{
		{Item: "minecraft:oak_planks", Count: 1},
		{Item: "minecraft:stone", Count: 2},
	}
	if len(ml.Items) != len(want) {
		t.Fatalf("Items = %+v, want %+v", ml.Items, want)
	}
	for i, got := range ml.Items {
		if got != want[i] {
			t.Errorf("Items[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestComputeMaterialList_AgainstRealStructure(t *testing.T) {
	s, err := LoadFile(simpleHutPath)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	ml := ComputeMaterialList(s)

	want := map[string]int{
		"minecraft:oak_log":     63,
		"minecraft:oak_stairs":  55,
		"minecraft:tall_grass":  4,
		"minecraft:oak_planks":  2,
		"minecraft:spruce_door": 1,
	}
	if got := len(ml.Items); got != len(want) {
		t.Fatalf("len(Items) = %d, want %d (air must be excluded entirely)", got, len(want))
	}
	for item, count := range want {
		if got := ml.Count(item); got != count {
			t.Errorf("Count(%q) = %d, want %d", item, got, count)
		}
	}
	if ml.Count("minecraft:air") != 0 {
		t.Errorf("Count(minecraft:air) = %d, want 0 (air is not a material)", ml.Count("minecraft:air"))
	}

	wantTotal := 63 + 55 + 4 + 2 + 1
	if got := ml.Total(); got != wantTotal {
		t.Errorf("Total() = %d, want %d", got, wantTotal)
	}
}

func TestMaterialList_Missing(t *testing.T) {
	ml := MaterialList{Items: []MaterialEntry{
		{Item: "minecraft:stone", Count: 10},
		{Item: "minecraft:oak_planks", Count: 4},
		{Item: "minecraft:glass", Count: 2},
	}}

	have := map[string]int{
		"minecraft:stone":      10, // exactly enough - not missing
		"minecraft:oak_planks": 1,  // short by 3
		// glass: not held at all - short by the full 2
	}
	missing := ml.Missing(func(item string) int { return have[item] })

	want := []MaterialEntry{
		{Item: "minecraft:oak_planks", Count: 3},
		{Item: "minecraft:glass", Count: 2},
	}
	if len(missing) != len(want) {
		t.Fatalf("Missing = %+v, want %+v", missing, want)
	}
	for _, w := range want {
		found := false
		for _, m := range missing {
			if m == w {
				found = true
			}
		}
		if !found {
			t.Errorf("Missing is missing entry %+v: got %+v", w, missing)
		}
	}
}

func TestMaterialList_Missing_NothingShort(t *testing.T) {
	ml := MaterialList{Items: []MaterialEntry{{Item: "minecraft:stone", Count: 5}}}
	if missing := ml.Missing(func(string) int { return 100 }); missing != nil {
		t.Errorf("Missing = %+v, want nil", missing)
	}
}

func TestMaterialList_WriteJSON_LoadMaterialListJSON_RoundTrip(t *testing.T) {
	original := MaterialList{
		Source: "testdata/generated/reallyoldfogie/structure/simple_hut.nbt",
		Items: []MaterialEntry{
			{Item: "minecraft:oak_log", Count: 63},
			{Item: "minecraft:oak_planks", Count: 2},
		},
	}

	path := filepath.Join(t.TempDir(), "materials.json")
	if err := original.WriteJSON(path); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	loaded, err := LoadMaterialListJSON(path)
	if err != nil {
		t.Fatalf("LoadMaterialListJSON: %v", err)
	}

	if loaded.Source != original.Source {
		t.Errorf("Source = %q, want %q", loaded.Source, original.Source)
	}
	if len(loaded.Items) != len(original.Items) {
		t.Fatalf("Items = %+v, want %+v", loaded.Items, original.Items)
	}
	for i, want := range original.Items {
		if loaded.Items[i] != want {
			t.Errorf("Items[%d] = %+v, want %+v", i, loaded.Items[i], want)
		}
	}
}

func TestLoadMaterialListJSON_MissingFile_ReturnsError(t *testing.T) {
	if _, err := LoadMaterialListJSON("/nonexistent/materials.json"); err == nil {
		t.Fatal("expected an error for a missing file, got none")
	}
}
