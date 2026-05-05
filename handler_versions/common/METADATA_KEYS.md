# Entity Metadata Keys Registry

This document describes the entity metadata keys used in Minecraft network protocol as of Feb 18, 2026.

## Overview

Entity metadata is sent via the `EntityMetadata` packet and contains update information about entities. Each metadata entry has:
- **Key (Index)**: A byte indicating which field is being updated (0-255)
- **Type**: The data type of the value (float, boolean, int, etc.)
- **Value**: The actual data

## Metadata Structure

Metadata keys are **entity-type specific**. Different entity types use different indices for different purposes. For example:
- Index 9 is **health** (float) for living entities (zombies, players, etc.)
- Index 9 is **piercing level** (byte) for arrows
- Index 10 is **potion effect color** (int) for living entities
- Index 10 is **isInGround** (boolean) for arrows

## Key Ranges

### Base Entity Metadata (0-7)
Applies to ALL entities. These are always available regardless of entity type:
- 0: Flags (byte) - bitfield for on fire, sneaking, sprinting, etc.
- 1: Custom name (optional component)
- 2: Name visible (boolean)
- 3: Silent (boolean)
- 4: No gravity (boolean)
- 5: Pose (pose enum)
- 6: Freezing ticks (VarInt)

### Living Entity Metadata (7-15)
Applies to entities that extend LivingEntity (mobs, players, etc.):
- 7: Rotation (rotations - pitch/yaw/roll)
- 8: Air supply (VarInt) - ticks of air remaining
- 9: Health (float) - current health in half-hearts
- 10: Potion effect color (int) - from active potion effects
- 11: Potion effect ambient (boolean) - from beacon
- 11: Arrow count (VarInt) - number of arrows in entity
- 12: Bee stinger count (VarInt) - number of bee stingers

### Arrow/Projectile Metadata
Applies to AbstractArrow and subclasses:
- 8: IsInWall (boolean) - whether arrow is in wall
- 9: Piercing level (byte)
- 10: IsInGround (boolean) - whether arrow is stuck in ground

### Player-Specific Metadata (16+)
Applies to Player/Human entities:
- 16: Absorption hearts (float)
- 17: Player score (VarInt)
- 18: Displayed skin parts (byte) - bitfield for which skin parts to show
- 19: Main hand (byte) - 0 = main hand, 1 = off hand

## How to Extend This Registry

When adding support for new entity types or discovering new metadata indices:

1. **Check minecraft.wiki**: Visit https://minecraft.wiki/w/Java_Edition_protocol/Entity_metadata for the official protocol documentation
2. **Identify the entity class**: Determine which entity type(s) use the metadata key
3. **Note the index and type**: Record the key index and its data type
4. **Add to appropriate section**: Add the constant to the correct range section in `metadata_keys.go`
5. **Document entity specificity**: Add comments if the key is only used for certain entity types

## Common Entity-Specific Metadata

### Zombie-specific (indices 16+)
- 16: Is converting (boolean)
- 17: Hands up (boolean)

### Armor Stand-specific (indices 16+)
- 16: Armor stand flags (byte) - small, has arms, has base plate, marker
- 17: Head rotation (rotations)
- 18: Body rotation (rotations)
- 19: Left arm rotation (rotations)
- 20: Right arm rotation (rotations)
- 21: Left leg rotation (rotations)
- 22: Right leg rotation (rotations)

### Horse-specific (indices 16+)
- 16: Flags (byte) - tamed, saddled, bred, eating
- 17: Owner UUID (optional UUID)
- 18: Variant (int)

### Villager-specific (indices 16+)
- 16: Head shake timer (VarInt)
- 17: Villager data (villager data struct)
- 18: Gossip data (complex structure)

## References

- Official Protocol: https://minecraft.wiki/w/Java_Edition_protocol/Entity_metadata
- Entity Type Documentation: https://minecraft.wiki/w/Entity
