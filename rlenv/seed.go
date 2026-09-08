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

// DefaultEpisodeSeeder implements EpisodeSeeder's minimum-viable contract:
// if Config.MineTargetBlock is set, ensures an instance exists within
// Config.MineSearchRadius (agent.SeedNearbyBlock); if
// Config.CraftTargetItem is set, gives the bot its recipe's ingredients
// (agent.SeedCraftIngredients). A failure here is returned as a real Reset
// error, unlike mine/craft's own steady-state "no target visible/ready" —
// if seeding was explicitly requested (Config.Seeder is set), a failure to
// seed should be visible to the caller, not silently swallowed into what
// would otherwise look like an ordinary "nothing to do yet" episode.
func DefaultEpisodeSeeder(ctx context.Context, agent SeedAgent, cfg Config) error {
	if cfg.MineTargetBlock != "" {
		radius := cfg.MineSearchRadius
		if radius <= 0 {
			radius = DefaultConfig().MineSearchRadius
		}
		if err := agent.SeedNearbyBlock(ctx, cfg.MineTargetBlock, radius); err != nil {
			return fmt.Errorf("seeding mine target: %w", err)
		}
	}
	if cfg.CraftTargetItem != "" {
		if err := agent.SeedCraftIngredients(ctx, cfg.CraftTargetItem); err != nil {
			return fmt.Errorf("seeding craft ingredients: %w", err)
		}
	}
	return nil
}
