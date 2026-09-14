package rlenv

import "github.com/reallyoldfogie/mc-agent/models"

// LiveAgent is the surface Environment needs from a bot session: full
// command-dispatch capability (so mapped actions can run through the same
// models.ActionRegistry the chat-command path uses), health/food tracking
// (so reward and observations can see damage/hunger), and reading back a
// single block's name by position (so mine reward can tell whether the
// target block actually changed — see reward.go and
// Environment.resolveMineTarget). *agent.agent satisfies this once it
// implements models.HealthProvider — see agent/tracking.go's Health method
// — and already implements BlockNameAt via models.WorldOperations
// (agent/actions.go). Only BlockNameAt is pulled in here, not all of
// WorldOperations (GetWorld() World) — raw World/BlockShapeManager access
// is a separate, optional capability (WalkabilityAgent, walkability.go),
// not part of LiveAgent's own required contract.
type LiveAgent interface {
	models.CommandAgent
	models.HealthProvider

	// BlockNameAt returns the block name at the given integer block
	// position (e.g. "minecraft:stone"), "minecraft:air" if broken/empty,
	// or an "unknown"/placeholder string if the chunk isn't loaded or the
	// state can't be resolved — see agent/actions.go's BlockNameAt.
	BlockNameAt(ix, iy, iz int) string

	// InventoryCount returns how many units of itemName the bot currently
	// holds — see agent.InventoryCount's doc comment (agent/craft.go) for
	// exactly which slots count. Used by the craft task's reward to judge
	// "did crafting actually produce more of the target item" from a
	// before/after count delta — docs/plans/RL_TRAINING_LOOP_PLAN.md
	// Phase 1d, the craft analogue of BlockNameAt's role for mine.
	InventoryCount(itemName string) int

	// Craftable reports whether itemName's recipe currently looks
	// assembleable from held ingredients — see agent.Craftable's doc
	// comment for exactly what "looks" means (an approximate signal, not a
	// guarantee). Used by the craftReady observation feature
	// (docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1c).
	Craftable(itemName string) bool
}
