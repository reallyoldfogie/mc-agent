# Fix: Bot Losing Track of Nearby Players

## Problem

When following a player, the bot would frequently lose track of them with the message "target no longer exists (disconnected or despawned)", even when the player was only 5 blocks away and still connected.

## Root Cause

The issue was in `following/manager.go:253-266`. When the bot reached the target (within `StopDistance` of 2 blocks):

1. The bot would enter `StateArrived`
2. It would call `LookAt()` to track the player's head
3. `LookAt()` only sends **rotation packets** (`ServerboundMovePlayerRot`), not position packets
4. The follow loop would return early without sending any position updates

**Critical Issue**: Minecraft servers expect clients to send position packets regularly (ideally every game tick, ~50ms). When a client only sends rotation packets without position updates for an extended period, the server may:

- Consider the client's state stale
- Aggressively unload entities to save resources
- Remove tracked entities from the client's view, including nearby players

This caused the server to send `ClientboundRemoveEntities` packets that removed the followed player from the `trackedEntities` map, triggering the "target no longer exists" error.

## The Fix

Modified `following/manager.go` to send **position+rotation packets** when stationary, instead of just rotation packets:

```go
// OLD CODE (incorrect):
if distance <= fm.config.StopDistance {
    if fm.state != StateArrived {
        fm.state = StateArrived
        fm.sendChatMessage(fmt.Sprintf("Arrived at %s (%.1f blocks)", fm.targetName, distance))
    }
    return nil  // Only rotation packets sent via LookAt(), no position updates!
}

// NEW CODE (correct):
if distance <= fm.config.StopDistance {
    if fm.state != StateArrived {
        fm.state = StateArrived
        fm.sendChatMessage(fmt.Sprintf("Arrived at %s (%.1f blocks)", fm.targetName, distance))
    }

    // Calculate look angles and send BOTH position and rotation
    // This keeps the server updated and prevents entity unloading
    [... angle calculation ...]
    fm.movementExecutor.SendPositionAndRotation(botX, botY, botZ, yaw, pitch, true)
    return nil
}
```

## Why This Works

By sending `ServerboundMovePlayerPosRot` packets every 50ms (matching the follow loop tick rate), the server receives regular position updates confirming:
- The bot is still active and responsive
- The bot's position is current and valid
- Entities near the bot should remain loaded

This prevents the server from considering the bot stale and aggressively unloading nearby entities.

## Testing

To verify the fix:
1. Start the bot and have it follow a nearby player
2. The bot should reach the player (within 2 blocks)
3. The bot should continue tracking the player without losing them
4. Check logs for continuous `[Movement] Sending ServerboundMovePlayerPosRot` messages even when stationary

## Additional Notes

- The follow loop runs at 50ms intervals (20 Hz), matching Minecraft's tick rate
- Position packets are sent at the same rate, ensuring the server always has fresh state
- This fix applies to all stationary scenarios, not just when following

## Complementary Fix: Soft-Delete System

In addition to sending regular position packets, we also implemented a "soft-delete" system for entity tracking (see `SOFT_DELETE_ENTITIES.md`). This handles cases where the server sends `ClientboundRemoveEntities` packets but continues sending position updates:

- Entities are marked as "removed" but kept in tracking for 30 seconds
- Position updates automatically un-remove entities
- Permanent deletion only happens after grace period

This two-pronged approach ensures robust player tracking:
1. **Regular position packets** prevent server from thinking bot is stale
2. **Soft-delete system** prevents bot from thinking entities are gone when server sends spurious remove packets

## Related Files

- `following/manager.go:253-288` - Main fix location (position+rotation when stationary)
- `movement/executor.go:82-89` - SendPositionAndRotation implementation
- `movement/packets.go:25-39` - Packet sending logic
- `SOFT_DELETE_ENTITIES.md` - Documentation for complementary soft-delete system
