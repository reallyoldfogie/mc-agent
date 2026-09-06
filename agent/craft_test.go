package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// These test the recipe-parsing/ingredient-resolution logic in isolation
// (no live server, no *agent instance needed — recipeGrid/resolveIngredient/
// resolveTag are all free functions) against synthetic JSON, matching the
// real cached datapack schema (docs/plans/RL_ACTION_SPACE_EXPANSION.md
// Phase 3's "Current State" table, verified against real cached output).
// CraftItem itself (the inventory-click sequence) needs a live server and
// isn't covered here.

func TestRecipeGrid_ShapedFitsIn2x2(t *testing.T) {
	rj := rawRecipeJSON{
		Type:    craftingShapedType,
		Key:     map[string]ingredientRef{"#": "minecraft:oak_planks"},
		Pattern: []string{"##", "##"},
	}
	grid, ok := recipeGrid("", rj, map[string][]string{})
	if !ok {
		t.Fatalf("recipeGrid: want ok=true for a 2x2 shaped recipe")
	}
	// Row-major, stride tableGridSize(3): (0,0)=0 (0,1)=1 (1,0)=3 (1,1)=4.
	for _, i := range []int{0, 1, 3, 4} {
		if len(grid[i]) != 1 || grid[i][0] != "minecraft:oak_planks" {
			t.Fatalf("grid[%d] = %v, want [minecraft:oak_planks]", i, grid[i])
		}
	}
	for _, i := range []int{2, 5, 6, 7, 8} {
		if len(grid[i]) != 0 {
			t.Fatalf("grid[%d] should be empty, got %v", i, grid[i])
		}
	}
	if !fitsInventoryGrid(grid) {
		t.Fatalf("fitsInventoryGrid: want true for a 2x2-only shaped recipe")
	}
}

func TestRecipeGrid_ShapedNarrowerThanGridLeavesSlotsEmpty(t *testing.T) {
	// Mirrors the real "stick" recipe: a 1-wide, 2-tall pattern — only
	// (row 0, col 0) and (row 1, col 0) should get an ingredient, i.e.
	// grid[0] and grid[3] (row-major, stride 3); everything else empty.
	rj := rawRecipeJSON{
		Type:    craftingShapedType,
		Key:     map[string]ingredientRef{"#": "minecraft:oak_planks"},
		Pattern: []string{"#", "#"},
	}
	grid, ok := recipeGrid("", rj, map[string][]string{})
	if !ok {
		t.Fatalf("recipeGrid: want ok=true")
	}
	if len(grid[0]) == 0 || len(grid[3]) == 0 {
		t.Fatalf("grid[0]/grid[3] should be populated, got %v / %v", grid[0], grid[3])
	}
	for _, i := range []int{1, 2, 4, 5, 6, 7, 8} {
		if len(grid[i]) != 0 {
			t.Fatalf("grid[%d] should be empty (narrower-than-grid pattern), got %v", i, grid[i])
		}
	}
	if !fitsInventoryGrid(grid) {
		t.Fatalf("fitsInventoryGrid: want true for a recipe fitting in one column of the 2x2 grid")
	}
}

func TestRecipeGrid_ShapedFitsIn3x3Only(t *testing.T) {
	// Mirrors the real "wooden_pickaxe" recipe: 3 planks across the top
	// row, 2 sticks down the center column — a real recipe that does not
	// fit in a 2x2 grid (docs/plans/CRAFTING_TABLE_3X3_PLAN.md Phase 5).
	rj := rawRecipeJSON{
		Type: craftingShapedType,
		Key: map[string]ingredientRef{
			"#": "minecraft:oak_planks",
			"X": "minecraft:stick",
		},
		Pattern: []string{"###", " X ", " X "},
	}
	grid, ok := recipeGrid("", rj, map[string][]string{})
	if !ok {
		t.Fatalf("recipeGrid: want ok=true for a 3x3 shaped recipe")
	}
	for _, i := range []int{0, 1, 2} {
		if len(grid[i]) != 1 || grid[i][0] != "minecraft:oak_planks" {
			t.Fatalf("grid[%d] = %v, want [minecraft:oak_planks]", i, grid[i])
		}
	}
	for _, i := range []int{4, 7} {
		if len(grid[i]) != 1 || grid[i][0] != "minecraft:stick" {
			t.Fatalf("grid[%d] = %v, want [minecraft:stick]", i, grid[i])
		}
	}
	if fitsInventoryGrid(grid) {
		t.Fatalf("fitsInventoryGrid: want false for a recipe using column 2 and row 2")
	}
}

