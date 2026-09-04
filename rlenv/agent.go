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
// WorldOperations (GetWorld() World): Environment has no other use for raw
// World access.
type LiveAgent interface {
	models.CommandAgent
	models.HealthProvider

	// BlockNameAt returns the block name at the given integer block
	// position (e.g. "minecraft:stone"), "minecraft:air" if broken/empty,
	// or an "unknown"/placeholder string if the chunk isn't loaded or the
	// state can't be resolved — see agent/actions.go's BlockNameAt.
	BlockNameAt(ix, iy, iz int) string
}
