# TODO Evaluation

## Overview

This document evaluates all TODO comments found in the codebase to determine which are completed, obsolete, or still needed.

## Date

2025-11-25

## Summary

- **Total TODOs Found**: 16
- **Legacy/Vendor Code (Ignore)**: 5
- **Completed/Obsolete (Can Remove)**: 4
- **Ready to Implement (World Integration Complete)**: 4
- **Future Work (Still TODO)**: 3

---

## ✅ COMPLETED / OBSOLETE (Can Remove or Update)

### 1. `movement/executor.go:107` - **COMPLETED**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/movement/executor.go:107`

**TODO Text**:
```go
// TODO: When world integration is ready, check if we need to step up/down
return botX, botY, botZ, nil
```

**Status**: ✅ **OBSOLETE - World integration is complete**

**Context**:
- This TODO was added when world integration wasn't complete
- MoveTowards() currently ignores vertical movement (maintains botY)
- With world integration, pathfinding generates valid paths with Y changes
- However, this code should NOW use `targetY` from the path instead of ignoring it

**Action Required**:
- Remove TODO comment
- Update code to use `targetY` parameter instead of maintaining `botY`
- Example fix:
```go
// World integration complete - use targetY from pathfinding
if horizontalDist <= 0.01 {
    if math.Abs(dy) > 0.01 {
        // Move vertically to match path step
        newY = targetY
        err = me.SendPositionAndRotation(botX, newY, botZ, yaw, pitch, onGround)
        return botX, newY, botZ, err
    }
    return botX, botY, botZ, nil
}

// Calculate new position (include vertical movement from pathfinding)
newX = botX + dxNorm*moveDistance
newY = targetY // Use Y from pathfinding, not botY
newZ = botZ + dzNorm*moveDistance
```

---

### 2. `following/manager.go:311` - **COMPLETED**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/following/manager.go:311`

**TODO Text**:
```go
// TODO Phase 4: Switch back to 3D distance when vertical movement is re-enabled
```

**Status**: ✅ **OBSOLETE - Vertical movement can now be enabled**

**Context**:
- Currently uses horizontal-only distance (ignores Y difference)
- Was disabled because bot couldn't detect ground without world integration
- World integration is complete, so 3D distance can be safely re-enabled

**Action Required**:
- Remove TODO comment
- Update to use 3D distance calculation:
```go
// Calculate 3D distance with world integration
dx := botX - targetX
dy := botY - targetY
dz := botZ - targetZ
distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
```

---

### 3. `following/manager.go:392` - **COMPLETED**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/following/manager.go:389-392`

**TODO Text (in comment)**:
```go
// IMPORTANT: Without world integration, we cannot detect ground level
// We maintain bot's current Y and trust the server to correct us if we're invalid
// The server will send position corrections if bot tries to walk on air
targetY := botY
```

**Status**: ✅ **OBSOLETE - Should use step.Position.Y now**

**Context**:
- Currently ignores the Y coordinate from the path step
- Uses `targetY := botY` instead of `targetY := float64(step.Position.Y) + 0.5`
- Path steps DO include Y changes (from Ascend/Descend movements)
- With world integration, paths are valid so we can trust `step.Position.Y`

**Action Required**:
- Remove obsolete comment
- Update to use path step's Y coordinate:
```go
// Calculate target position (center of block)
stepX, stepY, stepZ := float64(step.Position.X), float64(step.Position.Y), float64(step.Position.Z)
targetX := stepX + 0.5
targetY := stepY + 0.5  // Use Y from pathfinding step
targetZ := stepZ + 0.5
```

---

### 4. `bot/path/blocks.go:2` - **OBSOLETE**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/bot/path/blocks.go:2`

**TODO Text**:
```go
// TODO: USE DOWNLOADED BLOCK DATA AND TAGS TO REBUILD THIS FILE, AS WELL AS old/.../block.Block data AND world.BlockStatus (which this file references)
```

**Status**: ✅ **OBSOLETE - Replaced by pathfinding/blocks.go and mc-data-gen**

**Context**:
- This entire file is commented out (legacy)
- We now use `pathfinding/blocks.go` with BlockShapeManager
- mc-data-gen provides the downloaded block data this TODO refers to
- This file can be deleted

**Action Required**:
- Consider deleting `bot/path/blocks.go` entirely
- The functionality has been replaced by:
  - `pathfinding/blocks.go` - BlockShapeManager interface
  - `mc-data-gen/loader` - Actual block data

---

## 🟢 READY TO IMPLEMENT (World Integration Complete)

### 5. `pathfinding/movement.go:398` - **CAN IMPLEMENT NOW**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/pathfinding/movement.go:398-400`

**TODO Text**:
```go
// TODO: Check if there's a climbable block at destination using shapeMgr.IsClimbable()
// For now, return false until we integrate with world manager
return false
```

