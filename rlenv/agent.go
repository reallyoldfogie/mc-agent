package rlenv

import "github.com/reallyoldfogie/mc-agent/models"

// LiveAgent is the surface Environment needs from a bot session: full
// command-dispatch capability (so mapped actions can run through the same
// models.ActionRegistry the chat-command path uses) plus health/food
// tracking (so reward and observations can see damage/hunger). *agent.agent
// satisfies this once it implements models.HealthProvider — see
// agent/tracking.go's Health method.
type LiveAgent interface {
	models.CommandAgent
	models.HealthProvider
}
