# Autonomous Combat Loop

`agent.RunCombat` is the first production execution path for the combat controller. It runs at the Minecraft tick interval and performs:

1. Snapshot and rank tracked targets.
2. Build an observation containing health, enemy count, distance, direction, and visibility.
3. Ask `combat.Controller` for state, weapon, attack readiness, and movement intent.
4. Query loaded-world movement safety and filter the intent.
5. Apply one movement tick through the manual physics executor.
6. Send a melee attack only when the controller permits it and the selected weapon class is melee.
7. Commit cooldown state only after the attack packet succeeds.

The loop intentionally does not send ranged attacks yet. A target farther than melee range can still influence movement and weapon selection, but ranged execution belongs to the projectile integration phase and must not be represented as a melee packet.

The loop returns errors for unavailable movement/world state and returns the caller's context error on cancellation.

## Timing contract

The loop must follow the server's current tick rate rather than owning a fixed
20 TPS ticker. `ClientboundSetTickingState` updates the agent's atomic server
tick-rate value, which is also consumed by the continuous physics executor.
Combat recalculates its timer interval after every iteration, so runtime `/tick`
rate changes are honored. Before the server reports a rate, the existing
vanilla 20 TPS fallback is used.

Future work should add explicit stop/cleanup policy, metrics, target-loss
hysteresis, and a ranged executor.
