package agent

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

// stickRecipe mirrors the real stick recipe's shape: two oak_planks
// stacked vertically, i.e. grid cells 0 and 3 (row-major, stride 3) both
// holding ["minecraft:oak_planks"].
func stickRecipe() craftingRecipe {
	grid := [tableGridSize * tableGridSize][]string{}
	grid[0] = []string{"minecraft:oak_planks"}
	grid[3] = []string{"minecraft:oak_planks"}
	return craftingRecipe{grid: grid, fitsInventory: fitsInventoryGrid(grid)}
}

// plankRecipe mirrors oak_planks from oak_log: one log, one grid cell.
func plankRecipe() craftingRecipe {
	grid := [tableGridSize * tableGridSize][]string{}
	grid[0] = []string{"minecraft:oak_log"}
	return craftingRecipe{grid: grid, fitsInventory: fitsInventoryGrid(grid)}
}

func noHave(string) int { return 0 }

func noLocate(string) (models.V3, bool, float64, bool) { return models.V3{}, false, 0, false }

func TestRecipeIngredientRequirements_AggregatesSharedCells(t *testing.T) {
	reqs := recipeIngredientRequirements(stickRecipe())
	if len(reqs) != 1 {
		t.Fatalf("len(reqs) = %d, want 1 (both cells share the same candidate list)", len(reqs))
	}
	if reqs[0].Count != 2 {
		t.Errorf("reqs[0].Count = %d, want 2", reqs[0].Count)
	}
	if len(reqs[0].Candidates) != 1 || reqs[0].Candidates[0] != "minecraft:oak_planks" {
		t.Errorf("reqs[0].Candidates = %v, want [minecraft:oak_planks]", reqs[0].Candidates)
	}
}

func TestResolveDependencies_SatisfiedFromInventory(t *testing.T) {
	recipes := map[string]craftingRecipe{"minecraft:stick": stickRecipe()}
	have := func(item string) int {
		if item == "minecraft:stick" {
			return 4
		}
		return 0
	}

	node := resolveDependencies(recipes, have, noLocate, 1, "minecraft:stick", 0)
	if node.Source != models.SourceInventory {
		t.Fatalf("Source = %v, want SourceInventory", node.Source)
	}
}

func TestResolveDependencies_RecursesIntoIngredientRecipe(t *testing.T) {
	recipes := map[string]craftingRecipe{
		"minecraft:stick":      stickRecipe(),
		"minecraft:oak_planks": plankRecipe(),
	}

	node := resolveDependencies(recipes, noHave, noLocate, 1, "minecraft:stick", 0)
	if node.Source != models.SourceCraft {
		t.Fatalf("Source = %v, want SourceCraft", node.Source)
	}
	if len(node.Children) != 1 {
		t.Fatalf("len(Children) = %d, want 1", len(node.Children))
	}

	plankNode := node.Children[0]
	if plankNode.Item != "minecraft:oak_planks" || plankNode.Needed != 2 {
		t.Fatalf("plankNode = %+v, want Item=minecraft:oak_planks Needed=2", plankNode)
	}
	if plankNode.Source != models.SourceCraft {
		t.Fatalf("plankNode.Source = %v, want SourceCraft (oak_log has no source here)", plankNode.Source)
	}
	if len(plankNode.Children) != 1 || plankNode.Children[0].Item != "minecraft:oak_log" {
		t.Fatalf("plankNode.Children = %+v, want one child for minecraft:oak_log", plankNode.Children)
	}
	if plankNode.Children[0].Source != models.SourceUnknown {
		t.Errorf("oak_log Source = %v, want SourceUnknown (no recipe, no memory, no inventory)", plankNode.Children[0].Source)
	}
}

