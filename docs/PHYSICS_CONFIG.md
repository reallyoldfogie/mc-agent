# Physics System Configuration Guide

## Overview

The mc-agent physics system provides accurate Minecraft physics simulation for movement, rotation, projectiles, and fall damage. This guide covers configuration, tuning, and best practices.

## Physics Components

### 1. Core Physics Engine (`physics/`)

#### Movement Physics (`physics/state.go`)
- **Player dimensions**: 0.6×1.8 blocks (width×height)
- **Eye height**: 1.62 blocks above feet
- **Tick rate**: 20 ticks/second (50ms per tick)

**Key parameters:**
```go
const (
    PlayerWidth  = 0.6   // Hitbox width
    PlayerHeight = 1.8   // Hitbox height
    EyeHeight    = 1.62  // Eye level above feet
)
```

#### Physics Constants (`physics/constants.go`)
```go
const (
    // Core physics
    Gravity       = 0.08         // Blocks/tick² (downward acceleration)
    AirDrag       = 0.98         // Air resistance multiplier
    GroundFriction = 0.91        // Ground friction multiplier
    
    // Movement speeds (blocks/tick)
    WalkSpeed     = 0.21585      // Normal walking
    SprintSpeed   = 0.28061      // Sprinting (130% of walk)
    SneakSpeed    = 0.064755     // Sneaking (30% of walk)
    FlySpeed      = 0.21585      // Flying (creative mode)
    SwimSpeed     = 0.3          // Swimming horizontal
    ClimbSpeed    = 0.15         // Ladder climbing
    
    // Jump mechanics
    JumpVelocity  = 0.42         // Initial upward velocity
    JumpBoost     = 0.1          // Jump Boost effect per level
    
    // Water mechanics
    WaterDrag     = 0.8          // Water resistance
    WaterGravity  = 0.02         // Reduced gravity in water
    WaterJump     = 0.04         // Jump velocity in water
    
    // Collision
    StepHeight    = 0.6          // Max auto-step height
    CollisionMargin = 0.001      // AABB collision margin
)
```

### 2. Rotation System (`physics/rotation.go`)

Anti-cheat compliant rotation rate limiting:

```go
const (
    MaxYawChange   = 11.0  // Max degrees per tick (horizontal)
    MaxPitchChange = 7.0   // Max degrees per tick (vertical)
)
```

**Functions:**
- `LimitRotation()` - Apply rate limits to rotation changes
- `RotateToward()` - Gradually rotate toward target over multiple ticks
- `AngleDifference()` - Calculate shortest angular distance (handles wrapping)
- `IsLookingAt()` - Check if looking at position within tolerance
- `TicksToRotate()` - Calculate time needed to rotate to target

**Usage example:**
```go
// Gradually look at target
newYaw, newPitch := rotation.LimitRotation(
    currentYaw, currentPitch,
    targetYaw, targetPitch,
)

// Check if we can see target
canSee := rotation.IsLookingAt(
    botX, botY, botZ,
    yaw, pitch,
    targetX, targetY, targetZ,
    5.0, // tolerance in degrees
)
```

### 3. Projectile Physics (`physics/projectile.go`)

Generalized projectile simulation for 7 projectile types:

| Type | Gravity | Drag | Initial Speed | Notes |
|------|---------|------|---------------|-------|
| Arrow | 0.05 | 0.99 | 3.1 | Fully charged bow |
| Snowball | 0.03 | 0.99 | 1.5 | Throwable item |
| Egg | 0.03 | 0.99 | 1.5 | Throwable item |
| EnderPearl | 0.03 | 0.99 | 1.5 | Teleports on impact |
| SplashPotion | 0.05 | 0.99 | 1.0 | Area effect |
| Trident | 0.05 | 0.99 | 2.5 | Melee/ranged weapon |
| FishingBobber | 0.03 | 0.92 | 1.0 | Fishing rod |

**Functions:**
- `SimulateProjectile()` - Full trajectory simulation
- `FindOptimalTrajectory()` - Find best pitch/power to hit target
- `CalculateAiming()` - Calculate yaw/pitch/power for target
- `GetProjectilePhysics()` - Get constants for projectile type
- `WillHitTarget()` - Quick collision check
- `ProjectileFlightTime()` - Estimate time to target

**Usage example:**
```go
// Find best shot for arrow
yaw, pitch, power, hit := projectile.FindOptimalTrajectory(
    projectile.Arrow,
    botPos,
    targetPos,
    world, // for obstacle detection
)

if hit {
    // Execute shot with calculated parameters
    executor.AimAndShoot(yaw, pitch, power)
}
```

### 4. Fall Damage (`physics/fall_damage.go`)

Accurate fall damage calculation with water detection:

**Mechanics:**
- Fall damage threshold: 3 blocks
- Damage formula: `max(0, fallDistance - 3.0)` hearts
- **Water landing**: 0 damage from ANY height
- **Special blocks**: Hay bale (80% reduction), slime block (0 damage)

