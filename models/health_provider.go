package models

// HealthProvider is satisfied by an agent that tracks its own health/food
// state from HealthChange events. Kept separate from CommandAgent (rather
// than adding these methods to that already-large interface) so existing
// CommandAgent implementations/fakes don't need to change to keep
// compiling; a caller that needs both embeds or requires both interfaces.
type HealthProvider interface {
	// Health returns the most recently observed health, food level, and
	// food saturation, and whether any value has been observed yet (known
	// is false before the first HealthChange event, e.g. before the bot
	// has finished joining).
	Health() (health float32, food int32, saturation float32, known bool)
}
