# mc-data-gen Updates Impact Assessment

**Assessment Date:** 2025-11-23
**Plan Version:** 1.2
**Loader Version:** Updated (Nov 23 14:55)

## Summary

The mc-data-gen loader package has been significantly enhanced with **9 new properties** and **9 new helper functions**. These updates **positively impact** the implementation plan by:

1. **Simplifying** block classification (no heuristics needed)
2. **Enabling** fluid navigation (water/lava detection)
3. **Improving** danger avoidance (direct lava detection)
4. **Enhancing** movement precision (slab/stair/door detection)

**Overall Impact:** ✅ **POSITIVE** - Makes implementation easier and more capable

## New Properties Added

### Block Classification Properties

| Property | Type | Purpose | Pathfinding Use |
|----------|------|---------|-----------------|
| `Climbable` | bool | Ladders, vines, scaffolding | Enable climbing movement |
| `DoorLike` | bool | Doors, trapdoors, fence gates | Handle openable blocks |
| `FenceLike` | bool | Fences, walls | Detect barriers, add jump cost |
| `Slab` | bool | All slab variants | Precise collision detection |
| `Stair` | bool | All stair variants | Precise step-up movement |
| `LogOrLeaf` | bool | Tree blocks | Optional: avoid/traverse trees |

### Fluid Properties

| Property | Type | Purpose | Pathfinding Use |
|----------|------|---------|-----------------|
| `Water` | bool | Water blocks (all levels) | Swimming navigation |
| `Lava` | bool | Lava blocks (all levels) | Danger avoidance |
| `Fluid` | bool | Any fluid (water or lava) | General fluid handling |

## New Helper Functions

### Direct Property Access Methods

```go
// Climbing
func (info ShapeInfo) IsClimbable() bool

// Fluids
func (info ShapeInfo) IsFluid() bool
func (info ShapeInfo) IsWater() bool
func (info ShapeInfo) IsLava() bool

// Block Types
func (info ShapeInfo) IsDoorLike() bool
func (info ShapeInfo) IsFenceLike() bool
func (info ShapeInfo) IsSlab() bool
func (info ShapeInfo) IsStair() bool
func (info ShapeInfo) IsLogOrLeaf() bool
```

**Before (from plan v1.2):**
```go
// Required blockID parameter and special function
climbable := mdl.IsClimbableFor("minecraft:ladder", info)
```

**After (with updates):**
```go
// Direct property access, no blockID needed
climbable := info.IsClimbable()
```

## Impact on Implementation Phases

### Phase 1: Core Movement System
**Impact:** ✅ No change (movement packets unaffected)

### Phase 2: Basic Pathfinding
**Impact:** ✅ **Simplified**

**Changes Required:**
1. Use `info.IsClimbable()` instead of `IsClimbableFor(blockID, info)`
2. Add lava detection for danger avoidance
3. Simplify BlockShapeManager wrapper

**Updated BlockShapeManager:**
```go
// IsClimbable - SIMPLIFIED (no blockID param needed)
func (bsm *BlockShapeManager) IsClimbable(blockID string, props map[string]string) bool {
    key := mdl.StateKey{BlockID: blockID, PropsKey: mdl.MakePropsKey(props)}
    info, ok := bsm.shapeData[key]
    if !ok {
        return false
    }

    return info.IsClimbable() // Direct access!
}

// NEW: IsDangerous - Detect lava/dangerous blocks
func (bsm *BlockShapeManager) IsDangerous(blockID string, props map[string]string) bool {
    key := mdl.StateKey{BlockID: blockID, PropsKey: mdl.MakePropsKey(props)}
    info, ok := bsm.shapeData[key]
    if !ok {
        return false
    }

    return info.IsLava() // Avoid lava in pathfinding!
}

// NEW: IsSlab - Precise collision detection
func (bsm *BlockShapeManager) IsSlab(blockID string, props map[string]string) bool {
    key := mdl.StateKey{BlockID: blockID, PropsKey: mdl.MakePropsKey(props)}
    info, ok := bsm.shapeData[key]
    if !ok {
        return false
    }

    return info.IsSlab()
}

// NEW: IsStair - Precise collision detection
func (bsm *BlockShapeManager) IsStair(blockID string, props map[string]string) bool {
    key := mdl.StateKey{BlockID: blockID, PropsKey: mdl.MakePropsKey(props)}
    info, ok := bsm.shapeData[key]
    if !ok {
        return false
    }

    return info.IsStair()
}
```

