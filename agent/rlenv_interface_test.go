package agent

import "github.com/reallyoldfogie/mc-agent/rlenv"

// Compile-time check that *agent satisfies rlenv.LiveAgent, so
// cmd/rl-train's runtime type assertion (models.Agent -> rlenv.LiveAgent)
// is guaranteed to succeed rather than discovered broken only when someone
// actually runs it — see docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 3.
var (
	_ rlenv.LiveAgent  = (*agent)(nil)
	_ rlenv.SeedAgent  = (*agent)(nil)
	_ rlenv.ResetAgent = (*agent)(nil)
)
