# Melee and Defense Integration

The first live adapter is intentionally narrow: `agent.ApplyCombatMovement` applies one `combat.MovementIntent` to the existing manual physics executor. It does not enter or exit manual mode, because a combat loop must retain manual mode across ticks rather than repeatedly resetting the executor.

Current movement mapping:

- `ApproachTarget`: move along the horizontal target direction until melee reach.
- `StrafeTarget`: move perpendicular to the target direction; the controller should alternate the selected side.
- `EvadeTarget`: strafe with sprint and jump enabled.
- `RetreatFromTarget`: move opposite the target with sprint and jump enabled.

`combat.FilterMovement` is the safety boundary before these intents reach the executor. It stops on unstable ground or fall risk, disables sprinting in water, and changes exposed approach/strafe movement into evasive movement against a marked hazardous target.

Still required before this phase is complete:

- `agent.CombatMovementEnvironment` now populates `MovementEnvironment` from loaded terrain, water, forward fall-risk, and line-of-sight cover queries.
- Add weapon-specific melee attack strength and critical-hit preconditions.
- Add shield equip/block timing.
- `agent.RunCombat` now provides the first autonomous observation, ranking, decision, safety-filter, movement, and melee-attack loop.
