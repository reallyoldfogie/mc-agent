# Sneaking Mechanics Implementation Notes

**Created**: 2025-12-20
**Status**: Planned for Phase 5
**Related**: PHYSICS_INTEGRATION_PLAN.md Phase 5

---

## Overview

Sneaking (crouching) is a critical movement mechanic in Minecraft that allows players to:
1. **Fit through smaller spaces** - 1.5 block height instead of 1.8
2. **Navigate edges safely** - Cannot fall off block edges while sneaking
3. **Move stealthily** - Reduced movement speed (30% of normal)

The mc-agent currently has **partial sneaking support** (speed reduction) but lacks the full mechanics needed for realistic pathfinding.

---

## Current Implementation (Phase 2)

### What's Already Implemented ✅

In `physics/state.go` and `physics/constants.go`:
- ✅ `Inputs.Sneak` - Sneak input flag
- ✅ `SneakMultiplier = 0.3` - Speed reduction to 30% of normal
- ✅ Speed reduction applied in `applyMovementInputs()`

```go
// physics/state.go:267-273
if input.Sprint {
    throttleX *= SprintMultiplier  // 1.3x
    throttleZ *= SprintMultiplier
} else if input.Sneak {
    throttleX *= SneakMultiplier   // 0.3x
    throttleZ *= SneakMultiplier
}
```

### What's Missing ❌

