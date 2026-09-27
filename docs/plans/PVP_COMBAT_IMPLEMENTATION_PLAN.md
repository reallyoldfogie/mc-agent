# PvP and Combat Implementation Plan

Status: Phase 3 in progress
Worktree: `.worktrees/pvp-combat`
Branch: `pvp-combat`

This plan turns the requirements in [`docs/prompts/combat_prompt.md`](../prompts/combat_prompt.md) into incremental, testable work. The target is an autonomous combat subsystem that can operate against hostile mobs and players; packet primitives alone do not count as completion.

## Current baseline

Already available:

- Version-specific entity attack packets and entity interaction packet parsing.
- Entity position, health, attributes, equipment, effects, and visibility tracking.
- Bow trajectory simulation, line-of-sight validation, targeted bow firing, and projectile callbacks.
- Incoming damage-event parsing and knockback handling.
- Basic live tests for attacking a zombie and sending repeated attacks.

Known gaps are recorded in the assessment below and will be closed by the phases that follow.

## Phases

### Phase 1 — Combat contracts and safe primitives

Goal: expose a usable high-level attack surface and define contracts that the controller can depend on.

- [x] Create this implementation plan.
- [x] Create a dedicated worktree and branch.
- [x] Expose `AgentActions.AttackEntity(ctx, entityID, sneaking)`.
- [x] Add reach, line-of-sight, and target-validity checks around the primitive.
- [x] Add attack-cooldown checks around the primitive.
- [ ] Add unit tests for cancellation, missing handlers, and successful dispatch.
- [x] Define combat data types and interfaces in a focused package without coupling the decision logic to packet handlers.

### Phase 2 — Threat model and target selection

Goal: turn tracked entities into prioritized combat targets.

- [x] Define hostile/neutral/player classification and explicit opt-in filters.
- [x] Define the initial target snapshot and ranking contract.
- [x] Implement the prompt’s priority ordering: player > aggressive mob > neutral mob, proximity, and low-health tie-breakers.
- [x] Handle removed, invisible, and unknown targets in the pure ranker.
- [x] Add deterministic unit tests for ranking and filtering.

Supporting document: `docs/plans/combat/THREAT_MODEL.md`.

### Phase 3 — Combat decision/state controller

Goal: provide autonomous decisions independent of the transport implementation.

- [x] Implement `Idle`, `Engaging`, `Evading`, and `Retreating` states.
- [x] Add transitions based on target availability, health ratio, enemy count, and line of sight.
- [x] Add weapon selection: melee within 5 blocks, ranged beyond 8, hysteresis in the 5–8 block band.
- [x] Add initial attack scheduling and cooldown policy.
- [x] Keep timing injected through timestamps so controller tests do not require a Minecraft server.
- [x] Add a stateful tick controller with explicit successful-attack commit semantics.

Supporting document: `docs/plans/combat/STATE_MACHINE.md`.

### Phase 4 — Melee behavior and defense

Goal: make close combat survivable and mechanically correct.

- [x] Implement the initial target approach and spacing intent at melee reach.
- [x] Add the initial terrain/cover safety filter for movement intents.
- [ ] Implement strafing/position variation and critical-hit preconditions.
- [ ] Implement shield equip/block timing and shield-aware target behavior.
- [ ] Add Java 1.21.11+ spear support: recognize tiered spear items, select Jab versus held Charge, honor spear minimum/extended reach, and account for velocity/view-angle damage rules.
- [ ] Implement low-health evasion below 40%, retreat, cover seeking, and anti-surround positioning.
- [ ] Account for terrain, water, obstacles, and hazardous targets such as creepers and skeletons.
- [ ] Add live tests for melee damage, cooldown behavior, shield blocking, knockback, retreat, and multi-target handling.

Supporting document: `docs/plans/combat/DEFENSE_AND_MELEE.md`.

### Phase 5 — Ranged weapon integration

Goal: complete the projectile side of the combat contract.

- [ ] Define a ranged-attack request containing target entity, predicted position, weapon, priority, and cancellation policy.
- [ ] Integrate bow firing with entity targets and moving-target leading.
- [ ] Add crossbow charge/fire support.
- [ ] Add trident throw support, including inventory selection and trajectory validation.
- [ ] Preserve projectile hit callbacks and correlate hits with the selected target.
- [ ] Add moving-target and obstruction tests.

Supporting document: `docs/plans/combat/RANGED_INTEGRATION.md`.

### Phase 6 — PvP and end-to-end validation

