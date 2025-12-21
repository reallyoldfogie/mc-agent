# Entity Soft-Delete System

## Overview

The bot implements a "soft delete" system for entity tracking to handle cases where the Minecraft server sends `ClientboundRemoveEntities` packets but continues sending position updates for those entities.

## The Problem

Minecraft servers sometimes send entity removal packets in scenarios that don't represent true entity removal:

1. **Chunk loading/unloading optimization** - Server may remove entities as chunks unload, then re-add them
2. **Entity update batching** - Server may remove and re-add entities as part of optimization
3. **Network race conditions** - Remove packets may arrive before final position updates
4. **View distance changes** - Entities at the edge of view distance may flicker in/out

If we immediately deleted entities upon receiving removal packets, the bot would:
- Lose track of players it's following
- Generate false "player disconnected" events
- Miss position updates that arrive after removal packets

## The Solution: Soft Delete with Grace Period

Instead of immediately deleting entities, we:

1. **Mark entities as removed** (`Removed = true`, `RemovedAt = timestamp`)
2. **Keep them in the tracking map** for a grace period (default: 30 seconds)
3. **Continue updating their position** if we receive movement packets
4. **Automatically un-remove them** if we receive any update packets
5. **Permanently delete them** only after the grace period expires

## Implementation Details

### TrackedEntity Structure

```go
type TrackedEntity struct {
    EntityID   int32
    EntityType int32
    UUID       [16]byte
    X, Y, Z    float64
    Yaw        int8
    Pitch      int8
    // Soft delete tracking
    Removed   bool      // True if server sent remove packet
    RemovedAt time.Time // When the remove packet was received
}
```

### Constants

```go
const (
    EntityRemovalGracePeriod = 30 * time.Second  // How long to keep removed entities
    EntityCleanupInterval    = 10 * time.Second  // How often to check for cleanup
)
```

### Packet Handler Behavior

#### ClientboundRemoveEntities
- **Old behavior**: Immediately deleted entity from map
- **New behavior**: Marks entity as removed with timestamp

```go
if entity, ok := trackedEntities.entities[int32(id)]; ok {
    entity.Removed = true
    entity.RemovedAt = time.Now()
    log.Printf("Soft-deleted entity %d (grace period: %v)", id, EntityRemovalGracePeriod)
}
```

#### Position Update Packets (MoveEntityPos, MoveEntityPosRot, TeleportEntity)
- **New behavior**: Un-removes entity if it was marked removed

```go
if entity.Removed {
    entity.Removed = false
    log.Printf("Entity %d un-removed (received position update after removal)", EntityID)
}
```

#### ClientboundAddEntity (Spawn)
- **New behavior**: Checks if entity exists and un-removes it instead of creating duplicate

```go
if existingEntity, exists := trackedEntities.entities[int32(EntityID)]; exists {
    // Update position and un-remove
    existingEntity.X = float64(X)
    existingEntity.Y = float64(Y)
    existingEntity.Z = float64(Z)
    if existingEntity.Removed {
        existingEntity.Removed = false
        log.Printf("Entity %d un-removed (respawned)", EntityID)
    }
}
```

### Background Cleanup

A goroutine runs every 10 seconds to permanently delete entities that have been marked removed for longer than the grace period:

```go
func cleanupRemovedEntities() {
    ticker := time.NewTicker(EntityCleanupInterval)
    for range ticker.C {
        trackedEntities.mu.Lock()
        now := time.Now()
        for id, entity := range trackedEntities.entities {
            if entity.Removed && now.Sub(entity.RemovedAt) > EntityRemovalGracePeriod {
                delete(trackedEntities.entities, id)
                log.Printf("Permanently removed entity %d (grace period expired)", id)
            }
        }
        trackedEntities.mu.Unlock()
    }
}
```

## Benefits

### 1. Smoother Player Following
- Bot no longer loses track of followed players when server sends spurious remove packets
- Position updates continue seamlessly during grace period
- Automatic recovery when position updates arrive

### 2. Reduced False Positives
- No more "player disconnected" messages for players that are still online
- No more "player may have moved too far away" errors for nearby players
- Following system remains stable during chunk loading/unloading

### 3. Graceful Degradation
- If entity truly is removed, it gets cleaned up after 30 seconds
- No memory leaks from accumulated removed entities
- System automatically adapts to different server behaviors

## Configuration

Grace period can be adjusted by modifying the constant:

```go
const EntityRemovalGracePeriod = 30 * time.Second  // Adjust as needed
```

**Recommendations**:
- **10-15 seconds**: For fast-paced servers with frequent entity updates
- **30 seconds** (default): Good balance for most servers
- **60+ seconds**: For servers with aggressive entity optimization or slow networks

## Monitoring

The system logs all soft-delete operations:

```
Soft-deleted entity 123 (will be permanently removed after 30s grace period)
Entity 123 un-removed (received position update after removal)
Entity 123 un-removed (respawned)
Permanently removed entity 123 (grace period expired)
```

Watch for patterns:
- **Frequent un-removals**: Server is sending spurious remove packets (system working as intended)
- **Many permanent removals**: Entities are actually being removed (normal operation)
- **No un-removals**: Server is not sending spurious removes (soft-delete overhead minimal)

## Related Files

- `main.go:134-144` - TrackedEntity struct definition
- `main.go:60-67` - Constants
- `main.go:245-269` - cleanupRemovedEntities() function
- `main.go:549-577` - handleEntityPosition (with un-remove logic)
- `main.go:579-610` - handleEntityPositionRotation (with un-remove logic)
- `main.go:612-646` - handleTeleportEntity (with un-remove logic)
- `main.go:648-665` - handleRemoveEntities (soft-delete logic)
- `main.go:510-543` - handleSpawnEntity (respawn un-remove logic)
- `main.go:410` - Cleanup goroutine startup

## Interaction with Following System

The following system (`following/target.go`) is **completely transparent** to soft-deletes:

- `getTrackedEntities()` returns all entities, including soft-deleted ones
- The returned `following.TrackedEntity` struct doesn't include the `Removed` field
- Following system continues to track players during grace period
- Position updates automatically un-remove entities before following system checks them

This design ensures the following system "just works" without needing to know about soft-deletes.
