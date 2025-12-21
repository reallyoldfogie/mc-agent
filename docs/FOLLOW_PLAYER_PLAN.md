# Follow Player Implementation Plan

## Overview

This document outlines the plan for implementing a player-following feature for the mc-agent Minecraft bot. The bot will be able to track a target player's position and navigate to maintain proximity using pathfinding and movement systems.

## Goals

1. **Primary Goal**: Enable the bot to follow a specified player at a configurable distance
2. **Secondary Goals**:
   - Maintain smooth, natural-looking movement
   - Handle obstacles and terrain intelligently
   - Recover gracefully from stuck states
   - Allow dynamic target switching
   - Provide status feedback via chat

## Current State Analysis

### Existing Infrastructure ✅

1. **Position Tracking** (✅ Recently Fixed)
   - Bot position is now correctly tracked via `ClientboundPosition` packet handler (main.go:468-519)
   - Player entity tracking via `ClientboundAddEntity`, movement packets, and teleportation (main.go:309-466)
   - Nearest player calculation implemented (main.go:1352-1408)

2. **Distance Calculation** (✅ Working)
   - 3D Euclidean distance calculation in `findNearestPlayer()` (main.go:1389-1392)
   - Real-time position updates from server

3. **Look-At System** (✅ Working)
   - `DoLookAt()` function calculates yaw/pitch to look at coordinates (main.go:1343-1367)
   - Accounts for eye-level positioning

4. **Movement Packet Sending** (✅ Partially Implemented)
   - `DoPlayerAction()` for player actions (main.go:1285-1294)
   - Rotation packets via `ServerboundMovePlayerRot` (main.go:1365)

5. **World Manager** (✅ Available)
   - Chunk loading/unloading tracked (main.go:211-214, 682-693)
   - Block data via `blockMgr` (main.go:75-77, 180)

### Existing Code References

1. **Pathfinding Framework** (⚠️ Commented Out)
   - Location: `bot/path/` directory
   - Files:
     - `path.go` - A* pathfinding implementation (commented out)
     - `movement.go` - Movement primitives (traverse, jump, drop, ascend/descend, etc.) (commented out)
     - `blocks.go` - Block collision detection
     - `inputs.go` - Player input generation
   - Status: Infrastructure exists but is currently disabled

2. **Legacy Implementation** (📚 Reference)
   - Location: `old/spbbot/activity/followPlayer.go`
   - Key concepts to port:
     - Path recalculation loop
     - Step-by-step path execution
     - Stuck detection and recovery
     - Target proximity handling

### Missing Components ❌

1. **Movement Packet System**
   - Need to send position update packets to server
   - Packet types needed:
     - `ServerboundMovePlayerPos` - Position only
     - `ServerboundMovePlayerPosRot` - Position + rotation
     - `ServerboundMovePlayerRot` - Rotation only

2. **Pathfinding System**
   - Need to uncomment/update existing A* implementation
   - Need to port movement primitives to work with current version
   - Need block collision detection for current protocol version

3. **Physics Simulation** 
   - Client-side physics for smooth movement
   - Velocity tracking
   - Collision detection

4. **Activity/State Management**
   - Following state (idle, calculating path, following, stuck)
   - Command system integration
   - Stop/start/pause controls

## Architecture Design

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Follow Player System                     │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
┌──────────────┐    ┌──────────────┐      ┌──────────────┐
│   Target     │    │  Pathfinding │      │   Movement   │
│  Selection   │    │    Engine    │      │   Executor   │
└──────────────┘    └──────────────┘      └──────────────┘
        │                     │                     │
        │                     │                     │
        ▼                     ▼                     ▼
┌──────────────────────────────────────────────────────────┐
│              Shared State & Position Data                │
│  • Bot Position (botPosition)                            │
│  • Tracked Entities (trackedEntities)                    │
│  • World Data (worldManager)                             │
│  • Player List (playerList)                              │
└──────────────────────────────────────────────────────────┘
```

### Core Components

#### 1. Follow Manager (`FollowManager`)

**Responsibilities:**
- Maintain following state (active/inactive)
- Target player selection and tracking
- Coordinate pathfinding and movement
- Handle stop/start commands
- Status reporting

**State Machine:**
```
┌──────────┐
│   Idle   │ ◄──────────────────────┐
└────┬─────┘                        │
     │ startFollowing(target)       │
     ▼                              │
┌──────────────┐                    │
│  Targeting   │                    │
└────┬─────────┘                    │
     │ target found                 │
     ▼                              │
┌──────────────┐                    │
│ Calculating  │ ◄──────────┐       │
│     Path     │            │       │
└────┬─────────┘            │       │
     │ path ready           │       │
     ▼                      │       │
┌──────────────┐            │       │
│  Following   │ ───────────┘       │
│     Path     │  recalculate       │
└────┬─────────┘                    │
     │ arrived / lost / stop        │
     └──────────────────────────────┘
```

**Interface:**
```go
type FollowManager interface {
   Start(targetPlayerName string) error
   Stop() error
   IsActive() bool
   GetState() FollowState
   Update() error // Called periodically
}

func NewFollowManager() FollowManager{...}

