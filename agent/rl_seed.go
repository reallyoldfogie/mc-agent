package agent

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// seedBlockOffset is how far in front of the bot (along +X) SeedNearbyBlock
// places a block.  The seed is placed on the floor beneath the bot's feet,
// rather than at the bot's feet: in the modern flat-world layout used by the
// trainer, player feet are at Y=0 and a block at Y=0 intersects the player.
// Keeping the seed one block below the player makes the placement safe while
// leaving it within the requested search radius.
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
//
// 2026-10-02: raised 2s -> 10s, matching seedSyncTimeout — see
// craftConfirmTimeout's doc comment (agent/craft.go) for the live evidence
// (shared-server load under multi-bot training) that motivated bringing
// every client-sync wait in this file up to the same 10s deadline.
const (
	teleportSyncTimeout      = 10 * time.Second
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
//
// 2026-10-02: raised 5s -> 10s, same pass as teleportSyncTimeout above.
const (
	chunkSyncTimeout      = 10 * time.Second
	chunkSyncPollInterval = 100 * time.Millisecond
)

// seedBlockCoords is where SeedNearbyBlock places its block for a bot at
// (x, y, z): seedBlockOffset cells along +X, in the floor cell below the
// bot's feet. Floor, not int64() truncation: truncation rounds negative
// coordinates toward zero, so a bot at y=-59.5 (mid-fall after a teleport,
// in a superflat world whose ground is negative) must still seed against the
// floor below its current feet cell.
func seedBlockCoords(x, y, z float64) (int64, int64, int64) {
	return int64(math.Floor(x)) + seedBlockOffset, int64(math.Floor(y)) - 1, int64(math.Floor(z))
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

// CloneArea copies the box between the two source corners (inclusive) onto
// the destination corner via RCON's /clone, restoring both ground damage
// and leftover placed-block debris in one pass - see rlenv.AreaCloner and
// Config.CloneAreaFrom's own doc comment for why this exists alongside
// ClearAir. Deliberately omits /clone's "move" mode: the source template
// must survive every call, since every future episode clones from it
// again. The box (source, same as destination since they're equal in
// size) must stay under vanilla's 32,768-block fill limit, same as
// ClearAir. Training convenience only, same scope note as ClearAir;
// requires RCON. A source or destination that isn't loaded is a harmless
// no-op on the server side.
func (a *agent) CloneArea(ctx context.Context, srcX1, srcY1, srcZ1, srcX2, srcY2, srcZ2, dstX1, dstY1, dstZ1 int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("clone area: RCON not configured for this agent")
	}
	cmd := fmt.Sprintf("clone %d %d %d %d %d %d %d %d %d replace normal",
		srcX1, srcY1, srcZ1, srcX2, srcY2, srcZ2, dstX1, dstY1, dstZ1)
	resp, err := a.cfg.RCON.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("clone via RCON: %w", err)
	}
	// Same reasoning as ClearAir's own wait: give this bot's own world view
	// time to receive the resulting block updates before the next thing
	// Reset does (seeding a crafting table) runs against stale client state.
	if strings.Contains(resp, "Successfully cloned") {
		return sleepWithContext(ctx, clearAirSyncDelay)
	}
	return nil
}

// clearAirSyncDelay is how long ClearAir waits after a fill that removed
// blocks for the resulting block-update packets to reach this bot.
const clearAirSyncDelay = 300 * time.Millisecond

// SeedCraftIngredients gives the bot, via RCON, one stack
// of each distinct ingredient itemName's recipe needs — the first
// candidate per grid cell for a tag-gated ingredient (e.g. "any plank
// variant"), not an attempt to grant exactly the minimum needed. Training
// convenience only, same scope note as SeedNearbyBlock. Requires RCON for
// both inventory cleanup and giving, and itemName to have a known recipe (see
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
// Deliberately RCON-only, not a player/chat command: the latter requires
// this bot to be opped (chat commands run at the issuing player's own
// permission level), which training has no reason to require — found live
// when a non-op test bot's chat "give" was silently rejected server-side
// with no transport error to catch, so a chat-command-first design can
// mask the failure entirely instead of surfacing it. RCON's "give <player>
// ..." still targets this bot by name, so the server still pushes the
// resulting inventory update to this bot's own connection — not a
// console-side action with no client-visible effect.
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

