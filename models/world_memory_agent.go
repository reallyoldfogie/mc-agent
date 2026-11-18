package models

import "context"

// WorldMemoryAgent is the command-level capability surface for acting
// directly on the persistent world-memory store
// (docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md): go mine a remembered
// block, or go fetch a remembered item from a chest — as opposed to the
// store merely informing an existing action's observation guidance
// (the craft/place/mine memory fallbacks), which needed no new command
// at all.
type WorldMemoryAgent interface {
	// MineRememberedBlock looks up blockName's nearest remembered,
	// unreserved, loose (non-container) sighting, navigates there, and
	// attempts to mine it. found=false if nothing is remembered.
	MineRememberedBlock(ctx context.Context, blockName string) (found bool, err error)

	// FetchRememberedItem looks up itemName's nearest remembered,
	// unreserved container sighting, navigates there, opens the
	// container, takes the item into inventory, and closes it again.
	// found=false if nothing is remembered.
	FetchRememberedItem(ctx context.Context, itemName string) (found bool, err error)

	// ResolveDependencies walks itemName's full crafting-ingredient tree
	// (live inventory, then remembered-and-unreserved locations, then
	// recursing into known sub-recipes) — the read-only "where is
	// everything I need for this" query the chat-facing craftPlan
	// command (actions/commands.go) prints, as opposed to
	// MineRememberedBlock/FetchRememberedItem, which each act on a
	// single resolved location rather than reporting the whole tree.
	ResolveDependencies(itemName string, needed int) (DependencyNode, error)
}
