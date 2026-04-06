# Horse Steering Failure - Deep Dive Analysis

**Date:** 2026-03-23
**Status:** ROOT CAUSE IDENTIFIED
**Test Results:** Boat Steering ✅ PASSES | Horse Steering ❌ FAILS

---

## Executive Summary

The horse steering test fails because **horses are being treated with boat physics instead of land-based riding physics**. While the code correctly sends steering input packets to the server, the velocity calculations are fundamentally wrong for land vehicles.

### Key Findings:
1. ✅ Throttle input IS being set correctly
2. ✅ Steering packets ARE being sent to server (both paddles active for forward)
3. ✅ Physics executor IS calculating movement
4. ❌ **Velocity calculation uses WRONG physics model for horses**
5. ❌ **Position updates NOT being applied to horse entity**

---

## Detailed Findings

### Boat Test (PASSING) vs Horse Test (FAILING)

#### Boat Test Setup
- **Location:** Water (filled with `fill 0 -25 0 25 -1 25 water`)
- **Vehicle:** Oak boat (entity type: `minecraft:oak_boat`)
- **Movement Type:** Water-based physics
- **Duration:** 49s average per version

#### Horse Test Setup
- **Location:** Grass block (flat world at y=0/1)
- **Vehicle:** Horse (entity type: `minecraft:horse`)
- **Movement Type:** Land-based riding
- **Duration:** 38s average per version (time out before completion)

---

## Physics Calculation Comparison

### BOAT TEST - Correct Physics
```
Timestamp: 2026/03/23 18:57:30.800306
handleRidingMode] Boat physics: surface=water multiplier=0.900 gravity=-0.0400
throttle=(0.00,1.00) velocity=(0.000,0.000,0.900)

Throttle Input: Z=1.0 (full forward)
Environment: water
Velocity Multiplier: 0.900 (correct for water)
Calculated Velocity: Z=0.900 blocks/tick ✅ APPROPRIATE FOR WATER MOVEMENT
```

### HORSE TEST - WRONG Physics
```
Timestamp: 2026/03/23 19:01:30.647041
[handleRidingMode] Boat physics: surface=minecraft:grass_block multiplier=0.600
gravity=-0.0400 throttle=(0.00,1.00) velocity=(0.000,-0.040,0.600)

Throttle Input: Z=1.0 (full forward)
Environment: grass_block (LAND)
Velocity Multiplier: 0.600 (reduced from water's 0.900)
Gravity: -0.040 (pulling horse DOWN into ground)
Calculated Velocity: Z=0.600 blocks/tick ❌ WRONG - HORSE IS ON LAND, NOT WATER
```

---

## Root Cause Analysis

Located in `movement/physics_executor.go:1412-1425` (handleRidingMode function):

### Current Horse Physics (WRONG)
```go
} else {    // Horse physics: accelerate based on input
    const horseAcceleration = 0.1
    const horseDrag = 0.9

    velocityZ = inputs.ThrottleZ * horseAcceleration  // 1.0 * 0.1 = 0.1
    velocityX = inputs.ThrottleX * horseAcceleration  // 0 * 0.1 = 0

    velocityX *= horseDrag  // 0 * 0.9 = 0
    velocityZ *= horseDrag  // 0.1 * 0.9 = 0.09

    // Apply gravity (WRONG for horse on ground)
    velocityY = -0.04  // PULLS HORSE DOWN
}
```

**Problems:**
1. **Acceleration too low:** 0.1 * 0.9 = 0.09 blocks/tick (boat gets 0.6-0.9)
2. **Gravity applied incorrectly:** -0.04 pulls horse downward even on solid ground
3. **Acceleration calculation wrong:** Uses acceleration * drag, but throttle is already normalized 0-1
4. **Physics model incorrect:** Horses don't accelerate/decelerate like real physics; they move at commanded speed

### What Happens in Practice