func TestResolveDependencies_PrefersMemoryOverCraftingWhenAvailable(t *testing.T) {
	recipes := map[string]craftingRecipe{
		"minecraft:stick":      stickRecipe(),
		"minecraft:oak_planks": plankRecipe(),
	}
	rememberedPos := models.V3{X: 7, Y: 8, Z: 9}
	locate := func(item string) (models.V3, bool, float64, bool) {
		if item == "minecraft:oak_planks" {
			return rememberedPos, false, 0.9, true
		}
		return models.V3{}, false, 0, false
	}

	node := resolveDependencies(recipes, noHave, locate, 1, "minecraft:stick", 0)
	if node.Source != models.SourceCraft {
		t.Fatalf("Source = %v, want SourceCraft", node.Source)
	}
	plankNode := node.Children[0]
	if plankNode.Source != models.SourceMemory {
		t.Fatalf("plankNode.Source = %v, want SourceMemory (a remembered location should win over recursing further)", plankNode.Source)
	}
	if plankNode.LocationPos != rememberedPos {
		t.Errorf("plankNode.LocationPos = %v, want %v", plankNode.LocationPos, rememberedPos)
	}
	if plankNode.Confidence != 0.9 {
		t.Errorf("plankNode.Confidence = %v, want 0.9 (threaded through from locate)", plankNode.Confidence)
	}
}

func TestFirstMemoryLocation_FindsDeepestRememberedNode(t *testing.T) {
	rememberedPos := models.V3{X: 1, Y: 2, Z: 3}
	tree := models.DependencyNode{
		Item:   "minecraft:stick",
		Source: models.SourceCraft,
		Children: []models.DependencyNode{
			{
				Item:   "minecraft:oak_planks",
				Source: models.SourceCraft,
				Children: []models.DependencyNode{
					{Item: "minecraft:oak_log", Source: models.SourceMemory, LocationPos: rememberedPos},
				},
			},
		},
	}

	pos, found := firstMemoryLocation(tree)
	if !found {
		t.Fatalf("firstMemoryLocation: found = false, want true")
	}
	if pos != rememberedPos {
		t.Errorf("pos = %v, want %v", pos, rememberedPos)
	}
}

func TestFirstMemoryLocation_NotFoundWhenNoMemoryNodeExists(t *testing.T) {
	tree := models.DependencyNode{
		Item:   "minecraft:stick",
		Source: models.SourceCraft,
		Children: []models.DependencyNode{
			{Item: "minecraft:oak_planks", Source: models.SourceUnknown},
		},
	}
	if _, found := firstMemoryLocation(tree); found {
		t.Fatalf("firstMemoryLocation: found = true, want false (no SourceMemory node anywhere in the tree)")
	}
}

func TestResolveDependencies_UnknownSourceWhenNoRecipe(t *testing.T) {
	node := resolveDependencies(map[string]craftingRecipe{}, noHave, noLocate, 1, "minecraft:diamond", 0)
	if node.Source != models.SourceUnknown {
		t.Fatalf("Source = %v, want SourceUnknown", node.Source)
	}
}

func TestResolveDependencies_DepthLimitStopsRecursion(t *testing.T) {
	// A pathological self-referential "recipe" (A needs A) must not
	// recurse forever - the depth bound should turn it into
	// SourceUnknown once maxDependencyDepth is reached rather than
	// stack-overflowing.
	grid := [tableGridSize * tableGridSize][]string{}
	grid[0] = []string{"minecraft:cursed"}
	recipes := map[string]craftingRecipe{
		"minecraft:cursed": {grid: grid, fitsInventory: fitsInventoryGrid(grid)},
	}

	node := resolveDependencies(recipes, noHave, noLocate, 1, "minecraft:cursed", 0)
	if node.Source != models.SourceCraft {
		t.Fatalf("top-level Source = %v, want SourceCraft", node.Source)
	}

	// Walk down until we hit the depth limit; every node along the way
	// must be well-formed (no panic, no infinite loop) and the walk must
	// terminate in SourceUnknown.
	current := node
	for depth := range maxDependencyDepth + 2 {
		if current.Source != models.SourceCraft {
			break
		}
		if len(current.Children) != 1 {
			t.Fatalf("depth %d: len(Children) = %d, want 1", depth, len(current.Children))
		}
		current = current.Children[0]
	}
	if current.Source != models.SourceUnknown {
		t.Fatalf("final Source = %v, want SourceUnknown once the depth bound is hit", current.Source)
	}
}