type followManager struct {
    mu              sync.RWMutex
    active          bool
    targetEntityID  int32
    currentPath     []PathStep
    pathIndex       int
    lastPathUpdate  time.Time
    state           FollowState
    stopChan        chan struct{}
}

func (fm *followManager) Start(targetPlayerName string) error
func (fm *followManager) Stop() error
func (fm *followManager) IsActive() bool
func (fm *followManager) GetState() FollowState
func (fm *followManager) Update() error // Called periodically
```

#### 2. Pathfinding Engine

**Responsibilities:**
- Calculate path from bot to target
- Use A* algorithm with Minecraft-specific movement costs
- Handle terrain obstacles (blocks, water, lava, air gaps)
- Generate movement primitives

**Key Types:**
```go
type PathFinder interface {
   FindPath(start, end V3, maxSteps int) ([]PathStep, error)
   IsPathable(pos V3) bool
}

func NewPathFinder(world *world.World, blockMgr mc_versions.BlockMgr) PathFinder{...}

type pathFinder struct {
    world *world.World
    blockMgr mc_versions.BlockMgr
}

type PathStep struct {
    Position  V3            // Target position
    Movement  MovementType  // How to get there
    Cost      float64       // Movement cost
}

type MovementType int
const (
    Traverse      MovementType = iota  // Walk on same level
    Ascend                              // Jump up one block
    Descend                             // Drop down
    Jump2                               // Jump across gap
    // ... more movement types
)

func (pf *PathFinder) FindPath(start, end V3, maxSteps int) ([]PathStep, error)
func (pf *PathFinder) IsPathable(pos V3) bool
```

**Algorithm:**
- Use A* pathfinding (existing in `bot/path/path.go`)
- Heuristic: 3D Euclidean distance with Y-axis weight adjustment
- Cost function: Movement type base cost + terrain penalties
- Maximum search depth: Configurable (e.g., 100 blocks)

#### 3. Movement Executor

**Responsibilities:**
- Execute path steps by sending movement packets
- Smooth interpolation between positions
- Handle sprint/sneak states
- Collision detection and response

**Key Functions:**
```go
type MovementExecutor interface {
   ExecuteStep(step PathStep) error
   SendPosition(x, y, z float64, onGround bool) error
   SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error
   SendRotation(yaw, pitch float32, onGround bool) error
}

func NewMovementExecutor(client *bot.Client, packetMgr protocol_models.PacketMgr) MovementExecutor {...}

type movementExecutor struct {
    client    *bot.Client
    packetMgr protocol_models.PacketMgr
}

func (me *movementExecutor) ExecuteStep(step PathStep) error
func (me *movementExecutor) SendPosition(x, y, z float64, onGround bool) error
func (me *movementExecutor) SendPositionAndRotation(x, y, z float64, yaw, pitch float32, onGround bool) error
func (me *movementExecutor) SendRotation(yaw, pitch float32, onGround bool) error
```

**Movement Packets:**
```go
// ServerboundMovePlayerPos - Position only (ID varies by version)
// Fields: X (Double), Y (Double), Z (Double), OnGround (Boolean)

// ServerboundMovePlayerPosRot - Position + Rotation
// Fields: X (Double), Y (Double), Z (Double), Yaw (Float), Pitch (Float), OnGround (Boolean)

// ServerboundMovePlayerRot - Rotation only (already used in DoLookAt)
// Fields: Yaw (Float), Pitch (Float), OnGround (Boolean)
```

#### 4. Target Selection

**Responsibilities:**
- Find target player by name or UUID
- Validate target exists and is in range
- Handle target disconnection

**Key Functions:**
```go
func FindPlayerByName(name string) (*TrackedEntity, error)
func FindPlayerByUUID(uuid [16]byte) (*TrackedEntity, error)
func IsPlayerInRange(entityID int32, maxDist float64) bool
```

### Data Flow

```
1. User Command: "startFollowing ReallyOldFogie"
   │
   ▼
2. FollowManager.Start("ReallyOldFogie")
   │
   ├─▶ Find player entity in trackedEntities
   │   │
   │   ▼
   ├─▶ Validate player exists and position is known
   │   │
   │   ▼
3. Start follow loop (goroutine)
   │
   ▼
4. Calculate path to target
   │   PathFinder.FindPath(botPos, targetPos)
   │   │
   │   ├─▶ A* search with movement primitives
   │   │
   │   ▼
5. Execute path steps
   │   for each step in path:
   │   │
   │   ├─▶ Look at next position (DoLookAt)
   │   │
   │   ├─▶ Execute movement (MovementExecutor)
   │   │
   │   ├─▶ Wait for position update from server
   │   │
   │   ▼
6. Check if recalculation needed
   │   │
   │   ├─▶ Target moved too far? → Recalculate
   │   ├─▶ Path exhausted? → Recalculate
   │   ├─▶ Stuck detected? → Recalculate with alternate route
   │   │
   │   ▼