**Status**: 🟢 **READY - World integration complete**

**Context**:
- CanClimb() currently always returns false
- TODO says "until we integrate with world manager"
- World integration IS complete
- We can now query block data and check if it's climbable

**Implementation**:
```go
func (mv *MovementValidator) CanClimb(from, to V3) bool {
    dx := to.X - from.X
    dz := to.Z - from.Z
    dy := to.Y - from.Y

    // Must be moving vertically (up or down)
    if dy == 0 {
        return false
    }

    // Must be same horizontal position or adjacent
    if dx*dx+dz*dz > 1 {
        return false
    }

    // Check if there's a climbable block at destination
    stateID := mv.world.GetBlockAt(to.X, to.Y, to.Z)
    if stateID == 0 {
        return false // Air, not climbable
    }

    blockID := mv.blockMgr.BlockIDByStateID(stateID)
    block := mv.blockMgr.GetByID(blockID)

    return mv.shapeMgr.IsClimbable(block.Name, nil)
}
```

---

### 6. `pathfinding/movement.go:419-420` - **CAN IMPLEMENT NOW**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/pathfinding/movement.go:419-422`

**TODO Text**:
```go
// TODO: Check if destination is water using shapeMgr.IsWater()
// TODO: Check if from position is also water (must already be swimming)
// For now, return false until we integrate with world manager
return false
```

**Status**: 🟢 **READY - World integration complete**

**Context**:
- CanSwim() currently always returns false
- World integration allows us to check if blocks are water
- shapeMgr.IsWater() is already implemented

**Implementation**:
```go
func (mv *MovementValidator) CanSwim(from, to V3) bool {
    dx := to.X - from.X
    dz := to.Z - from.Z
    dy := to.Y - from.Y

    // Must be on same Y level
    if dy != 0 {
        return false
    }

    // Must be adjacent (cardinal or diagonal)
    if dx*dx+dz*dz == 0 || dx*dx+dz*dz > 2 {
        return false
    }

    // Check if destination is water
    toStateID := mv.world.GetBlockAt(to.X, to.Y, to.Z)
    if toStateID == 0 {
        return false
    }
    toBlockID := mv.blockMgr.BlockIDByStateID(toStateID)
    toBlock := mv.blockMgr.GetByID(toBlockID)
    if !mv.shapeMgr.IsWater(toBlock.Name, nil) {
        return false
    }

    // Check if from position is also water (must already be swimming)
    fromStateID := mv.world.GetBlockAt(from.X, from.Y, from.Z)
    if fromStateID == 0 {
        return false
    }
    fromBlockID := mv.blockMgr.BlockIDByStateID(fromStateID)
    fromBlock := mv.blockMgr.GetByID(fromBlockID)

    return mv.shapeMgr.IsWater(fromBlock.Name, nil)
}
```

---

### 7. `pathfinding/movement.go:441` - **CAN IMPLEMENT NOW**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/pathfinding/movement.go:441-443`

**TODO Text**:
```go
// TODO: Check if both positions are water using shapeMgr.IsWater()
// For now, return false until we integrate with world manager
return false
```

**Status**: 🟢 **READY - Same as CanSwim**

**Implementation**: Similar to CanSwim() but for vertical swimming up

---

### 8. `pathfinding/movement.go:570` - **CAN UNCOMMENT NOW**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/pathfinding/movement.go:570-620`

**TODO Text**:
```go
// TODO Phase 4: Enable climbing, swimming when we integrate with world manager
// These require checking block types via shapeMgr

// // Climbing
// [commented out code for climbing moves]
//
// // Swimming
// [commented out code for swimming moves]
```

**Status**: 🟢 **READY - Can uncomment after implementing Can* methods**