**Pathfinding Enhancements:**
```go
// In A* cost calculation
func calculateMovementCost(from, to V3, movement MovementType) float64 {
    baseCost := movement.BaseCost()

    // Add penalties for dangerous blocks
    if shapeMgr.IsDangerous(toBlockID, toProps) {
        return baseCost + 1000.0 // Avoid lava!
    }

    // Slabs and stairs have lower jump cost
    if shapeMgr.IsSlab(toBlockID, toProps) || shapeMgr.IsStair(toBlockID, toProps) {
        return baseCost * 0.8 // Easier to traverse
    }

    return baseCost
}
```

### Phase 3: Follow Manager & Target Selection
**Impact:** ✅ No change (management logic unaffected)

### Phase 4: Advanced Movement
**Impact:** ✅ **Major Enhancement**

**New Capabilities Enabled:**

1. **Water Navigation** (NEW)
   ```go
   // Detect water and enable swimming
   if shapeMgr.IsWater(blockID, props) {
       // Use swimming movement primitive
       // Slower movement speed
       // Can descend/ascend in water
   }
   ```

2. **Lava Avoidance** (NEW)
   ```go
   // Avoid lava in pathfinding
   if shapeMgr.IsLava(blockID, props) {
       // Add high cost penalty
       // Or mark as impassable
   }
   ```

3. **Fluid Handling** (NEW)
   ```go
   // General fluid detection
   if shapeMgr.IsFluid(blockID, props) {
       // Apply fluid physics
       // Adjust movement speed
   }
   ```

4. **Door Navigation** (NEW)
   ```go
   // Detect doors that may need opening
   if shapeMgr.IsDoorLike(blockID, props) {
       // Attempt to open door
       // Or find alternate route
   }
   ```

5. **Fence Handling** (NEW)
   ```go
   // Detect fences requiring jump
   if shapeMgr.IsFenceLike(blockID, props) {
       // Add jump movement cost
       // Or path around if too high
   }
   ```

**Updated Advanced Movement Tasks:**
- ✅ Add diagonal movement
- ✅ Add jump-across-gap (2-block jumps)
- ✅ Add ladder climbing support (using `IsClimbable()`)
- ✅ **Add swimming/water navigation** (using `IsWater()`)
- ✅ **Add lava avoidance** (using `IsLava()`)
- ✅ **Add door opening** (using `IsDoorLike()`)
- ✅ Improve movement smoothness with interpolation

### Phase 5: Robustness & Edge Cases
**Impact:** ✅ **Enhanced**

**Stuck Detection Improvements:**
```go
// Better stuck detection using block types
func isStuck(currentPos V3) bool {
    blockBelow := getBlockAt(currentPos.X, currentPos.Y - 1, currentPos.Z)

    // Check if in dangerous situation
    if shapeMgr.IsLava(blockBelow.ID, blockBelow.Props) {
        return true // STUCK IN LAVA!
    }

    // Check if in water (different stuck logic)
    if shapeMgr.IsWater(blockBelow.ID, blockBelow.Props) {
        // Swimming stuck detection
        // ...
    }

    return false
}
```

### Phase 6: Polish & Advanced Features
**Impact:** ✅ **Enhanced**

**Additional Features Enabled:**
- Tree navigation using `IsLogOrLeaf()`
- Precise collision for complex terrain using `IsSlab()`, `IsStair()`
- Fluid-aware pathfinding costs

## Required Actions Before Implementation

### ⚠️ IMPORTANT: Data Regeneration Required

The loader code has been updated, but the JSON data files need regeneration to include new properties.

**Current State:**
- Loader updated: `Nov 23 14:55` ✅
- Data generated: `Nov 23 09:22` ⚠️ (before loader update)

**Action Required:**
```bash
cd /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-data-gen
go run ./cmd/mc-data-gen -config mc-data-gen.yaml -work-dir ./work
```

This will regenerate all version data with new properties:
- `climbable`, `water`, `lava`, `fluid`
- `door_like`, `fence_like`, `slab`, `stair`, `log_or_leaf`

**Verification:**
After regeneration, verify a sample file:
```bash
# Should now include all new properties
cat data/1.21.5/blocks/minecraft/ladder.json | grep climbable
# Expected: "climbable": true
```

## Updated Phase 2 Task List

**Original Tasks:**
1. Add `mc-data-gen/loader` dependency
2. Create `BlockShapeManager` wrapper
3. Uncomment A* implementation
4. Replace hardcoded block lists
5. Implement basic movement primitives
6. Integrate with world manager
7. Test on multiple versions

**Updated Tasks (accounting for new properties):**
1. Add `mc-data-gen/loader` dependency
2. **⚠️ Regenerate mc-data-gen data for all versions**
3. Create `BlockShapeManager` wrapper with new helper methods:
   - `IsClimbable()` (simplified)
   - `IsDangerous()` (new - uses `IsLava()`)
   - `IsSlab()` (new)
   - `IsStair()` (new)