7. Repeat from step 4 until stopped
```

## Implementation Plan

### Phase 1: Core Movement System (Week 1)

**Tasks:**
1. ✅ Fix position tracking (COMPLETED - see main.go:468-519)
2. Implement movement packet sending
   - `SendPosition()` function
   - `SendPositionAndRotation()` function
   - Test with simple movements (forward, backward, strafe)
3. Create `MovementExecutor` struct
4. Test basic movement commands

**Deliverables:**
- Bot can move in response to commands (e.g., "moveForward 5")
- Position updates are reflected in bot tracking
- Movement is smooth and server-validated

**Testing:**
- Send bot to specific coordinates
- Verify server accepts movements
- Check position synchronization

### Phase 2: Basic Pathfinding (Week 2)

**Tasks:**
1. Add `mc-data-gen/loader` dependency to `go.mod`
2. **⚠️ Regenerate mc-data-gen data files** (Required - see MC_DATA_GEN_UPDATES_IMPACT.md)
   ```bash
   cd /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-data-gen
   go run ./cmd/mc-data-gen -config mc-data-gen.yaml -work-dir ./work
   ```
3. Create `BlockShapeManager` wrapper for mc-data-gen collision data with enhanced methods:
   - Core: `IsPassable()`, `IsSolid()`, `GetCollisionBoxes()`
   - Movement: `IsClimbable()` (simplified API)
   - Fluids: `IsFluid()`, `IsWater()`, `IsLava()`
   - Danger: `IsDangerous()` (detects lava, expandable)
   - Block types: `IsDoorLike()` (doors/trapdoors/fence gates), `IsFenceLike()` (1.5 block barriers), `IsSlab()`, `IsStair()`, `IsLogOrLeaf()`
4. Uncomment and update `bot/path/path.go` A* implementation
5. Replace `bot/path/blocks.go` hardcoded lists with mc-data-gen queries
6. Implement basic movement primitives using shape data:
   - Traverse (walk on same level) - use `IsPassable()` and `IsSolid()`
   - Ascend (jump up one block) - check standing surface height
   - Descend (fall down 1-2 blocks) - validate landing surface
7. Add danger avoidance in pathfinding cost calculation (lava detection)
8. Integrate with world manager for block data
9. Test pathfinding to static coordinates on multiple versions (1.21.1, 1.21.5)

**Deliverables:**
- Pathfinding works for simple terrain (flat, stairs, small drops)
- Bot can navigate around obstacles
- Path visualization (chat messages or debug logs)

**Testing:**
- Navigate to waypoint 10 blocks away on flat ground
- Navigate over 1-block walls
- Navigate down small hills

### Phase 3: Follow Manager & Target Selection (Week 3)

**Tasks:**
1. Create `FollowManager` struct with state machine
2. Implement target selection:
   - Find player by name
   - Validate target position
3. Implement follow loop with path recalculation
4. Add distance threshold logic (stop when within N blocks)
5. Add chat commands:
   - `startFollowing <player>`
   - `stopFollowing`
   - `followStatus`

**Deliverables:**
- Bot can follow a player on flat terrain
- Path recalculates when target moves
- Bot stops when close enough to target
- Chat feedback for state changes

**Testing:**
- Follow player walking straight
- Follow player making turns
- Verify bot stops at correct distance
- Test with multiple players

### Phase 4: Advanced Movement (Week 4)

**Tasks:**
1. Add diagonal movement
2. Add jump-across-gap (2-block jumps)
3. Add ladder climbing support (using `IsClimbable()`)
4. **Add swimming/water navigation** (using `IsWater()`)
5. **Add lava avoidance** (using `IsLava()`)
6. **Add door/gate navigation** (using `IsDoorLike()` - includes fence gates)
7. **Add fence detection** (using `IsFenceLike()` - treat as barriers requiring pathfinding around)
8. Improve movement smoothness with interpolation
9. Add precise collision for slabs/stairs (using `IsSlab()`, `IsStair()`)

**Deliverables:**
- Bot can follow through complex terrain
- Handles water, ladders, gaps, doors/gates
- Paths around fences (1.5 blocks tall - not jumpable)
- Avoids dangerous blocks (lava)
- Precise collision detection for complex blocks
- Movement looks more natural

**Testing:**
- Follow player across ravines
- Follow up ladders
- Follow through water
- Avoid lava pools
- Navigate through doors and fence gates
- Path around fences (not jumpable - 1.5 blocks tall)
- Traverse slabs and stairs accurately

### Phase 5: Robustness & Edge Cases (Week 5)

**Tasks:**
1. Stuck detection:
   - Position not changing after N seconds
   - Path recalculation attempts exhausted
2. Recovery strategies:
   - Jump in place
   - Try alternate nearby destinations
   - Stop following with error message
3. Target validation:
   - Handle target disconnect
   - Handle target teleport
   - Handle target entering unloaded chunks
4. Performance optimization:
   - Limit pathfinding search depth
   - Cache partial paths
   - Rate-limit path recalculation

**Deliverables:**
- Bot handles stuck states gracefully
- Bot recovers from target disconnection
- No excessive CPU usage from pathfinding

**Testing:**
- Block bot's path and verify recovery
- Disconnect target player mid-follow
- Follow target into unloaded territory

### Phase 6: Polish & Advanced Features (Week 6)

**Tasks:**
1. Configurable follow distance
2. Sprint when far from target
3. Sneak when very close
4. Face target while stationary
5. Advanced movement:
   - Vine climbing
   - Boat usage (stretch goal)
   - Elytra flying (stretch goal)
6. Multi-target support (follow nearest of group)

**Deliverables:**
- Feature-complete following system
- Natural-looking behavior
- Configurable parameters

**Testing:**
- Test all movement types in realistic scenarios
- Performance testing (following for extended periods)
- Edge case validation

## Technical Considerations

### Performance

1. **Pathfinding Cost**
   - Limit A* search depth (max 100-200 blocks)
   - Use early termination when path found
   - Cache path segments when target moves slightly
   - Rate limit: Only recalculate every 1-2 seconds unless target moves significantly

2. **CPU Usage**
   - Run pathfinding in separate goroutine
   - Use channels for async path results
   - Avoid blocking main game loop

3. **Memory**
   - Limit path storage (e.g., next 20 steps only)
   - Clean up old path data
   - Reuse pathfinding node pool if possible

### Minecraft Protocol Considerations

1. **Server-Side Validation**
   - Server validates all movement (max speed, collision, etc.)
   - Invalid movements may cause teleport-backs
   - Need to respect server tick rate (20 TPS)

2. **Movement Speed Limits**
   - Walking: ~4.3 m/s
   - Sprinting: ~5.6 m/s
   - Jumping: Initial velocity ~0.42 m/tick
   - Must not exceed or server will rubber-band

3. **OnGround Flag**
   - Critical for jump mechanics
   - Server checks this for validation
   - Must be accurate or movement rejected

### Error Handling

1. **Path Not Found**
   - Log reason (target too far, terrain impassable)
   - Send chat message to user
   - Retry with relaxed constraints or stop following

2. **Movement Rejected by Server**
   - Detect via position correction packets
   - Reset to server position
   - Recalculate path from corrected position

3. **Target Lost**
   - Player entity removed from tracking
   - Player disconnected
   - Player entered unloaded chunks
   - Action: Stop following, send notification

### Security & Safety

1. **Prevent Malicious Commands**
   - Validate target player exists
   - Check distance limits (don't follow across 1000 blocks)
   - Respect server boundaries (world border)

2. **Avoid Dangerous Terrain**
   - Detect lava, void, fire
   - Add high cost penalties in pathfinding
   - Refuse paths through certain blocks

## Proposed File Structure

```
mc-agent/
├── main.go                          # Main bot file (existing)
├── FOLLOW_PLAYER_PLAN.md           # This document
│
├── following/                       # New package for following system
│   ├── manager.go                   # FollowManager implementation
│   ├── target.go                    # Target selection/validation
│   └── state.go                     # State machine definitions
│
├── pathfinding/                     # New package (or update bot/path/)
│   ├── astar.go                     # A* algorithm
│   ├── movement.go                  # Movement primitives
│   ├── cost.go                      # Cost calculations
│   └── blocks.go                    # Block collision detection
│
└── movement/                        # New package for movement execution
    ├── executor.go                  # MovementExecutor
    ├── packets.go                   # Movement packet helpers
    └── physics.go                   # (Optional) Client-side physics