// SeedGiveItem ensures at least count of itemName is in the bot's own
// inventory, giving it via RCON if not already present - see
// rlenv.ToolSeedAgent. Deliberately RCON-only, not a player/chat command -
// see SeedCraftIngredients' doc comment for why: training has no reason to
// require this bot be opped. Unlike SeedCraftIngredients, this gives
// itemName itself, not a recipe's ingredients, and never clears the rest of
// the inventory first: mine tasks, unlike craft/chain tasks, have never
// cleared it to begin with (DefaultEpisodeSeeder's MineTargetBlock branch
// only calls SeedNearbyBlock). Idempotent on purpose - an episode that
// starts with a durable tool already in inventory (left over from a
// previous episode, since nothing clears it) is left alone, so this doesn't
// hand out a fresh, redundant tool every single episode the way an
// unconditional give would.
func (a *agent) SeedGiveItem(ctx context.Context, itemName string, count int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("seed give item: RCON not configured for this agent")
	}
	if a.InventoryCount(itemName) >= count {
		return nil
	}

	normalized := normalizeItemName(itemName)
	if _, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("give %s %s %d", a.cfg.Name, normalized, count)); err != nil {
		return fmt.Errorf("give %s via RCON: %w", normalized, err)
	}

	deadline := time.Now().Add(seedSyncTimeout)
	for a.InventoryCount(itemName) == 0 {
		if !time.Now().Before(deadline) {
			return fmt.Errorf("seed give item: gave %s but it never synced to this bot's inventory within %s", normalized, seedSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(seedSyncPollInterval):
		}
	}
	return nil
}

// elytraItemName/fireworkRocketItemName name the two items SeedElytra
// cares about — kept as named constants here rather than inline literals
// since this file references each of them twice.
const (
	elytraItemName         = "minecraft:elytra"
	fireworkRocketItemName = "minecraft:firework_rocket"
)

// flySeedRocketCount is how many firework rockets SeedElytra tops the bot
// up to — generous relative to a single flight's actual consumption
// (climb + periodic re-boosts during cruise + landing, agent.FlyTo's own
// fireRocket calls), so a training episode doesn't run out mid-flight.
const flySeedRocketCount = 16

// SeedElytra ensures the bot has an elytra equipped and a working supply
// of firework rockets, via RCON — training convenience for Flight episode
// seeding (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3), the
// flight-task analogue of SeedCraftIngredients: one call does everything
// that task needs prepared, not just one piece of it. Gives an elytra (if
// not already held/worn) and equips it — agent.FlyTo itself refuses to
// fly without one worn — then tops up firework rockets to
// flySeedRocketCount if below it. Requires RCON (via SeedGiveItem).
func (a *agent) SeedElytra(ctx context.Context) error {
	if !a.IsEquipped(elytraItemName) {
		if err := a.SeedGiveItem(ctx, elytraItemName, 1); err != nil {
			return fmt.Errorf("seed elytra: giving elytra: %w", err)
		}
		if err := a.Equip(ctx, elytraItemName); err != nil {
			return fmt.Errorf("seed elytra: equipping elytra: %w", err)
		}
	}
	if err := a.SeedGiveItem(ctx, fireworkRocketItemName, flySeedRocketCount); err != nil {
		return fmt.Errorf("seed elytra: giving firework rockets: %w", err)
	}
	return nil
}

// combatSeedWeapon is the weapon SeedCombatMob equips before an episode's
// combat task — fixed, not configurable, for a first pass (see
// docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 5's own note on
// deliberately not exposing a weapon choice yet).
const combatSeedWeapon = "minecraft:iron_sword"