4. Uncomment A* implementation
5. Replace hardcoded block lists with shape data queries
6. Implement basic movement primitives with danger avoidance
7. Integrate with world manager
8. Test on multiple versions (1.21.1, 1.21.5)

## Updated Code Examples

### BlockShapeManager (Full Implementation)

```go
package path

import (
    mdl "github.com/reallyoldfogie/mc-data-gen/loader"
)

type BlockShapeManager interface {
    // Core collision
    IsPassable(blockID string, props map[string]string) bool
    IsSolid(blockID string, props map[string]string) bool
    GetCollisionBoxes(blockID string, props map[string]string, x, y, z int) []mdl.AABB

    // Movement types
    IsClimbable(blockID string, props map[string]string) bool

    // NEW: Fluid detection
    IsFluid(blockID string, props map[string]string) bool
    IsWater(blockID string, props map[string]string) bool
    IsLava(blockID string, props map[string]string) bool

    // NEW: Danger detection
    IsDangerous(blockID string, props map[string]string) bool

    // NEW: Block type classification
    IsDoorLike(blockID string, props map[string]string) bool
    IsFenceLike(blockID string, props map[string]string) bool
    IsSlab(blockID string, props map[string]string) bool
    IsStair(blockID string, props map[string]string) bool
    IsLogOrLeaf(blockID string, props map[string]string) bool
}

type blockShapeManager struct {
    version   string
    shapeData map[mdl.StateKey]mdl.ShapeInfo
}

func NewBlockShapeManager(version string, dataPath string) (BlockShapeManager, error) {
    data, err := mdl.LoadBlocksDir(dataPath)
    if err != nil {
        return nil, err
    }

    return &blockShapeManager{
        version:   version,
        shapeData: data,
    }, nil
}

// Core methods (unchanged)
func (bsm *blockShapeManager) IsPassable(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsPassable()
}

func (bsm *blockShapeManager) IsSolid(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.SolidBlock || info.IsStandingSurface() > 0
}

// NEW: Simplified climbing (no blockID logic needed!)
func (bsm *blockShapeManager) IsClimbable(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsClimbable()
}

// NEW: Fluid detection
func (bsm *blockShapeManager) IsFluid(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsFluid()
}

func (bsm *blockShapeManager) IsWater(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsWater()
}

func (bsm *blockShapeManager) IsLava(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsLava()
}

// NEW: Danger detection (currently just lava, expandable)
func (bsm *blockShapeManager) IsDangerous(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    // Could add: fire, magma blocks, cactus, etc.
    return info.IsLava()
}

// NEW: Block type helpers
func (bsm *blockShapeManager) IsDoorLike(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsDoorLike()
}

func (bsm *blockShapeManager) IsFenceLike(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsFenceLike()
}

func (bsm *blockShapeManager) IsSlab(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsSlab()
}

func (bsm *blockShapeManager) IsStair(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsStair()
}

func (bsm *blockShapeManager) IsLogOrLeaf(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsLogOrLeaf()
}

func (bsm *blockShapeManager) GetCollisionBoxes(blockID string, props map[string]string, x, y, z int) []mdl.AABB {
    info := bsm.getInfo(blockID, props)
    return info.WorldCollisionBoxesAt(x, y, z)
}

// Helper to get ShapeInfo
func (bsm *blockShapeManager) getInfo(blockID string, props map[string]string) mdl.ShapeInfo {
    key := mdl.StateKey{
        BlockID:  blockID,
        PropsKey: mdl.MakePropsKey(props),
    }

    info, ok := bsm.shapeData[key]
    if !ok {
        // Return empty/air-like info for unknown blocks
        return mdl.ShapeInfo{Air: true}
    }

    return info
}
```

## Conclusion

**Overall Assessment:** ✅ **HIGHLY POSITIVE**

The mc-data-gen updates significantly improve the implementation plan by:

1. **Simplifying** implementation (direct property access, no heuristics)
2. **Enabling** new capabilities (water navigation, lava avoidance)
3. **Improving** precision (slab/stair/door detection)
4. **Maintaining** version agnosticism (all properties generated per version)

**Required Changes:**
1. ⚠️ **Regenerate data** before Phase 2 implementation
2. ✅ **Update BlockShapeManager** to expose new helper methods
3. ✅ **Enhance Phase 4** to include fluid navigation
4. ✅ **Add danger avoidance** to pathfinding cost calculation

**No Breaking Changes:** All updates are additive, plan remains valid

**Recommendation:** ✅ **Proceed with implementation** after data regeneration

---

**Impact Assessment Version:** 1.0
**Date:** 2025-11-23
**Status:** Complete - Ready for Plan Update