Goal: validate the complete subsystem against real server behavior.

- [ ] Add player-vs-player target discovery and explicit friendly-fire policy.
- [ ] Add tests for player targeting, armor/equipment changes, shields, projectile attacks, and death/respawn.
- [ ] Add multi-version live coverage for supported protocol versions.
- [ ] Add combat metrics/logging sufficient to diagnose target choice, state transitions, attacks, misses, and retreats.
- [ ] Update README/testing status and mark this plan complete only after the autonomous scenario passes.

## Completion criteria

Combat is complete only when the agent can, without direct per-attack commands:

1. Detect and prioritize configured hostile mobs and players.
2. Select and use an appropriate melee or ranged weapon.
3. Respect reach, visibility, cooldowns, and projectile travel.
4. Attack, strafe, block, dodge, retreat, and recover according to state.
5. Provide deterministic unit coverage plus live end-to-end coverage.

## Progress log

### 2026-09-25 — Initial implementation started

- Created worktree `.worktrees/pvp-combat` on branch `pvp-combat`.
- Added this phased plan.
- Implemented the first high-level primitive: `AgentActions.AttackEntity` with context cancellation and delegation to the existing version-specific entity handler.
- Added the transport-independent `combat` target classifier/ranker and its unit tests.
- Added `docs/plans/combat/THREAT_MODEL.md`.
- `GOWORK=off GOCACHE=/tmp/mc-agent-go-build go test ./combat ./items ./agent ./physics` reached compilation and passed `combat`, `items`, and `physics`.
- The `agent` package tests were blocked by the sandbox's disabled network while initialization attempted to download the Mojang version manifest; this is environmental and not a compile error from the change.
- The nested worktree requires `GOWORK=off` because the parent checkout's Go workspace otherwise treats the worktree path as part of the module import path.

### 2026-09-25 — Decision layer continued

- Added `combat.Decide`, `ChooseWeapon`, `AttackInterval`, and `ReadyToAttack`.
- Added deterministic state-transition, weapon-hysteresis, and cooldown tests.
- Added `docs/plans/combat/STATE_MACHINE.md`.
- Added tracked-target, reach, and line-of-sight validation to `agent.AttackEntity`.
- Added the `agent.RankCombatTargets` adapter, which resolves tracked entities, registry categories, distance, and line of sight before ranking.
- Added global melee cooldown enforcement around the attack primitive.
- The next step is wiring a controller executor for approach/strafe/retreat movement and beginning the melee-defense phase.

### 2026-09-25 — Movement intent boundary

