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

// teleportSyncTimeout/teleportSyncPollInterval bound how long TeleportTo
// waits for this bot's own client-tracked position to catch up with an
// RCON teleport it just issued — the same client-sync race
// SeedNearbyBlock's doc comment describes, here for a
// player-position-update packet instead of a block-update one.
// teleportSyncThreshold is how close (in blocks) the tracked position
// must land to the requested destination to count as "arrived" — not
// exact float equality, since the server may round/clamp the landing
// spot (e.g. to solid ground).
const (
	teleportSyncTimeout      = 2 * time.Second
	teleportSyncPollInterval = 100 * time.Millisecond
	teleportSyncThreshold    = 0.5
)

// chunkSyncTimeout/chunkSyncPollInterval bound how long TeleportTo waits,
// after the position itself has synced, for the destination chunk's block
// data to arrive. Position sync (above) only confirms a
// player-position-update packet landed; it says nothing about whether the
// server has sent chunk data for the new area yet, which a teleport into an
// unvisited/far-away region can easily outrun. Found live via rsi-trainer:
// Environment.Reset's reachability checks (rlenv/walkability.go) run
// immediately after TeleportTo returns, and a pathfinding search launched
// over a not-yet-loaded chunk correctly (if unhelpfully) reports zero
// possible moves via World.GetBlockAt's own loaded=false - this was
// previously masked because slower pathfinding (pre block-cache/EPEA*
// fixes, see pathfinding/movement.go) accidentally gave the chunk enough
// wall-clock time to arrive before anything queried it; once pathfinding
// got faster, that accidental buffer shrank enough for the race to start
// actually losing. World.GetBlockAt's own loaded return is the correct
// signal to wait on directly, so this closes the actual gap rather than
// re-introducing incidental slowness as a workaround.
const (
	chunkSyncTimeout      = 5 * time.Second
	chunkSyncPollInterval = 100 * time.Millisecond
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
// Clears (via RCON's "clear" command) each ingredient candidate and
// itemName itself from the bot's inventory before giving a fresh
// seedGiveCount of each ingredient — without this, a repeated-episode
// caller (rlenv.Environment.Reset, called once per episode with no other
// mechanism to consume the crafted item or the ingredient surplus a
// craft's own consumption doesn't fully use up) accumulates both without
// bound: found live via testing/rl_train_test.go's
// TestRLTrainingLoop_LongRunShowsLearningOnCraftTaskLive, where an
// uncleared give-every-episode policy filled every one of the bot's 36
// main-inventory+hotbar slots with maxed-out (64) stacks of ingredients
// and never-consumed crafted output after roughly 200-230 successful
// craft episodes (a real accumulation, not a hypothetical one — the
// numbers work out: minecraft:stick's recipe nets +7 surplus oak_planks
// and +4 never-consumed sticks per successful craft, and 220 episodes'
// worth of that already exceeds 36 slots) — at which point a newly
// crafted item has nowhere to land, and every subsequent craft attempt
// fails CraftItem's own awaitInventoryIncrease confirmation, indefinitely
// (docs/bugs/mine-task-fifty-percent-equilibrium.md-adjacent finding,
// documented in RL_TRAINING_LOOP_PLAN.md rather than that file since it's
// Craft-specific, not a Mine-task recurrence).
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

	if _, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("clear %s %s", a.cfg.Name, itemName)); err != nil {
		return fmt.Errorf("clear %s via RCON: %w", itemName, err)
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
		if _, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("clear %s %s", a.cfg.Name, candidate)); err != nil {
			return fmt.Errorf("clear %s via RCON: %w", candidate, err)
		}
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

// TeleportTo moves the bot to the given world coordinates via RCON
// (testenv.RCONHelper.Teleport), used by rlenv.Environment.Reset
// (rlenv.Config.ResetOrigin) to restore a known starting position each
// episode — training convenience only, same scope note as
// SeedNearbyBlock/SeedCraftIngredients above: never used by any chat
// command or other real gameplay path. Requires RCON (a.cfg.RCON) to be
// configured.
//
// Waits (bounded by teleportSyncTimeout) for the bot's own tracked
// position to land within teleportSyncThreshold blocks of the requested
// destination before returning, not just for the RCON command to succeed
// server-side — the same client-sync race SeedNearbyBlock's doc comment
// describes, here for a player-position-update packet. Then, separately
// (bounded by chunkSyncTimeout), waits for the destination chunk's block
// data to actually be loaded — see chunkSyncTimeout's own doc comment for
// why this is a distinct wait from position sync, not a duplicate of it.
func (a *agent) TeleportTo(ctx context.Context, x, y, z float64) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("teleport: RCON not configured for this agent")
	}
	if _, err := a.cfg.RCON.Teleport(ctx, a.cfg.Name, x, y, z).Exec(ctx); err != nil {
		return fmt.Errorf("teleport via RCON: %w", err)
	}

	deadline := time.Now().Add(teleportSyncTimeout)
	for {
		if pos, ok := a.GetPositionSimple(); ok {
			dx, dy, dz := pos.X-x, pos.Y-y, pos.Z-z
			if dx*dx+dy*dy+dz*dz <= teleportSyncThreshold*teleportSyncThreshold {
				return a.waitForChunkLoaded(ctx, x, y, z)
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("teleport: position never synced to (%.2f, %.2f, %.2f) within %s", x, y, z, teleportSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(teleportSyncPollInterval):
		}
	}
}

// waitForChunkLoaded polls World.GetBlockAt's own loaded return at (x, y, z)
// until the destination chunk has arrived, bounded by chunkSyncTimeout — see
// that constant's doc comment for why TeleportTo needs this in addition to
// (not instead of) its position-sync wait above. Skips the wait entirely if
// this agent has no world wired up (a minimal test double, say), matching
// how other optional-capability checks in this codebase degrade (e.g.
// rlenv/walkability.go's WalkabilityAgent) rather than erroring.
func (a *agent) waitForChunkLoaded(ctx context.Context, x, y, z float64) error {
	if a.worldMgr == nil {
		return nil
	}

	deadline := time.Now().Add(chunkSyncTimeout)
	for {
		if _, loaded := a.worldMgr.GetBlockAt(x, y, z); loaded {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("teleport: destination chunk at (%.2f, %.2f, %.2f) never loaded within %s", x, y, z, chunkSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(chunkSyncPollInterval):
		}
	}
}
