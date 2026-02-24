# Manual Input Control

## Overview

The manual input control system allows frame-by-frame control of the agent's movement and rotation through the physics executor. This is useful for direct agent manipulation, testing, and precise control scenarios.

## Modes

The physics executor supports three operational modes:

- **Idle**: Default mode where the agent doesn't move
- **Manual**: Frame-by-frame control via SetManual* methods
- **Autopilot**: Autonomous movement (not covered here)

## Usage

### Entering Manual Mode

```go
executor.EnterManualMode()
```

Switches the executor to manual mode. This is idempotent—calling it when already in manual mode is a no-op. Upon entry, manual inputs are initialized with:
- Movement throttle set to zero
- Current sprint/sneak state preserved from the previous mode
- Yaw/pitch initialized from current physics state

### Setting Manual Inputs

#### Movement (Throttle)

```go
err := executor.SetManualThrottle(throttleX, throttleZ)
```

- `throttleX`: X-axis movement (-1.0 to +1.0, negative=west, positive=east)
- `throttleZ`: Z-axis movement (-1.0 to +1.0, negative=north, positive=south)
- Values are automatically clamped to [-1.0, 1.0]
- Returns error if not in manual mode

#### Rotation (Look Direction)

```go
err := executor.SetManualRotation(yaw, pitch)
```

- `yaw`: Horizontal look direction in degrees (0=south, 90=west, 180=north, 270=east)
- `pitch`: Vertical look direction in degrees (-90=up, 0=forward, 90=down)
- Pass `math.NaN()` for yaw or pitch to keep the current value without updating
- Returns error if not in manual mode

#### Jump

```go
err := executor.SetManualJump(enabled)
```

Sets whether the jump button is pressed.

#### Sprint

```go
err := executor.SetManualSprint(enabled)
```

Sets whether the sprint button is pressed.

#### Sneak

```go
err := executor.SetManualSneak(enabled)
```

Sets whether the sneak button is pressed.

#### Climb Direction

```go
err := executor.SetManualClimbDirection(direction)
```

- `direction`: +1.0=climb up, -1.0=climb down, 0.0=no climb
- Automatically clamped to [-1.0, 1.0]
- Returns error if not in manual mode

### Batch Input Setting

```go
err := executor.SetManualInputs(inputs)
```

Sets all manual inputs at once using a `models.Inputs` struct. Returns error if not in manual mode.

### Resetting Inputs

```go
err := executor.ResetManualInputs()
```

Clears all movement and button inputs to zero while preserving the current yaw and pitch from the physics state. This maintains look direction when clearing movement.

### Exiting Manual Mode

```go
executor.ExitManualMode()
```

Returns the executor to idle mode. This is idempotent—calling it when not in manual mode is a no-op. The current manual input sprint/sneak state is applied to the base executor to maintain agent stability. For example, if the agent is sneaking in manual mode to avoid falling off a cliff, exiting manual mode will keep the agent sneaking.

### Checking Current Mode

```go
isManual := executor.IsManualMode()
```

Returns true if the executor is currently in manual mode.

## Example: Frame-by-Frame Control

```go
executor.Start()
defer executor.Stop()

// Enter manual mode
executor.EnterManualMode()

// Control movement frame by frame
for i := 0; i < 100; i++ {
    // Move forward
    executor.SetManualThrottle(1.0, 0.0)
    
    // Look around
    executor.SetManualRotation(45.0, math.NaN())
    
    // Jump every 20 frames
    if i%20 == 0 {
        executor.SetManualJump(true)
    } else {
        executor.SetManualJump(false)
    }
    
    // Wait for frame to process
    time.Sleep(50 * time.Millisecond)
}

// Return to idle
executor.ExitManualMode()
```

## Thread Safety

All manual input operations are thread-safe. Multiple goroutines can safely call SetManual* methods and GetManualInputs() concurrently. Thread safety is ensured through RWMutex protection.

## Error Handling

All SetManual* methods return an error if the executor is not in manual mode. Always check the error return value:

```go
if err := executor.SetManualThrottle(1.0, 0.0); err != nil {
    log.Printf("Failed to set throttle: %v", err)
}
```

## State Preservation

When entering manual mode, the executor preserves the current sprint and sneak state from the previous mode. This is important for maintaining agent stability—if the agent is sneaking to avoid falling off a cliff, entering manual mode will keep the agent sneaking.

When exiting manual mode, the current manual input sprint and sneak state is applied to the base executor. This ensures continuity—if you're sneaking in manual mode and exit, the agent will continue sneaking in idle mode.

## Implementation Details

Manual inputs are stored internally in the executor and applied during physics simulation. The system preserves yaw and pitch values when transitioning between modes or resetting inputs, ensuring smooth look direction continuity.
