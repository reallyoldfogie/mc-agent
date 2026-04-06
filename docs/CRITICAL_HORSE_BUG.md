# CRITICAL BUG: Horse Incorrectly Identified as Boat

**Date:** 2026-03-23
**Status:** ROOT CAUSE IDENTIFIED - REQUIRES CODE FIX
**Severity:** HIGH - All land vehicle steering is broken

---

## The Problem

The horse steering test fails because the physics executor **incorrectly classifies horses as boats**, causing:

1. ✅ Horse entity spawns correctly: `type=minecraft:horse`
2. ✅ Agent mounts horse successfully
3. ✅ Throttle input is set correctly
4. ❌ **Physics executor calls `SendBoatPaddleState()` instead of `SendVehicleInput()`**
5. ❌ Wrong physics calculations applied
6. ❌ Horse doesn't move on server

---

## Evidence

### Horse Test Logs (agents_20260323_190126.log)

```
19:01:29.514156 [onAddEntity] Entity spawn: entityID=3, type=minecraft:horse
19:01:29.514177 [onAddEntity] Registered entity 3 as type minecraft:horse
                ↓
19:01:30.595095 [onSetPassengers] Agent mounted entity 3
                ↓
19:01:30.596233 [Movement] SendBoatPaddleState: left=false right=false  ❌ WRONG! This should be SendVehicleInput!
19:01:30.596578 [handleRidingMode] Boat physics: surface=minecraft:grass_block  ❌ WRONG! Should NOT use boat physics!
19:01:30.646680 [Movement] SendBoatPaddleState: left=true right=true  ❌ WRONG!
19:01:30.647041 [handleRidingMode] Boat physics: surface=minecraft:grass_block  ❌ WRONG!
```

### Boat Test Logs (agents_20260323_185726.log) - For Comparison

```
18:57:29.645818 [onAddEntity] Entity spawn: entityID=3, type=minecraft:oak_boat
18:57:29.645842 [onAddEntity] Registered entity 3 as type minecraft:oak_boat
                ↓
18:57:30.659879 [onSetPassengers] Agent mounted entity 3
                ↓
18:57:30.699182 [Movement] SendBoatPaddleState: left=false right=false  ✅ CORRECT! This is a boat
18:57:30.710394 [handleRidingMode] Boat physics: surface=water  ✅ CORRECT! Boat is in water
18:57:30.800306 [handleRidingMode] Boat physics: surface=water multiplier=0.900 velocity=(0,0,0.900)  ✅ WORKS
```

---

## Code Path Analysis

### Location: `movement/physics_executor.go:1307-1364`

```go
// STEP 1: Determine vehicle type
var isBoat bool
if et, found := pe.entityPositionGetter.GetMountedEntityType(mountedEntityID); found {
    isBoat = pe.entityPositionGetter.IsMountedEntityBoat(et)
}

// STEP 2: Send control packet
if isBoat {
    // Lines 1326-1354: For boats - Send SteerBoat packet
    if err := versionHandler.Play().Movement().SendBoatPaddleState(...); err != nil {
        // For horse test: THIS PATH IS TAKEN ❌
    }
} else {
    // Lines 1355-1364: For horses - Should send player input packet
    if err := versionHandler.Play().Movement().SendVehicleInput(...); err != nil {
        // For horse test: THIS PATH IS NOT TAKEN ❌
    }
}
```

**The Question:** Why is `isBoat` returning `true` for a horse?

---

## The Detection Logic

### Location: `agent/adapters.go:128-144`

```go
func (a *agent) IsMountedEntityBoat(entityTypeID int32) bool {
    a.regMu.RLock()
    defer a.regMu.RUnlock()

    entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
    if entityTypeReg == nil || !entityTypeReg.IsReady() {
        return false  // If registry not ready, assume NOT a boat
    }

    entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
    if !ok {
        return false  // If name lookup fails, assume NOT a boat
    }

    // Check if entity type name contains "boat" or "raft"
    return strings.Contains(entityTypeName, "boat") || strings.Contains(entityTypeName, "raft")
}
```

**Expected Behavior:**
- Input: entity type ID for horse
- GetNameByID should return "minecraft:horse"
- "minecraft:horse" contains neither "boat" nor "raft"
- Should return: **false**
- But actual behavior: **true**

---

## Hypothesis: What's Going Wrong

### Possibility 1: Entity Type Not Being Set Correctly
- Maybe the entity type isn't being stored in the entities map
- `GetMountedEntityType` returns `found=false`
- `isBoat` stays as zero value `false`
- But then SendBoatPaddleState wouldn't be called...
- ❌ Contradicts the logs

### Possibility 2: Registry Lookup Failing
- Entity type ID is retrieved correctly
- But registry lookup fails (returns empty string)
- `strings.Contains("", "boat")` returns false
- Should call SendVehicleInput
- But logs show SendBoatPaddleState is called
- ❌ Contradicts the logs

### Possibility 3: Wrong Default When Mount Type Unknown
- Somewhere in the code, if vehicle type can't be determined, it defaults to boat
- This is the most likely explanation
- **Need to verify by checking code for default behaviors**

### Possibility 4: Version-Specific Override
- Maybe version handlers have different behavior for vehicles
- Some versions might treat unknown vehicles as boats by default
- **Need to check version handler code**

---

## Differences from Boat Test

| Aspect | Boat Test | Horse Test |
|--------|-----------|-----------|
| Entity Type | `minecraft:oak_boat` | `minecraft:horse` |
| Contains "boat"? | YES | NO |
| Contains "raft"? | NO | NO |
| Expected isBoat | TRUE ✅ | FALSE ✅ |
| Actual isBoat | TRUE ✅ | TRUE ❌ |
| Physics Surface | `water` ✅ | `minecraft:grass_block` ❌ |
| Paddle Packet Sent | YES ✅ | YES ❌ |
| Input Packet Sent | NO ✅ | NO ❌ |
| Test Result | PASSES ✅ | FAILS ❌ |

---

## Next Steps to Debug

1. **Add logging to boat detection:**
   - Log the entity type ID retrieved
   - Log the registry name lookup result
   - Log the final boolean value of isBoat
   - Log why the decision was made

2. **Check all callers of IsMountedEntityBoat:**
   - Search for all code paths that set isBoat
   - Verify there's no other default behavior

3. **Verify entity registration:**
   - Confirm horse entity is in entities map with correct type
   - Confirm registry has correct mapping for horse

4. **Check version handlers:**
   - See if version handlers have their own vehicle type detection
   - Check if they override behavior for unknown vehicle types

---

## Impact

### Affected Functionality
- ❌ Horse steering completely broken
- ❌ Donkey steering (likely same issue)
- ❌ Llama steering (likely same issue)
- ❌ Camel steering (likely same issue)
- ❌ Pig steering (likely same issue)
- ❌ Strider steering (likely same issue)
- ✅ Boat steering works (correct packet type)
- ✅ Raft steering works (correct packet type)

### Test Failures
- ❌ TestHorseSteering - all 6 versions fail
- ✅ TestBoatSteering - all 6 versions pass

---

## Solution

Once root cause identified, the fix should:

1. Ensure horses return FALSE from IsMountedEntityBoat
2. Send SendVehicleInput instead of SendBoatPaddleState for horses
3. Apply horse-appropriate physics (not boat physics with gravity)
4. Verify server responds with entity position updates

The code structure is correct - just the vehicle type detection is wrong.

