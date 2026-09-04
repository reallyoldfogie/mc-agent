package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	agentutils "github.com/reallyoldfogie/mc-agent/utils"
)

// craftGridSize is the player-inventory crafting grid's width/height (a 2x2
// grid, window 0 slots 1-4 - see agent/adapters.go's window-0 layout
// comment: "0=craft output, 1-4=craft grid"). docs/plans/RL_ACTION_SPACE_EXPANSION.md
// Phase 3 scopes this MVP to that grid only - a real 3x3 crafting-table
// grid is a follow-up, not attempted here.
const craftGridSize = 2

const (
	craftingShapedType    = "minecraft:crafting_shaped"
	craftingShapelessType = "minecraft:crafting_shapeless"
)

// craftOutputTimeout bounds how long CraftItem waits for the server to
// populate the result slot (window-0 slot 0) after ingredients are placed.
const craftOutputTimeout = 2 * time.Second

// craftIngredientSlotStart is the first main-inventory slot index in
// window 0 (see agent/adapters.go's window-0 layout comment: 0=craft
// output, 1-4=craft grid, 5-8=armor, 9-35=main, 36-44=hotbar). Ingredient
// search (findIngredientSlot) is restricted to this index and beyond -
// never the output/grid/armor slots before it - so placing one unit of a
// multi-unit ingredient (e.g. "stick" needs 2 planks, in two different grid
// slots) can't "re-find" the unit just placed into the grid itself and
// shuffle it to the other grid slot instead of pulling a fresh one from the
// inventory.
//
// Found live, not anticipated: models.CommandAgent's existing
// FindSlotWith (agent/item_search.go) scans the *entire* window-0 slot
// array including the grid, and grid slots sort earlier in that array than
// main-inventory slots - so a naive second FindSlotWith call after the
// first ingredient placement reliably rediscovers the just-placed item
// instead of the next one, silently crafting the wrong thing (e.g. a
// single stray plank in the grid matches "oak_button", not "stick") rather
// than erroring. Confirmed via testing/craft_test.go before this fix: an
// oak_button appeared in inventory instead of the requested stick.
const craftIngredientSlotStart = 9

// findIngredientSlot searches window 0's main inventory + hotbar (slots
// craftIngredientSlotStart and up) for itemName - mirroring FindSlotWith's
// matching logic, but scoped to that range; see craftIngredientSlotStart's
// doc comment for why FindSlotWith itself isn't safe to reuse here.
func (a *agent) findIngredientSlot(itemName string) (int, bool, error) {
	inv := a.GetInventory()
	if inv == nil {
		return -1, false, fmt.Errorf("inventory not available")
	}
	a.itemMgrMu.RLock()
	itemMgr := a.itemMgr
	a.itemMgrMu.RUnlock()
	if itemMgr == nil {
		return -1, false, fmt.Errorf("item manager not initialized")
	}

	normalized := normalizeItemName(itemName)
	slots := inv.GetSlots()
	for idx := craftIngredientSlotStart; idx < len(slots); idx++ {
		if slots[idx].Count <= 0 {
			continue
		}
		if normalizeItemName(itemMgr.GetItemNameByID(int32(slots[idx].ID))) == normalized {
			return idx, true, nil
		}
	}
	return -1, false, nil
}

// craftingRecipe is a 2x2-grid-craftable recipe resolved from cached
// datapack JSON (see loadCraftingRecipes) into exactly what CraftItem
// needs: for each of the grid's 4 slots (index 0 = window-0 slot 1, ...
// index 3 = window-0 slot 4), the list of item names that would satisfy
// that position - a single name for a concrete-item ingredient, several
// for a tag reference (e.g. "#minecraft:planks" resolves to every plank
// variant - see resolveIngredient). A nil/empty entry means that grid slot
// must stay empty for this recipe.
type craftingRecipe struct {
	grid [craftGridSize * craftGridSize][]string
}

