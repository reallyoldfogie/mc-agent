package rlenv

import (
	"context"
	"fmt"
)

// SeedAgent is the minimal capability an EpisodeSeeder needs beyond
// LiveAgent — placing a block or granting items via RCON is a
// training-only convenience (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4),
// not part of LiveAgent's own contract (a chat-command-driven session
// never needs it, and LiveAgent must stay satisfiable by any bot session,
// not only ones with RCON configured).
type SeedAgent interface {
	// SeedNearbyBlock ensures a block named blockName exists within radius
	// blocks of the bot's current position — see agent.SeedNearbyBlock's
	// doc comment for exactly how ("training convenience," not a general
	// world-building capability).
	SeedNearbyBlock(ctx context.Context, blockName string, radius int) error

	// SeedCraftIngredients grants the bot itemName's recipe ingredients —
	// see agent.SeedCraftIngredients' doc comment.
	SeedCraftIngredients(ctx context.Context, itemName string) error
}

// ToolSeedAgent is the optional capability a mining task's starting tool
// (Config.MineRequiredTool) needs: giving the bot itemName directly, not a
// recipe's ingredients — see agent.SeedGiveItem's doc comment.
// DefaultEpisodeSeeder uses it when the SeedAgent passed in also implements
// it; a SeedAgent that doesn't is free to leave MineRequiredTool unhonored
// rather than fail the episode over it.
type ToolSeedAgent interface {
	// SeedGiveItem ensures at least count of itemName is in the bot's
	// inventory.
	SeedGiveItem(ctx context.Context, itemName string, count int) error
}

// EpisodeSeeder, if set on Config, is called once per Reset (after origin
// is captured, before mine/craft state is resolved for the returned
// observation) to prepare the world for this episode's configured task(s).
//
// This is a minimum-viable convenience for solo/standalone training runs
// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 4) — not mc-rsi-trainer's real
// curriculum/task generator, which owns varying tasks across episodes (see
// that plan's own scope note on why this stops well short of a real
// curriculum: one fixed task per Environment instance, seeded the same way
// every Reset, never varied). nil (the default) leaves Reset exactly as it
// already behaved before this existed: mine/craft tasks degrade to a safe
// no-op when nothing is found/held, matching RL_ACTION_SPACE_EXPANSION.md
// Phase 2e's existing "usable for manual Step calls against a world that
// happens to cooperate" framing — this hook exists purely to make that
// world more likely to cooperate, not to replace that framing.
type EpisodeSeeder func(ctx context.Context, agent SeedAgent, cfg Config) error

// defaultMineRequiredToolCount is how many of Config.MineRequiredTool
// DefaultEpisodeSeeder gives — 1 is enough for any tool (it isn't consumed
// the way a crafting ingredient is), kept as a named constant only so the
// intent ("just one") reads as deliberate, not an arbitrary leftover
// ingredient-count copy from seedGiveCount.
const defaultMineRequiredToolCount = 1

// anyItemPickupSeedItem is what DefaultEpisodeSeeder drops for
// Config.PickupAnyItem — a cheap, common item, since there's no specific
// name to seed for an "any item" task (docs/plans/
// RL_ACTION_SPACE_EXPANSION_III.md item 1).
const anyItemPickupSeedItem = "minecraft:stick"

