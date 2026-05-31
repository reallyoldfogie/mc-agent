# Camel Pose Data Initialization Analysis

## Problem Summary

**TestCamelSittingDetectionUnmounted** fails at line 544-545:
```go
assert.True(t, camelInfo.HasPose, "camel should have pose data (pre-data merge)")
assert.Equal(t, "standing", camelInfo.PoseName, "newly summoned camel should be standing")
```

Expected: `HasPose=true`, `PoseName="standing"`
Actual: `HasPose=false`, `PoseName=""`

The camel entity IS tracked (found in GetTrackedEntities), but pose data is not initialized.

## Root Cause: Race Condition in Metadata Packet Delivery

### How Pose Data Is Set

**Entity Creation Flow:**
1. Server sends `AddEntity` packet → `onAddEntity()` creates trackedEntity with:
   - `Pose=0`, `PoseName=""`, `HasPose=false` (all zeroed)
2. Server sends `SetEntityMetadata` packet → `onSetEntityMetadata()` updates:
   - `e.Pose = poseOrdinal`
   - `e.PoseName = poseName`
   - `e.HasPose = true`

**Key Code in `agent/handlers.go:onSetEntityMetadata()`:**
```go
if hasPose {
    e.Pose = poseOrdinal
    e.PoseName = poseName
    e.HasPose = true
}
```

The pose data is ONLY set when a `SetEntityMetadata` packet containing the pose metadata entry is received.

### Why TestCamelSittingDetectionUnmounted Fails

**Timeline:**
```
T=0.00s: Summon camel via RCON
T=0.01s: Server sends AddEntity packet
T=0.05s: Test waits 3 seconds (sleep 3s)
T=3.00s: Test checks GetTrackedEntities() → HasPose is still FALSE
         (SetEntityMetadata packet may not have arrived yet)
```

**Why it's a race condition:**
- The test waits 3 seconds after summoning
- But 3 seconds is NOT guaranteed to include the metadata packet delivery
- The metadata packet may be:
  - Bundled with other packets in a delayed batch
  - Lost/retransmitted (rare but possible)
  - Simply arriving later due to network/processing delays

### Why TestCamelSittingDetection Passes (Comparison)

**Timeline:**
```
T=0.00s: Summon camel via RCON
T=0.01s: Server sends AddEntity packet
T=0.05s: Test waits 3 seconds (sleep 3s)
T=3.00s: Test sends RCON: data merge entity @e[type=minecraft:camel,limit=1] {LastPoseTick:-439L}
T=3.50s: Server sends SetEntityMetadata packet with updated pose (from the data merge)
T=3.55s: Test mounts camel
T=3.60s: Test checks pose via RidingPhysicsInspector
         (SetEntityMetadata has definitely arrived by now)
```

**Why it passes:**
1. The data merge command FORCES a metadata update from the server
2. By the time the test mounts and checks the pose, enough time has passed for the metadata to arrive
3. The mount operation itself may trigger additional metadata updates
4. The inspector check happens AFTER mount (line 653-657), when pose data is guaranteed to exist

## Key Difference: When Pose Is Checked

| Test | Action at T=3s | When Checked | Status |
|------|---|---|---|
| Unmounted | Check pose immediately | Before any operations | ❌ FAILS - metadata not arrived |
| Sitting   | Send data merge RCON | After mount + inspector | ✅ PASSES - metadata guaranteed |

## Evidence from Code

**TestCamelSittingDetectionUnmounted (line 519-600):**
```go
// Line 538: Wait 3 seconds
time.Sleep(3 * time.Second)

// Line 541-546: Check pose IMMEDIATELY - THIS IS WHERE IT FAILS
entities := helper.ManagedAgent.Agent.GetTrackedEntities()
camelInfo, found := entities[camelEntityID]
require.True(t, found, "camel should be tracked")
assert.True(t, camelInfo.HasPose, "camel should have pose data (pre-data merge)")  // ← FAILS
assert.Equal(t, "standing", camelInfo.PoseName, "newly summoned camel should be standing")
```

