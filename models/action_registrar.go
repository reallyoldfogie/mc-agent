package models

// ActionRegistrar lets external code add a command to an agent's existing
// action registry (see actions.NewRegistry/RegisterDefaults), without
// forking or replacing it. Anything registered this way is immediately
// reachable through every trigger surface that already dispatches through
// that registry: chat commands via the ">>>botName<<<" convention (see
// agent/commands.go's handleChatCommand) and any RL/LLM executor driving
// the same ActionRegistry[CommandAgent] mechanism (e.g. cmd/rl-train) -
// both already call Execute on a registry of exactly this shape, so one
// registered Action reaches both for free.
type ActionRegistrar interface {
	RegisterAction(action Action[CommandAgent])
}