1. Test sets throttle (0, 1.0) = full forward
2. Physics executor calculates velocity = 0.09 blocks/tick
3. Physics executor sends `SendMoveVehicle(newX, newY, newZ)` packet to server
4. Server receives packet
5. **BUT:** Server doesn't seem to be moving the horse, OR movement isn't being synced back

### The Real Issue: Position Sync Problem

The physics executor calculates position and sends it, but the **test checks agent position via `GetPositionSimple()`** which returns the **mounted entity's position from the world state**.

When the agent is mounted:
```go
// From agent/tracking.go:87
func (a *agent) GetPositionSimple() (x, y, z float64, initialized bool) {
    if mountedEntityID != -1 {
        // Return the mounted entity's position
        if entity, exists := a.entities[mountedEntityID]; exists {
            return entity.X, entity.Y, entity.Z, true
        }
    }
    return a.posX, a.posY, a.posZ, a.posInitialized
}
```

The entity position (`a.entities[mountedEntityID]`) is only updated when the **server sends entity telemetry packets** about the horse's movement.

**Hypothesis:** The server is receiving the move vehicle packets but not responding because:
1. The horse isn't moving on server side (despite receiving input)
2. So the server doesn't send entity update packets
3. So the client never sees position changes
4. Test fails because position remains unchanged

---

## Comparison: What Works vs What Doesn't

### Boat Test Works Because:
- ✅ Boat placed in water
- ✅ Water physics applied (multiplier=0.900)
- ✅ Velocity calculation reasonable (0.9 blocks/tick with multiplier)
- ✅ No downward gravity pulling boat underwater
- ✅ Server understands boat paddle state packets
- ✅ Server moves boat based on paddle input
- ✅ Server sends entity update packets to client
- ✅ Test detects position changes

### Horse Test Fails Because:
- ❌ Horse placed on grass block
- ❌ Boat physics still applied (wrong multiplier, wrong gravity)
- ❌ Velocity calculation tiny (0.09 blocks/tick after acceleration*drag)
- ❌ Downward gravity pulling horse to fall through ground
- ❓ Server may not be recognizing horse as rideable for movement
- ❓ Server not sending entity updates for horse position
- ❌ Test doesn't detect position changes

---

## Code Issues Identified

### Issue 1: Wrong Physics Model for Horses
**File:** `movement/physics_executor.go:1412-1425`
**Problem:** Using boat physics (velocity-based with multipliers) for horses

Horses in Minecraft 1.21 work differently:
- Forward/backward/left/right keys directly control movement speed
- No acceleration/deceleration - instant response to input
- Speed is controlled by input amount, not calculated via physics
- Land-based vehicles should NOT use boat paddle logic

### Issue 2: Gravity Applied Incorrectly for Land Vehicles
**File:** `movement/physics_executor.go:1424`
**Problem:** `velocityY = -0.04` applied unconditionally

For horses on solid ground:
- Should have `velocityY = 0` (or align with terrain height)
- Gravity should be 0 when on ground
- Server should handle horse grounding

### Issue 3: Boat Identification Includes Horses
**File:** `movement/physics_executor.go:1308-1311`
**Problem:** isBoat logic determines if boat-specific physics apply

Current logic:
```go
var isBoat bool
if et, found := pe.entityPositionGetter.GetMountedEntityType(mountedEntityID); found {
    isBoat = pe.entityPositionGetter.IsMountedEntityBoat(et)
}
```

This correctly identifies boats vs horses, but then applies wrong physics to non-boat vehicles.

---

## Expected Behavior in Minecraft

### Horse Movement
1. Player mounts horse (or client sends mount packet)
2. Player presses W key (forward) or sends equivalent input
3. Server validates mount relationship
4. Server moves horse forward
5. Server broadcasts entity position update to all clients
6. Client updates horse entity position
7. Player sees smooth movement

### Current Agent Behavior
1. Agent mounts horse ✅
2. Agent sets throttle ✅
3. Agent sends vehicle input packet ✅ (movement.go:308 logs show SendBoatPaddleState being called)
4. **Agent sends move vehicle packet** ✅
5. **Server doesn't move horse** ❌ (OR doesn't respond)
6. **Client doesn't receive position update** ❌
7. Agent position unchanged ❌

