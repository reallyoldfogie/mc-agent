# Ranged Combat Integration

The ranged executor consumes `combat.RangedAttackRequest`. The combat package
owns target identity, weapon family, predicted position inputs, projectile
timing, priority, and target-loss cancellation policy; the agent adapter owns
inventory selection, trajectory validation, packet emission, and callbacks.

`RangedAttackRequest.LeadPosition` provides a deterministic first-order lead
for moving targets. It does not replace the existing physics trajectory solver,
which remains responsible for gravity, obstructions, and weapon-specific aim.

## Implementation sequence

- [x] Define and validate the transport-independent ranged request.
- [x] Add deterministic moving-target lead tests.
- [x] Adapt bow execution to consume the request and commit cooldown only after
  the fire request succeeds.
- [x] Add base crossbow charge/fire support; trident throw is routed through
  the existing projectile primitive. Enchantment-specific charge timing and
  loaded-state tracking remain.
- [x] Make autonomous ranged execution try bow, crossbow, then trident and
  reject crossbow trajectories blocked by loaded-world geometry.
- [x] Propagate tracked-entity velocity into moving-target lead requests.
- [x] Refuse execution when the requested ranged item is absent instead of
  attempting to fire the currently held item.
- [ ] Correlate projectile callbacks with the request target.
- [ ] Add moving-target and obstruction integration tests.
