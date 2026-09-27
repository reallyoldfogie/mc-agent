# Combat State Machine

The controller makes a decision from a fresh observation; it does not send packets itself. An agent adapter supplies health, enemy count, target visibility/distance, and the currently selected weapon, then executes the returned decision.

Rules currently implemented in `combat.Decide`:

- No target enters `Idle`.
- Health at or below 20%, or four or more enemies, enters `Retreating`.
- Health below 40% or loss of line of sight enters `Evading`.
- Otherwise the agent is `Engaging`; an attack is permitted only with a visible target.
- Melee is selected at 5 blocks or closer, ranged beyond 8 blocks, and the current weapon is retained in the transition band.

`ReadyToAttack` receives timestamps from the adapter rather than reading a package clock. This keeps cooldown tests deterministic and allows the production controller to use its tick clock.
