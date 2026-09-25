package agent

import (
	"context"
	"fmt"
	"math"
	"strings"
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

// craftTableSeedRadius bounds how far SeedCraftIngredients looks for (and,
// via SeedNearbyBlock, places) a crafting table when the target recipe
// doesn't fit the player's own 2x2 inventory grid — see
// craftingRecipe.fitsInventoryGrid (craft.go) for that check and
// craftTableSearchRadius (craft.go) for the larger radius CraftItem's own
// openCraftingTable later searches within during the real craft attempt;
// keeping this well inside that radius means a table SeedNearbyBlock just
// placed is never at risk of falling outside CraftItem's own later search.
const craftTableSeedRadius = 8

// seedSyncTimeout/seedSyncPollInterval bound how long SeedNearbyBlock/
// SeedCraftIngredients wait for this bot's own client-tracked state to
// catch up with an RCON write it just made — see their doc comments for
// why this polling exists at all (found live, not anticipated: a real
// race, not a hypothetical one — testing/rl_train_test.go).
const (
	seedSyncTimeout      = 10 * time.Second
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

// seedBlockCoords is where SeedNearbyBlock places its block for a bot at
// (x, y, z): seedBlockOffset cells along +X, in the cell containing the
// bot's feet. Floor, not int64() truncation: truncation rounds negative
// coordinates toward zero, so a bot at y=-59.5 (mid-fall after a teleport,
// in a superflat world whose ground is negative) got the block placed one
// cell too high - floating two above the floor and unreachable.
func seedBlockCoords(x, y, z float64) (int64, int64, int64) {
	return int64(math.Floor(x)) + seedBlockOffset, int64(math.Floor(y)), int64(math.Floor(z))
}

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
	x, y, z := seedBlockCoords(pos.X, pos.Y, pos.Z)
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

// RestoreBlock puts blockName back at (x, y, z) via RCON, undoing a mining
// episode - see rlenv.BlockRestorer. Training convenience only, same scope
// note as SeedNearbyBlock; requires RCON.
func (a *agent) RestoreBlock(ctx context.Context, x, y, z int, blockName string) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("restore block: RCON not configured for this agent")
	}
	if _, err := a.cfg.RCON.SetBlock(ctx, int64(x), int64(y), int64(z), normalizeItemName(blockName), "replace").Exec(ctx); err != nil {
		return fmt.Errorf("setblock via RCON: %w", err)
	}
	return nil
}

// ClearAir fills the box between the two corners (inclusive) with air via
// RCON, removing whatever blocks - seeded ones, crafting tables, stacked
// leftovers - a training episode left there. See rlenv.AreaClearer; the box
// must stay under vanilla's 32,768-block fill limit. Training convenience
// only, same scope note as SeedNearbyBlock; requires RCON. A box that isn't
// loaded is a harmless no-op on the server side.
func (a *agent) ClearAir(ctx context.Context, x1, y1, z1, x2, y2, z2 int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("clear air: RCON not configured for this agent")
	}
	cmd := fmt.Sprintf("fill %d %d %d %d %d %d minecraft:air", x1, y1, z1, x2, y2, z2)
	resp, err := a.cfg.RCON.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("fill via RCON: %w", err)
	}
	// If blocks were actually removed, give this bot's own world view time to
	// receive the block updates before returning: the very next thing a Reset
	// does is seed a crafting table, and SeedNearbyBlock skips placing one if
	// its client-side FindVisibleBlock still sees the table that was just
	// removed - leaving the episode with no table at all (found live: ~2% of
	// table episodes failed 100 steps in a row with "no crafting table
	// found"). A fill that changed nothing has nothing to wait for.
	if strings.Contains(resp, "Successfully filled") {
		return sleepWithContext(ctx, clearAirSyncDelay)
	}
	return nil
}

// clearAirSyncDelay is how long ClearAir waits after a fill that removed
// blocks for the resulting block-update packets to reach this bot.
const clearAirSyncDelay = 300 * time.Millisecond

