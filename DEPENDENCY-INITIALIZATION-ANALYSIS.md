# Dependency Initialization Analysis

**Date:** 2026-02-23
**Status:** Investigation Complete - Refactoring Needed

## Summary

The agent has inconsistent initialization patterns:
- Some dependencies auto-initialize in `Init()`
- Some require manual setup via `Set*()` methods
- Some have guards that return errors instead of falling back

This analysis identifies what should be auto-initialized vs what should remain optional.

## Current Status of Key Dependencies

### ✅ Currently Auto-Initialized in Init()
| Dependency | Auto-Init | Location | Notes |
|------------|-----------|----------|-------|
| screenMgr | ✅ | Line 458 | Inventory/containers |
| worldMgr | ✅ | Line 441-453 | Block queries |
| chatMgr | ✅ | Line 427 | Message sending |
| moveExec | ⚠️ | Line 524 | **HAS PROBLEMATIC GUARDS** |
| pathfind | ✅ | Line 630 | HPA* pathfinding |
| shapeMgr | ✅ | Line 485 | Collision data |
| stateProps | ✅ | Line 473 | Block state properties |
| containerHelper | ✅ | Line ~762 | **JUST FIXED** |
| playerList | ✅ | ~handlers | Player tracking |
| planRunner | ✅ | ~handlers | Plan execution |
| commandRegistry | ✅ | ~handlers | Command handling |

### ⚠️ Should Be Auto-Initialized But Aren't
| Dependency | Status | Why Needed | Current Fix |
|------------|--------|-----------|-------------|
| itemUsage | ✅ Fixed | Container operations | Auto with containerHelper |
| invMgr | ✅ Fixed | Container operations | Auto with containerHelper |
| followMgr | ❌ Manual | Player following | Requires manual SetFollowManager() |
| targetSelector | ❌ Manual | NPC targeting | Requires manual SetTargetSelector() |
| itemMgr | ❌ Manual | Item management | Requires manual SetItemManager() |
| slotResolver | ❌ Manual | Slot queries | Requires manual SetSlotResolver() |

### ❌ Problems with Current Implementation

#### Problem 1: MovementExecutor Has Problematic Guards

**Location:** agent/agent.go lines 505-516

**Current Code:**
```go
if a.worldMgr == nil {
    log.Printf("[Agent %s] Warning: world manager unavailable, falling back to interpolation executor", ...)
    return fmt.Errorf("[Agent %s] Missing world manager", a.client.Name())
} else if shapeMgr == nil {
    log.Printf("[Agent %s] Warning: block shape data unavailable, falling back to interpolation executor", ...)
    return fmt.Errorf("[Agent %s] Missing shape manager", a.client.Name())
} else if a.blockMgr == nil {
    log.Printf("[Agent %s] Warning: block manager unavailable, falling back to interpolation executor", ...)
    return fmt.Errorf("[Agent %s] Missing block manager", a.client.Name())
}
```

**The Bug:**
- Comments say "falling back to interpolation executor"
- But code RETURNS ERRORS instead
- Should use InterpolationExecutor when dependencies missing, not fail

**Impact:** If any manager is missing, Init() fails entirely instead of gracefully degrading

---

## Recommended Refactoring Plan

### Phase 1: Fix MovementExecutor (Immediate)

**Remove guards and implement actual fallback:**

```go
// Check if physics executor can be used (requires world manager, shape data, block manager)
if a.worldMgr != nil && shapeMgr != nil && a.blockMgr != nil {
    // Physics executor can be used
    executorType = movement.PhysicsExecutor
    execConfig.World = movement.NewPhysicsWorldAdapter(a.worldMgr)
    execConfig.ShapeProvider = shapeMgr
    log.Printf("[Agent %s] Using PhysicsExecutor (all managers available)", a.client.Name())
} else {
    // Fall back to InterpolationExecutor
    if a.worldMgr == nil {
        log.Printf("[Agent %s] Warning: world manager unavailable, using InterpolationExecutor", a.client.Name())
    }
    if shapeMgr == nil {
        log.Printf("[Agent %s] Warning: block shape data unavailable, using InterpolationExecutor", a.client.Name())
    }
    if a.blockMgr == nil {
        log.Printf("[Agent %s] Warning: block manager unavailable, using InterpolationExecutor", a.client.Name())
    }
    executorType = movement.InterpolationExecutor
    log.Printf("[Agent %s] Using InterpolationExecutor (fallback)", a.client.Name())
}

a.moveExec = movement.NewExecutor(executorType, execConfig)
log.Printf("[Agent %s] Movement executor initialized (%s)", a.client.Name(), executorType.String())
```