func TestRecipeGrid_ShapedTooLargeRejected(t *testing.T) {
	rj := rawRecipeJSON{
		Type:    craftingShapedType,
		Key:     map[string]ingredientRef{"#": "minecraft:oak_planks"},
		Pattern: []string{"####", "####", "####", "####"},
	}
	if _, ok := recipeGrid("", rj, map[string][]string{}); ok {
		t.Fatalf("recipeGrid: want ok=false for a 4x4 pattern (out of 3x3 scope)")
	}
}

func TestRecipeGrid_ShapedUnknownKeySymbolRejected(t *testing.T) {
	rj := rawRecipeJSON{
		Type:    craftingShapedType,
		Key:     map[string]ingredientRef{"#": "minecraft:oak_planks"},
		Pattern: []string{"X"}, // 'X' has no entry in Key
	}
	if _, ok := recipeGrid("", rj, map[string][]string{}); ok {
		t.Fatalf("recipeGrid: want ok=false for a pattern symbol missing from key")
	}
}

func TestRecipeGrid_ShapelessFitsIn2x2(t *testing.T) {
	rj := rawRecipeJSON{
		Type:        craftingShapelessType,
		Ingredients: []ingredientRef{"minecraft:chest", "minecraft:acacia_boat"},
	}
	grid, ok := recipeGrid("", rj, map[string][]string{})
	if !ok {
		t.Fatalf("recipeGrid: want ok=true for a 2-ingredient shapeless recipe")
	}
	if len(grid[0]) != 1 || grid[0][0] != "minecraft:chest" {
		t.Fatalf("grid[0] = %v, want [minecraft:chest]", grid[0])
	}
	if len(grid[1]) != 1 || grid[1][0] != "minecraft:acacia_boat" {
		t.Fatalf("grid[1] = %v, want [minecraft:acacia_boat]", grid[1])
	}
	if len(grid[2]) != 0 || len(grid[3]) != 0 {
		t.Fatalf("grid[2]/grid[3] should be empty, got %v / %v", grid[2], grid[3])
	}
	if !fitsInventoryGrid(grid) {
		t.Fatalf("fitsInventoryGrid: want true for a 2-ingredient shapeless recipe")
	}
}

func TestRecipeGrid_ShapelessFitsIn3x3Only(t *testing.T) {
	// 5 ingredients: fits a 3x3 grid but not a 2x2 one.
	rj := rawRecipeJSON{
		Type:        craftingShapelessType,
		Ingredients: []ingredientRef{"minecraft:a", "minecraft:b", "minecraft:c", "minecraft:d", "minecraft:e"},
	}
	grid, ok := recipeGrid("", rj, map[string][]string{})
	if !ok {
		t.Fatalf("recipeGrid: want ok=true for a 5-ingredient shapeless recipe (fits 3x3)")
	}
	if fitsInventoryGrid(grid) {
		t.Fatalf("fitsInventoryGrid: want false for 5 shapeless ingredients (needs a table)")
	}
}

func TestRecipeGrid_ShapelessTooManyIngredientsRejected(t *testing.T) {
	rj := rawRecipeJSON{
		Type: craftingShapelessType,
		Ingredients: []ingredientRef{
			"minecraft:a", "minecraft:b", "minecraft:c", "minecraft:d", "minecraft:e",
			"minecraft:f", "minecraft:g", "minecraft:h", "minecraft:i", "minecraft:j",
		},
	}
	if _, ok := recipeGrid("", rj, map[string][]string{}); ok {
		t.Fatalf("recipeGrid: want ok=false for 10 ingredients (doesn't fit a 3x3 grid)")
	}
}

func TestRecipeGrid_OtherRecipeTypesRejected(t *testing.T) {
	for _, recipeType := range []string{"minecraft:smelting", "minecraft:stonecutting", "minecraft:smithing_transform"} {
		rj := rawRecipeJSON{Type: recipeType}
		if _, ok := recipeGrid("", rj, map[string][]string{}); ok {
			t.Fatalf("recipeGrid: want ok=false for type %q (not a crafting-grid recipe)", recipeType)
		}
	}
}