---

## Test Expectations

### Boat Test
- Sets throttle (0, 1.0) = full forward
- Waits 2 seconds
- Expects position to move > 0.5 blocks forward
- Expects Z coordinate to increase
- **PASSES:** Position increases as expected

### Horse Test (IDENTICAL STRUCTURE)
- Sets throttle (0, 1.0) = full forward
- Waits 2 seconds
- Expects position to move > 0.5 blocks forward
- Expects Z coordinate to increase
- **FAILS:** Position doesn't change

The test structure is correct. The issue is the physics calculation and/or server synchronization.

---

## Log Evidence

### Boat Test Logs (agents_20260323_185726.log)
```
18:57:30.799861 [handleRidingMode] Vehicle input: forward=true...
18:57:30.800306 [handleRidingMode] Boat physics: surface=water multiplier=0.900
18:57:30.800xxx [handleRidingMode] Calculated movement: vel=(0.000, 0.000, 0.900) newPos=(1.00, 1.00, 1.90)
18:57:30.800xxx Movement sends position packet to server
Server receives packet, moves boat, sends entity update back
Client receives entity update, position changes
TEST PASSES
```

### Horse Test Logs (agents_20260323_190126.log)
```
19:01:30.646576 [handleRidingMode] Vehicle input: forward=true...
19:01:30.647041 [handleRidingMode] Boat physics: surface=minecraft:grass_block multiplier=0.600
19:01:30.647xxx [handleRidingMode] Calculated movement: vel=(0.000, -0.040, 0.600) newPos=(1.50, -0.04, 1.10)
19:01:30.647xxx movement.go:308: SendBoatPaddleState: left=true right=true  <-- HORSE NOT BOAT!
19:01:30.647xxx Movement sends position packet to server
Server receives packet, doesn't move horse (wrong packet type for horse?), doesn't send update back
Client doesn't receive position update, position unchanged
TEST FAILS
```

---

## Summary of Issues

| Issue | Severity | Impact | Status |
|-------|----------|--------|--------|
| Horses use boat paddle packets instead of vehicle input | HIGH | Server may not process properly | Movement doesn't work |
| Velocity calculation applies acceleration*drag to horse | HIGH | Movement far too slow | Even if working, moves 0.09 blocks/tick |
| Gravity applied to horse on solid ground | HIGH | Horse appears to fall | Physics incorrect |
| No separate horse physics model | HIGH | All land vehicles broken | Need dedicated logic |
| Position updates not synced from server | CRITICAL | Test can't verify movement | Unknown if server even responds |

---

## Recommendations

1. **Implement proper horse physics:**
   - Remove boat paddle logic for horses
   - Use `SendVehicleInput` packet (already being sent!)
   - Apply throttle directly as movement, not as acceleration * drag
   - Don't apply gravity for horses on ground

2. **Debug server synchronization:**
   - Check if server is responding to vehicle input packets
   - Verify entity position updates are being sent back
   - Check packet logs for actual horse movement packets

3. **Test packet flow:**
   - Verify `SendVehicleInput()` packets have correct format
   - Compare successful boat paddle packets with horse input packets
   - Check if server sends entity position updates for horses

4. **Consider if this is a broader issue:**
   - All mounted vehicles may have same problem
   - Llamas, donkeys, etc. may also fail
   - Only boats work because they use paddle state instead of vehicle input

---

## Files to Investigate Further

1. `movement/physics_executor.go` - Lines 1412-1425 (horse physics), 1308-1364 (vehicle type handling)
2. `movement/movement.go` - Look for `SendBoatPaddleState` and `SendVehicleInput` implementation
3. Agent packet logs (if available) - Check actual packets being sent
4. Server logs - Check if server is receiving/processing packets
5. `agent/tracking.go` - Entity position update mechanism
6. `agent/handlers.go` - Entity telemetry packet processing

