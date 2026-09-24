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
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"
)

// inventoryGridSize is the player-inventory crafting grid's width/height (a
// 2x2 grid, window 0 slots 1-4 - see agent/adapters.go's window-0 layout
// comment: "0=craft output, 1-4=craft grid"). tableGridSize is a real
// crafting table's 3x3 grid (window type 12, slots 1-9 - see
// craftingTableLayout). Every craftingRecipe is stored as a 3x3
// (tableGridSize) row-major grid regardless of which layout it ends up
// executing against; fitsInventoryGrid decides whether the smaller layout
// suffices (see docs/plans/CRAFTING_TABLE_3X3_PLAN.md).
const (
	inventoryGridSize = 2
	tableGridSize     = 3
)

const (
	craftingShapedType    = "minecraft:crafting_shaped"
	craftingShapelessType = "minecraft:crafting_shapeless"
)

// craftOutputTimeout bounds how long CraftItem waits for the server to
// populate the result slot after ingredients are placed.
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
//
// A crafting table window has its own, larger inventory-start offset - see
// craftTableContainerSlots - so this constant is only ever used via
// inventoryGridLayout, never hardcoded elsewhere.
const craftIngredientSlotStart = 9

// findIngredientSlot searches a window's main inventory + hotbar (slots
// layout.inventoryStart and up) for itemName - mirroring FindSlotWith's
// matching logic, but scoped to that range; see craftIngredientSlotStart's
// doc comment for why FindSlotWith itself isn't safe to reuse here. Reads
// via craftWindowSlots (not a.GetInventory() directly) so the search looks
// at the right window whether it's the player's own or a crafting table's.
func (a *agent) findIngredientSlot(itemName string, layout craftWindowLayout) (int, bool, error) {
	slots, err := a.craftWindowSlots(layout)
	if err != nil {
		return -1, false, err
	}
	a.itemMgrMu.RLock()
	itemMgr := a.itemMgr
	a.itemMgrMu.RUnlock()
	if itemMgr == nil {
		return -1, false, fmt.Errorf("item manager not initialized")
	}

	normalized := normalizeItemName(itemName)
	for idx := layout.inventoryStart; idx < len(slots); idx++ {
		if slots[idx].Count <= 0 {
			continue
		}
		if normalizeItemName(itemMgr.GetItemNameByID(int32(slots[idx].ID))) == normalized {
			return idx, true, nil
		}
	}
	return -1, false, nil
}

// craftingRecipe is a recipe resolved from cached datapack JSON (see
// loadCraftingRecipes) into exactly what CraftItem needs: a 3x3
// (tableGridSize), row-major grid of ingredient candidates - grid[row*3+col]
// - for each of tableGridSize*tableGridSize possible slots, the list of item
// names that would satisfy that position (a single name for a
// concrete-item ingredient, several for a tag reference, e.g.
// "#minecraft:planks" resolves to every plank variant - see
// resolveIngredient). A nil/empty entry means that grid cell must stay
// empty for this recipe.
//
// Shaped recipes populate cells at their real pattern position. Shapeless
// recipes have no real position - resolveIngredient's caller (recipeGrid)
// packs them via shapelessGridSlotOrder so that a shapeless recipe with 4 or
// fewer ingredients still lands entirely in the top-left 2x2 sub-region
// (see fitsInventoryGrid) rather than spilling into column/row 2 by
// coincidence of iteration order.
//
// fitsInventory is computed once, when the recipe is built (see
// loadCraftingRecipes / fitsInventoryGrid), rather than recomputed on every
// fitsInventoryGrid() call.
type craftingRecipe struct {
	grid          [tableGridSize * tableGridSize][]string
	fitsInventory bool
}

// fitsInventoryGrid reports whether this recipe can be crafted using the
// player's own 2x2 inventory grid (window 0) rather than needing a real
// crafting table's 3x3 grid.
func (r craftingRecipe) fitsInventoryGrid() bool {
	return r.fitsInventory
}