```

## Command Interface

### Chat Commands

The following chat commands will be added to `handleChatCommand()` in main.go:

```go
case "startTracking":
    // Existing tracking command (looks at nearest player)
    go startTracking()

case "stopTracking":
    // Existing tracking stop
    stopTracking()

case "startFollowing":
    // New: Start following nearest player
    go startFollowing("")

case "follow":
    // New: Start following specific player
    // Usage: >>>ROF_bot<<< follow ReallyOldFogie
    if len(parts) > 1 {
        playerName := parts[1]
        go startFollowing(playerName)
    } else {
        chatHandler.SendMessage("Usage: follow <playername>")
    }

case "stopFollowing":
    // New: Stop following
    stopFollowing()

case "followStatus":
    // New: Report following status
    reportFollowStatus()
```

### Example Usage

```
Player: >>>ROF_bot<<< follow ReallyOldFogie
Bot:    Started following ReallyOldFogie (24.5 blocks away)

Player: >>>ROF_bot<<< followStatus
Bot:    Following ReallyOldFogie | Distance: 3.2 blocks | State: Following Path

Player: >>>ROF_bot<<< stopFollowing
Bot:    Stopped following ReallyOldFogie
```

## Testing Strategy

### Unit Tests

1. **Pathfinding**
   - Test A* with known start/end points
   - Verify path cost calculations
   - Test movement primitive validation

2. **Movement**
   - Test packet generation
   - Verify coordinate calculations
   - Test rotation math

3. **Target Selection**
   - Test player lookup by name
   - Test distance calculations
   - Test entity validation

### Integration Tests

1. **End-to-End Following**
   - Bot follows stationary player
   - Bot follows moving player
   - Bot stops at correct distance

2. **Terrain Navigation**
   - Navigate flat terrain
   - Navigate stairs/hills
   - Navigate around obstacles

3. **Edge Cases**
   - Target disconnects
   - Bot gets stuck
   - Target teleports

### Manual Testing Checklist

- [ ] Bot follows player on flat ground
- [ ] Bot follows player up stairs
- [ ] Bot follows player down hills
- [ ] Bot follows player around corners
- [ ] Bot stops when close enough
- [ ] Bot handles target disconnect gracefully
- [ ] Bot recovers from stuck states
- [ ] Bot respects distance threshold
- [ ] Bot provides chat feedback
- [ ] Performance is acceptable (no lag)

## Potential Issues & Mitigations

### Issue 1: Pathfinding Too Slow
**Symptom:** Bot lags behind target, path calculation takes too long
**Mitigation:**
- Reduce search depth
- Use simpler heuristic
- Cache partial paths
- Consider simpler "direct pursuit" mode for open terrain

### Issue 2: Server Rejects Movements
**Symptom:** Bot rubber-bands, position corrections from server
**Mitigation:**
- Respect movement speed limits
- Validate OnGround flag
- Add slight delays between movements
- Implement client-side physics prediction

### Issue 3: Bot Gets Stuck
**Symptom:** Bot stops moving, repeats same failed movement
**Mitigation:**
- Implement stuck detection (position unchanged for N ticks)
- Add recovery behaviors (jump, back up, try alternate path)
- Timeout and stop following if unrecoverable

### Issue 4: High CPU Usage
**Symptom:** Pathfinding consumes excessive CPU
**Mitigation:**
- Run pathfinding in goroutine with rate limiting
- Use iterative deepening A*
- Implement path caching
- Add early termination conditions

### Issue 5: Target in Unloaded Chunks
**Symptom:** Bot can't find path, world data unavailable
**Mitigation:**
- Check if target position is in loaded chunks
- Stop following with message if target too far
- Wait for chunks to load before recalculating

## Future Enhancements

1. **Formation Following**
   - Multiple bots maintain formation around target
   - Support for "escort" patterns

2. **Patrol Mode**
   - Follow waypoint circuit
   - Return to starting point

3. **Smart Following**
   - Predict target movement
   - Take shortcuts when possible
   - Use teleports/portals if available

4. **Follow Multiple Targets**
   - Follow nearest of group
   - Dynamically switch between targets

5. **Advanced Terrain**
   - Parkour movements (complex jumps)
   - Scaffolding (place blocks to reach target)
   - Break blocks to clear path

## References

- **Legacy implementation:** `old/spbbot/activity/followPlayer.go`
- **Pathfinding framework:** `bot/path/`
- **Position tracking:** `main.go:468-519` (ClientboundPosition handler)
- **Entity tracking:** `main.go:309-466`
- **Movement examples:** `main.go:1216-1294` (fireBow, DoUseItem, DoPlayerAction)
- **Block collision data:** `github.com/reallyoldfogie/mc-data-gen` (local: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-data-gen`)
  - Loader package: `github.com/reallyoldfogie/mc-data-gen/loader`
  - Data location: `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-data-gen/data/{version}/blocks`
  - Available versions: 1.21.1 - 1.21.10
  - **Impact Assessment:** `MC_DATA_GEN_UPDATES_IMPACT.md` - Documents 9 new properties and helper functions
  - **Updates:** Loader updated Nov 23 14:55 with new properties (Climbable, Water, Lava, Fluid, DoorLike, FenceLike, Slab, Stair, LogOrLeaf)

