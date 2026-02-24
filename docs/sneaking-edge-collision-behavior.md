# Minecraft Client Sneaking Edge Block Collision Behavior

## Overview

This document details the exact behavior of Minecraft 1.21.8 client when a sneaking player attempts to move near block edges. This analysis is based on decompiled source code from `PlayerEntity.java` and related classes.

## Core Mechanism

The sneaking edge collision system is implemented in `PlayerEntity.adjustMovementForSneaking()` (lines 1074-1119 of PlayerEntity.java).

### Call Site: Entity.move() Method

The `adjustMovementForSneaking()` method is called from the main entity movement pipeline in `Entity.move()` at line 978:

```java
public void move(MovementType type, Vec3d movement) {
    // ... setup code ...
    
    // LINE 978: APPLY SNEAKING EDGE LOGIC
    movement = this.adjustMovementForSneaking(movement, type);
    
    // LINE 979: THEN apply general collision resolution
    Vec3d vec3d = this.adjustMovementForCollisions(movement);
    
    // ... apply final movement ...
}
```

**Movement Processing Order:**
1. Check no-clip mode
2. Adjust for piston movement
3. Apply movement multipliers (ice, honey, etc.)
4. **Apply sneaking edge logic** ← This method
5. General collision resolution
6. Fall damage and landing effects
7. Velocity and callback effects

**Key insight:** Sneaking reduction happens BEFORE general collision detection, allowing the reduced movement to propagate through the collision system naturally.

### Activation Conditions

The edge block sneaking behavior is triggered when ALL of these conditions are true:

1. Player is NOT in creative mode flying (`!this.abilities.flying`)
2. Movement does NOT have an upward component (`!(movement.y > 0.0)`)
3. Movement type is either SELF or PLAYER (excludes PISTON and other special movement types)
4. Player is sneaking (`clipAtLedge()` returns true when player holds sneak key)
5. Player is either on ground OR sufficiently close to ground (`method_30263()` returns true)

### Algorithm Overview

When activated, the algorithm attempts to find valid movement by testing incrementally reduced movements. The process has three phases:

1. **X-Axis Reduction Phase**: Test pure X-axis movement in 0.05 block steps
2. **Z-Axis Reduction Phase**: Test pure Z-axis movement in 0.05 block steps  
3. **Diagonal Phase**: Test simultaneous X and Z movement reduction

## Detailed Algorithm

### Phase 1: X-Axis Testing

```java
double h = Math.signum(d) * 0.05;  // Direction multiplier: -0.05 or +0.05
for (i = Math.signum(e) * 0.05;    // Z direction multiplier (set but not used here)
     d != 0.0 && this.isSpaceAroundPlayerEmpty(d, 0.0, f);
     d -= h) {
    if (Math.abs(d) <= 0.05) {
        d = 0.0;
        break;
    }
}
```

**Behavior:**
- Starts with the full requested X-axis movement (`d`)
- Repeatedly reduces it by `h` (0.05 in the direction of travel)
- Tests if space is empty at each reduction using `isSpaceAroundPlayerEmpty()`
- Continues until movement cannot be further reduced or collision occurs
- When remaining movement is ≤ 0.05, clamps to 0.0

**Example:**
- If player requests 0.3 blocks right movement:
  - Tests: 0.3 (collision) → 0.25 (collision) → 0.20 (collision) → 0.15 (success)
  - Returns 0.15 as safe movement

### Phase 2: Z-Axis Testing

```java
while (e != 0.0 && this.isSpaceAroundPlayerEmpty(0.0, e, f)) {
    if (Math.abs(e) <= 0.05) {
        e = 0.0;
        break;
    }
    e -= i;  // i = Math.signum(e) * 0.05
}
```

**Behavior:**
- Identical to X-axis testing, but for Z-axis (forward/backward) movement
- Completely independent from X-axis result
- Uses `i` as the directional multiplier for Z-axis

### Phase 3: Diagonal Testing

```java
while (d != 0.0 && e != 0.0 && this.isSpaceAroundPlayerEmpty(d, e, f)) {
    if (Math.abs(d) <= 0.05) {
        d = 0.0;
    } else {
        d -= h;
    }
    
    if (Math.abs(e) <= 0.05) {
        e = 0.0;
    } else {
        e -= i;
    }
}
```

**Behavior:**
- Tests simultaneous X and Z movement with reduced values from phases 1 and 2
- Reduces BOTH axes together in 0.05 block increments
- Each axis is independently clamped when it reaches ≤ 0.05
- Attempts to find a diagonal movement that fits the available space
- Only executes if BOTH `d` and `e` are non-zero after phases 1 and 2