**Functions:**
- `CalculateFallDamage()` - Compute damage from fall distance
- `IsSafeDrop()` - Check if drop is safe (water or short)
- `GetDamageReduction()` - Get reduction multiplier for block
- `PathfindingDropCost()` - Integrate with pathfinding costs

**Block damage reduction:**
```go
Block Type     | Damage Reduction
---------------|------------------
Water          | 100% (no damage)
Slime Block    | 100% (no damage)
Honey Block    | 80% reduction
Hay Bale       | 80% reduction
Bed            | 50% reduction
Powder Snow    | 100% (no damage)
```

**Pathfinding integration:**
```go
// Water drop from any height: cost = 1.2
// Land drop from 10 blocks: cost = 1.5 + (10-3)*10 = 71.5
// Land drop from 3 blocks: cost = 1.5 (no damage)

cost := fall_damage.PathfindingDropCost(
    fromPos, toPos,
    shapeProvider,
    1.5, // base drop cost
)
```

### 5. Movement Prediction (`physics/state.go`)

Simulate movement ahead without modifying state:

**Functions:**
- `PredictMovement()` - Simulate N ticks with inputs
- `PredictPosition()` - Fast position-only prediction
- `WillCollide()` - Quick collision check for path

**Usage example:**
```go
// Predict 10 ticks ahead
inputs := []physics.Inputs{
    {Forward: true, Sprint: true},
}
futureStates := state.PredictMovement(inputs, 10, world)

// Check if path is clear
if state.WillCollide(targetPos, world) {
    // Path blocked, find alternative
}
```

### 6. Collision Avoidance (`pathfinding/collision_check.go`)

Pre-validate paths before execution:

**Functions:**
- `CheckPathClear()` - Verify path has no collisions
- `PathCollisionCost()` - Add cost for risky paths
- `IsMovementPossible()` - Validate movement type feasibility

**Integration with pathfinding:**
```go
// Check if movement is possible
if !collisionChecker.IsMovementPossible(
    movementType, fromPos, toPos,
) {
    // Skip this neighbor in pathfinding
    continue
}

// Add collision risk to cost
baseCost := movementType.BaseCost()
collisionCost := collisionChecker.PathCollisionCost(fromPos, toPos)
totalCost := baseCost + collisionCost
```

### 7. Item Usage (`items/usage.go`)

Block placement and item usage system:

**Functions:**
- `PlaceBlock()` - Place block at position/face
- `UseItemOnBlock()` - Use item on block (e.g., bucket on cauldron)
- `UseItemOnEntity()` - Use item on entity (e.g., bucket on fish)
- `AttackEntity()` - Attack entity with current item
- `SwitchToSlot()` - Change active hotbar slot

**Sequence number tracking:**
```go
// Anti-cheat: sequence numbers must increment
type ItemUsage struct {
    nextSequence int32
}

func (iu *ItemUsage) PlaceBlock(pos V3, face BlockFace) {
    sequence := iu.nextSequence
    iu.nextSequence++
    
    // Send packet with sequence
    pk.Marshal(packetID, 
        hand, pos, face, 
        cursorX, cursorY, cursorZ,
        insideBlock, sequence,
    )
}
```

**Reach distance limits:**
- Blocks: 4.5 blocks (5.0 in creative)
- Entities: 3.0 blocks (4.5 in creative)

## Movement Types

The pathfinding system supports multiple movement types:

| Type | Base Cost | Description |
|------|-----------|-------------|
| Traverse | 1.0 | Walk on same level |
| Sprint | 0.77 | Sprint (130% speed) |
| Sneak | 3.33 | Sneak (30% speed) |
| Ascend | 1.5 | Jump up one block |
| Descend | 1.2 | Drop down 1-3 blocks |
| Jump2 | 2.0 | Jump across 2-block gap |
| DiagonalTraverse | 1.414 | Walk diagonally |
| Swim | 2.0 | Move through water |
| Climb | 1.8 | Climb ladder/vine |
| SneakThrough | 3.0 | Navigate through 1.5-block gaps |
| SneakTraverse | 3.0 | Edge safety while sneaking |

## Performance Tuning

### Prediction Performance
- Target: <0.1ms per N-tick simulation
- Optimization: Use `PredictPosition()` instead of `PredictMovement()` when only position needed
- Cache: Reuse collision boxes for chunks

### Pathfinding Performance
- Collision checks add ~0.1ms per path segment
- Use `IsMovementPossible()` early to reject invalid neighbors
- Batch collision queries when possible

### Projectile Simulation
- Target: <1ms for full trajectory (400 ticks)
- Trade-off: Search precision vs speed
  - Fine search: 0.1° pitch step, 0.05 power step
  - Coarse search: 1.0° pitch step, 0.1 power step

## Configuration Examples