// rawRecipeJSON mirrors the standard datapack recipe schema (verified
// directly against this codebase's own cached data_generator output - see
// docs/plans/RL_ACTION_SPACE_EXPANSION.md Phase 3's "Current State" table).
// Only the fields crafting_shaped/crafting_shapeless recipes use are
// captured; other recipe types (smelting, stonecutting, smithing, ...)
// don't populate Pattern/Ingredients and are filtered out by Type before
// these fields are read.
type rawRecipeJSON struct {
	Type        string                   `json:"type"`
	Key         map[string]ingredientRef `json:"key"`
	Pattern     []string                 `json:"pattern"`
	Ingredients []ingredientRef          `json:"ingredients"`
	Result      struct {
		ID    string `json:"id"`
		Count int    `json:"count"`
	} `json:"result"`
}

// ingredientRef is one recipe ingredient descriptor, decoded from either of
// two on-disk schema variants observed across this codebase's own cached
// recipe JSON for otherwise-identical recipes: a flat string
// ("minecraft:iron_ingot", or the tag-reference form "#minecraft:planks") -
// used from version 1.21.2 onward - or an older object form
// ({"item": "minecraft:iron_ingot"} or {"tag": "minecraft:planks"}) - the
// only form seen in 1.21.1's cache. Confirmed by direct comparison of the
// same recipe (e.g. stick.json) across versions, not assumed. Normalizes to
// the same flat-string shape resolveIngredient already expects either way,
// so the rest of this file doesn't need to know which variant it came from.
type ingredientRef string

