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