// combatMobNBT tags a seeded combat mob NoAI + PersistenceRequired by
// default — isolates "can the policy learn to use a weapon" from "can it
// survive a reactive, AI-driven opponent" (a genuinely harder, separate
// task, not this pass's job), matching
// testing/combat_integration_test.go's own established convention for the
// identical reason. This is not a workaround for unhandled bot death —
// that's already handled generically (rlenv/reward.go's computeReward,
// independent of any task) — it's purely about isolating the skill being
// trained.
const combatMobNBT = "{NoAI:1b,PersistenceRequired:1b}"

// SeedCombatMob ensures a live, non-aggressive instance of mobType exists
// within radius blocks of the bot (via SeedEntity) and that a weapon is
// equipped, via RCON — training convenience for Combat episode seeding
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 5), the combat-task
// analogue of SeedElytra: one call does everything that task needs
// prepared. Requires RCON (via SeedEntity/SeedGiveItem).
func (a *agent) SeedCombatMob(ctx context.Context, mobType string, radius int) (int32, error) {
	entityID, err := a.SeedEntity(ctx, mobType, radius, combatMobNBT)
	if err != nil {
		return 0, fmt.Errorf("seed combat mob: %w", err)
	}
	if !a.IsEquipped(combatSeedWeapon) {
		if err := a.SeedGiveItem(ctx, combatSeedWeapon, 1); err != nil {
			return 0, fmt.Errorf("seed combat mob: giving weapon: %w", err)
		}
		if err := a.Equip(ctx, combatSeedWeapon); err != nil {
			return 0, fmt.Errorf("seed combat mob: equipping weapon: %w", err)
		}
	}
	return entityID, nil
}

// creeperGunpowderMobType is the entity type SeedCreeperForGunpowder
// always summons — KillCreeperForGunpowder itself is hardcoded to
// creepers by design (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item
// 5), so there's no mob-type parameter here, unlike SeedCombatMob's own.
const creeperGunpowderMobType = "minecraft:creeper"

// SeedCreeperForGunpowder ensures a live creeper exists within radius
// blocks of the bot (via SeedEntity) and that a supported melee weapon is
// equipped, via RCON — training convenience for KillCreeperForGunpowder
// episode seeding, the creeper-gunpowder-task analogue of SeedCombatMob.
// Deliberately NOT NoAI-tagged (empty nbt), unlike SeedCombatMob's own
// combatMobNBT — the one real design decision that differs from Combat's
// own seeding philosophy: this task's entire point is managing a live,
// explosive threat safely (KillCreeperForGunpowder's own health-
// cancellation guard exists for exactly that); tagging the seeded
// creeper NoAI would remove the actual skill being trained. Reuses
// combatSeedWeapon (not a second weapon constant) — same reasoning as
// SeedCombatMob's own doc comment on why the weapon choice is a fixed
// constant, not configurable, for a first pass. Requires RCON (via
// SeedEntity/SeedGiveItem).
func (a *agent) SeedCreeperForGunpowder(ctx context.Context, radius int) (int32, error) {
	entityID, err := a.SeedEntity(ctx, creeperGunpowderMobType, radius, "")
	if err != nil {
		return 0, fmt.Errorf("seed creeper for gunpowder: %w", err)
	}
	if !a.IsEquipped(combatSeedWeapon) {
		if err := a.SeedGiveItem(ctx, combatSeedWeapon, 1); err != nil {
			return 0, fmt.Errorf("seed creeper for gunpowder: giving weapon: %w", err)
		}
		if err := a.Equip(ctx, combatSeedWeapon); err != nil {
			return 0, fmt.Errorf("seed creeper for gunpowder: equipping weapon: %w", err)
		}
	}
	return entityID, nil
}

// spearSeedWeapon is the weapon SeedSpearMob equips before an episode's
// spear-charge task — fixed, not configurable, for a first pass, same
// reasoning as combatSeedWeapon's own doc comment. "minecraft:iron_spear"
// matches testing/combat_integration_test.go's own established spear-item
// convention (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 6).
const spearSeedWeapon = "minecraft:iron_spear"

