package agent

import (
	"context"
	"fmt"
	"time"
)

// seedBlockOffset is how far in front of the bot (along +X, at the bot's
// own Y level) SeedNearbyBlock places a block — simple and deterministic,
// not an attempt to find a "nice" or context-appropriate spot; good enough
// for training convenience, not a real world-building capability.
const seedBlockOffset = 2

// seedGiveCount is how many units of each ingredient SeedCraftIngredients
// gives — comfortably more than any known recipe needs of a single
// ingredient, so a training episode doesn't run out mid-attempt.
const seedGiveCount = 9

// seedSyncTimeout/seedSyncPollInterval bound how long SeedNearbyBlock/
// SeedCraftIngredients wait for this bot's own client-tracked state to
// catch up with an RCON write it just made — see their doc comments for
// why this polling exists at all (found live, not anticipated: a real
// race, not a hypothetical one — testing/rl_train_test.go).
const (
	seedSyncTimeout      = 2 * time.Second
	seedSyncPollInterval = 100 * time.Millisecond
)

// SeedNearbyBlock ensures a block named blockName exists within radius
// blocks of the bot's current position, via RCON — training convenience
// only (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4's minimum-viable
// episode seeding), never used by any chat command or by CraftItem/mine's
// own real gameplay paths. Requires RCON (a.cfg.RCON) to be configured.
// A no-op if FindVisibleBlock already finds an instance within radius —
// doesn't stack an extra block onto a world that already has one.
//
// Waits (bounded by seedSyncTimeout) for the placed block to actually
// become visible to this bot's own FindVisibleBlock before returning, not
// just for the RCON command to succeed server-side. Found live via
// testing/rl_train_test.go: an immediate FindVisibleBlock call right after
// a successful RCON setblock reported nothing found — the RCON round-trip
// confirms the server placed the block, but this bot's own client still
// needs the resulting block-update packet to arrive and be processed
// separately, and that lag (tens of milliseconds, but nonzero) was enough
// to race a caller (rlenv.Environment.Reset) that checks visibility
// immediately afterward in the same call. Returns an error if the block
// never becomes visible within the timeout, rather than returning success
// on the RCON write alone and leaving the caller to discover the gap
// itself.
func (a *agent) SeedNearbyBlock(ctx context.Context, blockName string, radius int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("seed nearby block: RCON not configured for this agent")
	}
	if _, _, _, found, err := a.FindVisibleBlock(ctx, blockName, radius); err == nil && found {
		return nil
	}
	pos, ok := a.GetPositionSimple()
	if !ok {
		return fmt.Errorf("seed nearby block: position not yet known")
	}
	x, y, z := int64(pos.X)+seedBlockOffset, int64(pos.Y), int64(pos.Z)
	if _, err := a.cfg.RCON.SetBlock(ctx, x, y, z, normalizeItemName(blockName), "replace").Exec(ctx); err != nil {
		return fmt.Errorf("setblock via RCON: %w", err)
	}

	deadline := time.Now().Add(seedSyncTimeout)
	for {
		if _, _, _, found, err := a.FindVisibleBlock(ctx, blockName, radius); err == nil && found {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("seed nearby block: placed %s but it never became visible to this bot within %s", blockName, seedSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(seedSyncPollInterval):
		}
	}
}

// SeedCraftIngredients gives the bot, via RCON's "give" command, one stack
// of each distinct ingredient itemName's recipe needs — the first
// candidate per grid cell for a tag-gated ingredient (e.g. "any plank
// variant"), not an attempt to grant exactly the minimum needed. Training
// convenience only, same scope note as SeedNearbyBlock. Requires RCON to
// be configured and itemName to have a known recipe (see
// loadCraftingRecipes) — errors otherwise, since a seeding request that
// silently does nothing would be more confusing than a clear failure.
//
// Waits (bounded by seedSyncTimeout) for every given item to actually
// appear in this bot's own tracked inventory (InventoryCount) before
// returning — the same client-sync race SeedNearbyBlock's doc comment
// describes: an RCON give confirms the server granted the item, not that
// this bot's connection has yet processed the resulting
// ContainerSetSlot/SetContainerContent packet (see craft_test.go's
// waitForAgentHasItem for the same precedent, established independently
// before this function existed).
func (a *agent) SeedCraftIngredients(ctx context.Context, itemName string) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("seed craft ingredients: RCON not configured for this agent")
	}
	recipes, err := a.loadCraftingRecipes()
	if err != nil {
		return fmt.Errorf("load recipes: %w", err)
	}
	recipe, ok := recipes[normalizeItemName(itemName)]
	if !ok {
		return fmt.Errorf("no known crafting recipe for %s", itemName)
	}

	given := make(map[string]bool)
	for _, candidates := range recipe.grid {
		if len(candidates) == 0 {
			continue
		}
		candidate := candidates[0]
		if given[candidate] {
			continue
		}
		given[candidate] = true
		cmd := fmt.Sprintf("give %s %s %d", a.cfg.Name, candidate, seedGiveCount)
		if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
			return fmt.Errorf("give %s via RCON: %w", candidate, err)
		}
	}

	deadline := time.Now().Add(seedSyncTimeout)
	for candidate := range given {
		for a.InventoryCount(candidate) == 0 {
			if !time.Now().Before(deadline) {
				return fmt.Errorf("seed craft ingredients: gave %s but it never synced to this bot's inventory within %s", candidate, seedSyncTimeout)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(seedSyncPollInterval):
			}
		}
	}
	return nil
}