**TestCamelSittingDetection (line 604-708):**
```go
// Line 623: Wait 3 seconds
time.Sleep(3 * time.Second)

// Line 638: Send RCON command to force metadata update
sitStartTick := -worldAge
dataCmd := fmt.Sprintf("data merge entity @e[type=minecraft:camel,limit=1] {LastPoseTick:%dL}", sitStartTick)
resp, err := helper.Instance.RCON.Exec(ctx, dataCmd)

// Line 643: Wait 3 more seconds
time.Sleep(3 * time.Second)

// Line 646: Mount the camel
err = helper.MountEntity(ctx, camelEntityID)
require.NoError(t, err, "mount camel")

// Line 653-657: Check pose AFTER mounting - pose data guaranteed to exist by now
camelState, hasCamel := inspector.GetCamelState()
require.True(t, hasCamel && camelState != nil, "should be mounted on a camel")
```

## Solutions

### Option 1: Wait for Pose Data (Recommended for This Test)
Add a polling loop before checking pose:

```go
// Unmounted test: wait for metadata arrival
deadline := time.Now().Add(10 * time.Second)
for {
    entities := helper.ManagedAgent.Agent.GetTrackedEntities()
    if camelInfo, ok := entities[camelEntityID]; ok && camelInfo.HasPose {
        assert.Equal(t, "standing", camelInfo.PoseName)
        break
    }
    if time.Now().After(deadline) {
        t.Fatal("camel pose metadata never arrived after 10 seconds")
    }
    time.Sleep(100 * time.Millisecond)
}
```

### Option 2: Force Metadata Update via RCON (Like Sitting Test)
Send a data merge immediately to force server to send metadata:

```go
world := helper.ManagedAgent.Agent.GetWorld()
worldAge, hasWorldAge := world.GetWorldAge()
require.True(t, hasWorldAge, "world age should be available")

sitStartTick := -worldAge
dataCmd := fmt.Sprintf("data merge entity @e[type=minecraft:camel,limit=1] {LastPoseTick:%dL}", sitStartTick)
resp, err := helper.Instance.RCON.Exec(ctx, dataCmd)
require.NoError(t, err, "data merge")

time.Sleep(1 * time.Second)  // Wait for metadata packet

// Now pose data is guaranteed to be set
entities := helper.ManagedAgent.Agent.GetTrackedEntities()
camelInfo, found := entities[camelEntityID]
assert.True(t, found)
assert.True(t, camelInfo.HasPose)
assert.Equal(t, "standing", camelInfo.PoseName)
```

### Option 3: Mount Before Checking (Integration Test Style)
Mount the camel, which triggers full metadata exchange:

```go
// Mount the camel
err = helper.MountEntity(ctx, camelEntityID)
require.NoError(t, err, "mount camel")

// Now check via inspector (guaranteed to have pose)
inspector, ok := helper.GetRidingPhysicsInspector()
camelState, hasCamel := inspector.GetCamelState()
require.True(t, camelState != nil)
require.False(t, camelState.IsSitting())  // Check via inspector instead
```

## Recommended Fix

**Use Option 1** (polling for metadata) because:
- The test name says "without mounting" (Unmounted) - so mounting defeats the purpose
- It directly tests the entity tracking system's pose handling
- It makes the flakiness explicit and timeout-bounded
- It's generic (works for all entity types needing metadata)

The test should be:
```go
// Wait for metadata to arrive before checking
helper.WaitForEntityMetadata(ctx, camelEntityID, 10*time.Second, func(info models.TrackedEntityInfo) bool {
    return info.HasPose
})

// Now check pose
entities := helper.ManagedAgent.Agent.GetTrackedEntities()
camelInfo := entities[camelEntityID]
assert.True(t, camelInfo.HasPose)
assert.Equal(t, "standing", camelInfo.PoseName)
```

Or implement a helper method in `common_test.go`:
```go
func (vh *VehicleTestHelper) WaitForEntityPose(ctx context.Context, entityID int32, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    ticker := time.NewTicker(50 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if ent := vh.GetTrackedEntity(entityID); ent != nil && ent.HasPose {
                return nil
            }
            if time.Now().After(deadline) {
                return fmt.Errorf("entity %d pose metadata not received after %v", entityID, timeout)
            }
        }
    }
}
```

Then use it:
```go
err := helper.WaitForEntityPose(ctx, camelEntityID, 10*time.Second)
require.NoError(t, err)

entities := helper.ManagedAgent.Agent.GetTrackedEntities()
camelInfo := entities[camelEntityID]
assert.Equal(t, "standing", camelInfo.PoseName)
```