// TestIngredientRef_DecodesBothSchemaVariants covers the real cross-version
// bug found while writing this loader: 1.21.1's cached recipe JSON encodes
// ingredients as objects ({"item": "..."} / {"tag": "..."}), while 1.21.2+
// uses flat strings for the same concept — decoding only the flat-string
// form silently dropped every shaped/shapeless recipe against a real 1.21.1
// cache (confirmed live via TestLoadCraftingRecipes_AgainstRealCache before
// this was fixed).
func TestIngredientRef_DecodesBothSchemaVariants(t *testing.T) {
	cases := []struct {
		name string
		json string
		want ingredientRef
	}{
		{"flat concrete", `"minecraft:iron_ingot"`, "minecraft:iron_ingot"},
		{"flat tag", `"#minecraft:planks"`, "#minecraft:planks"},
		{"object item", `{"item":"minecraft:iron_ingot"}`, "minecraft:iron_ingot"},
		{"object tag", `{"tag":"minecraft:planks"}`, "#minecraft:planks"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ref ingredientRef
			if err := ref.UnmarshalJSON([]byte(tc.json)); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", tc.json, err)
			}
			if ref != tc.want {
				t.Fatalf("got %q, want %q", ref, tc.want)
			}
		})
	}
}

func TestResolveIngredient_ConcreteItem(t *testing.T) {
	candidates, err := resolveIngredient("", "minecraft:iron_ingot", map[string][]string{})
	if err != nil {
		t.Fatalf("resolveIngredient: %v", err)
	}
	if len(candidates) != 1 || candidates[0] != "minecraft:iron_ingot" {
		t.Fatalf("candidates = %v, want [minecraft:iron_ingot]", candidates)
	}
}

func TestResolveIngredient_ConcreteItemWithoutNamespaceIsNormalized(t *testing.T) {
	// Mirrors normalizeItemName's existing "iron_ingot" == "minecraft:iron_ingot"
	// contract (see item_search.go) — a bare name should resolve the same way.
	candidates, err := resolveIngredient("", "iron_ingot", map[string][]string{})
	if err != nil {
		t.Fatalf("resolveIngredient: %v", err)
	}
	if len(candidates) != 1 || candidates[0] != "minecraft:iron_ingot" {
		t.Fatalf("candidates = %v, want [minecraft:iron_ingot]", candidates)
	}
}

func TestResolveIngredient_TagReference(t *testing.T) {
	dataDir := t.TempDir()
	tagDir := filepath.Join(dataDir, "minecraft", "tags", "item")
	if err := os.MkdirAll(tagDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tagJSON := `{"values":["minecraft:oak_planks","minecraft:spruce_planks"]}`
	if err := os.WriteFile(filepath.Join(tagDir, "planks.json"), []byte(tagJSON), 0644); err != nil {
		t.Fatalf("write tag file: %v", err)
	}

	candidates, err := resolveIngredient(dataDir, "#minecraft:planks", map[string][]string{})
	if err != nil {
		t.Fatalf("resolveIngredient: %v", err)
	}
	want := map[string]bool{"minecraft:oak_planks": true, "minecraft:spruce_planks": true}
	if len(candidates) != 2 || !want[candidates[0]] || !want[candidates[1]] {
		t.Fatalf("candidates = %v, want the two planks tag members", candidates)
	}
}

func TestResolveIngredient_MissingTagFileErrors(t *testing.T) {
	dataDir := t.TempDir() // no tags/item/planks.json written
	if _, err := resolveIngredient(dataDir, "#minecraft:planks", map[string][]string{}); err == nil {
		t.Fatalf("resolveIngredient: want error for a tag file that doesn't exist")
	}
}

func TestResolveTag_CachesAcrossCalls(t *testing.T) {
	dataDir := t.TempDir()
	tagDir := filepath.Join(dataDir, "minecraft", "tags", "item")
	if err := os.MkdirAll(tagDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tagDir, "planks.json"), []byte(`{"values":["minecraft:oak_planks"]}`), 0644); err != nil {
		t.Fatalf("write tag file: %v", err)
	}

	cache := map[string][]string{}
	if _, err := resolveTag(dataDir, "#minecraft:planks", cache, 0); err != nil {
		t.Fatalf("resolveTag: %v", err)
	}
	// Delete the backing file: a second call should still succeed, proving
	// it served the cached result rather than re-reading disk.
	if err := os.Remove(filepath.Join(tagDir, "planks.json")); err != nil {
		t.Fatalf("remove tag file: %v", err)
	}
	candidates, err := resolveTag(dataDir, "#minecraft:planks", cache, 0)
	if err != nil {
		t.Fatalf("resolveTag (cached): %v", err)
	}
	if len(candidates) != 1 || candidates[0] != "minecraft:oak_planks" {
		t.Fatalf("candidates = %v, want cached [minecraft:oak_planks]", candidates)
	}
}