**Action Required**:
1. First implement the Can* methods above (#5-7)
2. Then uncomment the climbing/swimming move generation code
3. Remove the TODO comment

---

## 🔴 FUTURE WORK (Still TODO)

### 9. `pathfinding/movement.go:202` - **FUTURE ENHANCEMENT**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/pathfinding/movement.go:202`

**TODO Text**:
```go
// TODO: Parse props to check "open" state
penalty += 2.0 // Small penalty for door interaction
```

**Status**: 🔴 **FUTURE - Requires state properties parsing**

**Context**:
- Currently we pass `nil` for block properties
- Block state IDs contain property information we don't extract yet
- Would allow checking if doors are open/closed, fences are connected, etc.

**Why Not Done**:
- Requires parsing state ID into block ID + properties
- mc-protocol-go doesn't expose state property extraction yet
- Current approach (nil props = default state) works for basic pathfinding

**Future Implementation**:
- Add method to extract properties from state ID
- Store property mappings per block type
- Pass actual props to shapeMgr methods

---

### 10. `main.go:798` - **FUTURE ENHANCEMENT**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/main.go:798`

**TODO Text**:
```go
//TODO: Change this to be the bot's logged in name, not hard-coded.
const rofBotFlag = ">>>ROF_bot<<<"
```

**Status**: 🔴 **FUTURE - Low priority cosmetic change**

**Context**:
- Currently uses hardcoded ">>>ROF_bot<<<" command prefix
- Could use bot's actual login name instead

**Implementation**:
```go
// Use bot's name from authentication
botName := client.Name
rofBotFlag := fmt.Sprintf(">>>%s<<<", botName)
```

---

### 11. `main.go:852-853` - **FUTURE ENHANCEMENT**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/main.go:852-853`

**TODO Text**:
```go
// TODO: Update to handle additional windows besides just inventory
// TODO: Remove depence on mc-go/registryid - it is out of date and locked to a version (need version agnostic altenative ItemMgr (maybe in mc-protocol-go/mc-data-gen?))
```

**Status**: 🔴 **FUTURE - Requires ItemMgr similar to BlockMgr**

**Context**:
- Currently uses `registryid.Item` which is version-specific
- Need version-agnostic item manager (similar to BlockMgr)
- Would be part of mc-protocol-go or mc-data-gen

**Future Implementation**:
- Create ItemMgr interface in mc-protocol-go
- Generate version-specific item data
- Handle multiple window types (chests, furnaces, etc.)

---

### 12. `bot/world/entity/entity.go:3` - **FUTURE ENHANCEMENT**

**Location**: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent/bot/world/entity/entity.go:3`

**TODO Text**:
```go
// TODO: REPLACE RELIANCE ON go-mc/data/entity - it is out of date and version specific to whatever version go-mc is on
import "github.com/Tnze/go-mc/data/entity"
```

**Status**: 🔴 **FUTURE - Requires EntityMgr**

**Context**:
- Currently relies on go-mc's entity data (version-locked)
- Need version-agnostic entity manager
- Similar to BlockMgr and ItemMgr

**Future Implementation**:
- Create EntityMgr interface in mc-protocol-go
- Generate version-specific entity data
- Handle entity metadata and properties

---

## 🗑️ LEGACY (Ignore - Vendor/Old Code)

These are in legacy vendor directories and can be ignored:

1. `old/vendor/github.com/Tnze/go-mc/realms/server.go:113`
2. `old/examples/tellie/tellie.go:340`
3. `src/vendor/github.com/Tnze/go-mc/bot/configuration.go:53`
4. `src/vendor/github.com/Tnze/go-mc/bot/configuration.go:135`
5. `src/vendor/github.com/Tnze/go-mc/yggdrasil/user/user.go:48`

---

## Action Items

### Immediate (Can Do Now)

1. **Enable vertical movement** (movement/executor.go:107, following/manager.go:392)
   - Update MoveTowards() to use targetY parameter
   - Update followCurrentPath() to use step.Position.Y
   - Test that bot correctly ascends/descends

2. **Switch to 3D distance** (following/manager.go:311)
   - Update distance calculation to include Y component
   - Test following on stairs/hills

3. **Implement climbing/swimming** (pathfinding/movement.go:398-620)
   - Implement CanClimb() using world data
   - Implement CanSwim(), CanSwimUp(), CanSwimDown() using world data
   - Uncomment climbing/swimming move generation
   - Test in water and with ladders

4. **Clean up obsolete files**
   - Consider deleting `bot/path/blocks.go` (commented out, replaced)

### Future Work (Deferred)

1. **Block property parsing** (pathfinding/movement.go:202)
   - Extract properties from state IDs
   - Use actual props for door states, fence connections, etc.

2. **ItemMgr implementation** (main.go:852-853)
   - Create version-agnostic item manager
   - Handle multiple window types

3. **EntityMgr implementation** (bot/world/entity/entity.go:3)
   - Create version-agnostic entity manager

4. **Dynamic bot name** (main.go:798)
   - Use actual login name for command prefix

---

## Conclusion

**World integration unblocked 4 major TODOs**:
- ✅ Vertical movement can now be enabled
- ✅ 3D distance can be used
- ✅ Climbing/swimming can be implemented
- ✅ Path step Y coordinates can be trusted

**Next phase should focus on**:
1. Re-enabling vertical movement (#1-2)
2. Implementing climbing/swimming (#3)
3. Testing bot behavior on varied terrain

**Future enhancements deferred**:
- Block property parsing
- ItemMgr/EntityMgr implementation
- UI/UX improvements (bot name, etc.)

---

**Status**: ✅ Evaluation Complete
**Date**: 2025-11-25
**Total Active TODOs**: 11 (4 ready, 3 future, 4 obsolete/completed)