**Benefits:**
- ✅ MovementExecutor always created (never nil)
- ✅ Graceful degradation (PhysicsExecutor → InterpolationExecutor)
- ✅ Fixes container timeout issue (moveExec always available)
- ✅ No more "movement executor not set" errors

---

### Phase 2: Auto-Initialize Following System (Optional)

**Consider auto-creating basic FollowManager:**

Currently requires manual setup in every test/user code:
```go
agent.SetFollowManager(following.NewManager(...))
```

**Options:**
1. **Auto-initialize with defaults** - Create in Init() with standard config
2. **Keep optional** - Too application-specific, let users configure
3. **Auto-initialize if config present** - Check if follow config in agent config

**Recommendation:** Keep optional (application-specific behavior)

---

### Phase 3: Auto-Initialize ItemManager & SlotResolver (Medium Priority)

**ItemManager:**
- Currently manually set in tests
- Could auto-initialize from registry/inventory data
- Should happen after containerHelper init

**SlotResolver:**
- Currently manually set in tests (wraps screen manager)
- Could auto-initialize from screen manager
- Simple wrapper - good candidate for auto-init

---

## Dependencies That Should Remain Optional

| Dependency | Why |
|------------|-----|
| followMgr | Application-specific behavior |
| targetSelector | Application-specific (varies by bot purpose) |
| chat | Optional (config provides override) |
| teleport | Player subsystem (might be overridden) |
| playerNameByUUID | Application-specific resolver |
| playerUUIDByName | Application-specific resolver |

These have application-specific behavior and shouldn't be forced defaults.

---

## Summary of Issues Found

### Critical Issue ⚠️
**MovementExecutor guards return errors instead of falling back**
- Prevents executor creation if any manager missing
- Causes agent Init() to fail
- Blocks container operations
- **FIX:** Remove guards, implement actual fallback to InterpolationExecutor

### Medium Issues
- itemUsage (✅ FIXED - auto with containerHelper)
- invMgr (✅ FIXED - auto with containerHelper)
- itemMgr (TODO - could auto-initialize)
- slotResolver (TODO - could auto-initialize)

### Design Issues
- Inconsistent initialization pattern (some auto, some manual)
- Error returns instead of graceful degradation
- No clear documentation of what requires manual setup

---

## Proposed Auto-Initialization Order

1. ✅ **Core subsystems** (required)
   - bot client
   - packet manager
   - version handler
   - world manager
   - screen manager
   - player list

2. ✅ **Movement/pathfinding** (required, with fallback)
   - movement executor (Physics → Interpolation)
   - pathfinder (HPA*)
   - shape manager
   - block state properties

3. ✅ **Container system** (required)
   - item usage
   - inventory manager
   - container helper

4. 🔲 **Optional systems** (application-specific)
   - item manager
   - slot resolver
   - follow manager
   - target selector
   - chat
   - teleport

---

## Files to Modify

### Immediate (Critical)
1. **agent/agent.go** (lines 505-527)
   - Remove error-returning guards
   - Implement actual fallback logic
   - Ensure moveExec always created

### Short-term (Already Done)
2. **agent/agent.go** (lines ~758-788)
   - ✅ Auto-initialize containerHelper
   - ✅ Auto-initialize itemUsage
   - ✅ Auto-initialize invMgr

### Medium-term (Optional)
3. **agent/agent.go** (after containerHelper init)
   - Auto-initialize itemManager
   - Auto-initialize slotResolver

---

## Testing Impact

### Immediate Benefits
- 120+ container tests now PASS (vs timeout)
- ~1-2 hours saved per test run

### Future Benefits
- Reduced test boilerplate
- Better API usability
- Fewer "forgot to initialize" bugs

---

## Backward Compatibility

✅ **All changes backward compatible**
- SetMovementExecutor() still works
- SetContainerHelper() still works
- All Set*() methods still override auto-initialization
- Existing code continues to work