## Version Agnosticism

### Current State

The plan is **mostly version-agnostic** but requires attention in certain areas:

#### ✅ Version-Agnostic Components

1. **Packet Handling** - All packet operations use `packetMgr` which handles version differences
   - Movement packets: `ServerboundMovePlayerPos`, `ServerboundMovePlayerPosRot`, `ServerboundMovePlayerRot`
   - Entity tracking: `ClientboundAddEntity`, `ClientboundMoveEntityPos`, etc.
   - Position updates: `ClientboundPosition` (already fixed for 1.21.5+ structure)

2. **Block Data Access** - Uses version-specific `blockMgr`
   - `blockMgr.GetByID(id)` - Get block by ID
   - `blockMgr.BlockIDByStateID(stateID)` - Convert state to ID
   - Block properties available: Name, Hardness, Transparent, MinStateID, MaxStateID, etc.

3. **Sound & Registry Data** - Uses `soundMgr` and custom registries
   - All version-specific data accessed through managers

#### ❌ Not Version-Agnostic (Needs Work)

1. **Block Collision Detection** (`bot/path/blocks.go`)
   - **Problem:** Currently uses hardcoded block lists from old Minecraft versions
   - **Example:** Hardcoded `block.Stone`, `block.OakStairs`, `block.Ladder`, etc.
   - **Impact:** Won't work correctly across versions as block IDs change
   - **Note:** File has TODO comment acknowledging this issue
   - **✅ SOLUTION:** Use `mc-data-gen` block shape data (see below)

2. **Block Behavior Classification**
   - **Problem:** Need to determine if blocks are:
     - Walkable (air, grass, torch, etc.)
     - Solid/steppable (stone, dirt, planks, etc.)
     - Climbable (ladders, vines, scaffolding)
     - Dangerous (lava, fire, magma)
     - Partial (slabs, stairs)
   - **Current Approach:** Hardcoded lists (not version-safe)
   - **✅ SOLUTION:** Use `mc-data-gen` shape data with helper functions