func (r *ingredientRef) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*r = ingredientRef(s)
		return nil
	}
	var obj struct {
		Item string `json:"item"`
		Tag  string `json:"tag"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	if obj.Tag != "" {
		*r = ingredientRef("#" + obj.Tag)
		return nil
	}
	*r = ingredientRef(obj.Item)
	return nil
}

// rawTagJSON mirrors the standard datapack item-tag schema
// (data/<namespace>/tags/item/<name>.json - same cached data_generator
// tree recipes come from).
type rawTagJSON struct {
	Values []string `json:"values"`
}

// craftingRecipeDataDir returns the cached data_generator "data" directory
// for the connected version - the same tree
// agent.downloadJarsAndGenerateReports populates unconditionally at Init,
// containing both data/minecraft/recipe/*.json and
// data/<namespace>/tags/item/*.json. No field on *agent stores this path
// (downloadJarsAndGenerateReports doesn't either); recomputed here the same
// way that function computes it.
func (a *agent) craftingRecipeDataDir() (string, error) {
	baseCacheDir, err := agentutils.FindOrCreateCacheDir()
	if err != nil {
		return "", fmt.Errorf("find cache directory: %w", err)
	}
	return filepath.Join(baseCacheDir, "downloads", a.cfg.Version, "data_generator", "data"), nil
}

// loadCraftingRecipes reads every cached recipe JSON file for the connected
// version and returns an index of 2x2-grid-craftable recipes keyed by
// normalized result item name.
//
// Known simplification, not solved here (docs/plans/RL_ACTION_SPACE_EXPANSION.md
// Phase 3's MVP framing): reloaded from disk and re-parsed on every
// CraftItem call rather than cached on the agent - a few hundred small JSON
// files, acceptable for a command that isn't called in a tight loop; revisit
// if that changes. Only minecraft:crafting_shaped/crafting_shapeless
// recipes that fit within a 2x2 grid are indexed - smelting/stonecutting/
// smithing/etc., and anything needing a real 3x3 crafting table, are out of
// this MVP's scope. If multiple recipes produce the same result item, only
// the first one encountered (directory iteration order, not otherwise
// meaningful) is kept.
func (a *agent) loadCraftingRecipes() (map[string]craftingRecipe, error) {
	dataDir, err := a.craftingRecipeDataDir()
	if err != nil {
		return nil, err
	}
	recipeDir := filepath.Join(dataDir, "minecraft", "recipe")
	entries, err := os.ReadDir(recipeDir)
	if err != nil {
		return nil, fmt.Errorf("read recipe directory %s: %w", recipeDir, err)
	}

	tagCache := map[string][]string{}
	recipes := make(map[string]craftingRecipe)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(recipeDir, entry.Name()))
		if err != nil {
			continue // best-effort: skip unreadable files rather than failing the whole load
		}
		var rj rawRecipeJSON
		if err := json.Unmarshal(raw, &rj); err != nil {
			continue
		}

		grid, ok := recipeGrid(dataDir, rj, tagCache)
		if !ok {
			continue
		}

		resultItem := normalizeItemName(rj.Result.ID)
		if resultItem == "" || resultItem == "minecraft:" {
			continue
		}
		if _, exists := recipes[resultItem]; exists {
			continue
		}
		recipes[resultItem] = craftingRecipe{grid: grid}
	}
	return recipes, nil
}

// recipeGrid resolves one parsed recipe into 2x2 grid-slot ingredient
// candidates, or ok=false if it isn't a crafting_shaped/crafting_shapeless
// recipe that fits in a 2x2 grid (or references an ingredient that can't be
// resolved at all - e.g. a tag file missing from the cache).
func recipeGrid(dataDir string, rj rawRecipeJSON, tagCache map[string][]string) (grid [craftGridSize * craftGridSize][]string, ok bool) {
	switch rj.Type {
	case craftingShapedType:
		if len(rj.Pattern) == 0 || len(rj.Pattern) > craftGridSize {
			return grid, false
		}
		for _, row := range rj.Pattern {
			if len(row) > craftGridSize {
				return grid, false
			}
		}
		for r, row := range rj.Pattern {
			for c, symbol := range row {
				if symbol == ' ' {
					continue
				}
				descriptor, found := rj.Key[string(symbol)]
				if !found {
					return grid, false
				}
				candidates, err := resolveIngredient(dataDir, string(descriptor), tagCache)
				if err != nil || len(candidates) == 0 {
					return grid, false
				}
				grid[r*craftGridSize+c] = candidates
			}
		}
		return grid, true

	case craftingShapelessType:
		if len(rj.Ingredients) == 0 || len(rj.Ingredients) > craftGridSize*craftGridSize {
			return grid, false
		}
		for i, descriptor := range rj.Ingredients {
			candidates, err := resolveIngredient(dataDir, string(descriptor), tagCache)
			if err != nil || len(candidates) == 0 {
				return grid, false
			}
			grid[i] = candidates
		}
		return grid, true

	default:
		return grid, false
	}
}

// resolveIngredient turns one recipe ingredient descriptor - a concrete
// item id ("minecraft:iron_ingot") or a tag reference
// ("#minecraft:planks") - into the list of concrete item names that would
// satisfy it.
func resolveIngredient(dataDir, descriptor string, tagCache map[string][]string) ([]string, error) {
	if !strings.HasPrefix(descriptor, "#") {
		normalized := normalizeItemName(descriptor)
		if normalized == "" {
			return nil, fmt.Errorf("empty ingredient descriptor")
		}
		return []string{normalized}, nil
	}
	return resolveTag(dataDir, descriptor, tagCache, 0)
}

// resolveTag reads a datapack item-tag file (data/<namespace>/tags/item/<name>.json,
// the same cached data_generator tree recipes come from) and returns the
// concrete item names it contains. Tag members that are themselves tag
// references are resolved recursively, bounded by depth - every tag file
// actually observed in this codebase's cache is a flat item list, but
// deeper nesting is possible in the datapack format in general.
func resolveTag(dataDir, tagRef string, cache map[string][]string, depth int) ([]string, error) {
	if cached, ok := cache[tagRef]; ok {
		return cached, nil
	}
	if depth > 2 {
		return nil, fmt.Errorf("tag reference nested too deep: %s", tagRef)
	}
	ref := strings.TrimPrefix(tagRef, "#")
	namespace, name, found := strings.Cut(ref, ":")
	if !found {
		return nil, fmt.Errorf("malformed tag reference %q", tagRef)
	}
	tagPath := filepath.Join(dataDir, namespace, "tags", "item", name+".json")
	raw, err := os.ReadFile(tagPath)
	if err != nil {
		return nil, fmt.Errorf("read tag %s: %w", tagRef, err)
	}
	var tj rawTagJSON
	if err := json.Unmarshal(raw, &tj); err != nil {
		return nil, fmt.Errorf("parse tag %s: %w", tagRef, err)
	}
	var resolved []string
	for _, v := range tj.Values {
		if strings.HasPrefix(v, "#") {
			nested, err := resolveTag(dataDir, v, cache, depth+1)
			if err != nil {
				continue // best-effort: skip an unresolvable nested tag member
			}
			resolved = append(resolved, nested...)
			continue
		}
		if n := normalizeItemName(v); n != "" {
			resolved = append(resolved, n)
		}
	}
	cache[tagRef] = resolved
	return resolved, nil
}

// CraftItem crafts itemName using the player's own 2x2 inventory crafting
// grid (window 0, slots 1-4) - no crafting table needed. Looks up a
// matching 2x2-fitting recipe (see loadCraftingRecipes), moves one of each
// required ingredient from the main inventory into the grid, waits for the
// server to populate the result slot (slot 0), then shift-clicks it to
// collect the crafted item.
//
// docs/plans/RL_ACTION_SPACE_EXPANSION.md Phase 3 MVP scope: only 2x2-grid
// recipes (no crafting table / 3x3 shapes). On a missing-ingredient failure
// partway through, whatever was already placed into the grid is left there
// rather than moved back - matches what a player fumbling a recipe by hand
// would see, and keeps this from needing rollback machinery for a first
// pass.
func (a *agent) CraftItem(ctx context.Context, itemName string) error {
	normalized := normalizeItemName(itemName)

	recipes, err := a.loadCraftingRecipes()
	if err != nil {
		return fmt.Errorf("load recipes: %w", err)
	}
	recipe, ok := recipes[normalized]
	if !ok {
		return fmt.Errorf("no known 2x2-craftable recipe for %s", itemName)
	}

	for i, candidates := range recipe.grid {
		if len(candidates) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		gridSlot := int16(1 + i)
		if err := a.placeCraftIngredient(ctx, gridSlot, candidates); err != nil {
			return fmt.Errorf("craft %s: %w", itemName, err)
		}
	}

	result, ok := a.waitForCraftOutput(ctx, craftOutputTimeout)
	if !ok {
		return fmt.Errorf("craft %s: grid did not produce a result (ingredients may not actually match the recipe)", itemName)
	}
	if err := a.ShiftClickSlot(0, result); err != nil {
		return fmt.Errorf("collect crafted %s: %w", itemName, err)
	}
	return nil
}

// placeCraftIngredient finds the first inventory item matching one of
// candidates and moves a single unit of it into gridSlot (1-4).
func (a *agent) placeCraftIngredient(ctx context.Context, gridSlot int16, candidates []string) error {
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		srcSlot, found, err := a.findIngredientSlot(candidate)
		if err != nil {
			return fmt.Errorf("search inventory for %s: %w", candidate, err)
		}
		if !found {
			continue
		}

		inv := a.GetInventory()
		if inv == nil {
			return fmt.Errorf("inventory not available")
		}
		slots := inv.GetSlots()
		if srcSlot < 0 || srcSlot >= len(slots) || int(gridSlot) >= len(slots) {
			return fmt.Errorf("invalid slot index")
		}
		srcItem := itemStackFromScreenSlot(slots[srcSlot])
		destItem := itemStackFromScreenSlot(slots[gridSlot])
		if err := a.MoveSingle(int16(srcSlot), gridSlot, srcItem, destItem); err != nil {
			return fmt.Errorf("place %s: %w", candidate, err)
		}
		return nil
	}
	return fmt.Errorf("missing ingredient (need one of: %s)", strings.Join(candidates, ", "))
}

// waitForCraftOutput polls the crafting result slot (window-0 slot 0) until
// it becomes non-empty or timeout elapses. Doesn't check the item matches
// the expected recipe result: the result slot in a real crafting UI only
// ever shows a valid output for the grid's current contents (or stays
// empty), so "non-empty" is already a sufficient, simpler signal than
// resolving an item name back to an ID just to compare.
func (a *agent) waitForCraftOutput(ctx context.Context, timeout time.Duration) (models.ItemStack, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if inv := a.GetInventory(); inv != nil {
			slots := inv.GetSlots()
			if len(slots) > 0 && slots[0].Count > 0 {
				return itemStackFromScreenSlot(slots[0]), true
			}
		}
		select {
		case <-ctx.Done():
			return models.ItemStack{}, false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return models.ItemStack{}, false
}
