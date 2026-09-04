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
//   - Two task types, independently configurable and both optional on any
//     given instance: reach a target position a fixed offset from wherever
//     the bot was standing at Reset (Config.TargetOffset), and mine the
//     nearest visible instance of a configured block name
//     (Config.MineTargetBlock — docs/plans/RL_ACTION_SPACE_EXPANSION.md
//     Phase 2). Real episode resets (reconnect/respawn/teleport, or a
//     restored world snapshot — RSI_TRAINING_PLAN.md item 2) aren't wired
//     up yet — see Environment.Reset. Posing an actual mine episode (making
//     sure a minable block of the target type exists reachable at Reset)
//     is also still open — RL_ACTION_SPACE_EXPANSION.md Phase 2e, deferred
//     to mc-rsi-trainer's task generator.
//   - A four-action vocabulary (Wait, GoToTarget, ReturnHome, Mine), not
//     yet the full GO_TO/MINE/CRAFT/RETURN_HOME/WAIT set
//     RL_POLICY_INTEGRATION_PLAN.md sketched — CRAFT has no mapped
//     capability yet (RL_ACTION_SPACE_EXPANSION.md Phase 3, not started).
//   - Goal-conditioning for the mine task is coarse (RL_ACTION_SPACE_EXPANSION.md
//     Phase 2a option (a)): one fixed target block name per Environment
//     instance, set once via Config, not a per-episode observation feature
//     a policy could vary. The finer-grained version (docs/plans/06's
//     "Note: on the observation side..." / cRL-go's
//     docs/archive/plans/13's "goal block appended to Observation.Values")
//     is still the path once a second block type or task type actually
//     needs to vary within a single training run — don't build it
//     speculatively.
//   - No persistent world-knowledge memory (RL_POLICY_INTEGRATION_PLAN.md's
//     "Does the Observation Builder need access to *persistent* world
//     knowledge" open question): out of scope here too.
//
// These are documented simplifications, not hidden gaps — see each file's
// doc comment for the specific design decision and why.
package rlenv