3. **Movement Physics Constants** (Potential Issue)
   - Walking speed, sprint speed, jump height may vary between versions
   - Currently not a problem but should be monitored

### Solution: mc-data-gen Block Shape Data

**The project has access to comprehensive, version-specific block collision data** via the `mc-data-gen` repository.

#### Repository Information

- **Location:** `/home/reallyoldfogie/src/github.com/reallyoldfogie/mc-data-gen`
- **GitHub:** `github.com/reallyoldfogie/mc-data-gen`
- **Loader Module:** `github.com/reallyoldfogie/mc-data-gen/loader`
- **Available Versions:** 1.21.1 through 1.21.10

#### What mc-data-gen Provides

**Per-Block Shape Data** stored in JSON files:

```json
{
  "block_id": "minecraft:stone",
  "states": [
    {
      "properties": {},
      "collision_boxes": [{"min": [0,0,0], "max": [1,1,1]}],
      "outline_boxes": [{"min": [0,0,0], "max": [1,1,1]}],
      "air": false,
      "opaque": true,
      "solid_block": true,
      "replaceable": false,
      "blocks_movement": true
    }
  ]
}
```

**Each block state includes:**
- `collision_boxes` - Physical collision AABBs (for movement/pathfinding)
- `outline_boxes` - Visual selection boxes
- `air` - Whether the block is air
- `opaque` - Whether the block blocks light
- `solid_block` - Whether it's a full solid cube
- `replaceable` - Whether it can be replaced (like grass)
- `blocks_movement` - Whether entities can pass through
- **NEW:** `climbable` - Ladders, vines, scaffolding
- **NEW:** `water` - Water blocks (all levels)
- **NEW:** `lava` - Lava blocks (all levels)
- **NEW:** `fluid` - Any fluid (water or lava)
- **NEW:** `door_like` - Doors, trapdoors, fence gates
- **NEW:** `fence_like` - Fences, walls
- **NEW:** `slab` - All slab variants
- **NEW:** `stair` - All stair variants
- **NEW:** `log_or_leaf` - Tree blocks

**Complex blocks** (stairs, slabs, etc.) have multiple states with different collision boxes per state.

#### Loader Package API

**Installation:**
```go
import mdl "github.com/reallyoldfogie/mc-data-gen/loader"
```

**Loading data:**
```go
// Load all blocks for a specific version
blockData, err := mdl.LoadBlocksDir("./data/1.21.5/blocks")
if err != nil { panic(err) }

// Look up a specific block state
key := mdl.StateKey{
    BlockID:  "minecraft:stone",
    PropsKey: mdl.MakePropsKey(nil), // Empty properties
}
info := blockData[key]
```

**Available Helper Methods on `ShapeInfo`:**

```go
// Core properties
passable := info.IsPassable()        // Is the block passable for entity movement?
surfaceY := info.IsStandingSurface() // Get the maximum Y of collision boxes
seeThrough := info.CanSeeThrough()   // Can entities see through this block?
boxes := info.WorldCollisionBoxesAt(x, y, z) // Get world-space collision boxes

// Movement types (NEW in mc-data-gen updates)
climbable := info.IsClimbable()      // Is this block climbable? (SIMPLIFIED - no blockID needed!)

// Fluid detection (NEW)
fluid := info.IsFluid()              // Is this any fluid?
water := info.IsWater()              // Is this water?
lava := info.IsLava()                // Is this lava?

// Block type classification (NEW)
doorLike := info.IsDoorLike()        // Door/trapdoor/fence gate?
fenceLike := info.IsFenceLike()      // Fence/wall?
slab := info.IsSlab()                // Slab?
stair := info.IsStair()              // Stairs?
logOrLeaf := info.IsLogOrLeaf()      // Tree log or leaf?
```

#### Integration Plan

**Phase 2: Basic Pathfinding**

Replace `daze/bot/path/blocks.go` with mc-data-gen integration:

```go
package path

import (
    mdl "github.com/reallyoldfogie/mc-data-gen/loader"
    "github.com/reallyoldfogie/mc-protocol-go/models"
)

// BlockShapeManager loads and provides access to block shape data
type BlockShapeManager interface {
   // Core collision
   IsPassable(blockID string, props map[string]string) bool
   IsSolid(blockID string, props map[string]string) bool
   GetCollisionBoxes(blockID string, props map[string]string, x, y, z int) []mdl.AABB

   // Movement types
   IsClimbable(blockID string, props map[string]string) bool

   // Fluid detection (NEW in mc-data-gen updates)
   IsFluid(blockID string, props map[string]string) bool
   IsWater(blockID string, props map[string]string) bool
   IsLava(blockID string, props map[string]string) bool

   // Danger detection (NEW in mc-data-gen updates)
   IsDangerous(blockID string, props map[string]string) bool

   // Block type classification (NEW in mc-data-gen updates)
   IsDoorLike(blockID string, props map[string]string) bool
   IsFenceLike(blockID string, props map[string]string) bool
   IsSlab(blockID string, props map[string]string) bool
   IsStair(blockID string, props map[string]string) bool
   IsLogOrLeaf(blockID string, props map[string]string) bool
}

type blockShapeManager struct {
    version   string
    shapeData map[mdl.StateKey]mdl.ShapeInfo
}

// NewBlockShapeManager creates a manager for a specific MC version
func NewBlockShapeManager(version string, dataPath string) (BlockShapeManager, error) {
    // dataPath example: "/path/to/mc-data-gen/data/1.21.5/blocks"
    data, err := mdl.LoadBlocksDir(dataPath)
    if err != nil {
        return nil, err
    }

    return &blockShapeManager{
        version:   version,
        shapeData: data,
    }, nil
}

// IsPassable checks if a block allows entity movement
func (bsm *BlockShapeManager) IsPassable(blockID string, props map[string]string) bool {
    key := mdl.StateKey{
        BlockID:  blockID,
        PropsKey: mdl.MakePropsKey(props),
    }

    info, ok := bsm.shapeData[key]
    if !ok {
        // Unknown block, assume passable (air-like)
        return true
    }

    return info.IsPassable()
}

// IsSolid checks if a block is a solid standing surface
func (bsm *BlockShapeManager) IsSolid(blockID string, props map[string]string) bool {
    key := mdl.StateKey{BlockID: blockID, PropsKey: mdl.MakePropsKey(props)}
    info, ok := bsm.shapeData[key]
    if !ok {
        return false
    }

    return info.SolidBlock || info.IsStandingSurface() > 0
}

// IsClimbable checks if a block can be climbed (SIMPLIFIED in mc-data-gen updates)
func (bsm *blockShapeManager) IsClimbable(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsClimbable() // Direct property access!
}

// IsFluid checks if a block is any fluid (NEW)
func (bsm *blockShapeManager) IsFluid(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsFluid()
}

// IsWater checks if a block is water (NEW)
func (bsm *blockShapeManager) IsWater(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsWater()
}

// IsLava checks if a block is lava (NEW)
func (bsm *blockShapeManager) IsLava(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsLava()
}

// IsDangerous detects dangerous blocks (NEW - currently lava, expandable)
func (bsm *blockShapeManager) IsDangerous(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    // Could add: fire, magma blocks, cactus, etc.
    return info.IsLava()
}

// IsDoorLike checks if block is a door/trapdoor/gate (NEW)
func (bsm *blockShapeManager) IsDoorLike(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsDoorLike()
}

// IsFenceLike checks if block is a fence/wall (NEW)
func (bsm *blockShapeManager) IsFenceLike(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsFenceLike()
}

// IsSlab checks if block is a slab (NEW)
func (bsm *blockShapeManager) IsSlab(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsSlab()
}

// IsStair checks if block is stairs (NEW)
func (bsm *blockShapeManager) IsStair(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsStair()
}

// IsLogOrLeaf checks if block is tree log/leaf (NEW)
func (bsm *blockShapeManager) IsLogOrLeaf(blockID string, props map[string]string) bool {
    info := bsm.getInfo(blockID, props)
    return info.IsLogOrLeaf()
}

// GetCollisionBoxes returns world-space collision AABBs
func (bsm *blockShapeManager) GetCollisionBoxes(blockID string, props map[string]string, x, y, z int) []mdl.AABB {
    info := bsm.getInfo(blockID, props)
    return info.WorldCollisionBoxesAt(x, y, z)
}

// getInfo is a helper to retrieve ShapeInfo for a block state
func (bsm *blockShapeManager) getInfo(blockID string, props map[string]string) mdl.ShapeInfo {
    key := mdl.StateKey{
        BlockID:  blockID,
        PropsKey: mdl.MakePropsKey(props),
    }

    info, ok := bsm.shapeData[key]
    if !ok {
        // Return empty/air-like info for unknown blocks
        return mdl.ShapeInfo{Air: true}
    }

    return info
}
```

**Usage in Pathfinding:**

```go
// In pathfinding initialization (version will come from version detection on server connection)
shapeMgr, err := path.NewBlockShapeManager("1.21.5", "/path/to/mc-data-gen/data/1.21.5/blocks")
if err != nil {
    panic(err)
}

// In movement validation
blockID := "minecraft:stone"
props := map[string]string{} // or get from block state
if shapeMgr.IsPassable(blockID, props) {
    // Can move through this block
}

if shapeMgr.IsSolid(blockID, props) {
    // Can stand on this block
}

// NEW: Danger avoidance and terrain costs in pathfinding
func calculateMovementCost(from, to V3, movement MovementType) float64 {
    baseCost := movement.BaseCost()

    // Add penalties for dangerous blocks
    if shapeMgr.IsDangerous(toBlockID, toProps) {
        return baseCost + 1000.0 // Avoid lava!
    }

    // Fences are 1.5 blocks tall - not jumpable, treat as barrier
    if shapeMgr.IsFenceLike(toBlockID, toProps) {
        return baseCost + 500.0 // High cost - prefer to path around
    }

    // Slabs and stairs have lower jump cost
    if shapeMgr.IsSlab(toBlockID, toProps) || shapeMgr.IsStair(toBlockID, toProps) {
        return baseCost * 0.8 // Easier to traverse
    }

    return baseCost
}

// NEW: Water navigation (Phase 4)
if shapeMgr.IsWater(blockID, props) {
    // Enable swimming movement primitive
    // Slower movement speed, can ascend/descend in water
}

// NEW: Door/gate handling (Phase 4)
if shapeMgr.IsDoorLike(blockID, props) {
    // Includes doors, trapdoors, and fence gates
    // Attempt to open or find alternate route
}
```

