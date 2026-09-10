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
//   - Three task types, independently configurable and all optional on any
//     given instance: reach a target position a fixed offset from wherever
//     the bot was standing at Reset (Config.TargetOffset), mine the
//     nearest visible instance of a configured block name
//     (Config.MineTargetBlock — docs/plans/RL_ACTION_SPACE_EXPANSION.md
//     Phase 2), and craft a configured item (Config.CraftTargetItem —
//     docs/plans/RL_TRAINING_LOOP_PLAN.md Phase 1). Real episode resets
//     (RSI_TRAINING_PLAN.md item 2) are now wired for the "reach a target"
//     task — Config.ResetOrigin teleports the bot via RCON, Config.Jitter
//     varies the posed origin/target per episode — see Environment.Reset;
//     a restored-world-snapshot alternative is still open. Posing an
//     actual mine episode (making sure a minable block of the target type
//     exists reachable at Reset) is also still open —
//     RL_ACTION_SPACE_EXPANSION.md Phase 2e, deferred to mc-rsi-trainer's
//     task generator.
//   - A four-action vocabulary (Wait, GoToTarget, Mine, Craft).
//     RL_POLICY_INTEGRATION_PLAN.md originally sketched a fifth,
//     RETURN_HOME, but it was dropped (2026-09-09, see rlenv/action.go's
//     own doc comment) — no reward component ever gave it a job, and an
//     always-reward-irrelevant action is pure wasted action-space
//     probability. Note that all four actions are always present on every
//     Environment instance regardless of which of the three task types
//     above are actually configured — an unconfigured Mine/Craft becomes a
//     safe no-op (see resolveDispatch) that resolveDispatch itself still
//     never sends anywhere. Real per-instance action masking now exists on
//     the cRL-go side (built 2026-09-10, commit d3a3153 — see cRL-go's
//     docs/plans/19-training-time-action-masking.md) and this package
//     implements it (Environment.ActionMask, action.go): pkg/reinforce and
//     pkg/ppo's rollout loops pick it up automatically via rl.ActionMasker,
//     no extra wiring needed per training run.
//   - Goal-conditioning for the mine/craft tasks is coarse
//     (RL_ACTION_SPACE_EXPANSION.md Phase 2a option (a),
//     RL_TRAINING_LOOP_PLAN.md Phase 1a): one fixed target block/item name
//     per Environment instance, set once via Config, not a per-episode
//     observation feature a policy could vary. The finer-grained version
//     (docs/plans/06's "Note: on the observation side..." / cRL-go's
//     docs/archive/plans/13's "goal block appended to Observation.Values")
//     is still the path once a second block/item type or task type
//     actually needs to vary within a single training run — don't build it
//     speculatively.
//   - No persistent world-knowledge memory (RL_POLICY_INTEGRATION_PLAN.md's
//     "Does the Observation Builder need access to *persistent* world
//     knowledge" open question): out of scope here too.
//
// These are documented simplifications, not hidden gaps — see each file's
// doc comment for the specific design decision and why.
package rlenv