// SeedCraftIngredients gives the bot, via its player command connection (with
// an RCON fallback), one stack
// of each distinct ingredient itemName's recipe needs — the first
// candidate per grid cell for a tag-gated ingredient (e.g. "any plank
// variant"), not an attempt to grant exactly the minimum needed. Training
// convenience only, same scope note as SeedNearbyBlock. Requires RCON for
// inventory cleanup and a live player connection or RCON fallback for giving,
// and itemName to have a known recipe (see
// loadCraftingRecipes) — errors otherwise, since a seeding request that
// silently does nothing would be more confusing than a clear failure.
//
// Clears the bot's entire inventory (via RCON's "clear" command) before giving a fresh
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
// The player-command path avoids RCON's shared response buffer and executes
// the command as this bot, so the server sends the resulting inventory update
// on the same connection we are already observing. RCON remains a fallback
// for callers without a live command-capable connection.
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
		return fmt.Errorf("seed craft ingredients: RCON not configured for inventory cleanup")
	}
	recipe, err := a.craftRecipeFor(itemName)
	if err != nil {
		return err
	}

	// A recipe that doesn't fit the player's own 2x2 inventory grid needs
	// a real crafting table's 3x3 grid instead (see
	// craftingRecipe.fitsInventoryGrid, CraftItem's own openCraftingTable)
	// — ensure one exists in range before giving ingredients, the same way
	// SeedNearbyBlock already guarantees a Mine task's target block exists.
	// Without this, every episode for such an item fails outright (found
	// live: "no crafting table found within 32 blocks" on 100% of attempts
	// for minecraft:chest/minecraft:bowl against a fresh, tableless world),
	// since nothing else in episode seeding ever places one.
	if !recipe.fitsInventoryGrid() {
		if err := a.SeedNearbyBlock(ctx, "minecraft:crafting_table", craftTableSeedRadius); err != nil {
			return fmt.Errorf("seed craft ingredients: ensuring a crafting table for %s: %w", itemName, err)
		}
	}
	return a.giveCraftIngredients(ctx, itemName, recipe)
}

// SeedCraftIngredientsAt is SeedCraftIngredients with the crafting table (if
// the recipe needs one) placed at (x, y, z) instead of beside the bot - see
// rlenv.FarSeedAgent.
func (a *agent) SeedCraftIngredientsAt(ctx context.Context, itemName string, x, y, z int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("seed craft ingredients: RCON not configured for inventory cleanup")
	}
	recipe, err := a.craftRecipeFor(itemName)
	if err != nil {
		return err
	}
	if !recipe.fitsInventoryGrid() {
		if err := a.SeedBlockAt(ctx, "minecraft:crafting_table", x, y, z); err != nil {
			return fmt.Errorf("seed craft ingredients: placing a crafting table for %s: %w", itemName, err)
		}
	}
	return a.giveCraftIngredients(ctx, itemName, recipe)
}

// SeedBlockAt places blockName at (x, y, z) via RCON - see
// rlenv.FarSeedAgent. Unlike SeedNearbyBlock it doesn't wait for this bot to
// see the block: the position is deliberately somewhere the bot can't yet.
func (a *agent) SeedBlockAt(ctx context.Context, blockName string, x, y, z int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("seed block at: RCON not configured for this agent")
	}
	if _, err := a.cfg.RCON.SetBlock(ctx, int64(x), int64(y), int64(z), normalizeItemName(blockName), "replace").Exec(ctx); err != nil {
		return fmt.Errorf("setblock via RCON: %w", err)
	}
	return nil
}

// craftRecipeFor loads itemName's crafting recipe.
func (a *agent) craftRecipeFor(itemName string) (craftingRecipe, error) {
	recipes, err := a.loadCraftingRecipes()
	if err != nil {
		return craftingRecipe{}, fmt.Errorf("load recipes: %w", err)
	}
	recipe, ok := recipes[normalizeItemName(itemName)]
	if !ok {
		return craftingRecipe{}, fmt.Errorf("no known crafting recipe for %s", itemName)
	}
	return recipe, nil
}

// giveCraftIngredients is SeedCraftIngredients' inventory half: clears the
// bot's inventory and gives it recipe's ingredients, waiting for them to
// sync to this bot's own inventory tracking.
func (a *agent) giveCraftIngredients(ctx context.Context, itemName string, recipe craftingRecipe) error {
	// Clear the complete inventory, not just the items we are about to give.
	// The inventory can contain unrelated crafted output and surplus materials
	// from earlier episodes. If every slot is occupied, /give succeeds but
	// drops the item into the world instead of producing an inventory update
	// for this bot, which makes the synchronization wait fail.
	if _, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("clear %s", a.cfg.Name)); err != nil {
		return fmt.Errorf("clear inventory via RCON: %w", err)
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
		cmd := fmt.Sprintf("give @s %s %d", candidate, seedGiveCount)
		if err := a.SendCommand(cmd); err != nil {
			a.logf("SeedCraftIngredients: player command %q failed, falling back to RCON: %v", cmd, err)
			if a.cfg.RCON == nil {
				return fmt.Errorf("give %s via player command: %w", candidate, err)
			}
			if _, rconErr := a.cfg.RCON.Exec(ctx, fmt.Sprintf("give %s %s %d", a.cfg.Name, candidate, seedGiveCount)); rconErr != nil {
				return fmt.Errorf("give %s via player command (%v) or RCON: %w", candidate, err, rconErr)
			}
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