// DefaultEpisodeSeeder implements EpisodeSeeder's minimum-viable contract:
// if Config.MineTargetBlock is set, ensures an instance exists within
// Config.MineSearchRadius (agent.SeedNearbyBlock) — unless
// Config.MineTargetBuried says it's already present elsewhere in the world
// and must not get a second, freshly-placed-beside-the-bot instance (see
// that field's own doc comment: caught live, this is not a hypothetical) —
// and, if Config.MineRequiredTool is also set and agent implements
// ToolSeedAgent, ensures that tool is in inventory first (agent.
// SeedGiveItem, unaffected by MineTargetBuried); if Config.CraftTargetItem
// is set, gives the bot its recipe's ingredients (agent.
// SeedCraftIngredients). A failure here is returned as a real Reset error,
// unlike mine/craft's own steady-state "no target visible/ready" — if
// seeding was explicitly requested (Config.Seeder is set), a failure to
// seed should be visible to the caller, not silently swallowed into what
// would otherwise look like an ordinary "nothing to do yet" episode.
func DefaultEpisodeSeeder(ctx context.Context, agent SeedAgent, cfg Config) error {
	if cfg.MineTargetBlock != "" {
		if cfg.MineRequiredTool != "" {
			if toolSeeder, ok := agent.(ToolSeedAgent); ok {
				if err := toolSeeder.SeedGiveItem(ctx, cfg.MineRequiredTool, defaultMineRequiredToolCount); err != nil {
					return fmt.Errorf("seeding mine required tool: %w", err)
				}
			}
		}
		if !cfg.MineTargetBuried {
			radius := cfg.MineSearchRadius
			if radius <= 0 {
				radius = DefaultConfig().MineSearchRadius
			}
			if err := agent.SeedNearbyBlock(ctx, cfg.MineTargetBlock, radius); err != nil {
				return fmt.Errorf("seeding mine target: %w", err)
			}
		}
	}
	if cfg.CraftTargetItem != "" {
		if err := agent.SeedCraftIngredients(ctx, cfg.CraftTargetItem); err != nil {
			return fmt.Errorf("seeding craft ingredients: %w", err)
		}
	}
	if cfg.PickupTargetItem != "" {
		// Optional, like ToolSeedAgent above: a SeedAgent that doesn't also
		// implement ItemDropSeedAgent is free to leave this task unseeded
		// rather than fail the episode over it (docs/plans/
		// RL_ACTION_SPACE_EXPANSION_II.md Phase 2) — *agent implements both,
		// so this only matters for a SeedAgent implementation that doesn't.
		if itemSeeder, ok := agent.(ItemDropSeedAgent); ok {
			radius := cfg.PickupSearchRadius
			if radius <= 0 {
				radius = DefaultConfig().PickupSearchRadius
			}
			if err := itemSeeder.SeedDroppedItem(ctx, cfg.PickupTargetItem, radius); err != nil {
				return fmt.Errorf("seeding pickup target: %w", err)
			}
		}
	} else if cfg.PickupAnyItem {
		// No specific item name to drop for "any item" mode — seeds a
		// fixed, cheap placeholder (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
		// item 1) so the task is still seedable standalone, the same way
		// every other task's seeder picks something concrete rather than
		// leaving the episode to depend on whatever happens to already be
		// lying around.
		if itemSeeder, ok := agent.(ItemDropSeedAgent); ok {
			radius := cfg.PickupSearchRadius
			if radius <= 0 {
				radius = DefaultConfig().PickupSearchRadius
			}
			if err := itemSeeder.SeedDroppedItem(ctx, anyItemPickupSeedItem, radius); err != nil {
				return fmt.Errorf("seeding pickup target (any item): %w", err)
			}
		}
	}
	if cfg.FlyTargetActive {
		// Optional, same pattern as ToolSeedAgent/ItemDropSeedAgent above.
		if flightSeeder, ok := agent.(FlightSeedAgent); ok {
			if err := flightSeeder.SeedElytra(ctx); err != nil {
				return fmt.Errorf("seeding flight elytra: %w", err)
			}
		}
	}
	if cfg.RideVehicleType != "" {
		// Optional, same pattern as the other task branches above.
		if vehicleSeeder, ok := agent.(VehicleSeedAgent); ok {
			radius := cfg.RideSearchRadius
			if radius <= 0 {
				radius = DefaultConfig().RideSearchRadius
			}
			// tame=true unconditionally: an untamed horse/donkey/mule
			// refuses mounting at all (see agent.SeedVehicle's own doc
			// comment), so there's no real-episode case where an
			// untamed instance would be useful here. The resulting
			// entity ID isn't needed here — Environment resolves its
			// own target position/ID separately via refreshRideTarget.
			if _, err := vehicleSeeder.SeedVehicle(ctx, cfg.RideVehicleType, radius, true); err != nil {
				return fmt.Errorf("seeding ride vehicle: %w", err)
			}
		}
	}
	if cfg.CombatTargetMobType != "" {
		// Optional, same pattern as the other task branches above.
		if combatSeeder, ok := agent.(CombatSeedAgent); ok {
			radius := cfg.CombatRadius
			if radius <= 0 {
				radius = DefaultConfig().CombatRadius
			}
			// The resulting entity ID isn't needed here — Environment
			// resolves its own target position/ID separately via
			// refreshCombatTarget.
			if _, err := combatSeeder.SeedCombatMob(ctx, cfg.CombatTargetMobType, int(radius)); err != nil {
				return fmt.Errorf("seeding combat mob: %w", err)
			}
		}
	}
	if cfg.CreeperGunpowderActive {
		// Optional, same pattern as the other task branches above.
		if creeperSeeder, ok := agent.(CreeperSeedAgent); ok {
			radius := cfg.CreeperGunpowderRadius
			if radius <= 0 {
				radius = DefaultConfig().CreeperGunpowderRadius
			}
			// The resulting entity ID isn't needed here — Environment
			// resolves its own target position/ID separately via
			// refreshCreeperTarget.
			if _, err := creeperSeeder.SeedCreeperForGunpowder(ctx, int(radius)); err != nil {
				return fmt.Errorf("seeding creeper for gunpowder: %w", err)
			}
		}
	}
	if cfg.SpearChargeTargetMobType != "" {
		// Optional, same pattern as the other task branches above.
		if spearSeeder, ok := agent.(SpearSeedAgent); ok {
			radius := cfg.SpearChargeRadius
			if radius <= 0 {
				radius = DefaultConfig().SpearChargeRadius
			}
			// The resulting entity ID isn't needed here — Environment
			// resolves its own target position/ID separately via
			// refreshSpearTarget.
			if _, err := spearSeeder.SeedSpearMob(ctx, cfg.SpearChargeTargetMobType, int(radius)); err != nil {
				return fmt.Errorf("seeding spear mob: %w", err)
			}
		}
	}
	if cfg.ChainStage == 0 && cfg.PlaceTargetItem != "" {
		// ToolSeedAgent is the base SeedAgent's own optional capability
		// already used for Config.MineRequiredTool — reused as-is, no new
		// capability needed (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md
		// item 2).
		if toolSeeder, ok := agent.(ToolSeedAgent); ok {
			if err := toolSeeder.SeedGiveItem(ctx, cfg.PlaceTargetItem, 1); err != nil {
				return fmt.Errorf("seeding standalone place target: %w", err)
			}
		}
	}
	return nil
}