// fitsInventoryGrid reports whether every populated cell of a raw 3x3
// recipe grid falls within the top-left 2x2 sub-region (row < 2 && col < 2
// in the row-major tableGridSize indexing) - a pure function of the grid's
// shape, not the recipe it came from, so unit tests can call it directly
// against recipeGrid's output.
func fitsInventoryGrid(grid [tableGridSize * tableGridSize][]string) bool {
	for i, candidates := range grid {
		if len(candidates) == 0 {
			continue
		}
		row, col := i/tableGridSize, i%tableGridSize
		if row >= inventoryGridSize || col >= inventoryGridSize {
			return false
		}
	}
	return true
}

// shapelessGridSlotOrder maps a shapeless recipe's Nth ingredient (0-based -
// order is otherwise irrelevant for shapeless matching) to a grid index.
// The first 4 ingredients land on the top-left 2x2 sub-region (row-major:
// (0,0),(0,1),(1,0),(1,1)) so a shapeless recipe with 4 or fewer ingredients
// satisfies fitsInventoryGrid; the remaining 5 fill out the rest of the 3x3
// grid row-major (row 0 col 2, then row 2), reachable only via a crafting
// table.
var shapelessGridSlotOrder = [tableGridSize * tableGridSize]int{0, 1, 3, 4, 2, 5, 6, 7, 8}

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

// ingredientListPrefix/ingredientListSeparator encode a recipe
// ingredient's third on-disk schema variant - a raw JSON array of
// concrete item alternatives (e.g. torch.json's own
// "X": ["minecraft:coal", "minecraft:charcoal"], vanilla's "any of these
// items" ingredient form, distinct from both a single item id and a
// "#namespace:tag" tag reference) - into one ingredientRef string, so the
// rest of this file's single-descriptor-per-grid-cell pipeline
// (rawRecipeJSON.Key/Ingredients -> resolveIngredient) doesn't need a
// parallel []string case threaded through it. resolveIngredient splits
// a descriptor with this prefix back apart on the separator rather than
// treating the joined string as one (unresolvable) item id or (malformed)
// tag reference. Neither character can appear in a real item id or tag
// reference (both are restricted to [a-z0-9_.-] plus ':' and a leading
// '#'), so there's no collision risk with a genuine descriptor.
//
// Found live: ingredientRef.UnmarshalJSON only handled the flat-string and
// {item,tag}-object variants, so torch.json's array-form "X" key failed
// json.Unmarshal(raw, &rj) outright - loadCraftingRecipes' per-file
// best-effort `continue` on that error then silently dropped the entire
// recipe, not just that one ingredient, surfacing as "no known crafting
// recipe for minecraft:torch" despite the file being valid and present.
const (
	ingredientListPrefix    = "$"
	ingredientListSeparator = "\x1f"
)

// ingredientRef is one recipe ingredient descriptor, decoded from any of
// three on-disk schema variants observed across this codebase's own cached
// recipe JSON for otherwise-identical or sibling recipes: a flat string
// ("minecraft:iron_ingot", or the tag-reference form "#minecraft:planks") -
// used from version 1.21.2 onward - an older object form
// ({"item": "minecraft:iron_ingot"} or {"tag": "minecraft:planks"}) - the
// only form seen in 1.21.1's cache - or a raw array of alternative item ids
// (see ingredientListPrefix). Confirmed by direct comparison of the same
// recipe (e.g. stick.json) across versions, not assumed. Normalizes to the
// same flat-string shape resolveIngredient already expects either way, so
// the rest of this file doesn't need to know which variant it came from.
type ingredientRef string

func (r *ingredientRef) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*r = ingredientRef(s)
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*r = ingredientRef(ingredientListPrefix + strings.Join(list, ingredientListSeparator))
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

