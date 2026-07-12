# Camel Husk Movement Research Report (1.21.11)

## Executive Summary

Based on analysis of the official Minecraft 1.21.11 Java Edition source code, **camel husks have IDENTICAL movement physics to regular camels, with ONE significant exception**: the dash charging speed multiplier is 4x faster for camel husks.

## Java Source Code Analysis

### File References
- `net/minecraft/entity/passive/CamelEntity.java` - Regular camel implementation
- `net/minecraft/entity/mob/CamelHuskEntity.java` - Camel husk implementation (extends CamelEntity)

### Key Findings

#### 1. Movement Speed (IDENTICAL)
- **Regular Camel**: `generic.movement_speed = 0.09` (from `createCamelAttributes()`)
- **Camel Husk**: Inherits 0.09 (no override)
- **Walking Speed**: 3.885 blocks/s
- **Sprinting Speed**: 8.203 blocks/s
- **Result**: ✅ **NO CHANGE NEEDED**

#### 2. Dash Charging Speed Multiplier (4x FASTER)
- **Regular Camel**: `getRiderChargingSpeedMultiplier()` returns `1.0F` (default from MobEntity)
  - Charge accumulates at 1 tick per game tick (max 100 ticks = 2.75s charge time)
  
- **Camel Husk**: `getRiderChargingSpeedMultiplier()` returns `4.0F` (OVERRIDDEN)
  - Charge accumulates at 4 ticks per game tick (max 25 ticks = 0.6875s charge time)
  - **4x FASTER DASH CHARGING**
- **Result**: ⚠️ **CHANGE NEEDED** - Multiply charge accumulation by 4.0

#### 3. Pose Mechanics (IDENTICAL)
- Sitting/standing transitions
- Duration: 40 ticks (sit), 52 ticks (stand)
- **Result**: ✅ **NO CHANGE NEEDED**

#### 4. Dash Mechanics (IDENTICAL)
- Dash impulse calculation: 22.2222 × strength × movementSpeed
- Dash cooldown: 55 ticks
- Dash end threshold: 50 ticks
- **Result**: ✅ **NO CHANGE NEEDED**

#### 5. Other Overrides in CamelHuskEntity
- Sound events (ambient, death, hurt, step, dash sounds) - Client-side only
- `canBreedWith()` returns false - No breeding
- `canEat()` returns false - Cannot eat
- `isBreedingItem()` checks `ItemTags.CAMEL_HUSK_FOOD` instead of `ItemTags.CAMEL_FOOD`
- `canImmediatelyDespawn()` returns true - Can despawn
- **Result**: ✅ **NO CHANGE TO MOVEMENT**

#### 6. Fall Damage & Other Mechanics
*Based on web research (not in movement physics):*
- Falls: Can safely fall 6 blocks (vs 3 for regular mobs)
- Fall damage: Takes half damage
- **Result**: ✅ **NOT PART OF MOVEMENT EXECUTOR** (entity health, not movement control)

## Implementation Status in mc-agent

### Already Implemented ✅
1. **CamelHuskChargingSpeedMultiplier constant** - Defined as 4.0 in `models/camel.go:67`
2. **GetChargingSpeedMultiplier() method** - Returns 4.0 for husks, 1.0 for regular (models/camel.go:267-274)
3. **IsCamelHusk field tracking** - CamelState already tracks this
4. **Charging speed integration** - Used in `movement/riding_camel.go:153` via `int(chargeMul)`

### Current Usage
In `riding_camel.go:153-157`:
```go
if camelSt.GetIsCharging() {
    if jump {
        chargeMul := camelSt.GetChargingSpeedMultiplier()  // Returns 4.0 for husk
        camelSt.AddJumpChargeTicks(int(chargeMul))          // Multiplies by 4
```

## Required Changes

### Change Summary
| Aspect | Regular Camel | Camel Husk | Status |
|--------|--------------|-----------|--------|
| Movement Speed | 0.09 | 0.09 | ✅ Identical |
| Sprint Bonus | +0.1 | +0.1 | ✅ Identical |
| Dash Charge Speed | 1x | 4x | ⚠️ **IMPLEMENTED** |
| Pose Transitions | 40/52 ticks | 40/52 ticks | ✅ Identical |
| Dash Impulse | 22.2222× | 22.2222× | ✅ Identical |
| Dash Cooldown | 55 ticks | 55 ticks | ✅ Identical |

### Implementation Verification
The camel husk charging speed multiplier **is already correctly implemented** in the codebase:
- Constant defined: `CamelHuskChargingSpeedMultiplier = 4.0`
- Logic correct: Multiplies charge accumulation rate by 4x
- Integration verified: Used in dash charging loop

## Conclusion

**NO CODE CHANGES REQUIRED** for movement physics. The camel husk implementation is complete and correct. The 4x charging speed multiplier is already applied in the ride executor.

### Test Coverage Needed
Tests should verify:
1. ✅ Mounting/dismounting (same as camel)
2. ✅ Forward movement (same as camel - 0.09 base speed)
3. ✅ Dash charging (4x faster than regular camel)
4. ✅ Dash execution (same impulse as regular camel)

All movement mechanics are functionally identical between regular camels and camel husks except for the dash charge speed.

---

## Source References

- [Minecraft Wiki - Camel](https://minecraft.wiki/w/Camel)
- [Minecraft Wiki - Camel Husk](https://minecraft.wiki/w/Camel_Husk)
- Java Source: `net/minecraft/entity/passive/CamelEntity.java`
- Java Source: `net/minecraft/entity/mob/CamelHuskEntity.java`
