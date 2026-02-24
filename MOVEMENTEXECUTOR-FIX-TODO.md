# MovementExecutor Guard Issue - Manual Fix Required

**Date:** 2026-02-23
**Priority:** HIGH - Blocks container operations
**Status:** Identified, needs manual code change

## Problem

**File:** `agent/agent.go`
**Lines:** 505-521

The guard conditions for MovementExecutor creation have **comments that say one thing but code that does another:**

### Current Code (BROKEN)
```go
// Check if physics executor can be used (requires world manager, shape data, block manager)
if a.worldMgr == nil {
    log.Printf("[Agent %s] Warning: world manager unavailable, falling back to interpolation executor", ...)
    // executorType = movement.InterpolationExecutor  ← COMMENTED OUT
    return fmt.Errorf("[Agent %s] Missing world manager", ...)  ← RETURNS ERROR INSTEAD
} else if shapeMgr == nil {
    log.Printf("[Agent %s] Warning: block shape data unavailable, falling back to interpolation executor", ...)
    // executorType = movement.InterpolationExecutor  ← COMMENTED OUT
    return fmt.Errorf("[Agent %s] Missing shape manager", ...)  ← RETURNS ERROR INSTEAD
} else if a.blockMgr == nil {
    log.Printf("[Agent %s] Warning: block manager unavailable, falling back to interpolation executor", ...)
    // executorType = movement.InterpolationExecutor  ← COMMENTED OUT
    return fmt.Errorf("[Agent %s] Missing block manager", ...)  ← RETURNS ERROR INSTEAD
} else {
    // Physics executor can be used
    execConfig.World = movement.NewPhysicsWorldAdapter(a.worldMgr)
    execConfig.ShapeProvider = shapeMgr
}
```

### Impact
- Comments promise fallback behavior
- Code returns errors instead
- Prevents `moveExec` from ever being created if any manager is missing
- Causes agent Init() to fail entirely
- Tests fail at agent creation time (not at container time)

## Solution

Replace lines 504-521 with:

```go
	// Check if physics executor can be used (requires world manager, shape data, block manager)
	// If any required component is missing, gracefully fall back to InterpolationExecutor
	if a.worldMgr == nil || shapeMgr == nil || a.blockMgr == nil {
		// At least one required component is missing - use fallback executor
		executorType = movement.InterpolationExecutor
		if a.worldMgr == nil {
			log.Printf("[Agent %s] Warning: world manager unavailable, using InterpolationExecutor", a.client.Name())
		}
		if shapeMgr == nil {
			log.Printf("[Agent %s] Warning: block shape data unavailable, using InterpolationExecutor", a.client.Name())
		}
		if a.blockMgr == nil {
			log.Printf("[Agent %s] Warning: block manager unavailable, using InterpolationExecutor", a.client.Name())
		}
	} else {
		// All required components available - physics executor can be used
		execConfig.World = movement.NewPhysicsWorldAdapter(a.worldMgr)
		execConfig.ShapeProvider = shapeMgr
		log.Printf("[Agent %s] All movement prerequisites available, using PhysicsExecutor", a.client.Name())
	}

	// Create movement executor (always succeeds - uses fallback if needed)
	a.moveExec = movement.NewExecutor(executorType, execConfig)
```

## What This Fixes

✅ **MovementExecutor is ALWAYS created** (never nil)
✅ **Graceful degradation:** PhysicsExecutor → InterpolationExecutor
✅ **No more early Init() failures** due to missing managers
✅ **Container operations now work** (moveExec.LookAt() available)
✅ **Cleaner error handling** (warnings instead of fatal errors)

## Related Changes Already Done

✅ `agent/agent.go` line ~758-788: ContainerHelper auto-initialization
✅ `testing/container_suite_test.go`: Removed manual setup boilerplate

## Next Steps for Other Dependencies

After MovementExecutor fix, consider:

1. **ItemManager** - Could auto-initialize from slot resolver
2. **SlotResolver** - Could auto-initialize from screen manager
3. **FollowManager** - Keep optional (application-specific)
4. **TargetSelector** - Keep optional (application-specific)

---

## Edit Tool Issue Workaround

The Bash Edit tool is having whitespace/tabs matching issues with this specific block.
**Manual edit recommended:**

1. Open `agent/agent.go`
2. Go to line 504
3. Replace lines 504-521 with the solution code above
4. Save and verify compilation: `go build ./agent`