// loadCraftingRecipes returns an index of craftable recipes (2x2
// inventory grid or 3x3 crafting table - see fitsInventoryGrid) keyed by
// normalized result item name, loading and parsing every cached recipe
// JSON file for the connected version on the *first* call and reusing
// that result (via craftingRecipesOnce) for every call after - the
// underlying files never change during one connected session, so this is
// a pure, behavior-preserving cache, not a semantic change.
//
// Was previously reloaded from disk and re-parsed on every single
// CraftItem/SeedCraftIngredients call ("a few hundred small JSON files,
// acceptable for a command that isn't called in a tight loop" per this
// function's own earlier doc comment) - found live (2026-09-23,
// cmd/rsi-train's -parallel-envs x tick-rate scaling investigation) to
// actually be 1,373 files for 1.21.5's cached data_generator output, and
// very much in a tight loop once curriculum-driven training calls this
// repeatedly across several concurrently-running agents - real, avoidable
// I/O contention that plausibly contributed to a separate finding from
// the same investigation (bursty, multi-agent-synchronized pathfinding
// failures, consistent with system-wide I/O/scheduler pressure during a
// craft-heavy burst).
func (a *agent) loadCraftingRecipes() (map[string]craftingRecipe, error) {
	a.craftingRecipesOnce.Do(func() {
		a.craftingRecipesCache, a.craftingRecipesCacheErr = a.loadCraftingRecipesUncached()
	})
	return a.craftingRecipesCache, a.craftingRecipesCacheErr
}

// loadCraftingRecipesUncached does the actual disk read/parse work
// loadCraftingRecipes now only performs once per agent - see that
// function's own doc comment. Only minecraft:crafting_shaped/
// crafting_shapeless recipes that fit within a 3x3 grid are indexed -
// smelting/stonecutting/smithing/etc., and dynamic crafting_special_*
// recipes (armor dye, book cloning, ...), are out of scope entirely (see
// docs/plans/CRAFTING_TABLE_3X3_PLAN.md's cross-cutting notes). If
// multiple recipes produce the same result item, only the first one
// encountered (directory iteration order, not otherwise meaningful) is
// kept.
func (a *agent) loadCraftingRecipesUncached() (map[string]craftingRecipe, error) {
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
		recipes[resultItem] = craftingRecipe{grid: grid, fitsInventory: fitsInventoryGrid(grid)}
	}
	return recipes, nil
}

// recipeGrid resolves one parsed recipe into 3x3 (tableGridSize) grid-slot
// ingredient candidates, or ok=false if it isn't a
// crafting_shaped/crafting_shapeless recipe that fits in a 3x3 grid (or
// references an ingredient that can't be resolved at all - e.g. a tag file
// missing from the cache). See shapelessGridSlotOrder for how a shapeless
// recipe's position-independent ingredients are packed into the grid.
func recipeGrid(dataDir string, rj rawRecipeJSON, tagCache map[string][]string) (grid [tableGridSize * tableGridSize][]string, ok bool) {
	switch rj.Type {
	case craftingShapedType:
		if len(rj.Pattern) == 0 || len(rj.Pattern) > tableGridSize {
			return grid, false
		}
		for _, row := range rj.Pattern {
			if len(row) > tableGridSize {
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
				grid[r*tableGridSize+c] = candidates
			}
		}
		return grid, true

	case craftingShapelessType:
		if len(rj.Ingredients) == 0 || len(rj.Ingredients) > tableGridSize*tableGridSize {
			return grid, false
		}
		for i, descriptor := range rj.Ingredients {
			candidates, err := resolveIngredient(dataDir, string(descriptor), tagCache)
			if err != nil || len(candidates) == 0 {
				return grid, false
			}
			grid[shapelessGridSlotOrder[i]] = candidates
		}
		return grid, true

	default:
		return grid, false
	}
}

