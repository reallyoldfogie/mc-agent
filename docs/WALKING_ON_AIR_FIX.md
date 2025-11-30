# Walking on Air Fix

## Problem

The bot was maintaining its initial Y coordinate but didn't know where the actual ground was. This caused it to appear to "walk on air" from the player's perspective.

**Example**:
- Server spawns bot at Y=100
- Player is on ground at Y=64
- Bot stays at Y=100 while trying to follow player
- Result: Bot appears to float 36 blocks above ground

## Root Cause

Without world integration (access to block data), the bot cannot:
1. Detect where the ground is beneath it
2. Know if a position is solid, air, or water
3. Adjust Y position to stand on actual terrain

**Previous "fix" in Phase 4**: Bot maintained its current Y level
- Problem: This only prevented Y from changing, didn't fix initial wrong Y
- If bot spawned in air, it stayed in air

## Solution (Phase 5)

**Gradually adjust bot's Y to match target player's Y level**

### Logic

```go
yDiff := playerY - botY

if yDiff > 1.0 {
    // Player is above bot by >1 block
    targetY = botY + 0.5  // Move up gradually
} else if yDiff < -1.0 {
    // Player is below bot by >1 block
    targetY = botY - 0.5  // Move down gradually
} else {
    // Within 1 block of player
    targetY = playerY  // Match player's Y exactly
}
```

### Why This Works

**Assumption**: The player is (usually) on the ground
- If player is at Y=64, they're standing on ground
- Bot adjusts Y to match player's Y=64
- Result: Bot is now at ground level too

**Gradual adjustment**: 0.5 blocks per update
- Prevents rapid falling through blocks
- Smooth visual transition
- Server can validate each step

**Tolerance**: ±1 block before adjusting
- Allows for natural Y variation (stairs, slabs)
- Prevents jittery movement
- Only adjusts for significant differences

## How It Behaves

### Scenario 1: Bot Spawns High Above Ground

```
Initial: Bot at Y=100, Player at Y=64
Update 1: yDiff = -36, targetY = 99.5 (move down 0.5)
Update 2: yDiff = -35, targetY = 99.0 (move down 0.5)
...
After ~72 updates (3.6 seconds): Bot reaches Y=64
Result: Bot is now at player's level (ground)
```

### Scenario 2: Bot and Player on Same Level

```
Bot at Y=64, Player at Y=64
yDiff = 0
targetY = 64 (no change needed)
Result: Bot stays at ground level
```

### Scenario 3: Player Goes Up Stairs

```
Initial: Bot at Y=64, Player at Y=65
Update 1: yDiff = 1.0, targetY = 64 (within tolerance)
Update 2: Player at Y=66, yDiff = 2.0, targetY = 64.5
Update 3: targetY = 65.0
Update 4: targetY = 65.5
Update 5: targetY = 66.0
Result: Bot follows player up stairs gradually
```

### Scenario 4: Player Falls Off Cliff

```
Bot at Y=80, Player at Y=60
yDiff = -20
Updates: Bot descends 0.5 blocks every 50ms
After 2 seconds: Bot at Y=60 (caught up to player)
Result: Bot follows player down cliff
```

## Limitations

1. **Assumes player is on ground**: If player is flying or swimming, bot will match their Y (may walk on air temporarily)
2. **No terrain detection**: Bot doesn't know if target Y has solid ground
3. **0.5 blocks per step**: Large Y differences take time to adjust
4. **No fall damage consideration**: Bot will descend even if it would cause fall damage

## Trade-offs

### Pros ✅
- Simple solution without world integration
- Works in most common cases
- Smooth, gradual adjustment
- No complex terrain detection needed

### Cons ❌
- Not perfect (doesn't detect actual ground)
- Assumes player is on valid ground
- May walk on air if player is flying
- Slow adjustment for large Y differences

## Future Improvements (Requires World Integration)

Once we have access to world block data:

1. **Proper ground detection**:
   ```go
   groundY := findGroundBelow(botX, botZ)
   targetY = groundY
   ```

2. **Terrain following**:
   ```go
   if isBlockSolid(botX, targetY-1, botZ) {
       targetY = targetY  // Stand on solid block
   } else {
       targetY = findGroundBelow(botX, botZ)  // Fall to ground
   }
   ```

3. **Stair/slope climbing**:
   ```go
   if isStair(blockAtFeet) {
       targetY += getStairHeight(blockAtFeet)
   }
   ```

4. **Water detection**:
   ```go
   if isWater(blockAtFeet) {
       targetY = playerY  // Swim to match player
   } else {
       targetY = groundY  // Walk on ground
   }
   ```

## Testing

### Before Fix
- Bot at Y=100, player at Y=64
- Bot walks on air 36 blocks above ground
- Never descends to ground level

### After Fix
- Bot at Y=100, player at Y=64
- Bot gradually descends 0.5 blocks every update
- After 3.6 seconds, bot reaches Y=64
- Bot now walks at ground level with player

### Test Cases

1. **Bot spawns high**: Should descend to player's level
2. **Bot spawns low**: Should ascend to player's level
3. **Player on stairs**: Bot should follow up/down gradually
4. **Player on flat ground**: Bot should stay at same Y
5. **Player jumps**: Bot shouldn't jump (within tolerance)
6. **Player falls off cliff**: Bot should descend to follow

## Implementation

**File**: `following/manager.go`
**Lines**: 389-409
**Phase**: 5 (Robustness fix)
**Date**: 2025-11-25

## Alternative Solutions Considered

1. **Teleport bot to ground on follow start**
   - Problem: Don't know where ground is
   - Would need world integration

2. **Make bot fall with gravity**
   - Problem: Complex physics simulation
   - Server-side validation would reject

3. **Use pathfinding Y coordinates**
   - Problem: Pathfinding assumes flat ground (no world data)
   - Would still walk on air

4. **Match player Y instantly**
   - Problem: Looks unnatural (instant teleports)
   - May clip through blocks

## Conclusion

This is a **pragmatic workaround** that works well in practice:
- Solves the "walking on air" visual issue
- Simple implementation without world integration
- Good enough for Phase 5 until world integration is ready
- Will be replaced with proper ground detection in future

---

**Status**: ✅ Implemented and Tested
**Date**: 2025-11-25
**Branch**: phase-5-robustness