// FarSeedAgent is the optional capability composite "go there, then
// mine/craft" episodes need: placing a block, or a crafting table plus the
// craft ingredients, at a specific position instead of beside the bot.
// Environment.Reset returns errFarSeedRequiresFarSeedAgent if it poses such
// an episode (with a Seeder set) to an agent without it.
type FarSeedAgent interface {
	// SeedBlockAt places blockName at (x, y, z).
	SeedBlockAt(ctx context.Context, blockName string, x, y, z int) error
	// SeedCraftIngredientsAt gives the bot itemName's ingredients (clearing
	// its inventory first, like SeedAgent.SeedCraftIngredients) and, if the
	// recipe needs a crafting table, places one at (x, y, z).
	SeedCraftIngredientsAt(ctx context.Context, itemName string, x, y, z int) error
}

// EntitySeedAgent is the optional capability a Combat episode needs
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0/5): summoning a
// hostile mob to fight, and cleaning it up at Reset if the episode didn't
// kill it. Not folded into SeedAgent: a chat-command-only session without
// RCON must never be required to implement it, same reasoning as
// ToolSeedAgent/FarSeedAgent.
type EntitySeedAgent interface {
	// SeedEntity ensures a live instance of mobType exists within radius
	// blocks of the bot (summoning one via RCON if none is), returning its
	// entity ID — see agent.SeedEntity's doc comment for exactly how nbt
	// is used.
	SeedEntity(ctx context.Context, mobType string, radius int, nbt string) (entityID int32, err error)
	// ClearEntitiesOfType kills every live instance of mobType within
	// radius blocks of (x, y, z) — see agent.ClearEntitiesOfType's doc
	// comment.
	ClearEntitiesOfType(ctx context.Context, x, y, z, radius int, mobType string) error
}

