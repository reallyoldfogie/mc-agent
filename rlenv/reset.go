package rlenv

import "context"

// ResetAgent is the minimal capability Environment.Reset needs beyond
// LiveAgent to restore an episode to a known starting position — see
// Config.ResetOrigin. Deliberately not folded into LiveAgent, same
// reasoning as SeedAgent (seed.go): a chat-command-driven session should
// never be required to have RCON.
type ResetAgent interface {
	// TeleportTo moves the bot to the given world coordinates via RCON and
	// waits for the bot's own tracked position to reflect the move before
	// returning — see agent.TeleportTo's doc comment for exactly how.
	TeleportTo(ctx context.Context, x, y, z float64) error
}