### Conservative Movement (Anti-cheat Safe)
```go
// Use rotation limiting
newYaw, newPitch := rotation.LimitRotation(
    currentYaw, currentPitch,
    targetYaw, targetPitch,
)

// Check path before execution
if !collisionChecker.CheckPathClear(from, to) {
    return errors.New("path blocked")
}

// Prefer safe drops
cost := fall_damage.PathfindingDropCost(from, to, world, 1.5)
if cost > 10.0 {
    // Too dangerous, find alternative
}
```

### Aggressive Movement (Fast Navigation)
```go
// Skip rotation limiting for teleports
state.Yaw = targetYaw
state.Pitch = targetPitch

// Allow risky paths
movementCost := movementType.BaseCost()
// Skip collision checks for speed

// Take calculated fall damage
if fallDamage < 5.0 {
    // Acceptable damage, take shortcut
}
```

### Precise Aiming (Combat)
```go
// Use fine-grained search
yaw, pitch, power, hit := projectile.FindOptimalTrajectory(
    projectile.Arrow,
    botPos,
    targetPos,
    world,
)

// Wait for rotation to complete
ticksNeeded := rotation.TicksToRotate(
    currentYaw, currentPitch,
    yaw, pitch,
)
time.Sleep(time.Duration(ticksNeeded) * 50 * time.Millisecond)

// Fire when ready
executor.Shoot(power)
```

## Best Practices

### 1. Always Use Rotation Limiting
```go
// ✅ GOOD: Gradual rotation
newYaw, newPitch := rotation.LimitRotation(
    currentYaw, currentPitch,
    targetYaw, targetPitch,
)

// ❌ BAD: Instant rotation (triggers anti-cheat)
state.Yaw = targetYaw
state.Pitch = targetPitch
```

### 2. Validate Paths Before Execution
```go
// ✅ GOOD: Pre-check collision
if state.WillCollide(targetPos, world) {
    // Find alternative path
}

// ❌ BAD: Execute blindly and get stuck
movement.MoveTo(targetPos)
```

### 3. Prefer Water Drops
```go
// ✅ GOOD: Check for water landing
if fall_damage.IsSafeDrop(from, to, world) {
    // Take water drop (fast and safe)
}

// ❌ BAD: Take fall damage when water available
movement.Drop(to)
```

### 4. Use Sequence Numbers
```go
// ✅ GOOD: Increment sequence
usage.nextSequence++
pk.Marshal(packetID, ..., sequence)

// ❌ BAD: Reuse sequence (anti-cheat violation)
pk.Marshal(packetID, ..., 0)
```

### 5. Respect Reach Distance
```go
// ✅ GOOD: Check reach
distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
if distance > 4.5 {
    // Move closer first
}

// ❌ BAD: Place blocks out of reach
usage.PlaceBlock(farPos, face)
```

## Debugging

### Enable Physics Logging
```go
// Log physics state every tick
func (s *State) Tick(inputs Inputs, world World) {
    log.Printf("Physics: pos=(%.2f,%.2f,%.2f) vel=(%.2f,%.2f,%.2f) onGround=%v",
        s.Position.X, s.Position.Y, s.Position.Z,
        s.Velocity.X, s.Velocity.Y, s.Velocity.Z,
        s.OnGround,
    )
    // ... rest of tick
}
```

### Visualize Trajectories
```go
// Log projectile path
positions := projectile.SimulateProjectile(
    projectileType, startPos, yaw, pitch, power,
)
for i, pos := range positions {
    log.Printf("Tick %d: (%.2f, %.2f, %.2f)", i, pos.X, pos.Y, pos.Z)
}
```

### Track Prediction Errors
```go
// Compare prediction to actual
predicted := state.PredictPosition(vel, 10, world)
// ... wait 10 ticks ...
actual := state.Position

error := math.Sqrt(
    math.Pow(predicted.X-actual.X, 2) +
    math.Pow(predicted.Y-actual.Y, 2) +
    math.Pow(predicted.Z-actual.Z, 2),
)
log.Printf("Prediction error: %.2f blocks", error)
```

## References

- Minecraft Wiki - Movement: https://minecraft.wiki/w/Movement
- Minecraft Wiki - Damage: https://minecraft.wiki/w/Damage
- Minecraft Wiki - Projectiles: https://minecraft.wiki/w/Arrow
- mc-agent Physics Plan: `docs/PHYSICS_INTEGRATION_PLAN.md`
- Phase 6 Implementation: `docs/PHYSICS_PHASE6_NOTES.md`

## Related Files

- `physics/state.go` - Core physics engine
- `physics/constants.go` - Physics constants
- `physics/rotation.go` - Rotation rate limiting
- `physics/projectile.go` - Projectile physics
- `physics/fall_damage.go` - Fall damage calculation
- `pathfinding/types.go` - Movement types
- `pathfinding/collision_check.go` - Collision avoidance
- `items/usage.go` - Item usage and block placement