1. **Dynamic hitbox height reduction** (1.8 → 1.5 blocks)
2. **Edge prevention** (can't walk off edges while sneaking)
3. **Movement primitives** (`SneakThrough`, `SneakTraverse`)
4. **Pathfinding integration** (recognize 1.5-1.8 block gaps as passable)

---

## Planned Implementation (Phase 5)

### 1. Dynamic Hitbox System

**File**: `physics/state.go`

**Changes**:
```go
// Add state tracking
type State struct {
    // ... existing fields ...
    isSneaking bool  // NEW: Track sneak state
}

// Update GetAABB to use dynamic height
func (s *State) GetAABB() AABB {
    height := s.height
    if s.isSneaking {
        height = PlayerHeightSneaking  // 1.5 blocks
    }
    return AABB{
        X: MinMax{Min: s.Pos.X - s.width/2, Max: s.Pos.X + s.width/2},
        Y: MinMax{Min: s.Pos.Y, Max: s.Pos.Y + height},  // Dynamic!
        Z: MinMax{Min: s.Pos.Z - s.width/2, Max: s.Pos.Z + s.width/2},
    }
}

// Update Tick to track sneak state
func (s *State) Tick(input Inputs, w World) error {
    s.isSneaking = input.Sneak  // Update state
    // ... rest of tick logic ...
}
```

**Impact**:
- Collision detection automatically uses correct hitbox height
- Player can fit through 1.5-1.8 block gaps when sneaking
- Seamless integration with existing collision system

### 2. Edge Prevention

**File**: `physics/state.go`

**Logic**:
```go
// In tickPosition(), before applying horizontal movement:
if s.isSneaking {
    // Check if next position would be over an edge
    nextPos := s.Pos.Add(V3{X: vel.X, Z: vel.Z})
    if isOverEdge(nextPos, w) {
        // Clamp movement to edge
        vel.X = clampToEdge(s.Pos.X, vel.X)
        vel.Z = clampToEdge(s.Pos.Z, vel.Z)
    }
}
```

**Edge Detection**:
- Check block below next position
- If air/passable, player is over edge
- Clamp velocity to prevent falling

### 3. New Movement Types

**File**: `pathfinding/types.go`

**New Primitives**:
```go
const (
    // ... existing movement types ...

    SneakThrough  MovementType = iota  // Navigate 1.5-1.8 block gaps
    SneakTraverse                       // Move along edges without falling
)
```

**Usage**:
- `SneakThrough`: Pathfinding through low ceiling areas (caves, tunnels, etc.)
- `SneakTraverse`: Building, navigating narrow ledges, crossing gaps

### 4. Movement Validation

**File**: `pathfinding/movement.go`

**New Function**:
```go
func canSneakThrough(from, to V3, world World, shapes BlockShapeManager) bool {
    // Check ceiling height at destination
    ceilingHeight := getCeilingHeight(to, world, shapes)

    // Can sneak if ceiling is 1.5-1.8 blocks high
    if ceilingHeight >= 1.5 && ceilingHeight < 1.8 {
        // Verify path is clear with sneaking hitbox (1.5 blocks)
        return isPathClear(from, to, 1.5, world, shapes)
    }

    return false
}
```

**Integration**:
- Called by pathfinding neighbor generation
- Adds `SneakThrough` edges when applicable
- Higher cost due to slow movement (3x base cost)

### 5. Input Generation

**File**: `pathfinding/input_generator.go`

**Logic**:
```go
func (ig *InputGenerator) GenerateInputs(step PathStep, currentState physics.PhysicsState) physics.Inputs {
    inputs := physics.Inputs{
        // ... calculate throttle and direction ...
    }

    // Auto-enable sneak for sneak-required movements
    if step.Movement == SneakThrough || step.Movement == SneakTraverse {
        inputs.Sneak = true
    }

    return inputs
}
```

---

## Swift Sneak Enchantment (Stretch Goal)

### What is Swift Sneak?

- **Enchantment**: Leggings only
- **Levels**: I, II, III
- **Effect**: Increases sneaking speed
- **Added**: Minecraft 1.19 (The Wild Update)

### Speed Multipliers

| Enchantment Level | Speed Multiplier | % of Normal | vs Base Sneak |
|-------------------|------------------|-------------|---------------|
| None (base)       | 0.3x             | 30%         | 1.0x          |
| Swift Sneak I     | 0.45x            | 45%         | 1.5x faster   |
| Swift Sneak II    | 0.60x            | 60%         | 2.0x faster   |
| Swift Sneak III   | 0.75x            | 75%         | 2.5x faster   |

### Implementation Requirements

**Prerequisites**:
1. **Equipment Tracking System**
   - Track player's equipped armor pieces
   - Listen to equipment update packets from server
   - Store equipment state in agent

2. **NBT Parsing**
   - Parse item NBT data for enchantments
   - Extract enchantment type and level
   - Handle enchantment changes (equipment swap)

3. **Dynamic Speed Calculation**
   - Query current equipment for Swift Sneak
   - Use appropriate multiplier in movement calculations
   - Update pathfinding costs when equipment changes

**Files to Modify**:
```go
// physics/state.go
type State struct {
    // ... existing fields ...
    swiftSneakLevel int  // 0-3 (0 = no enchantment)
}

func (s *State) GetSneakMultiplier() float64 {
    switch s.swiftSneakLevel {
    case 1: return SwiftSneakI    // 0.45
    case 2: return SwiftSneakII   // 0.60
    case 3: return SwiftSneakIII  // 0.75
    default: return SneakMultiplier // 0.30
    }
}

// pathfinding/movement.go
func (m Movement) GetCost(state AgentState) float64 {
    baseCost := m.getBaseCost()

    if m == SneakThrough || m == SneakTraverse {
        // Use dynamic sneak multiplier
        sneakMultiplier := state.GetSneakMultiplier()
        // Inverse because lower speed = higher cost
        return baseCost * (1.0 / sneakMultiplier)
    }

    return baseCost
}
```

**Equipment Tracking** (new system):
```go
// agent/equipment.go (NEW)
type EquipmentTracker struct {
    head, chest, legs, feet *Item
    enchantments map[EquipmentSlot][]Enchantment
}

func (et *EquipmentTracker) GetSwiftSneakLevel() int {
    if et.legs == nil {
        return 0
    }

    for _, ench := range et.enchantments[LegSlot] {
        if ench.Type == SwiftSneak {
            return ench.Level
        }
    }

    return 0
}
```

### Benefits

1. **Accurate Pathfinding Costs**
   - Bot calculates realistic movement times
   - Chooses optimal paths based on actual capabilities

2. **Leverage Player Gear**
   - Bot can take advantage of enchanted equipment
   - More efficient navigation with Swift Sneak III

3. **Realistic Behavior**
   - Movement speed matches player expectations
   - No desync between predicted and actual movement

---

## Testing Requirements

### Phase 5 Tests

**File**: `physics/state_test.go`

1. **TestState_SneakHitbox** - Verify 1.5 block height when sneaking
2. **TestState_SneakEdgePrevention** - Cannot walk off edges while sneaking
3. **TestState_SneakSpeed** - Speed reduced to 30%
4. **TestState_SneakThroughGap** - Can fit through 1.5 block gap

**File**: `pathfinding/movement_test.go`

5. **TestCanSneakThrough** - Validates 1.5-1.8 block gaps
6. **TestSneakMovementCost** - Correct cost calculation (3x base)
7. **TestPathfindingUsesSneak** - Pathfinder chooses sneak when optimal

### Stretch Goal Tests (Swift Sneak)

**File**: `physics/state_test.go`

8. **TestState_SwiftSneakI** - 45% speed with Swift Sneak I
9. **TestState_SwiftSneakII** - 60% speed with Swift Sneak II
10. **TestState_SwiftSneakIII** - 75% speed with Swift Sneak III

**File**: `agent/equipment_test.go` (NEW)

11. **TestEquipmentTracker_DetectSwiftSneak** - Parse NBT correctly
12. **TestEquipmentTracker_EquipmentChange** - Update on swap

---

## Implementation Timeline

### Phase 5 (Scheduled)
- Week 1: Dynamic hitbox + edge prevention
- Week 2: Movement primitives + pathfinding integration
- Week 3: Testing + polish

### Future (Stretch Goal)
- Equipment tracking system (1-2 weeks)
- NBT parsing integration (1 week)
- Swift Sneak detection + speed adjustment (3 days)
- Testing + validation (1 week)

**Total estimate for Swift Sneak**: 4-5 weeks (post-Phase 6)

---

## References

### Minecraft Wiki
- [Sneaking](https://minecraft.wiki/w/Sneaking)
- [Swift Sneak](https://minecraft.wiki/w/Swift_Sneak)
- [Player Entity Format](https://minecraft.wiki/w/Entity_format#Player)

### Related Documents
- `PHYSICS_INTEGRATION_PLAN.md` - Main implementation plan
- `FOLLOW_PLAYER_PLAN.md` - Movement system overview
- `physics/constants.go` - Physics constants including sneak multipliers

### Code References
- `physics/state.go` - Physics simulation (will add dynamic hitbox)
- `pathfinding/movement.go` - Movement validation (will add sneak checks)
- `pathfinding/types.go` - Movement types (will add SneakThrough, SneakTraverse)

---

## Notes

1. **Priority**: High - Sneaking is essential for realistic pathfinding
2. **Complexity**: Medium - Requires hitbox changes and new movement types
3. **Swift Sneak Priority**: Low - Nice-to-have, not critical for v1.0
4. **Testing**: Critical - Edge prevention must be bulletproof

**Status**: Documentation complete, ready for Phase 5 implementation
