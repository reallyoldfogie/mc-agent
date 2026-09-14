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
//   - Per-episode task selection and goal-conditioning
//     (../mc-rsi-trainer/docs/plans/06-per-episode-task-selection-and-goal-conditioning.md):
//     done, not speculative. Config.TaskSelector, if set, is called once
//     per Reset to choose which task(s) are active that specific episode
//     (see task.go's own doc comment), overriding TargetOffset/
//     MineTargetBlock/MineSearchRadius/CraftTargetItem/GoToTargetDisabled
//     for that episode only — a static Config with no TaskSelector behaves
//     exactly as before, unchanged. Every Observation this package
//     produces also now carries a three-bit goal-conditioning block
//     (indices 14-16 — see observation.go's own doc comment on
//     observationSize) telling a policy which task(s) are actually active
//     this episode, mirroring cRL-go's pkg/hierarchical subgoal-one-hot
//     pattern rather than inventing a new encoding. Promotes
//     testing/rl_train_test.go's live-verified
//     newAlternatingMineOrCraftTaskSelector proof-of-concept into this
//     reusable capability.
//   - No persistent world-knowledge memory (RL_POLICY_INTEGRATION_PLAN.md's
//     "Does the Observation Builder need access to *persistent* world
//     knowledge" open question): out of scope here too.
//
// These are documented simplifications, not hidden gaps — see each file's
// doc comment for the specific design decision and why.
package rlenv
