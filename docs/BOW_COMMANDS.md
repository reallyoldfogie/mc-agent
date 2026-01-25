# Bow Commands with Ballistic Calculation

## Overview

The agent includes physics-based bow firing that simulates Minecraft's arrow trajectory to accurately hit target coordinates. The system calculates the required pitch, yaw, and bow charge duration to hit a specified 3D position.

## Commands

### Basic Fire
```
>>>BotName<<< fireBow
```
Fires the equipped bow in the current look direction with default charge.

### Targeted Fire
```
>>>BotName<<< fireBowAt <x> <y> <z>
```
Calculates optimal aiming and fires at the specified coordinates.

**Example:**
```
>>>ROF_bot<<< fireBowAt 100 64 200
```

## Physics Model

### Minecraft Arrow Ballistics

The simulation uses Minecraft's actual arrow physics:

**Constants:**
- `Gravity`: 0.05 blocks/tick²
- `Drag`: 0.99 (velocity multiplier per tick)
- `Max Initial Speed`: 3.1 blocks/tick (fully charged)
- `Tick Rate`: 20 ticks/second

**Velocity Update (per tick):**
```
velY -= Gravity      // Apply gravity
velY *= Drag         // Apply drag
velXZ *= Drag        // Horizontal drag
```

**Position Update:**
```
posX += velXZ * cos(pitch)
posY += velY
posZ += velXZ * sin(pitch)
```

## Aiming Algorithm

### 1. Calculate Direction (Yaw)

```go
dx := targetX - botX
dz := targetZ - botZ
yaw := atan2(-dx, -dz) * 180 / π
```

### 2. Find Optimal Shot

The algorithm searches through combinations of pitch and power to find the shot that lands closest to the target:

**Search Space:**
- Pitch: -90° to +90° (step: 0.1°)
- Power: 0.1 to 1.0 (step: 0.05)

**For each combination:**
1. Calculate initial speed: `3.1 * powerFactor`
2. Simulate arrow trajectory for up to 400 ticks
3. Check if arrow passes through target position (±0.5 block tolerance)
4. Track the shot with minimum error

**Pseudocode:**
```go
bestPitch := 0.0
bestPower := 1.0
minError := infinity

for power := 0.1 to 1.0 step 0.05 {
    for pitch := -90 to 90 step 0.1 {
        hitX, hitY, hit := simulateArrow(pitch, power, target)
        if hit {
            error := distance(hit, target)
            if error < minError {
                minError = error
                bestPitch = pitch
                bestPower = power
            }
        }
    }
}
```

### 3. Convert Power to Hold Duration

```go
holdDurationSeconds := powerFactor * 1.0  // 0.1-1.0 seconds
holdDuration := time.Duration(holdDurationSeconds * float64(time.Second))
```

## Implementation

### Packet Sequence

The bot sends the following packet sequence to fire a bow:

1. **ServerboundUseItem** - Start using bow
   - Hand: 0 (main hand)
   - Sequence: incrementing counter

2. **ServerboundPlayerAction** (repeated) - Hold bow
   - Action: 0 (start digging - repurposed for holding)
   - Position: (0, 0, 0)
   - Face: 0
   - Sequence: incrementing

3. **ServerboundPlayerAction** - Release bow
   - Action: 5 (release item)
   - Position: (0, 0, 0)
   - Face: 0
   - Sequence: incrementing

### Code Example

```go
func (a *agent) cmdFireBowAt(x, y, z float64) {
    // Look at target
    err := a.moveExec.LookAt(x, y, z, true)
    if err != nil {
        a.SendChat("Failed to look at target: " + err.Error())
        return
    }

    // Get bot position
    botX, botY, botZ, _, _, initialized := a.GetPosition()
    if !initialized {
        a.SendChat("Bot position not initialized")
        return
    }

    // Calculate aiming
    yaw, pitch, powerFactor := CalculateAiming(
        Point{X: botX, Y: botY, Z: botZ}, 
        Point{X: x, Y: y, Z: z},
    )

    // Update bot look direction
    a.setPosition(x, y, z, float32(yaw), float32(pitch))

    // Calculate hold duration
    holdDuration := time.Duration(powerFactor * float64(time.Second))

    // Send packet sequence
    useID := a.packetMgr.GetServerboundPacketID("ServerboundUseItem")
    a.client.WritePacket(pk.Marshal(useID, ...))

    actID := a.packetMgr.GetServerboundPacketID("ServerboundPlayerAction")
    go func() {
        for i := 0; i < bowHoldIterations; i++ {
            a.client.WritePacket(pk.Marshal(actID, pk.VarInt(0), ...))
            time.Sleep(holdDuration / time.Duration(bowHoldIterations))
        }
        a.client.WritePacket(pk.Marshal(actID, pk.VarInt(5), ...))
    }()
}
```