// SeedSpearMob ensures a live, non-aggressive instance of mobType exists
// within radius blocks of the bot (via SeedEntity) and that a spear is
// equipped, via RCON — training convenience for SpearCharge episode
// seeding, the spear-task analogue of SeedCombatMob. NoAI-tagged like
// SeedCombatMob's own combatMobNBT (unlike SeedCreeperForGunpowder):
// isolates "can the policy learn to charge" from surviving a reactive
// opponent, the same reasoning SeedCombatMob's own doc comment gives.
// Requires RCON (via SeedEntity/SeedGiveItem).
func (a *agent) SeedSpearMob(ctx context.Context, mobType string, radius int) (int32, error) {
	entityID, err := a.SeedEntity(ctx, mobType, radius, combatMobNBT)
	if err != nil {
		return 0, fmt.Errorf("seed spear mob: %w", err)
	}
	if !a.IsEquipped(spearSeedWeapon) {
		if err := a.SeedGiveItem(ctx, spearSeedWeapon, 1); err != nil {
			return 0, fmt.Errorf("seed spear mob: giving weapon: %w", err)
		}
		if err := a.Equip(ctx, spearSeedWeapon); err != nil {
			return 0, fmt.Errorf("seed spear mob: equipping weapon: %w", err)
		}
	}
	return entityID, nil
}

// maceSeedWeapon is the weapon SeedMaceMob equips before an episode's
// mace-smash task — fixed, not configurable, for a first pass, same
// reasoning as combatSeedWeapon's own doc comment (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 7).
const maceSeedWeapon = "minecraft:mace"

// SeedMaceMob ensures a live, non-aggressive instance of mobType exists
// within radius blocks of the bot (via SeedEntity) and that a mace is
// equipped, via RCON — training convenience for MaceSmash episode
// seeding, the mace-task analogue of SeedCombatMob/SeedSpearMob.
// NoAI-tagged like those two: a falling-attack timing window is hard
// enough to hit without also fighting a reactive opponent. Requires RCON
// (via SeedEntity/SeedGiveItem). See SeedElevatedPlatform for the other
// half of this task's seeding (the terrain the smash actually needs).
func (a *agent) SeedMaceMob(ctx context.Context, mobType string, radius int) (int32, error) {
	entityID, err := a.SeedEntity(ctx, mobType, radius, combatMobNBT)
	if err != nil {
		return 0, fmt.Errorf("seed mace mob: %w", err)
	}
	if !a.IsEquipped(maceSeedWeapon) {
		if err := a.SeedGiveItem(ctx, maceSeedWeapon, 1); err != nil {
			return 0, fmt.Errorf("seed mace mob: giving weapon: %w", err)
		}
		if err := a.Equip(ctx, maceSeedWeapon); err != nil {
			return 0, fmt.Errorf("seed mace mob: equipping weapon: %w", err)
		}
	}
	return entityID, nil
}

// seedElevatedPlatformHeight/seedElevatedPlatformOffset/
// seedElevatedPlatformBlock control where/how SeedElevatedPlatform builds
// its platform relative to the seeded mace mob's own position:
// seedElevatedPlatformHeight blocks straight up, seedElevatedPlatformOffset
// blocks horizontally to one side. Chosen so a bot standing at the
// platform's center and walking toward the mob (approachCombatTarget's own
// continuous direction-to-live-target behavior — see
// agent.MaceSmashAtWithApproach) steps off the near edge within its first
// couple of movement ticks and falls comfortably past
// combat.MaceAdditionalDamageFall's threshold while closing within
// combat.MaceMaxReach by the time it lands. These exact values are a
// first-pass estimate, not derived from the real falling-physics
// trajectory (which this package doesn't simulate) — documented here as
// the thing to retune first if live testing shows the window is missed
// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 7 flags this as
// needing live validation once a server running a mace-supporting version
// is available).
const (
	seedElevatedPlatformHeight = 5
	seedElevatedPlatformOffset = 2
	seedElevatedPlatformBlock  = "minecraft:stone"
)

