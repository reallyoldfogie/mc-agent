# Spear Support (Java 1.21.11+)

Minecraft Java 1.21.11 adds tiered spear items with two distinct attacks:

- Jab: quick primary-action attack with material-dependent cooldown and
  multi-target behavior.
- Charge: held secondary-use attack with staged state, minimum reach, extended
  maximum reach, and damage/knockback/dismount behavior based on view angle and
  relative entity velocity.

The current generic `AttackEntity` primitive is not sufficient for Charge. It
uses the normal attack interaction and a fixed reach, so it must not be used as
the implementation of spear charge.

## Implementation checklist

- [x] Recognize all 1.21.11 spear item IDs in the combat policy layer.
- [x] Model Jab versus Charge as an explicit combat decision boundary.
- [x] Implement validated spear Jab execution with item selection and
  item-component-supplied reach bounds.
- [x] Implement the initial held Charge input with context-safe release and
  explicit eligibility thresholds.
- [x] Include caller-supplied attacker/target relative speed and view-angle
  thresholds in charge decisions.
- [x] Add 1.21.11+ version gating in the transport-independent policy and
  agent adapter.
- [x] Model the Engaged, Tired, and Disengaged stage machine and contact
  cooldown policy; item/version-specific timings remain adapter inputs.
- [x] Connect request hold duration to stage-specific contact outcomes while
  leaving damage/knockback/dismount resolution server-authoritative.
- [x] Implement profile-driven Jab cooldown and multi-target selection policy;
  material-specific values remain adapter inputs.
- [ ] Verify spear use against 1.21.11+ live servers and preserve fallback
  behavior for older versions.

The protocol handler already contains a `v1_21_11` package; spear behavior
still needs item-component and action-level validation before being marked
complete.