// TestLoadCraftingRecipes_AgainstRealCache is a best-effort sanity check
// against this repo's actual cached data_generator output, if any version's
// cache happens to be present on disk (populated by a live agent's Init —
// see agent.downloadJarsAndGenerateReports) — skipped, not failed, if none
// is found, since a plain `go test` run has no live server to populate one.
func TestLoadCraftingRecipes_AgainstRealCache(t *testing.T) {
	baseCacheDir, err := findAnyCachedVersionDataDir(t)
	if err != nil {
		t.Skipf("no cached data_generator output found on disk: %v", err)
	}

	a := &agent{cfg: models.AgentConfig{Version: baseCacheDir.version}}
	recipes, err := a.loadCraftingRecipes()
	if err != nil {
		t.Fatalf("loadCraftingRecipes: %v", err)
	}
	if len(recipes) == 0 {
		t.Fatalf("loadCraftingRecipes: got no recipes from a real cache — expected at least a few 2x2-fitting ones")
	}

	// "stick" (real recipe: a 1x2 shaped pattern gated by the #minecraft:planks
	// tag) is a good end-to-end check that pattern-narrower-than-grid AND
	// tag resolution both work against real cached data.
	if stick, ok := recipes["minecraft:stick"]; ok {
		// Row-major, stride tableGridSize(3): (0,0)=0, (1,0)=3.
		if len(stick.grid[0]) == 0 || len(stick.grid[3]) == 0 {
			t.Errorf("minecraft:stick recipe: grid[0]/grid[3] should be populated (tag-resolved plank candidates), got %v / %v", stick.grid[0], stick.grid[3])
		}
		if len(stick.grid[1]) != 0 || len(stick.grid[2]) != 0 {
			t.Errorf("minecraft:stick recipe: grid[1]/grid[2] should be empty, got %v / %v", stick.grid[1], stick.grid[2])
		}
		if !stick.fitsInventoryGrid() {
			t.Errorf("minecraft:stick recipe: fitsInventoryGrid() should be true")
		}
	}

	// "wooden_pickaxe" (real recipe: 3 planks across the top row, 2 sticks
	// down the center column) is a real 3x3-only shaped recipe — confirms
	// recipeGrid's raised shaped-pattern limit and fitsInventoryGrid's
	// rejection both work against real cached data, not just synthetic
	// fixtures (see TestRecipeGrid_ShapedFitsIn3x3Only for the synthetic
	// version of this same shape).
	if pickaxe, ok := recipes["minecraft:wooden_pickaxe"]; ok {
		for _, i := range []int{0, 1, 2} {
			if len(pickaxe.grid[i]) == 0 {
				t.Errorf("minecraft:wooden_pickaxe recipe: grid[%d] should be populated (top row planks), got %v", i, pickaxe.grid[i])
			}
		}
		for _, i := range []int{4, 7} {
			if len(pickaxe.grid[i]) == 0 {
				t.Errorf("minecraft:wooden_pickaxe recipe: grid[%d] should be populated (center-column sticks), got %v", i, pickaxe.grid[i])
			}
		}
		if pickaxe.fitsInventoryGrid() {
			t.Errorf("minecraft:wooden_pickaxe recipe: fitsInventoryGrid() should be false (needs a table)")
		}
	}
}

type cachedVersionDir struct {
	version string
}

// findAnyCachedVersionDataDir locates any version subdirectory under
// .agent/cache/downloads that already has generated reports, for
// TestLoadCraftingRecipes_AgainstRealCache. Doesn't use
// agentutils.FindOrCreateCacheDir directly since that would create a cache
// dir as a side effect of running tests; this only reads.
func findAnyCachedVersionDataDir(t *testing.T) (cachedVersionDir, error) {
	t.Helper()
	repoRoot, err := os.Getwd()
	if err != nil {
		return cachedVersionDir{}, err
	}
	// agent/ -> repo root
	downloadsDir := filepath.Join(repoRoot, "..", ".agent", "cache", "downloads")
	entries, err := os.ReadDir(downloadsDir)
	if err != nil {
		return cachedVersionDir{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		recipeDir := filepath.Join(downloadsDir, entry.Name(), "data_generator", "data", "minecraft", "recipe")
		if info, err := os.Stat(recipeDir); err == nil && info.IsDir() {
			return cachedVersionDir{version: entry.Name()}, nil
		}
	}
	return cachedVersionDir{}, os.ErrNotExist
}
