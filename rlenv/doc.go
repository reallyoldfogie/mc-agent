// Package rlenv is mc-agent's live rl.Environment adapter — item 5 of
// docs/plans/RL_POLICY_INTEGRATION_PLAN.md — wrapping a bot session so
// cRL-go's pkg/reinforce/pkg/ppo trainers, and later mc-rsi-trainer, can
// train and run policies against a live Minecraft session through the same
// rl.Environment interface the toy environments (snakeenv, gridworldenv)
// satisfy.
//
// # Scope of this first implementation
//
// This package answers RL_POLICY_INTEGRATION_PLAN.md's open questions
// concretely enough to produce a real, testable environment, but
// deliberately narrowly:
//
//   - One task type: reach a target position a fixed offset from wherever
//     the bot was standing at Reset (see Config.TargetOffset). Real episode
//     resets (reconnect/respawn/teleport, or a restored world snapshot —
//     RSI_TRAINING_PLAN.md item 2) aren't wired up yet — see Environment.Reset.
//   - A three-action vocabulary (Wait, GoToTarget, ReturnHome), not the
//     full GO_TO/MINE/CRAFT/RETURN_HOME/WAIT set RL_POLICY_INTEGRATION_PLAN.md
//     sketched: as of this branch's base commit, mc-agent's action registry
//     (actions.NewRegistry) has no mine/craft actions to map to, so this
//     package only maps to what actually exists (moveto).
//   - No goal-conditioning (docs/plans/06's "Note: on the observation
//     side..." / cRL-go's docs/archive/plans/13): there's only one task
//     type, so there's nothing to condition on yet. When a second task type
//     is added, the fixed-length observation vector (see observation.go)
//     is where a task-type feature would be introduced.
//   - No persistent world-knowledge memory (RL_POLICY_INTEGRATION_PLAN.md's
//     "Does the Observation Builder need access to *persistent* world
//     knowledge" open question): out of scope here too.
//
// These are documented simplifications, not hidden gaps — see each file's
// doc comment for the specific design decision and why.
package rlenv