## Space Checking Function

The `isSpaceAroundPlayerEmpty()` method (lines 1125-1132) validates whether a movement offset creates a collision:

```java
private boolean isSpaceAroundPlayerEmpty(double offsetX, double offsetZ, double stepHeight) {
    Box box = this.getBoundingBox();
    return this.getWorld().isSpaceEmpty(this, 
        new Box(
            box.minX + 1.0E-7 + offsetX,
            box.minY - stepHeight - 1.0E-7,    // Down to step height
            box.minZ + 1.0E-7 + offsetZ,
            box.maxX - 1.0E-7 + offsetX,
            box.minY,                          // Up to current Y
            box.maxZ - 1.0E-7 + offsetZ
        )
    );
}
```

### Box Dimensions

The collision check box has these properties:

- **Horizontal dimensions**: Same as player bounding box, offset by `(offsetX, offsetZ)`
- **Vertical dimensions**: From `(minY - stepHeight)` to `minY`
  - Checks if there's collision in this vertical range
  - `stepHeight` is typically 0.6 blocks for players
- **Epsilon contraction**: ±1.0E-7 on all sides to avoid floating-point edge cases

### What Gets Checked

- All block collision shapes within the test box
- Water/fluid collision (handled separately in movement system)
- Entities do NOT block sneaking (entities are skipped in `isSpaceEmpty()`)
- World border collision is checked separately in the main movement system

## Validation Method

The `method_30263()` helper (lines 1121-1123) determines if sneaking can be applied:

```java
private boolean method_30263(float stepHeight) {
    return this.isOnGround() 
        || (this.fallDistance < stepHeight 
            && !this.isSpaceAroundPlayerEmpty(0.0, 0.0, stepHeight - this.fallDistance));
}
```

**Returns true when:**
1. Player is on solid ground, OR
2. Player hasn't fallen far AND there's a block close enough to catch them

**Purpose:** Prevents sneaking edge behavior when the player is in mid-air with no support.

## Edge Case Behaviors

### 1. Partial Movement Success
If a player requests movement `(0.3, 0.0, 0.3)` (diagonal):
- X-phase might allow 0.15 blocks
- Z-phase might allow 0.25 blocks
- Diagonal phase will try to use both 0.15 and 0.25 together
- Final result could be `(0.1, 0.0, 0.2)` if that diagonal fits

### 2. Complete Blockage
If no movement in any direction/phase succeeds:
- `d = 0.0`, `e = 0.0`
- Returns movement `(0.0, movement.y, 0.0)`
- Player stays in place horizontally but can still fall/jump

### 3. Ledge Falloff Prevention
The algorithm naturally prevents ledge falloffs because:
- As player approaches an edge, `isSpaceAroundPlayerEmpty()` returns false
- Movement is reduced to 0
- Player cannot step off the edge

### 4. Step Climbing
The step height variable `f` allows the algorithm to bypass small obstacles:
- When `f = 0.6` (default player step height)
- Collisions within 0.6 blocks down are checked
- Player can naturally walk over 0.6 block high obstacles

## Return Value

The final return statement:

```java
return new Vec3d(d, movement.y, e);
```

**Components:**
- `d`: Reduced X-axis movement (0.0 to original value)
- `movement.y`: Unchanged vertical movement
- `e`: Reduced Z-axis movement (0.0 to original value)

## Implementation Validation Checklist for mc-agent

When implementing this behavior, ensure:

- [ ] **Incremental testing**: Use 0.05 block increments, not full-block checks
- [ ] **Independent axes**: Test X and Z independently before testing diagonals
- [ ] **Three-phase algorithm**: Don't skip from axis testing directly to final result
- [ ] **Height awareness**: Check collision from `minY - stepHeight` to `minY`
- [ ] **Clamping logic**: Clamp to 0.0 when remainingMovement ≤ 0.05
- [ ] **No box expansion**: Work within player's normal bounding box dimensions
- [ ] **Direction preservation**: Maintain sign of movement (keep traveling same direction)
- [ ] **Condition gating**: Only apply when ALL trigger conditions are met
- [ ] **Floating-point safety**: Use epsilon values (1.0E-7) to avoid precision issues

## Source References

- **File**: `net/minecraft/entity/player/PlayerEntity.java`
- **Method**: `adjustMovementForSneaking()` (lines 1074-1119)
- **Helper Methods**: 
  - `isSpaceAroundPlayerEmpty()` (lines 1125-1132)
  - `method_30263()` (lines 1121-1123)
