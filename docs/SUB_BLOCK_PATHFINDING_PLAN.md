# Sub-Block Precision Pathfinding Implementation Plan

## Executive Summary

This document details the implementation plan for adding sub-block Y coordinate precision to the pathfinding system, enabling the bot to properly navigate stairs, slabs, and other partial blocks.

**Status**: Planning
**Priority**: Medium (workaround currently in place, full fix needed for production quality)
**Dependencies**: World integration (✅ complete), mc-data-gen block shapes (✅ available)

---

## Problem Statement

### Current Limitations

The pathfinding system uses **integer-only Y coordinates**:
```go
type V3 struct {
    X, Y, Z int
}
```

This causes several issues:

1. **Stairs are treated as full blocks**
   - Stair at Y=64 has surface at Y=64.5
   - Pathfinder generates step at Y=64 or Y=65 (both wrong)
   - Bot oscillates or floats trying to reach integer Y

2. **Slabs ignored**
   - Bottom slab at Y=64 has surface at Y=64.5
   - Top slab at Y=64 has surface at Y=65.0
   - Both treated as Y=64 (incorrect standing position)

3. **Complex terrain misnavigated**
   - Carpets (Y+0.0625), snow layers (Y+0.125), etc.
   - Lily pads, pressure plates, and other thin blocks
   - Path height doesn't match actual standing surface

4. **Movement feels unnatural**
   - Bot doesn't smoothly transition onto stairs
   - Unnecessary jumps where stepping up would work
   - Server corrections cause jitter

### Current Workaround

The tolerance fix implemented in `pathfinding/movement.go:226-251`:
- Checks Y-2 for partial blocks when Y-1 is air
- Allows bot to recognize stairs/slabs exist
- **Limitation**: Path still uses wrong Y coordinates, just doesn't fail validation

---

## Goals

### Primary Goal
Enable pathfinding to generate **precise sub-block Y coordinates** matching actual block surface heights.

### Success Criteria

1. ✅ Bot stands at correct height on stairs (e.g., Y=64.5 for bottom half)
2. ✅ Bot transitions smoothly onto partial blocks without jumping
3. ✅ Path includes exact Y coordinates for all block types
4. ✅ No floating, oscillation, or server position corrections
5. ✅ Pathfinding cost accurately reflects height changes

### Non-Goals (Future Work)

- Boat navigation on water surfaces
- Elytra flight paths
- Entity collision avoidance
- Dynamic block updates (doors opening/closing mid-path)

---

## Architecture Design

### Option 1: Float Y in V3 (RECOMMENDED)

**Change V3 structure:**
```go
type V3 struct {
    X, Z int     // Block coordinates (unchanged)
    Y    float64 // Sub-block precision
}
```

**Pros:**
- Minimal code changes (Y already used as float in many places)
- Exact representation of surface heights
- Movement executor already uses float64 Y