### Action Items for Version Agnosticism

1. **Phase 1: Setup**
   - 📋 Add `mc-data-gen/loader` dependency to `daze/go.mod`
   - 📋 Verify data availability for target versions (1.21.1-1.21.10)
   - 📋 Test loader on sample blocks

2. **Phase 2: Basic Pathfinding**
   - ⚠️ **Regenerate mc-data-gen data files** (required for new properties)
   - ✅ Create `BlockShapeManager` wrapper around `mc-data-gen/loader`
   - ✅ Replace hardcoded block lists in `daze/bot/path/blocks.go`
   - ✅ Implement core helpers: `IsPassable()`, `IsSolid()`, `IsClimbable()` (simplified API)
   - ✅ Implement new helpers: `IsDangerous()`, `IsFluid()`, `IsWater()`, `IsLava()`, `IsDoorLike()`, `IsFenceLike()`, `IsSlab()`, `IsStair()`, `IsLogOrLeaf()`
   - ✅ Add danger avoidance to pathfinding cost calculation
   - ✅ Integrate with pathfinding A* algorithm
   - ✅ Test on at least 2 Minecraft versions (e.g., 1.21.1 and 1.21.5)

3. **Phase 3: World Integration**
   - 📋 Bridge world manager block data with shape manager
   - 📋 Handle block state properties (stairs facing, slab type, etc.)
   - 📋 Cache frequently accessed shape data for performance

4. **Phase 4: Advanced Movement**
   - 📋 Use precise collision boxes for complex pathfinding
   - 📋 Handle partial blocks (slabs, stairs) correctly using `IsSlab()`, `IsStair()`
   - 📋 Implement climbing detection using `IsClimbable()` (simplified API)
   - 📋 Implement water navigation using `IsWater()`
   - 📋 Implement lava avoidance using `IsLava()`
   - 📋 Implement door navigation using `IsDoorLike()`
   - 📋 Implement fence handling using `IsFenceLike()`

5. **Phase 6: Polish**
   - 📋 Document mc-data-gen integration
   - 📋 Add version compatibility notes to README
   - 📋 Performance testing with shape data lookups

### Testing Across Versions

To ensure version agnosticism, test on multiple Minecraft versions:

**Minimum Test Matrix:**
- 1.21.1 (baseline)
- 1.21.5 (current target)
- 1.21.8 (latest available in protocol repo)

**Test Cases:**
1. Navigate across common blocks (stone, dirt, grass)
2. Use stairs and slabs
3. Climb ladders
4. Avoid lava/fire
5. Handle new blocks added in later versions

### Version-Specific Packet Structure Handling

Already handled correctly by the existing codebase:

**Example: ClientboundPosition packet**
- Different structure in 1.21.5+ (new delta fields, field reordering)
- Solution: Version-specific packet definitions in `mc-protocol-go`
- Access: Via `packetMgr.GetClientboundPacketByID()`

The following system will continue to work as long as we use `packetMgr` and `blockMgr` for all protocol operations.

### Conclusion

**The plan architecture is fully version-agnostic** thanks to:

1. **Packet handling** via `packetMgr` (handles version-specific packet structures)
2. **Block metadata** via `blockMgr` (version-specific block properties)
3. **Block collision data** via `mc-data-gen` (comprehensive, version-specific shape data)

**The key breakthrough is mc-data-gen**, which provides:
- Exact collision box data for all blocks
- Version-specific data for 1.21.1 through 1.21.10
- Simple loader API with helper methods
- No hardcoding required

This eliminates the need for manual block classification or tag parsing. All collision detection will use authoritative shape data directly from Minecraft's internal systems.

## Success Criteria

The follow player feature will be considered complete when:

1. ✅ Bot can follow a target player by name or automatically follow nearest
2. ✅ Bot navigates simple terrain (flat, stairs, small drops) reliably
3. ✅ Bot maintains configurable distance from target
4. ✅ Bot handles target disconnection gracefully
5. ✅ Bot recovers from stuck states automatically
6. ✅ Bot provides status feedback via chat
7. ✅ Performance is acceptable (no noticeable lag)
8. ✅ Code is documented and maintainable

---

## Bonus Criteria

* Generalize from following a player to following an entity (i.e. follow a cow, pig, chicken, etc.) 

---

**Document Version:** 1.3
**Last Updated:** 2025-11-24
**Author:** Claude (via Claude Code)
**Status:** Draft - Ready for Implementation
**Version Agnosticism:** ✅ Fully Addressed - Uses mc-data-gen for collision data
**Block Collision:** ✅ Solved - mc-data-gen provides authoritative shape data with enhanced properties
**Updates:** Added 9 new block properties and helper functions from mc-data-gen (see MC_DATA_GEN_UPDATES_IMPACT.md)
