# HPA* Usage Example

## Basic Setup

```go
// Create low-level pathfinder (A* or EPEA*)
lowLevelPathfinder := NewAStarPathFinder(world, shapeMgr)

// Create HPA* pathfinder with 10x10x10 cluster size
clusterSize := 10
hpaPathfinder := NewHPAPathFinder(world, shapeMgr, lowLevelPathfinder, clusterSize)

// Use it like any PathFinder
path, err := hpaPathfinder.FindPath(start, goal, maxSteps)
```

## Handling Dynamic World Updates

### Scenario 1: Initial World Load (Many Updates)

During initial world load, you'll receive thousands of block updates. Use **batch mode** to handle them efficiently:

```go
// Create update handler
builder := // get builder from hpaPathfinder (needs accessor method)
updateHandler := NewWorldUpdateHandler(builder)

// Enable batch mode BEFORE receiving world updates
updateHandler.EnableBatchMode()

// As blocks arrive from server:
for _, blockUpdate := range worldLoadUpdates {
    pos := models.V3{X: blockUpdate.X, Y: blockUpdate.Y, Z: blockUpdate.Z}
    updateHandler.OnBlockChange(pos)
}

// When world load is complete, disable batch mode
updateHandler.DisableBatchMode()
```

**What happens:**
- Cluster invalidations are accumulated in memory (no rebuilding)
- Batch is flushed every 1 second OR after 100 clusters (configurable)
- Dirty clusters are only rebuilt when pathfinding actually needs them
- Result: No performance impact during initial load

### Scenario 2: Runtime Block Changes (Player Mining/Building)

During normal gameplay, handle updates immediately:

```go
// Batch mode is disabled by default
// Just notify the handler of changes

// Single block change
updateHandler.OnBlockChange(blockPos)

// Multiple blocks at once (more efficient)
blockPositions := []models.V3{pos1, pos2, pos3}
updateHandler.OnMultipleBlockChanges(blockPositions)
```

**What happens:**
- Clusters are marked dirty immediately
- Abstract graph edges are invalidated
- Rebuilding is still deferred until pathfinding needs the cluster
- Low overhead: just marking flags, no pathfinding computation

### Scenario 3: Neighbor Invalidation

Block changes near cluster boundaries affect multiple clusters:

```go
// Block at position (9, 5, 5) in cluster (0,0,0)
// This is on the east boundary (cluster ends at X=10)
pos := models.V3{X: 9, Y: 5, Z: 5}

updateHandler.OnBlockChange(pos)
// Automatically invalidates both clusters (0,0,0) and (1,0,0)
```

**What happens:**
- `getAffectedClusters()` detects boundary positions
- All adjacent clusters on affected boundaries are invalidated
- Ensures entrances between clusters stay valid

## Performance Characteristics

### Initial Load (10,000 block updates)

**Without batching:**
- 10,000 cluster lookups
- Potentially 10,000+ cluster invalidations
- Risk of repeated invalidation of same clusters

**With batching:**
- 10,000 cluster lookups (still fast - just map lookups)
- Accumulates unique cluster IDs (maybe 100-500 actual clusters)
- Single flush operation marking clusters dirty
- Zero rebuilding until pathfinding is needed

### Runtime Updates

**Per block change:**
- 1-7 cluster invalidations (1 main + up to 6 neighbors if on corner/edge)
- ~1-10 microseconds per invalidation (just setting dirty flag)
- Zero pathfinding computation

**Per pathfinding request:**
- Clusters needed for path are rebuilt on-demand
- Rebuilding: ~10-50ms per cluster (depends on entrances)
- Only happens for dirty clusters in the path

## Example: Agent Lifecycle

```go
// At bot startup
func (bot *Bot) Initialize() {
    // Create pathfinder
    bot.hpa = NewHPAPathFinder(bot.world, bot.shapeMgr, bot.lowLevelPF, 10)
    bot.updateHandler = NewWorldUpdateHandler(bot.hpa.GetBuilder())
    
    // Enable batch mode for world load
    bot.updateHandler.EnableBatchMode()
}

// As world chunks are loaded
func (bot *Bot) OnChunkLoaded(chunk *Chunk) {
    for _, block := range chunk.GetBlocks() {
        pos := models.V3{X: block.X, Y: block.Y, Z: block.Z}
        bot.updateHandler.OnBlockChange(pos)
    }
}

// When world load completes
func (bot *Bot) OnWorldLoadComplete() {
    bot.updateHandler.DisableBatchMode()
    log.Printf("World load complete, ready for pathfinding")
}

// When player mines/places blocks
func (bot *Bot) OnBlockUpdate(blockPos models.V3) {
    bot.updateHandler.OnBlockChange(blockPos)
}

// When bot needs to path somewhere
func (bot *Bot) MoveTo(target models.V3) {
    start := bot.GetPosition()
    
    // HPA* automatically rebuilds any dirty clusters needed for this path
    path, err := bot.hpa.FindPath(start, target, 0)
    if err != nil {
        log.Printf("Pathfinding failed: %v", err)
        return
    }
    
    bot.FollowPath(path)
}
```

## Monitoring and Debugging

```go
// Get update statistics
stats := updateHandler.GetStats()
fmt.Printf("Batch mode: %v\n", stats["batch_mode"])
fmt.Printf("Pending updates: %d\n", stats["pending_batch_size"])
fmt.Printf("Time since flush: %.2fs\n", stats["time_since_flush"])

// Manual flush if needed
updateHandler.FlushBatch()
```

## Configuration Tuning

```go
// Adjust batch parameters
updateHandler.batchInterval = 2 * time.Second  // Flush less frequently
updateHandler.maxBatchSize = 500               // Accumulate more updates

// Adjust cluster size (larger = less preprocessing, slower search)
clusterSize := 20  // Bigger clusters for huge worlds
```

## Memory Management (Future)

```go
// Not yet implemented, but planned:
updateHandler.ClearOldClusters(maxClusters)  // LRU eviction
```

## Summary

### Key Benefits of This Approach:

1. **Zero impact during initial load**: All updates are batched, no rebuilding
2. **Fast runtime updates**: Just marks flags (~microseconds)
3. **Lazy rebuilding**: Only rebuild clusters actually needed for pathfinding
4. **Neighbor awareness**: Automatically handles boundary cases
5. **Thread-safe**: Uses mutexes for concurrent updates
6. **Configurable**: Tune batch size and interval for your use case

### When to Use What:

- **Batch mode**: Initial world load, chunk loading, large terrain generation
- **Immediate mode**: Player mining/building, entity interactions, single block changes
- **Manual flush**: When you know batch mode should end (world load complete, chunk loaded)
