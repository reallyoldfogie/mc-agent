package models

// TeamsUpdate represents a complete scoreboard Teams packet update
// (vanilla's "Teams" packet) — used for combat-competition team
// membership and friendly-fire tracking, see
// docs/plans/COMBAT_COMPETITION_PLAN.md Phase 0 item 4. Only the fields
// this codebase actually consumes are extracted; nameTagVisibility,
// collisionRule, formatting, prefix, and suffix are not tracked.
type TeamsUpdate struct {
	// Team is the team's internal name, stable across modes.
	Team string
	// Mode mirrors the vanilla packet's own mode string exactly:
	// "add", "remove", "change", "join", or "leave".
	Mode string
	// FriendlyFire is only meaningful when Mode is "add" or "change".
	FriendlyFire bool
	// Players carries the affected player names when Mode is "add",
	// "join", or "leave"; nil for "remove"/"change".
	Players []string
}