// SeedElevatedPlatform locates the nearest live instance of mobType within
// radius blocks of the bot (the same FindNearestEntityByType primitive
// SeedEntity itself uses — this does not seed the mob; call SeedMaceMob
// first) and places a small 3x3 stone platform seedElevatedPlatformHeight
// blocks above it, seedElevatedPlatformOffset blocks to one side, then
// teleports the bot onto its center — training convenience for MaceSmash
// episode seeding (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 7):
// agent.MaceSmashAt itself only waits for a falling-attack window, never
// creates one, so without this seeding step a smash can never succeed (the
// bot starts on flat ground beside the mob, nothing to fall from).
// Requires RCON (via raw /fill, mirroring ClearAir's own RCON machinery —
// not SeedEntity/SeedVehicle, which summon entities, not place blocks) and
// TeleportTo.
func (a *agent) SeedElevatedPlatform(ctx context.Context, mobType string, radius int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("seed elevated platform: RCON not configured for this agent")
	}
	normalized := normalizeItemName(mobType)
	typeID, ok := a.GetEntityTypeID(normalized)
	if !ok {
		return fmt.Errorf("seed elevated platform: unknown entity type %q", mobType)
	}
	pos, ok := a.GetPositionSimple()
	if !ok {
		return fmt.Errorf("seed elevated platform: position not yet known")
	}
	entityID, distance, found := a.FindNearestEntityByType(typeID, pos.X, pos.Y, pos.Z, true)
	if !found || distance > float64(radius) {
		return fmt.Errorf("seed elevated platform: no %s within %d blocks to build toward", mobType, radius)
	}
	target, tracked := a.GetTrackedEntities()[entityID]
	if !tracked {
		return fmt.Errorf("seed elevated platform: resolved %s is no longer tracked", mobType)
	}

	platformX := target.X + seedElevatedPlatformOffset
	platformY := target.Y + seedElevatedPlatformHeight
	platformZ := target.Z
	minX, maxX := int64(math.Floor(platformX))-1, int64(math.Floor(platformX))+1
	minZ, maxZ := int64(math.Floor(platformZ))-1, int64(math.Floor(platformZ))+1
	y := int64(math.Floor(platformY))
	cmd := fmt.Sprintf("fill %d %d %d %d %d %d %s", minX, y, minZ, maxX, y, maxZ, seedElevatedPlatformBlock)
	if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("fill via RCON: %w", err)
	}

	return a.TeleportTo(ctx, platformX, platformY+1, platformZ)
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
		if _, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("give %s %s %d", a.cfg.Name, candidate, seedGiveCount)); err != nil {
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

// chunkPreloadRadius is how far around a teleport destination
// waitForChunkLoaded also waits for chunks to arrive, on top of the
// destination's own chunk — see chunkPreloadPoints and this function's own
// doc comment for why one point was never enough.
const chunkPreloadRadius = 16

// chunkPreloadPoints returns the destination point plus one point in each of
// its horizontal chunk neighbors (a 3x3 grid of chunk columns centered on
// (x, y, z), sampled once per column since a whole 16-block column loads as
// one unit) — everywhere within chunkPreloadRadius blocks of the
// destination, matching the common case of a mine/craft search radius run
// right after a Config.ResetOrigin teleport (rlenv.Environment.Reset).
func chunkPreloadPoints(x, y, z float64) [][3]float64 {
	offsets := []float64{-chunkPreloadRadius, 0, chunkPreloadRadius}
	points := make([][3]float64, 0, len(offsets)*len(offsets))
	for _, dx := range offsets {
		for _, dz := range offsets {
			points = append(points, [3]float64{x + dx, y, z + dz})
		}
	}
	return points
}

// waitForChunkLoaded polls World.GetBlockAt's own loaded return across
// chunkPreloadPoints until every one of them has arrived, bounded by
// chunkSyncTimeout — see that constant's doc comment for why TeleportTo
// needs this in addition to (not instead of) its position-sync wait above.
// Skips the wait entirely if this agent has no world wired up (a minimal
// test double, say), matching how other optional-capability checks in this
// codebase degrade (e.g. rlenv/walkability.go's WalkabilityAgent) rather
// than erroring.
//
// Checks a spread of points around the destination, not just the
// destination itself: found live (2026-09-27) via mc-rsi-trainer's
// cmd/rsi-infer teleporting a standalone bot somewhere it had never visited
// before — the destination's own chunk loaded and this wait returned, but a
// FindVisibleBlock search a few blocks later reached into a neighboring
// chunk that simply hadn't arrived yet on this bot's own connection (a
// client-tracked state entirely separate from what the server itself
// already knows), and silently found nothing for the rest of the episode.
// One point loading says nothing about a chunk chunkPreloadRadius blocks
// away.
func (a *agent) waitForChunkLoaded(ctx context.Context, x, y, z float64) error {
	if a.worldMgr == nil {
		return nil
	}

	points := chunkPreloadPoints(x, y, z)
	deadline := time.Now().Add(chunkSyncTimeout)
	for {
		allLoaded := true
		for _, p := range points {
			if _, loaded := a.worldMgr.GetBlockAt(p[0], p[1], p[2]); !loaded {
				allLoaded = false
				break
			}
		}
		if allLoaded {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("teleport: chunks within %d blocks of (%.2f, %.2f, %.2f) never all loaded within %s", chunkPreloadRadius, x, y, z, chunkSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(chunkSyncPollInterval):
		}
	}
}

// ClearInventory empties the bot's whole inventory via RCON, and waits (bounded
// by seedSyncTimeout) for the bot's own tracking to agree. Implements
// rlenv.ChainSeedAgent: a multi-step chain episode must start with nothing
// held, or leftovers from the last one would stand in for gathering.
func (a *agent) ClearInventory(ctx context.Context) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("clear inventory: RCON not configured")
	}
	if _, err := a.cfg.RCON.Exec(ctx, fmt.Sprintf("clear %s", a.cfg.Name)); err != nil {
		return fmt.Errorf("clear inventory via RCON: %w", err)
	}
	deadline := time.Now().Add(seedSyncTimeout)
	for !a.inventoryEmpty() {
		if !time.Now().Before(deadline) {
			return fmt.Errorf("clear inventory: the bot's own inventory never emptied within %s", seedSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(seedSyncPollInterval):
		}
	}
	return nil
}

// inventoryEmpty reports whether none of the items a chain episode cares
// about is held (the full inventory is not enumerated cheaply here, and these
// are the ones whose presence would corrupt an episode).
func (a *agent) inventoryEmpty() bool {
	for _, item := range []string{"minecraft:oak_log", "minecraft:oak_planks", "minecraft:stick", "minecraft:crafting_table", "minecraft:bowl", "minecraft:chest", "minecraft:wooden_pickaxe"} {
		if a.InventoryCount(item) > 0 {
			return false
		}
	}
	return true
}

// ClearDroppedItems kills item entities within radius blocks of (x, y, z) via
// RCON. Implements rlenv.ChainSeedAgent: a drop a previous episode left behind
// would be picked up by a later one as a free log.
func (a *agent) ClearDroppedItems(ctx context.Context, x, y, z, radius int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("clear dropped items: RCON not configured")
	}
	cmd := fmt.Sprintf("execute positioned %d %d %d run kill @e[type=item,distance=..%d]", x, y, z, radius)
	if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("clear dropped items via RCON: %w", err)
	}
	return nil
}