- **Related Classes**:
  - `Entity.java`: Base entity movement and collision
  - `LivingEntity.java`: `getStepHeight()` definition
  - `PlayerEntity.java`: Sneaking state and step validation

## Integration with Entity Movement System

### Where in the Movement Pipeline?

`adjustMovementForSneaking()` is called at a critical point in entity movement processing:

**Entity.move() flow:**
```
Input: velocity/movement vector
  ↓
Check no-clip
  ↓
Adjust for piston (if PISTON type)
  ↓
Apply movement multipliers (ice, honey effects)
  ↓
>>> adjustMovementForSneaking() <<<  ← SNEAKING EDGE LOGIC HERE
  ↓
adjustMovementForCollisions()  ← General block collision
  ↓
Set position
  ↓
Handle fall damage/landing
  ↓
Apply block effects
  ↓
Output: actual movement applied
```

### Why This Placement Matters

1. **After multipliers:** Ice and honey effects are already applied
2. **Before general collision:** Sneaking reduces movement, then normal collision handles the smaller value
3. **After piston:** Piston forces don't interact with sneaking edge logic
4. **Horizontal only:** Happens before vertical movement is processed, keeps y-component untouched

### Interaction with Other Systems

**`adjustMovementForCollisions()` (called after)**
- Receives the reduced movement from sneaking
- Further reduces if blocks are in the way
- Both systems work together

**`setPosition()` (called after collision)**
- Uses the final reduced movement vector
- Player is placed at the new position

### FallDistance Timing and State

Critical: fallDistance is read BEFORE it is updated within a single `move()` call.

1. Line 978: `adjustMovementForSneaking()` executes
   - Uses accumulated `fallDistance` from previous ticks.
   - `method_30263()` compares `this.fallDistance < stepHeight` using the pre-update value.
   - Sneaking logic does NOT modify `fallDistance`.
2. Lines 982–989: Optional fall detection
   - Checks `if (this.fallDistance != 0.0 && d >= 1.0)` using the pre-update value.
   - May call `onLanding()` (which resets `fallDistance` to 0) if a resettable fall raycast hits.
3. Lines 992–995: Position is updated via `setPosition()`
   - Player location is finalized for this step.
   - `fallDistance` has still not been incremented for this frame’s movement.
4. Line 1018: `fall(vec3d.y, isOnGround, state, pos)` is called
   - This is where `fallDistance` is actually updated for the frame.
   - Inside `fall()`: if moving downward, `this.fallDistance -= (float)heightDifference;` accumulates the distance.
   - If on ground, `onLandedUpon` is invoked and `onLanding()` resets `fallDistance` to 0.

Frame-by-frame example:
```
Frame N start:  fallDistance = 5.2 (accumulated)
  ↓
adjustMovementForSneaking() checks: fallDistance (5.2) < stepHeight (0.6)?
  ↓
setPosition() applies movement (e.g., -0.3y)
  ↓
fall(vec3d.y = -0.3) updates: fallDistance = 5.5
  ↓
Frame N end:    fallDistance = 5.5 (used next frame)
```

## Notes for Developers

1. **Performance**: The algorithm uses nested loops but is bounded by movement distance and 0.05 increments, so worst case is ~20 iterations per phase. Typical case is 5-10 iterations.

2. **Client-side only**: This sneaking edge behavior is CLIENT-SIDE logic. The server has its own stricter collision system and may reject movements that the client thinks are valid. The server will validate the final position.

3. **Movement priority**: The diagonal phase only runs if both X and Z have non-zero reduced values. If one axis is completely blocked, diagonal movement isn't tested.

4. **Vertical separation**: Vertical movement (`movement.y`) is never modified by this function—only horizontal components are affected.

5. **Integration with mc-agent**: When implementing in your agent:
   - Call this method at the same point in your movement pipeline (after multipliers, before general collision)
   - Always process sneaking before block collision detection
   - Pass the exact same parameters (movement vector and MovementType)
   - Use the returned vector for subsequent collision checks

6. **FallDistance State Management**: Critical implementation detail
   - `fallDistance` is a frame-accumulating value that tracks distance fallen since last landing
   - Within a single move call, `fallDistance` is READ at the start but UPDATED at the end
   - When implementing method_30263 equivalent, use the current frames fallDistance, not anticipating updates
   - Sneaking edge logic cannot directly modify fallDistance; it can only reduce movement
   - The players accumulated fall distance at the START of the frame is what validates whether sneaking edge can apply
   - After position updates and fall is called, fallDistance increases or resets based on whether the player landed