// resolveIngredient turns one recipe ingredient descriptor - a concrete
// item id ("minecraft:iron_ingot"), a tag reference ("#minecraft:planks"),
// or an inline list of item alternatives (see ingredientListPrefix) - into
// the list of concrete item names that would satisfy it.
func resolveIngredient(dataDir, descriptor string, tagCache map[string][]string) ([]string, error) {
	if strings.HasPrefix(descriptor, ingredientListPrefix) {
		items := strings.Split(strings.TrimPrefix(descriptor, ingredientListPrefix), ingredientListSeparator)
		var resolved []string
		for _, item := range items {
			if n := normalizeItemName(item); n != "" {
				resolved = append(resolved, n)
			}
		}
		if len(resolved) == 0 {
			return nil, fmt.Errorf("empty ingredient list")
		}
		return resolved, nil
	}
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

// craftWindowLayout describes one crafting-capable window's slot numbering,
// so the placement/collection logic in executeCraft can be shared between
// the player's own 2x2 grid (always window 0) and an opened crafting
// table's 3x3 grid (a dynamically-assigned window). windowID is which
// window craftWindowSlots reads from - see its doc comment for why this
// matters beyond just which window MoveSingle's clicks target.
type craftWindowLayout struct {
	windowID       byte
	outputSlot     int16
	gridSlotStart  int16
	gridWidth      int
	gridHeight     int
	inventoryStart int
}

// craftWindowSlots returns the raw slot contents for layout's window.
// a.GetInventory() (agent/subsystems.go) always returns the player's own
// window-0 inventory regardless of what's actually open - correct for
// inventoryGridLayout, but wrong for an opened crafting table's window, a
// completely separate tracked object with its own slot numbering.
//
// Found live: CraftItem's table path was silently reading window 0's own
// tracked slots for ingredient search *and* output-slot polling (both
// previously called a.GetInventory() unconditionally) while sending its
// actual placement clicks to the table's real window. Ingredient search
// still "worked" (window 0 happens to carry the same 36 physical
// main+hotbar slots, just starting at index 9 instead of the table
// window's 10, so a scan starting from either offset still finds
// something), so placeCraftIngredient never errored - but every slot index
// it handed to MoveSingle was one off from where that item actually lives
// in the table's real window, so ingredients landed in the wrong grid
// cells: no recipe matched, and waitForCraftOutput (also reading window 0)
// could never have seen a real result even if one had appeared. Symptom:
// "grid did not produce a result" despite every ingredient being found.
//
// Uses GenericContainer.GetSlots(), not its Slots field directly: the
// field has the same no-defensive-copy exposure the player-inventory type
// had before mc-bot-go v0.2.1 (GenericContainer wasn't part of that fix) -
// caught live via `go test -race` the first time this function actually
// ran against a real server (WARNING: DATA RACE between this read and
// GenericContainer.OnSetSlot), fixed the same way as a mc-bot-go follow-up.
func (a *agent) craftWindowSlots(layout craftWindowLayout) ([]mcscreen.Slot, error) {
	if layout.windowID == 0 {
		inv := a.GetInventory()
		if inv == nil {
			return nil, fmt.Errorf("inventory not available")
		}
		return inv.GetSlots(), nil
	}
	scr := a.GetScreen(int(layout.windowID))
	if scr == nil {
		return nil, fmt.Errorf("window %d not open", layout.windowID)
	}
	container, ok := scr.(*mcscreen.GenericContainer)
	if !ok {
		return nil, fmt.Errorf("window %d is not a crafting-table-shaped container (got %T)", layout.windowID, scr)
	}
	return container.GetSlots(), nil
}

// inventoryGridLayout is window 0's always-present 2x2 crafting grid -
// today's CraftItem behavior, unchanged.
var inventoryGridLayout = craftWindowLayout{
	windowID:       0,
	outputSlot:     0,
	gridSlotStart:  1,
	gridWidth:      inventoryGridSize,
	gridHeight:     inventoryGridSize,
	inventoryStart: craftIngredientSlotStart,
}

// craftTableContainerSlots is a crafting table window's own slot count (1
// output + 9 grid slots) before the player's 36-slot inventory section
// begins - confirmed against mc-bot-go's window-type registry
// (bot/screen/generic_container.go, type 12: "crafting", 10
// container-specific slots) and asserted live in
// testing/container_suite_crafting_test.go.
const craftTableContainerSlots = 10

// craftingTableLayout describes an opened crafting table's 3x3 grid at
// windowID (the window ID the server assigned this particular table) -
// slot numbering itself is fixed, only windowID varies per call.
func craftingTableLayout(windowID byte) craftWindowLayout {
	return craftWindowLayout{
		windowID:       windowID,
		outputSlot:     0,
		gridSlotStart:  1,
		gridWidth:      tableGridSize,
		gridHeight:     tableGridSize,
		inventoryStart: craftTableContainerSlots,
	}
}

// craftTableSearchRadius bounds how far CraftItem looks for a nearby
// crafting table when a recipe doesn't fit the 2x2 inventory grid - mirrors
// "mine <blockName>"'s mineSearchRadius (actions/commands.go).
const craftTableSearchRadius = 32

// craftTableOpenTimeout bounds how long opening the crafting table window
// may take - mirrors the 5s timeout convention used by other
// OpenContainer/OpenContainerAt call sites (see
// testing/container_suite_test.go's openContainer helper).
const craftTableOpenTimeout = 5 * time.Second

// interactPositionAttempts bounds how many candidate standing spots
// openCraftingTable tries walking to before reporting the table unreachable.
const interactPositionAttempts = 5

// openCraftingTable finds the nearest visible crafting table, walks to it
// if needed, opens it, and returns the craftWindowLayout for its 3x3 grid.
// Callers are responsible for CloseContainer once done (success or
// failure) - that already resets the window to 0 (see
// agent.CloseContainer's doc comment).
//
// No automatic crafting-table placement: if none is found, this errors
// rather than placing one from inventory, even if the agent is carrying one
// (see docs/plans/CRAFTING_TABLE_3X3_PLAN.md's cross-cutting notes).
func (a *agent) openCraftingTable(ctx context.Context) (craftWindowLayout, error) {
	x, y, z, found, err := a.FindVisibleBlock(ctx, "minecraft:crafting_table", craftTableSearchRadius)
	if err != nil {
		return craftWindowLayout{}, fmt.Errorf("find crafting table: %w", err)
	}
	if !found {
		return craftWindowLayout{}, fmt.Errorf("no crafting table found within %d blocks", craftTableSearchRadius)
	}

	// Mirrors pickUpNearbyItem's find-then-walk two-step (and
	// agent/plan/steps.go's FindChest/FindBlock with MoveToTarget=true):
	// FindVisibleBlock's search radius is much larger than interaction
	// range, and OpenContainer itself has no distance-closing logic of its
	// own.
	//
	// Unlike a dropped item (an entity, not a block), the crafting table
	// itself is solid - MoveTo can never get within its ~0.5-block goal
	// radius of the table's own coordinates, since that would mean standing
	// inside it. Walking straight to (x, y, z) here made A* search
	// exhaustively outward (never converging - see the "distToGoal keeps
	// growing" progress-log evidence in docs/bugs/hpa-star-slowness) until
	// it exhausted its step budget, every single time, regardless of how
	// close the table actually was. FindInteractPosition finds a walkable,
	// line-of-sight-verified position near the table instead.
	target := models.V3{X: x, Y: y, Z: z}
	// Walkable + line-of-sight doesn't imply reachable: the closest spot to
	// a table floating two cells above the floor is standing on top of it,
	// which A* correctly refuses to path to (found live: 100 identical
	// failed attempts in a row). Try the next-closest spots before giving up.
	moveTargets, _ := models.FindInteractPositions(ctx, a, target, interactPositionAttempts)
	if len(moveTargets) == 0 {
		moveTargets = []models.V3{target}
	}
	var moveErr error
	for _, moveTarget := range moveTargets {
		if moveErr = a.MoveToWithChat(ctx, moveTarget.X, moveTarget.Y, moveTarget.Z); moveErr == nil {
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	if moveErr != nil {
		return craftWindowLayout{}, fmt.Errorf("move to crafting table: %w", moveErr)
	}

	windowID, err := a.OpenContainerAt(ctx, x, y, z, models.FaceUp, craftTableOpenTimeout)
	if err != nil {
		return craftWindowLayout{}, fmt.Errorf("open crafting table: %w", err)
	}
	a.SetWindow(windowID)
	return craftingTableLayout(windowID), nil
}

// CraftItem crafts itemName using whichever crafting surface its recipe
// needs: the player's own 2x2 inventory grid (window 0, slots 1-4, no
// crafting table needed) when the recipe fits it, or a nearby crafting
// table's 3x3 grid otherwise (see openCraftingTable). Looks up a matching
// recipe (see loadCraftingRecipes), moves one of each required ingredient
// from the main inventory into the grid, waits for the server to populate
// the result slot, then shift-clicks it to collect the crafted item.
//
// A recipe that fits the 2x2 grid always prefers it, even if a table
// happens to be open/available: opening a container involves several
// real-time waits (see agent.OpenContainer's doc comment), so the
// zero-container-open path is the sensible default (see
// docs/plans/CRAFTING_TABLE_3X3_PLAN.md's Phase 3 design note).
//
// On a missing-ingredient failure partway through, whatever was already
// placed into the grid is left there rather than moved back - matches what
// a player fumbling a recipe by hand would see, and keeps this from needing
// rollback machinery.
func (a *agent) CraftItem(ctx context.Context, itemName string) error {
	normalized := normalizeItemName(itemName)

	recipes, err := a.loadCraftingRecipes()
	if err != nil {
		return fmt.Errorf("load recipes: %w", err)
	}
	recipe, ok := recipes[normalized]
	if !ok {
		return fmt.Errorf("no known crafting recipe for %s", itemName)
	}

	beforeCount := a.InventoryCount(itemName)

	if recipe.fitsInventoryGrid() {
		if err := a.executeCraft(ctx, itemName, recipe, inventoryGridLayout); err != nil {
			return err
		}
	} else {
		layout, err := a.openCraftingTable(ctx)
		if err != nil {
			return fmt.Errorf("craft %s: %w", itemName, err)
		}
		err = a.executeCraft(ctx, itemName, recipe, layout)
		a.CloseContainer()
		if err != nil {
			return err
		}
	}

	return a.awaitInventoryIncrease(ctx, itemName, beforeCount)
}

// craftConfirmTimeout/craftConfirmPollInterval bound how long
// awaitInventoryIncrease waits, after executeCraft's final ShiftClickSlot has
// been sent, for this bot's own client-tracked inventory (InventoryCount) to
// actually reflect the crafted item landing there — the same client-sync
// race SeedNearbyBlock's doc comment describes (agent/rl_seed.go) and
// MineBlockAt's awaitBlockChanged now guards against (agent/actions.go),
// here for the resulting ContainerSetSlot/SetContainerContent packet instead
// of a BlockChange one. Without this, CraftItem returned "success" the
// instant it sent the shift-click, before the server's confirmation had
// round-tripped back to this bot's own inventory state — found live via
// testing/rl_train_test.go's TestRLTrainingLoop_CraftTaskSeedingEarnsRewardAndEndsEpisode,
// which needed exactly one retry to register `craftedThisStep`, 5/5 runs
// (100%), before this fix (docs/plans/RL_TRAINING_LOOP_PLAN.md's Status
// section flagged this as the next place to check once the mine-task
// equivalent was found and fixed).
const (
	craftConfirmTimeout      = 2 * time.Second
	craftConfirmPollInterval = 50 * time.Millisecond
)

// awaitInventoryIncrease waits (bounded by craftConfirmTimeout) for
// a.InventoryCount(itemName) to read higher than beforeCount, mirroring
// MineBlockAt's awaitBlockChanged and SeedNearbyBlock's own confirmation
// wait. Returns an error if the increase never registers within the
// timeout, rather than silently returning success on the shift-click alone
// and leaving the caller to discover the gap itself.
func (a *agent) awaitInventoryIncrease(ctx context.Context, itemName string, beforeCount int) error {
	deadline := time.Now().Add(craftConfirmTimeout)
	for {
		if a.InventoryCount(itemName) > beforeCount {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("craft %s: inventory increase never confirmed within %s", itemName, craftConfirmTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(craftConfirmPollInterval):
		}
	}
}

// executeCraft runs recipe's placement/collection sequence against layout -
// shared between the player's 2x2 inventory grid and an opened crafting
// table's 3x3 grid, so both run the exact same code with different slot
// numbers rather than two parallel copies.
func (a *agent) executeCraft(ctx context.Context, itemName string, recipe craftingRecipe, layout craftWindowLayout) error {
	for i, candidates := range recipe.grid {
		if len(candidates) == 0 {
			continue
		}
		row, col := i/tableGridSize, i%tableGridSize
		if row >= layout.gridHeight || col >= layout.gridWidth {
			// CraftItem only ever selects a layout the recipe's
			// fitsInventoryGrid() result already confirmed fits - a
			// populated cell outside layout's bounds would mean that
			// invariant broke.
			return fmt.Errorf("craft %s: recipe cell (%d,%d) does not fit a %dx%d grid", itemName, row, col, layout.gridWidth, layout.gridHeight)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		gridSlot := layout.gridSlotStart + int16(row*layout.gridWidth+col)
		if err := a.placeCraftIngredient(ctx, gridSlot, layout, candidates); err != nil {
			return fmt.Errorf("craft %s: %w", itemName, err)
		}
	}

	result, ok := a.waitForCraftOutput(ctx, layout, craftOutputTimeout)
	if !ok {
		return fmt.Errorf("craft %s: grid did not produce a result (ingredients may not actually match the recipe)", itemName)
	}
	if err := a.ShiftClickSlot(layout.outputSlot, result); err != nil {
		return fmt.Errorf("collect crafted %s: %w", itemName, err)
	}
	return nil
}

// ingredientSearchTimeout/ingredientSearchPollInterval bound how long
// placeCraftIngredient retries its inventory scan before giving up -
// mirrors awaitInventoryIncrease/SeedNearbyBlock's own poll-until-synced
// idiom. Needed because opening a crafting table's window
// (agent.OpenContainer) only waits a blind, fixed 2s for the server's
// resulting slot data to arrive before returning - not an event-driven
// wait keyed on that data actually landing - so a single, immediate scan
// can lose the race against the container's ClientboundContainerSetContent
// under load (multiple concurrent bots sharing one server) and see an
// empty/stale container (or, per findIngredientSlot -> craftWindowSlots,
// a window not yet registered as open) even though the ingredient really
// is there. Found live: hundreds of "missing ingredient"/"window N not
// open" failures against real-crafting-table recipes (chest/bowl) that
// never happened against the 2x2 inventory-only path (window 0, no
// container-open race to lose).
const (
	ingredientSearchTimeout      = 2 * time.Second
	ingredientSearchPollInterval = 100 * time.Millisecond
)

// placeCraftIngredient finds the first inventory item matching one of
// candidates (searching layout's window from layout.inventoryStart onward -
// see findIngredientSlot) and moves a single unit of it into gridSlot
// (also in layout's window - see craftWindowSlots). Retries the whole scan
// (bounded by ingredientSearchTimeout) rather than searching once - see
// that constant's own doc comment for why a single immediate scan isn't
// reliable here, unlike a plain 2x2-grid craft against window 0.
func (a *agent) placeCraftIngredient(ctx context.Context, gridSlot int16, layout craftWindowLayout, candidates []string) error {
	deadline := time.Now().Add(ingredientSearchTimeout)
	var lastErr error
	for {
		for _, candidate := range candidates {
			if err := ctx.Err(); err != nil {
				return err
			}
			srcSlot, found, err := a.findIngredientSlot(candidate, layout)
			if err != nil {
				lastErr = fmt.Errorf("search inventory for %s: %w", candidate, err)
				continue
			}
			if !found {
				continue
			}

			slots, err := a.craftWindowSlots(layout)
			if err != nil {
				lastErr = err
				continue
			}
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
		if !time.Now().Before(deadline) {
			if lastErr != nil {
				return lastErr
			}
			return fmt.Errorf("missing ingredient (need one of: %s)", strings.Join(candidates, ", "))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ingredientSearchPollInterval):
		}
	}
}

// waitForCraftOutput polls layout's output slot (in layout's window - see
// craftWindowSlots) until it becomes non-empty or timeout elapses. Doesn't
// check the item matches the expected recipe result: the result slot in a
// real crafting UI only ever shows a valid output for the grid's current
// contents (or stays empty), so "non-empty" is already a sufficient,
// simpler signal than resolving an item name back to an ID just to compare.
func (a *agent) waitForCraftOutput(ctx context.Context, layout craftWindowLayout, timeout time.Duration) (models.ItemStack, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if slots, err := a.craftWindowSlots(layout); err == nil {
			outputSlot := layout.outputSlot
			if int(outputSlot) < len(slots) && slots[outputSlot].Count > 0 {
				return itemStackFromScreenSlot(slots[outputSlot]), true
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

// InventoryCount returns how many units of itemName the player currently
// holds in the main inventory + hotbar (window 0, slots
// craftIngredientSlotStart and up) — never the 2x2 crafting grid, output, or
// armor slots (0-8), since an item sitting in the output slot or a partially
// placed ingredient isn't "held" any more than one still in a chest would
// be. 0 if the inventory or item manager isn't available, or nothing
// matches — mirrors BlockNameAt's "never errors, just reports the empty
// case" contract (models.WorldOperations, agent/actions.go), since rlenv
// only needs an approximate signal here (see Craftable), not fine-grained
// failure diagnosis.
//
// Used by rlenv's craft reward to judge "did crafting actually produce more
// of the target item" from an inventory-count delta — the craft analogue of
// how BlockNameAt's before/after diff judges mine (see
// docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1d).
func (a *agent) InventoryCount(itemName string) int {
	inv := a.GetInventory()
	if inv == nil {
		return 0
	}
	a.itemMgrMu.RLock()
	itemMgr := a.itemMgr
	a.itemMgrMu.RUnlock()
	if itemMgr == nil {
		return 0
	}

	normalized := normalizeItemName(itemName)
	slots := inv.GetSlots()
	count := 0
	for idx := craftIngredientSlotStart; idx < len(slots); idx++ {
		if slots[idx].Count <= 0 {
			continue
		}
		if normalizeItemName(itemMgr.GetItemNameByID(int32(slots[idx].ID))) == normalized {
			count += int(slots[idx].Count)
		}
	}
	return count
}

// Craftable reports an approximate "does the player currently hold at least
// one of each required ingredient" check for itemName's recipe — used as
// rlenv's craftReady observation bit (docs/plans/RL_TRAINING_LOOP_PLAN.md
// Phase 1c) so a policy can learn when attempting the craft action is even
// worth trying, the same role FindVisibleBlock's result plays for mine's
// mineVisible bit.
//
// Deliberately approximate, not a guarantee CraftItem would actually
// succeed: it checks each grid cell's ingredient candidates independently
// via InventoryCount rather than reserving counts across cells, so a recipe
// needing two units of the *same* scarce item when only one is held reports
// true even though CraftItem would still fail partway through placement.
// Acceptable for a coarse "worth trying" signal; CraftItem's own real
// placement/error handling remains the source of truth for whether a craft
// actually succeeds. false if itemName has no known recipe or the recipe
// cache can't be read — mirrors InventoryCount's no-error contract.
func (a *agent) Craftable(itemName string) bool {
	recipes, err := a.loadCraftingRecipes()
	if err != nil {
		return false
	}
	recipe, ok := recipes[normalizeItemName(itemName)]
	if !ok {
		return false
	}
	for _, candidates := range recipe.grid {
		if len(candidates) == 0 {
			continue
		}
		found := false
		for _, candidate := range candidates {
			if a.InventoryCount(candidate) > 0 {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