// seedEntityOffset mirrors seedBlockOffset: how far in front of the bot
// (along +X) SeedEntity/SeedVehicle/SeedDroppedItem spawn their entity.
// Unlike a block seed, an entity stands ON the ground rather than
// replacing it, so it spawns at the bot's own feet level (seedBlockCoords'
// y-1 would place it a block too low).
const seedEntityOffset = 2

// seedEntitySyncRadius bounds how far from the spawn point SeedEntity/
// SeedVehicle looks for the entity it just summoned, independent of the
// caller's own radius argument (which instead answers "does a suitable
// instance already exist nearby" — a separate question). Generous enough
// to comfortably cover any summon-position rounding, small enough that a
// second, unrelated, same-type entity already in the world couldn't be
// mistaken for the one just summoned in the common case.
const seedEntitySyncRadius = 8.0

// seedEntityCoords is where SeedEntity/SeedVehicle/SeedDroppedItem spawn
// their entity for a bot at (x, y, z) — see seedEntityOffset's own doc
// comment. Floor, not truncation, for the same negative-coordinate reason
// seedBlockCoords uses it.
func seedEntityCoords(x, y, z float64) (int64, int64, int64) {
	return int64(math.Floor(x)) + seedEntityOffset, int64(math.Floor(y)), int64(math.Floor(z))
}