## Accuracy Considerations

### Factors Affecting Accuracy

1. **Simulation Precision**
   - Step size: 0.1° pitch, 0.05 power
   - Tolerance: ±0.5 blocks
   - Max simulation time: 400 ticks (20 seconds)

2. **Target Height Adjustment**
   - Bot eye height: 1.62 blocks (hardcoded)
   - Target center: assumed at block center

3. **Server-Side Factors**
   - Network latency
   - Server tick timing
   - Arrow entity spawn timing

### Typical Accuracy

- **Short range** (< 20 blocks): ±0.5 blocks
- **Medium range** (20-50 blocks): ±1-2 blocks  
- **Long range** (50-100 blocks): ±2-5 blocks
- **Very long range** (> 100 blocks): May miss entirely

High-angle shots (lobbing) are less accurate due to longer flight time and accumulated drag effects.

## Testing

### Unit Tests

```bash
go test ./agent -run TestCommand_FireBow -v
```

Tests cover:
- UseItem packet sequence
- Hold duration calculation
- Shoot after hold timing

### Manual Testing

1. Join a server with the bot
2. Place blocks at known coordinates
3. Command: `>>>bot<<< fireBowAt X Y Z`
4. Observe arrow impact
5. Adjust if needed (latency compensation)

### Debug Utility

The `cmd/targetBow` utility can be used to test aiming calculations without connecting to a server:

```bash
go run cmd/targetBow/main.go
```

This simulates various target positions and prints calculated yaw, pitch, and power.

## Limitations

1. **No Moving Targets**
   - Current implementation assumes stationary targets
   - Future: predict target velocity

2. **No Obstacle Detection**
   - Does not check for blocks in flight path
   - May aim through walls

3. **Fixed Eye Height**
   - Assumes standard player height (1.62)
   - Does not account for sneaking, swimming, etc.

4. **No Wind/Knockback**
   - Minecraft arrows are affected by knockback enchantments
   - Not modeled in current simulation

5. **Inventory Dependency**
   - Requires bow in main hand
   - Requires arrows in inventory
   - No automatic equipping

## Generalized Projectile System

The bow aiming functionality has been extracted and generalized into `physics/projectile.go`, which now supports multiple projectile types:

**Supported Projectiles:**
- Arrow (bow)
- Snowball
- Egg
- Ender Pearl
- Splash Potion
- Trident
- Fishing Bobber

**Key Functions:**
```go
// physics/projectile.go
func SimulateProjectile(projectileType ProjectileType, startPos V3, yaw, pitch, power float64) []V3
func FindOptimalTrajectory(projectileType ProjectileType, from, to V3, world World) (yaw, pitch, power float64, hit bool)
func CalculateAiming(botPos, targetPos Point) (yaw, pitch, powerFactor float64)
```

See `docs/PHYSICS_CONFIG.md` for detailed projectile physics configuration and usage examples.

## Future Enhancements

- [ ] Moving target prediction (lead calculation)
- [x] Obstacle/block detection using World data (available in FindOptimalTrajectory)
- [ ] Multi-target trajectory (hit multiple entities)
- [ ] Automatic bow/arrow equipping
- [ ] Enchantment effects (Power, Punch, Flame)
- [ ] Crossbow support
- [ ] Rapid fire mode (spam arrows)
- [ ] Parabolic path visualization in logs
- [x] Generalized projectile physics (completed - see physics/projectile.go)

## Physics References

- **Minecraft Wiki - Arrow**: https://minecraft.wiki/w/Arrow
- **Projectile Motion**: Standard physics with gravity + drag
- **Drag Formula**: `v_new = v_old * 0.99` per tick

## Code Files

- `agent/bowCommands.go` - Bow command implementation
- `physics/projectile.go` - Generalized projectile physics (NEW)
- `cmd/targetBow/main.go` - Standalone testing utility
- `agent/commands.go` - Command parsing (`fireBow`, `fireBowAt`)

## Related

- See [bow-fire-sequence.md](bow-fire-sequence.md) for detailed packet sequence documentation
- Movement executor (`movement/`) handles look direction changes