// VehicleSeedAgent is the optional capability a Mob Riding episode needs
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0/4): summoning a
// rideable vehicle, and cleaning it up at Reset. ClearEntitiesOfType has
// the same shape as EntitySeedAgent's own method of that name (a type+
// radius kill works identically for a vehicle as for a hostile mob) — an
// agent implementing both only needs one real method to satisfy both
// interfaces.
type VehicleSeedAgent interface {
	// SeedVehicle ensures a (if tame, tamed) instance of vehicleType
	// exists within radius blocks of the bot, returning its entity ID —
	// see agent.SeedVehicle's doc comment.
	SeedVehicle(ctx context.Context, vehicleType string, radius int, tame bool) (entityID int32, err error)
	ClearEntitiesOfType(ctx context.Context, x, y, z, radius int, mobType string) error
}

// ItemDropSeedAgent is the optional capability an Item Pickup episode needs
// when posed from a cold inventory rather than piggybacking on a mine
// task's own drop (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 0/2).
type ItemDropSeedAgent interface {
	// SeedDroppedItem drops one itemName on the ground within radius
	// blocks of the bot — see agent.SeedDroppedItem's doc comment.
	SeedDroppedItem(ctx context.Context, itemName string, radius int) error
}

// FlightSeedAgent is the optional capability a Flight episode needs
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 3): giving the bot an
// elytra (equipped) and a working firework-rocket supply.
type FlightSeedAgent interface {
	// SeedElytra ensures the bot has an elytra equipped and a working
	// firework-rocket supply — see agent.SeedElytra's doc comment.
	SeedElytra(ctx context.Context) error
}

// CombatSeedAgent is the optional capability a Combat episode needs
// (docs/plans/RL_ACTION_SPACE_EXPANSION_II.md Phase 5): summoning a
// non-aggressive mob to fight and equipping a weapon, in one call.
type CombatSeedAgent interface {
	// SeedCombatMob ensures a live instance of mobType within radius
	// blocks of the bot (via SeedEntity) and a weapon equipped — see
	// agent.SeedCombatMob's own doc comment.
	SeedCombatMob(ctx context.Context, mobType string, radius int) (entityID int32, err error)
}

// CreeperSeedAgent is the optional capability a creeper-gunpowder episode
// needs (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 5): summoning a
// live creeper and equipping a melee weapon, in one call.
type CreeperSeedAgent interface {
	// SeedCreeperForGunpowder ensures a live creeper within radius blocks
	// of the bot (via SeedEntity) and a melee weapon equipped — see
	// agent.SeedCreeperForGunpowder's own doc comment.
	SeedCreeperForGunpowder(ctx context.Context, radius int) (entityID int32, err error)
}

// SpearSeedAgent is the optional capability a SpearCharge episode needs
// (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md item 6): summoning a
// live mob and equipping a spear, in one call.
type SpearSeedAgent interface {
	// SeedSpearMob ensures a live instance of mobType within radius
	// blocks of the bot (via SeedEntity) and a spear equipped — see
	// agent.SeedSpearMob's own doc comment.
	SeedSpearMob(ctx context.Context, mobType string, radius int) (entityID int32, err error)
}

// ChainSeedAgent is the optional capability multi-step chain episodes
// (Config.ChainStage) need on top of FarSeedAgent: starting each one from an
// empty inventory and with no stray drops lying around. A drop a previous
// episode left near the tree, or near where the bot stood, would otherwise
// be picked up by a later episode as a free log.
type ChainSeedAgent interface {
	// ClearInventory empties the bot's inventory.
	ClearInventory(ctx context.Context) error
	// ClearDroppedItems removes dropped item entities within radius blocks
	// of (x, y, z).
	ClearDroppedItems(ctx context.Context, x, y, z, radius int) error
}