// SeedEntity ensures a live instance of mobType exists within radius blocks
// of the bot, via RCON "summon" — training convenience only, the entity
// analogue of SeedNearbyBlock (same scope note: never used by any chat
// command or real gameplay path), for Combat/Mob-Riding episode seeding
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0). nbt, if non-empty,
// must be a full NBT compound including its own braces (e.g.
// "{NoAI:1b,PersistenceRequired:1b}") — passed through to
// testenv.RCONHelper.SummonEntity unchanged; empty means no NBT at all,
// not an empty compound. Requires RCON.
//
// A no-op (returning the existing instance's ID) if FindNearestEntityByType
// already finds one within radius — doesn't stack an extra mob onto a
// world that already has one, mirroring SeedNearbyBlock's own "already
// there" check.
//
// Waits (bounded by seedSyncTimeout) for the summoned entity to actually
// become visible to this bot's own tracking before returning, within
// seedEntitySyncRadius of the spawn point — the same client-sync race
// SeedNearbyBlock's doc comment describes for a block-update packet, here
// for a SpawnEntity packet.
func (a *agent) SeedEntity(ctx context.Context, mobType string, radius int, nbt string) (int32, error) {
	if a.cfg.RCON == nil {
		return 0, fmt.Errorf("seed entity: RCON not configured for this agent")
	}
	normalized := normalizeItemName(mobType)
	typeID, ok := a.GetEntityTypeID(normalized)
	if !ok {
		return 0, fmt.Errorf("seed entity: unknown entity type %q", mobType)
	}
	pos, ok := a.GetPositionSimple()
	if !ok {
		return 0, fmt.Errorf("seed entity: position not yet known")
	}
	if entityID, distance, found := a.FindNearestEntityByType(typeID, pos.X, pos.Y, pos.Z, true); found && distance <= float64(radius) {
		return entityID, nil
	}

	x, y, z := seedEntityCoords(pos.X, pos.Y, pos.Z)
	// Built directly via Exec (matching testenv.RCONHelper.SummonEntity's
	// own "summon %s %f %f %f %s" format exactly) rather than chained
	// through SummonEntity(...).Exec(ctx): the latter's returned
	// *testenv.RCONResult binds to its originating *rconHelper via an
	// unexported field this package cannot populate in a test double, so
	// a unit test could never verify the command it sends — see
	// agent/rl_seed_test.go.
	cmd := fmt.Sprintf("summon %s %d %d %d %s", normalized, x, y, z, nbt)
	if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
		return 0, fmt.Errorf("summon %s via RCON: %w", normalized, err)
	}

	deadline := time.Now().Add(seedSyncTimeout)
	for {
		if entityID, distance, found := a.FindNearestEntityByType(typeID, float64(x), float64(y), float64(z), true); found && distance <= seedEntitySyncRadius {
			return entityID, nil
		}
		if !time.Now().Before(deadline) {
			return 0, fmt.Errorf("seed entity: summoned %s but it never became visible to this bot within %s", normalized, seedSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(seedSyncPollInterval):
		}
	}
}

