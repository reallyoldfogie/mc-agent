package agent

import (
	"strings"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	"github.com/reallyoldfogie/mc-agent/models"
)

// maxDependencyDepth bounds resolveDependencies' recursion into a
// pathological or circular recipe graph — depth-bounded the same way
// resolveTag already is (see that function's doc comment), just deeper,
// since a real multi-hop ingredient chain (e.g. stick -> plank -> log) is
// expected to go several levels, not just one.
const maxDependencyDepth = 8

// ingredientRequirement is one recipe grid cell shape, aggregated across
// every cell that shares it: Candidates is the "any of these" item list a
// single grid cell holds, Count is how many such cells the recipe has.
type ingredientRequirement struct {
	Candidates []string
	Count      int
}

// recipeIngredientRequirements aggregates recipe's 3x3 grid into one
// ingredientRequirement per distinct candidate list, in first-seen grid
// order. This is the per-ingredient shape Part 2 of
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md needs on top of craft.go's
// existing per-cell grid — not a rewrite of recipeGrid/resolveIngredient,
// just a different view of the same data.
func recipeIngredientRequirements(recipe craftingRecipe) []ingredientRequirement {
	order := make([]string, 0, len(recipe.grid))
	byKey := map[string][]string{}
	counts := map[string]int{}
	for _, candidates := range recipe.grid {
		if len(candidates) == 0 {
			continue
		}
		key := strings.Join(candidates, "\x1f")
		if _, seen := counts[key]; !seen {
			order = append(order, key)
			byKey[key] = candidates
		}
		counts[key]++
	}

	reqs := make([]ingredientRequirement, 0, len(order))
	for _, key := range order {
		reqs = append(reqs, ingredientRequirement{Candidates: byKey[key], Count: counts[key]})
	}
	return reqs
}

// dependencyHaveFunc mirrors structure.MaterialList.Missing's own
// have-count-as-a-function shape (see that function's doc comment) —
// deliberately a plain function, not a concrete inventory type, so
// resolveDependencies works the same whether "have" means live inventory
// (today, via agent.InventoryCount) or something richer later.
type dependencyHaveFunc func(item string) int

// dependencyLocateFunc looks up a remembered, currently-unreserved
// location for item. Implementations must already exclude reserved
// positions before returning found=true — see Part 4 of
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md ("any ordinary
// mining/foraging logic should treat a reserved position as off-limits for
// material-sourcing purposes").
type dependencyLocateFunc func(item string) (pos models.V3, inContainer bool, found bool)

// ResolveDependencies walks itemName's full crafting-ingredient tree, per
// Part 2 of docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md: "I need to craft
// A... I remember seeing B... C in a chest... D I don't know where." For
// each ingredient, in order: live inventory, then the persistent
// world-memory store for a remembered and unreserved location, then
// recursing into that ingredient's own recipe if it has one, and otherwise
// reporting it as an unknown-source raw material to go gather.
//
// Built as a new layer on top of agent/craft.go's existing single-level
// ingredient resolution (loadCraftingRecipes/recipeIngredientRequirements)
// — this does not change how one recipe's grid gets filled, only what
// happens with the result. Deliberately approximate in the same way
// Craftable already is: each ingredient requirement is checked
// independently, so a recipe needing the same scarce item across two
// different requirements can double-count it as available. Good enough
// for "where is everything I need," not a reservation-across-requirements
// planner.
func (a *agent) ResolveDependencies(itemName string, needed int) (models.DependencyNode, error) {
	recipes, err := a.loadCraftingRecipes()
	if err != nil {
		return models.DependencyNode{}, err
	}
	return resolveDependencies(recipes, a.InventoryCount, a.locateUnreserved, needed, normalizeItemName(itemName), 0), nil
}

// locateUnreserved looks up item's nearest remembered sighting and
// excludes it if that position has already been claimed by a build — see
// Part 4's "treat a reserved position as off-limits." found=false (never
// an error) if there's no world-memory store, no remembered sighting, or
// the only one found is reserved — mirrors InventoryCount's
// never-errors-just-reports-the-empty-case contract.
func (a *agent) locateUnreserved(item string) (pos models.V3, inContainer bool, found bool) {
	if a.worldMemory == nil {
		return models.V3{}, false, false
	}

	from := memorystore.Position{}
	if p, ok := a.GetPositionSimple(); ok {
		from = memorystore.Position{X: int(p.X), Y: int(p.Y), Z: int(p.Z)}
	}

	sighting, ok, err := a.worldMemory.NearestSighting(item, from, memorystore.SightingQuery{})
	if err != nil || !ok {
		return models.V3{}, false, false
	}
	if reserved, err := a.worldMemory.IsReserved(sighting.Position); err != nil || reserved {
		return models.V3{}, false, false
	}

	loc := models.V3{X: float64(sighting.Position.X), Y: float64(sighting.Position.Y), Z: float64(sighting.Position.Z)}
	return loc, sighting.Container != nil, true
}

// resolveDependencies is ResolveDependencies' testable core, taking every
// dependency as a plain function/map instead of reaching into *agent
// directly — same rationale as buildStructureWith/structurePlacer.
// depth guards against a pathological/circular recipe graph; past
// maxDependencyDepth, remaining ingredients are reported unknown-source
// rather than recursing further.
func resolveDependencies(recipes map[string]craftingRecipe, have dependencyHaveFunc, locate dependencyLocateFunc, needed int, itemName string, depth int) models.DependencyNode {
	node := models.DependencyNode{Item: itemName, Needed: needed}

	if have(itemName) >= needed {
		node.Source = models.SourceInventory
		return node
	}

	if pos, inContainer, ok := locate(itemName); ok {
		node.Source = models.SourceMemory
		node.LocationPos = pos
		node.InContainer = inContainer
		return node
	}

	recipe, hasRecipe := recipes[itemName]
	if !hasRecipe || depth >= maxDependencyDepth {
		node.Source = models.SourceUnknown
		return node
	}

	node.Source = models.SourceCraft
	for _, req := range recipeIngredientRequirements(recipe) {
		node.Children = append(node.Children, resolveRequirement(recipes, have, locate, req, depth+1))
	}
	return node
}

// resolveRequirement resolves one ingredient requirement (an "any of
// these candidates" set plus how many units are needed) into a single
// child DependencyNode: the first candidate inventory alone already
// covers, else the first candidate with a remembered unreserved location,
// else the first candidate with its own known recipe (recursed into), else
// the requirement's first candidate reported unknown-source. Picking
// "first candidate" rather than evaluating every candidate's own full
// subtree is the same simplification Craftable already makes for
// candidate lists.
func resolveRequirement(recipes map[string]craftingRecipe, have dependencyHaveFunc, locate dependencyLocateFunc, req ingredientRequirement, depth int) models.DependencyNode {
	for _, candidate := range req.Candidates {
		if have(candidate) >= req.Count {
			return models.DependencyNode{Item: candidate, Needed: req.Count, Source: models.SourceInventory}
		}
	}
	for _, candidate := range req.Candidates {
		if pos, inContainer, ok := locate(candidate); ok {
			return models.DependencyNode{Item: candidate, Needed: req.Count, Source: models.SourceMemory, LocationPos: pos, InContainer: inContainer}
		}
	}
	for _, candidate := range req.Candidates {
		if _, ok := recipes[candidate]; ok {
			return resolveDependencies(recipes, have, locate, req.Count, candidate, depth)
		}
	}
	return models.DependencyNode{Item: req.Candidates[0], Needed: req.Count, Source: models.SourceUnknown}
}