**Cons:**
- Hash map keys need special handling (can't hash floats directly)
- Need to round Y for chunk/section lookups

**Implementation:**
- Use `V3` for path steps with float Y
- Round Y to int when calling `world.GetBlockAt(x, int(y), z)`
- Hash function uses `(X, int(Y*2), Z)` for 0.5 block precision

---

### Option 2: Add TargetY Float Field (Alternative)

**Keep V3 as integers, add separate field:**
```go
type PathStep struct {
    Position V3      // Block position (integer)
    TargetY  float64 // Exact standing Y
    Movement Movement
    Cost     float64
}
```

**Pros:**
- V3 unchanged (backward compatible)
- Clear separation of "block position" vs "standing position"
- Hash maps work unchanged

**Cons:**
- More complex data structure
- Need to keep Position.Y and TargetY in sync
- Duplicated Y information

---

### Option 3: Fixed-Point Y (16-bit precision)

**Use integer with implied decimal:**
```go
type V3 struct {
    X, Z int
    Y    int // Y * 16 (0.0625 block precision)
}
```

**Pros:**
- Hash maps work natively
- No floating-point comparison issues
- Sufficient precision (0.0625 blocks = 1/16)

**Cons:**
- Less intuitive (Y=1024 means Y=64.0)
- Conversion overhead
- More complex than float

---

### **Recommended Approach: Option 1 (Float Y)**

**Rationale:**
- Movement executor already expects `float64` Y
- Surface heights from mc-data-gen are already floats
- Modern hash functions handle float keys well
- Simplest integration with existing code

---

## Implementation Plan

### Phase 1: Data Structure Changes

#### Step 1.1: Update V3 Structure

**File:** `pathfinding/astar.go` (or create `pathfinding/types.go`)

```go
// V3 represents a 3D position with sub-block Y precision
type V3 struct {
    X, Z int     // Block coordinates (integer)
    Y    float64 // Sub-block precision (0.0 to 320.0)
}

// Add creates a new V3 by adding offsets
func (v V3) Add(dx, dy, dz int) V3 {
    return V3{
        X: v.X + dx,
        Y: v.Y + float64(dy),
        Z: v.Z + dz,
    }
}

// AddFloat creates a new V3 by adding float offsets
func (v V3) AddFloat(dx, dy, dz float64) V3 {
    return V3{
        X: v.X + int(dx),
        Y: v.Y + dy,
        Z: v.Z + int(dz),
    }
}

// BlockY returns the integer block Y coordinate
func (v V3) BlockY() int {
    return int(math.Floor(v.Y))
}

// Hash returns a hash value for use in maps (0.5 block precision)
func (v V3) Hash() string {
    // Round Y to nearest 0.5 for hashing
    yHalf := int(math.Round(v.Y * 2))
    return fmt.Sprintf("%d,%d,%d", v.X, yHalf, v.Z)
}
```

#### Step 1.2: Update PathFinder Maps

**File:** `pathfinding/astar.go`

**Before:**
```go
openSet := make(map[V3]*node)
closedSet := make(map[V3]bool)
```

**After:**
```go
openSet := make(map[string]*node)   // Key is V3.Hash()
closedSet := make(map[string]bool)  // Key is V3.Hash()
```

**Access pattern:**
```go
// Old: openSet[pos] = nodePtr
// New:
hash := pos.Hash()
openSet[hash] = nodePtr
```

---

### Phase 2: Surface Height Calculation

#### Step 2.1: Add GetSurfaceY() Helper

**File:** `pathfinding/movement.go`

```go
// GetSurfaceY returns the exact Y coordinate of the standing surface at the given block position
func (mv *MovementValidator) GetSurfaceY(blockPos V3) float64 {
    // Get block at position
    stateID := mv.world.GetBlockAt(blockPos.X, blockPos.BlockY(), blockPos.Z)
    if stateID == 0 {
        // Air - no surface here, return block bottom
        return float64(blockPos.BlockY())
    }

    // Get block info
    blockID := mv.blockMgr.BlockIDByStateID(stateID)
    block := mv.blockMgr.GetByID(blockID)
    props := mv.propsFromStateID(stateID)

    // Get standing surface height (0.0 to 1.0 within block)
    surfaceHeight := mv.shapeMgr.GetStandingSurfaceHeight(block.Name, props)

    // Return absolute Y coordinate
    return float64(blockPos.BlockY()) + surfaceHeight
}
```

#### Step 2.2: Add GetStandingPosition() Helper

**File:** `pathfinding/movement.go`

```go
// GetStandingPosition returns the exact position (with sub-block Y) where the bot would stand
// at the given block coordinates
func (mv *MovementValidator) GetStandingPosition(blockPos V3) V3 {
    surfaceY := mv.GetSurfaceY(blockPos)
    return V3{
        X: blockPos.X,
        Y: surfaceY,
        Z: blockPos.Z,
    }
}
```

---

### Phase 3: Movement Validation Updates

#### Step 3.1: Update CanTraverse()

**File:** `pathfinding/movement.go:60-91`

**Current logic:**
```go
func (mv *MovementValidator) CanTraverse(from, to V3) bool {
    // Checks if Y difference is 0
    // Checks if horizontal blocks are passable
}
```

**Updated logic:**
```go
func (mv *MovementValidator) CanTraverse(from, to V3) bool {
    dx := to.X - from.X
    dz := to.Z - from.Z

    // Must be on same block Y level (but sub-block Y may differ)
    if from.BlockY() != to.BlockY() {
        return false
    }

    // Get exact surface heights
    fromSurfaceY := mv.GetSurfaceY(from)
    toSurfaceY := mv.GetSurfaceY(to)
    heightDiff := toSurfaceY - fromSurfaceY

    // If height difference is small (< 0.6 blocks), can step up without jumping
    if math.Abs(heightDiff) > 0.6 {
        return false // Need to jump or descend instead
    }

    // Check if path is clear (existing logic)
    // ...
}
```

#### Step 3.2: Update CanAscend()

**File:** `pathfinding/movement.go:93-128`

```go
func (mv *MovementValidator) CanAscend(from, to V3) bool {
    dx := to.X - from.X
    dz := to.Z - from.Z

    // Must be moving up by exactly 1 block
    dyBlocks := to.BlockY() - from.BlockY()
    if dyBlocks != 1 {
        return false
    }

    // Get exact surface heights
    fromSurfaceY := mv.GetSurfaceY(from)
    toSurfaceY := mv.GetSurfaceY(to)
    heightDiff := toSurfaceY - fromSurfaceY

    // Can step up 0.5-1.6 blocks without jumping
    if heightDiff < 0.5 || heightDiff > 1.6 {
        return false
    }

    // Check if path is clear (existing logic)
    // ...

    return true
}
```

#### Step 3.3: Update CanDescend()

**File:** `pathfinding/movement.go:130-147`

```go
func (mv *MovementValidator) CanDescend(from, to V3) bool {
    // Similar to CanAscend but for downward movement
    fromSurfaceY := mv.GetSurfaceY(from)
    toSurfaceY := mv.GetSurfaceY(to)
    heightDiff := fromSurfaceY - toSurfaceY // Note: reversed

    // Can safely drop 0.5-3.0 blocks
    if heightDiff < 0.5 || heightDiff > 3.0 {
        return false
    }

    // Rest of validation
    // ...
}
```

#### Step 3.4: Update hasGroundSupport()

**File:** `pathfinding/movement.go:220-248`

**Current tolerance workaround can be REMOVED:**

```go
func (mv *MovementValidator) hasGroundSupport(pos V3) bool {
    // Get block directly below feet
    blockBelow := V3{X: pos.X, Y: float64(pos.BlockY() - 1), Z: pos.Z}
    stateID := mv.world.GetBlockAt(blockBelow.X, blockBelow.BlockY(), blockBelow.Z)

    if stateID == 0 {
        // Air below - no ground support
        return false
    }

    // Get surface height of block below
    surfaceY := mv.GetSurfaceY(blockBelow)

    // Check if bot is standing on this surface (within tolerance)
    if math.Abs(pos.Y - surfaceY) > 0.1 {
        // Bot is not on this surface
        return false
    }

    // Check if block is solid
    blockID := mv.blockMgr.BlockIDByStateID(stateID)
    block := mv.blockMgr.GetByID(blockID)
    props := mv.propsFromStateID(stateID)

    return mv.shapeMgr.IsSolid(block.Name, props)
}
```

---

### Phase 4: A* Pathfinding Updates

#### Step 4.1: Update Neighbor Generation

**File:** `pathfinding/astar.go` (in A* algorithm)

**Current:**
```go
// Generate neighbors with integer Y offsets
neighbors := []V3{
    current.Add(1, 0, 0),
    current.Add(-1, 0, 0),
    current.Add(0, 1, 0),  // Up (integer)
    current.Add(0, -1, 0), // Down (integer)
    // ...
}
```

**Updated:**
```go
// Generate possible moves using MovementValidator
possibleMoves := mv.GetPossibleMoves(current)

for _, move := range possibleMoves {
    // Each move has exact target Y coordinate
    neighborPos := mv.GetStandingPosition(move.To)
    // neighborPos.Y is now a float (e.g., 64.5 for stairs)

    // Process neighbor with sub-block Y
    // ...
}
```

#### Step 4.2: Update Movement Cost Calculation

**File:** `pathfinding/movement.go:178-218`

```go
func (mv *MovementValidator) CalculateMoveCost(from, to V3, movement Movement) float64 {
    baseCost := 1.0

    // Get exact surface heights
    fromSurfaceY := mv.GetSurfaceY(from)
    toSurfaceY := mv.GetSurfaceY(to)
    heightChange := toSurfaceY - fromSurfaceY

    switch movement {
    case Traverse:
        // Small step-up (< 0.6 blocks) is cheap
        if heightChange > 0 && heightChange <= 0.6 {
            baseCost += heightChange * 0.5 // Minor cost for stepping up
        }

    case Ascend:
        // Full block step-up (0.6-1.6 blocks)
        baseCost += 2.0 + heightChange

    case Jump:
        // Jumping is expensive
        baseCost += 5.0 + heightChange * 2.0

    case Descend:
        // Dropping is cheap
        baseCost += 0.5 + (heightChange * 0.1)

    // ... other movement types
    }

    // Check for slabs/stairs - these are easier to navigate
    toStateID := mv.world.GetBlockAt(to.X, to.BlockY(), to.Z)
    if toStateID != 0 {
        blockID := mv.blockMgr.BlockIDByStateID(toStateID)
        block := mv.blockMgr.GetByID(blockID)
        props := mv.propsFromStateID(toStateID)

        if mv.shapeMgr.IsSlab(block.Name, props) || mv.shapeMgr.IsStair(block.Name, props) {
            baseCost *= 0.8 // 20% discount for partial blocks
        }
    }

    return baseCost
}
```

---

### Phase 5: Path Execution Updates

#### Step 5.1: Update FollowManager Path Following

**File:** `following/manager.go:468-514`

**Current:**
```go
stepX, stepY, stepZ := float64(step.Position.X), float64(step.Position.Y), float64(step.Position.Z)
targetX := stepX + 0.5
targetY := stepY + 0.5 // Always block center
targetZ := stepZ + 0.5
```

**Updated:**
```go
stepX, stepZ := float64(step.Position.X), float64(step.Position.Z)
stepY := step.Position.Y // Already a float with exact surface height

// Target is block center horizontally, exact surface Y vertically
targetX := stepX + 0.5
targetY := stepY // Use exact Y from pathfinding (e.g., 64.5 for stairs)
targetZ := stepZ + 0.5
```

#### Step 5.2: Movement Executor (Already Compatible)

**File:** `movement/executor.go`

The recent fix already handles float Y correctly:
```go
// Smart physics for Y movement (lines 142-152)
if dy > 0 || math.Abs(dy) < 0.1 {
    newY = targetY // Uses exact float Y
} else {
    fallAmount := math.Min(-dy, 0.5)
    newY = math.Max(targetY, botY-fallAmount)
}
```

✅ **No changes needed** - already supports sub-block Y coordinates!

---

### Phase 6: Testing & Validation

#### Test Case 1: Stairs Navigation
```
Setup:
- Place oak stairs going up from Y=64 to Y=68
- Each stair has surface at Y+0.5

Expected:
- Path includes steps: (x, 64.5, z) → (x+1, 65.5, z) → (x+2, 66.5, z)
- Bot smoothly walks up stairs without jumping
- No floating or position corrections

Validation:
- Debug log shows Y=64.5, 65.5, 66.5 in path
- Bot position matches path Y exactly
```

#### Test Case 2: Slab Navigation
```
Setup:
- Place bottom slabs at Y=64 (surface at 64.5)
- Place top slabs at Y=65 (surface at 66.0)

Expected:
- Path correctly identifies surface heights
- Bot stands at Y=64.5 on bottom slabs
- Bot stands at Y=66.0 on top slabs

Validation:
- No "floating 0.5 blocks" behavior
- Position matches slab surface exactly
```

#### Test Case 3: Mixed Terrain
```
Setup:
- Path includes grass (Y=64.0), stairs (Y=64.5), slabs (Y=65.5), full block (Y=66.0)

Expected:
- Path smoothly transitions between all surface types
- Correct movement types chosen (Traverse vs Ascend)
- No unnecessary jumps

Validation:
- Movement costs reflect actual difficulty
- Bot takes most efficient path
```

#### Test Case 4: Edge Cases
```
Test:
1. Carpet on stairs (Y=64.5 + 0.0625)
2. Snow layers (variable height)
3. Pressure plates on slabs
4. Lily pads on water surface

Expected:
- All surface heights calculated correctly
- Bot navigates all terrain types
```

---

## Migration Strategy

### Phase-by-Phase Rollout

**Phase 1: Data structures (Week 1)**
- Update V3 to use float Y
- Update hash maps in PathFinder
- Update existing code to use BlockY() where needed
- **Test**: Existing paths still work (backward compatible)

**Phase 2: Surface height helpers (Week 1)**
- Add GetSurfaceY() and GetStandingPosition()
- **Test**: Surface heights match mc-data-gen data

**Phase 3: Movement validation (Week 2)**
- Update CanTraverse(), CanAscend(), CanDescend()
- Update hasGroundSupport()
- **Test**: Movement validation more accurate

**Phase 4: A* pathfinding (Week 2)**
- Update neighbor generation
- Update cost calculation
- **Test**: Paths include sub-block Y coordinates

**Phase 5: Path execution (Week 3)**
- Update FollowManager to use float Y
- **Test**: Bot navigates stairs/slabs correctly

**Phase 6: Testing & refinement (Week 3)**
- Comprehensive testing on all terrain types
- Performance optimization
- Bug fixes

### Backward Compatibility

**Compatibility with integer Y code:**
```go
// Old code expecting int Y:
blockY := pos.Y // ERROR: Y is now float64

// New code:
blockY := pos.BlockY() // Correct: returns int

// Or for exact Y:
exactY := pos.Y // float64
```

**Migration helper:**
```go
// Convert old V3 (int Y) to new V3 (float Y)
func V3FromBlockCoords(x, y, z int) V3 {
    return V3{X: x, Y: float64(y), Z: z}
}

// Create V3 with exact Y
func V3FromExact(x int, y float64, z int) V3 {
    return V3{X: x, Y: y, Z: z}
}
```

---

## Performance Considerations

### Memory Impact

**Before (integer Y):**
- V3 size: 24 bytes (3 × int64)
- Hash map: integer keys (fast)

**After (float Y):**
- V3 size: 24 bytes (2 × int64 + 1 × float64)
- Hash map: string keys (slightly slower)

**Mitigation:**
- Use string interning for hash keys
- Limit Y precision to 0.5 blocks (adequate for all Minecraft blocks)
- Consider switching to custom hash function if profiling shows impact

### CPU Impact

**Additional operations:**
- Surface height lookups (query mc-data-gen)
- Float arithmetic instead of integer
- Hash string generation

**Mitigation:**
- Cache surface heights per block type
- Use lookup table for common blocks (grass, stone, stairs, slabs)
- Profile and optimize hot paths

**Estimated overhead:** < 5% (most time is in A* search, not coordinate arithmetic)

---

## Alternative Approaches Considered

### 1. Keep Integer Y, Add Metadata
```go
type PathStep struct {
    Position V3 // Integer
    Metadata struct {
        SurfaceOffset float64 // 0.0 to 1.0
    }
}
```
**Rejected**: Too complex, duplicates Y information

### 2. Use 0.25 Block Grid
```go
// All coordinates multiplied by 4
// Y=64.5 becomes Y=258 (64*4 + 2)
```
**Rejected**: Less intuitive, conversion overhead

### 3. Separate Pathfinding and Execution
```go
// Pathfinding uses integer Y
// Execution queries surface height just-in-time
```
**Rejected**: Path doesn't reflect actual bot position, hard to validate

---

## Dependencies

### Required:
- ✅ World integration (complete)
- ✅ mc-data-gen with GetStandingSurfaceHeight() (available)
- ✅ Movement executor supporting float Y (fixed in this session)

### Optional (future):
- Block state property parsing (for open doors, slab type, etc.)
- Improved climbing/swimming (can use same sub-block system)

---

## Success Metrics

### Performance Targets:
- Path generation time: < 50ms for 50-block path (same as before)
- Memory usage: < 10% increase
- CPU usage: < 5% increase

### Quality Targets:
- ✅ 100% accurate surface heights for all block types
- ✅ Zero position corrections from server (on stairs/slabs)
- ✅ Smooth visual movement (no oscillation)
- ✅ Efficient pathing (no unnecessary jumps)

### Reliability Targets:
- ✅ No crashes from float comparison issues
- ✅ No NaN or Inf values
- ✅ Handles all Minecraft block types (1.21.5)

---

## Implementation Checklist

### Phase 1: Data Structures
- [ ] Update V3 struct with float Y
- [ ] Add BlockY() method
- [ ] Add Hash() method
- [ ] Update V3.Add() and related methods
- [ ] Update PathFinder maps to use string keys
- [ ] Test: Compile and run with no crashes

### Phase 2: Surface Heights
- [ ] Implement GetSurfaceY()
- [ ] Implement GetStandingPosition()
- [ ] Add unit tests for surface height calculation
- [ ] Test: Surface heights match mc-data-gen data

### Phase 3: Movement Validation
- [ ] Update CanTraverse() with sub-block logic
- [ ] Update CanAscend() with exact height checks
- [ ] Update CanDescend() with exact height checks
- [ ] Remove tolerance workaround from hasGroundSupport()
- [ ] Test: Movement validation more accurate

### Phase 4: A* Pathfinding
- [ ] Update neighbor generation to use GetStandingPosition()
- [ ] Update cost calculation with height-based costs
- [ ] Test: Paths include correct Y coordinates

### Phase 5: Path Execution
- [ ] Update FollowManager to use step.Position.Y (float)
- [ ] Test: Bot reaches exact target positions

### Phase 6: Testing
- [ ] Test stairs navigation
- [ ] Test slab navigation
- [ ] Test mixed terrain
- [ ] Test edge cases (carpets, snow, etc.)
- [ ] Performance profiling
- [ ] Bug fixes and refinement

---

## Timeline

**Total Estimated Time**: 3 weeks

- **Week 1**: Phases 1-2 (data structures, surface heights)
- **Week 2**: Phases 3-4 (movement validation, pathfinding)
- **Week 3**: Phases 5-6 (execution, testing, refinement)

**Dependencies on go-live**: None (current tolerance workaround is acceptable for go-live)

---

## Risk Assessment

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| Float precision errors | High | Low | Use epsilon comparisons, limit to 0.5 precision |
| Performance regression | Medium | Medium | Profile before/after, optimize hot paths |
| Hash collision issues | Medium | Low | Use well-tested hash function, test thoroughly |
| Breaking existing paths | High | Low | Comprehensive testing, backward compatibility |
| mc-data-gen data incomplete | Low | Low | Fallback to block center if no surface data |

---

## Future Enhancements

Once sub-block Y is implemented, these features become easier:

1. **Precise collision detection**
   - 1×1×2 player bounding box
   - Head clearance checking
   - Tight space navigation

2. **Advanced movement**
   - Parkour jumps (precise landing)
   - Swimming (water surface = Y+0.875)
   - Boat navigation

3. **Complex terrain**
   - Fences (Y+1.5 height)
   - Walls with different heights based on connections
   - Redstone wire (Y+0.0625)

4. **Optimization**
   - Jump only when necessary
   - Step up when < 0.6 blocks
   - Minimize vertical movement

---

## Conclusion

Sub-block Y precision pathfinding is a **well-defined enhancement** with:
- ✅ Clear problem statement
- ✅ Tested dependencies (world integration, mc-data-gen)
- ✅ Incremental implementation path
- ✅ Backward compatibility strategy
- ✅ Comprehensive testing plan

**Recommended approach**: Float Y in V3 (Option 1)
**Estimated effort**: 3 weeks
**Priority**: Medium (workaround in place for go-live, full fix needed for production quality)

---

**Document Status**: ✅ Ready for Implementation
**Last Updated**: 2025-11-27
**Author**: Claude Code (Investigation + Planning)