// vehicleTameNBT is the NBT compound SeedVehicle passes when tame is true —
// an untamed horse/donkey/mule refuses mounting (confirmed by
// testing/vehicles' own {Tame:1b} convention), so this is the only NBT a
// mob-riding episode's seeding needs beyond what SeedEntity already does.
const vehicleTameNBT = "{Tame:1b}"

// SeedVehicle is SeedEntity for a rideable vehicle, tame controlling
// whether the summoned instance is tamed (see vehicleTameNBT) — the
// mob-riding analogue of SeedEntity's combat-mob role
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0/4).
func (a *agent) SeedVehicle(ctx context.Context, vehicleType string, radius int, tame bool) (int32, error) {
	nbt := ""
	if tame {
		nbt = vehicleTameNBT
	}
	return a.SeedEntity(ctx, vehicleType, radius, nbt)
}

// SeedDroppedItem drops one itemName on the ground within radius blocks of
// the bot, via RCON "summon minecraft:item" — the item-entity analogue of
// SeedNearbyBlock/SeedEntity, for an Item Pickup episode posed from a cold
// inventory rather than piggybacking on a mine task's own drop
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0/2). The
// `Item:{id:"...",Count:1}` NBT shape matches testing/lookaround_test.go's
// own already-live-tested convention. Training convenience only; requires
// RCON. A no-op if FindNearestVisibleItem already finds something within
// radius.
func (a *agent) SeedDroppedItem(ctx context.Context, itemName string, radius int) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("seed dropped item: RCON not configured for this agent")
	}
	if _, _, _, _, found, err := a.FindNearestVisibleItem(ctx, float64(radius)); err == nil && found {
		return nil
	}
	pos, ok := a.GetPositionSimple()
	if !ok {
		return fmt.Errorf("seed dropped item: position not yet known")
	}
	normalized := normalizeItemName(itemName)
	x, y, z := seedEntityCoords(pos.X, pos.Y, pos.Z)
	nbt := fmt.Sprintf(`{Item:{id:"%s",Count:1}}`, normalized)
	cmd := fmt.Sprintf("summon minecraft:item %d %d %d %s", x, y, z, nbt)
	if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("summon item via RCON: %w", err)
	}

	deadline := time.Now().Add(seedSyncTimeout)
	for {
		if _, _, _, _, found, err := a.FindNearestVisibleItem(ctx, float64(radius)); err == nil && found {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("seed dropped item: dropped %s but it never became visible to this bot within %s", normalized, seedSyncTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(seedSyncPollInterval):
		}
	}
}

// ClearEntitiesOfType kills every live instance of mobType within radius
// blocks of (x, y, z) via RCON — the Reset-time cleanup counterpart to
// SeedEntity/SeedVehicle (see rlenv.EntitySeedAgent/VehicleSeedAgent): a
// mob or vehicle a Combat/Mob-Riding episode didn't account for (the
// policy chose Wait all episode, or combat failed) would otherwise persist
// into the next Reset, the same "working area turns into an obstacle
// course" failure mode ClearDroppedItems/ClearAir already exist to prevent
// for drops/blocks (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0).
// A type+radius kill, not an ID-specific one: precise per-episode-entity
// targeting (to avoid a shared-server episode's cleanup killing another
// bot's own seeded entity) is deferred to whichever of Phase 4/5 actually
// needs it, not solved speculatively here — see that plan's own note.
func (a *agent) ClearEntitiesOfType(ctx context.Context, x, y, z, radius int, mobType string) error {
	if a.cfg.RCON == nil {
		return fmt.Errorf("clear entities of type: RCON not configured")
	}
	normalized := normalizeItemName(mobType)
	cmd := fmt.Sprintf("execute positioned %d %d %d run kill @e[type=%s,distance=..%d]", x, y, z, normalized, radius)
	if _, err := a.cfg.RCON.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("clear entities of type via RCON: %w", err)
	}
	return nil
}
