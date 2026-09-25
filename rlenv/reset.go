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

// BlockRestorer is the optional capability Environment.Reset uses to undo a
// training episode's mining: it puts a block back (via RCON, like
// SeedAgent's placement) so a task whose target is part of the terrain
// itself - dirt or grass in a superflat world - doesn't eat the floor a
// little more every episode. Found live: after hours of mine episodes the
// working areas were cratered, bots sank into pits after every reset
// teleport ("position never synced"), and episodes slowed steadily.
// Optional and type-asserted like ResetAgent: Reset simply skips it for an
// agent without it.
type BlockRestorer interface {
	RestoreBlock(ctx context.Context, x, y, z int, blockName string) error
}

// AreaClearer is the optional capability Environment.Reset uses, with
// Config.ClearAreaRadius, to wipe the space above the ground around the
// reset origin between episodes: seeding places blocks and crafting tables
// at the bot's own level and mining/crafting episodes leave them behind, so
// a working area slowly turned into an obstacle course (found live: a bot
// wedged in a notch between a seeded gravel block, a crafting table and a
// stray block). ClearAir fills the inclusive box between the corners with
// air.
type AreaClearer interface {
	ClearAir(ctx context.Context, x1, y1, z1, x2, y2, z2 int) error
}