- Added deterministic `MovementFor` intents for approach, alternating strafe, evasion, and retreat.
- Added tests covering approach direction, lateral evasion, sprint/jump retreat, and strafe variation.
- Added `agent.ApplyCombatMovement` to apply one movement intent tick to the manual physics executor.
- Added `docs/plans/combat/DEFENSE_AND_MELEE.md`.
- Added `combat.Controller`, which combines decisions, movement intents, cooldown readiness, and alternating lateral movement without sending packets.
- Added `combat.FilterMovement` and deterministic safety tests for fall risk, water, and exposed hazards.
- Added `agent.CombatMovementEnvironment`, which derives ground, water, forward fall-risk, and cover facts from loaded world data.
- Added `agent.RunCombat` for the first conservative melee-only observation/decision/safety/movement/attack loop.
- Added `docs/plans/combat/AUTONOMOUS_LOOP.md`.
- Verified that the initial loop's fixed 50 ms ticker violated the server-tick-rate contract: `onSetTickingState` already updates `serverTickRateBits`, and the physics executor dynamically consumes it.
- Changed `agent.RunCombat` to use a resettable timer derived from the current server tick rate, retaining the existing vanilla fallback only before the server reports a rate. Added tests for 10, 20, and 40 TPS intervals.
- Added the transport-independent `combat.RangedAttackRequest`, validation, and deterministic moving-target lead calculation as the first ranged executor boundary.
- Added the agent ranged adapter for bow and trident execution, target timing construction, and successful-fire cooldown commits in `RunCombat`.
- Hardened ranged execution so a missing inventory item is a failed attack, never an attempt to fire whatever item happens to be held.
- Added spear-specific combat policy primitives and `docs/plans/combat/SPEAR_SUPPORT.md`; generic attack packets are explicitly not treated as complete spear Charge support.
- Added validated 1.21.11+ spear Jab execution with tier selection and caller-supplied item-component reach bounds.
- Added base crossbow charge/fire support using context-aware held-use/release packets; enchantment-specific charge timing and loaded-state tracking remain.
- Added the initial spear Charge adapter with context-safe held-use/release, caller-supplied view/velocity eligibility thresholds, and explicit target aiming.
- Added the transport-independent spear Engaged/Tired/Disengaged stage model and contact cooldown policy with version/material-specific timing inputs.
- Connected spear Charge hold duration to stage-specific contact outcomes while keeping damage/knockback/dismount resolution server-authoritative.
- Made autonomous ranged execution fall back across bow, crossbow, and trident, and added crossbow trajectory obstruction validation before packet emission.
- Propagated tracked-entity velocity into ranged requests so autonomous projectile aiming can lead moving targets.
- Added Java 1.21.11+ spear version gating so older handlers reject spear actions explicitly.
- Added profile-driven spear Jab cooldown and multi-target selection policy, leaving material values to the version/item adapter.
- Added the first live autonomous combat integration test for melee target discovery, loop execution, and damage confirmation on the shared version/world harness.
- Added live autonomous bow coverage against a stationary target beyond melee range, including bow/arrow provisioning and server-tracked damage confirmation.
- Added live autonomous crossbow coverage against a stationary target, exercising weapon fallback and charge/release behavior.
- Added live moving-target coverage and an obstruction scenario verifying blocked line-of-sight before autonomous execution.
- Added version-gated 1.21.11+ live spear Jab damage and Charge packet-execution scenarios.
- Attempted `go test ./testing -run '^TestCombatFlatSuite$' -count=1 -v`; all configured versions were skipped because the environment could not access Docker at `/var/run/docker.sock` (`permission denied`). No version-specific implementation result is available yet.
- Investigated the first live run's non-timeout failures: the 1.21.1 crossbow setup raced the client inventory update, and the obstruction test checked LOS before the RCON `fill` changes reached the agent's local world cache.
- Fixed the crossbow setup to wait for the item in client-tracked inventory and use inventory-aware `SwitchToItem`; fixed obstruction setup to wait for a wall block in the client world before checking LOS.
- Verified `GOWORK=off GOCACHE=/tmp/mc-agent-go-build go test ./testing -run '^$'` and `git diff --check`.
- The next focused live run completed all configured versions without a global timeout. Crossbow and obstruction coverage passed; the remaining failures were intermittent `/give` inventory races in direct sword/bow setup and spear Charge setup.
- Generalized combat test equipment setup to wait for client-tracked inventory state and use `SwitchToItem` for sword, bow, crossbow, and spear cases.
- Verified the updated integration package compiles with `GOWORK=off GOCACHE=/tmp/mc-agent-go-build go test ./testing -run '^$'` and passes `git diff --check`.
- Next step: rerun the focused live combat suite to validate the generalized inventory synchronization, then investigate any failures that reach the combat assertions or packet execution.

### 2026-09-26 — Projectile lock-order deadlock

- Diagnosed the two-hour integration timeout in `1.21.9/TestRunCombatMovingTarget` as an inverted lock ordering between `entitiesMu` and `activeProjectilesMu`.
- Refactored projectile rendering so interpolation state is updated under `activeProjectilesMu`, while block/entity collision checks and entity snapshots run after releasing it.
- Refactored entity velocity updates to check projectile state and entity state in separate critical sections, eliminating nested acquisition of the two mutexes.
- Added `agent/render_lock_test.go`, which concurrently exercises projectile rendering and entity velocity updates under the former deadlock scenario.
- Audited all `entitiesMu`/`activeProjectilesMu` uses in the agent package; no other path retains one while acquiring the other.
- Verified the focused regression test with and without `-race`. Full `agent` tests still report pre-existing skin-manager test failures unrelated to this change.
- Next step: rerun the focused live combat suite and confirm `TestRunCombatMovingTarget` completes through all configured versions without a timeout or leaked server container.

### 2026-09-26 — Spear integration failure fixes

- Fixed transient server-to-client inventory convergence in `SwitchToItem` by retrying item discovery for a bounded two-second window; this covers the 1.21.11 spear Jab failure where the item was present in the client inventory but not yet visible to the next lookup.
- Fixed spear Charge aiming and visibility validation to use the target entity body rather than its feet position, avoiding ground-block occlusion on 1.21.11 and 26.1 flat-world scenarios.
- Verified the agent deadlock regression test with and without `-race`, the combat package, and the integration package compilation.
- The next step is to rerun the full focused live combat suite and verify all 1.21.11+ spear Jab and Charge cases pass.
