# Telemetry Architecture

## Overview

The movement telemetry system tracks agent behavior during path execution to validate navigation correctness. This document explains how telemetry recording works and addresses concerns about duplicate recording.

## Telemetry Interface

The `MovementTelemetryRecorder` interface (defined in `models/telemetry.go`) provides three methods:

```go
type MovementTelemetryRecorder interface {
    RecordTick(x, y, z float64, onGround bool, climbing bool, sneaking bool)
    RecordJump()
    RecordStep(stepType string)
}
```

### What Gets Tracked

The test implementation (`testing/movement_telemetry.go`) tracks:

- **TotalTicks**: Total physics ticks executed
- **ClimbTicks**: Ticks spent on ladders/vines
- **SneakTicks**: Ticks spent sneaking  
- **JumpCount**: Number of jumps executed
- **MaxDeltaY**: Maximum vertical change per tick
- **PathSteps**: Number of path steps completed
- **MovementTypes**: Sequence of movement types used (e.g., "Walk", "Climb", "ExitClimb")

## Execution Mode

The `PhysicsMovementExecutor` uses a **continuous execution mode** that runs at 20 TPS.

**How it works:**
1. `Start()` is called during agent initialization, spawning a `continuousTickLoop()` goroutine
2. `ExecutePath()` sets the path via `SetPath()` and waits for completion
3. Background loop automatically executes the path tick-by-tick

**Telemetry recording locations:**
- `recordTelemetry()` (called every tick): Records jumps and ticks
- `generateNavigationInputs()` (when step completes): Records step completion

**Code path:**
```
continuousTickLoop()
└─> tick() [every 50ms = 20 TPS]
    ├─> generateNavigationInputs()
    │   └─> if step complete: RecordStep()
    ├─> physicsState.Tick()
    └─> recordTelemetry()
        ├─> RecordJump() [if jumping]
        └─> RecordTick()
```

## Agent Startup

The agent's `Start()` method (`agent/agent.go`) automatically starts continuous mode:

```go
if physicsExec, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
    log.Printf("[Agent %s] Starting continuous physics executor", a.client.Name())
    a.waitForGroundData(baseCtx, 5*time.Second)
    physicsExec.Start()  // <-- Starts continuous mode
}
```

**All tests and production code use continuous mode**. The executor must be started with `Start()` before calling `ExecutePath()`.

## Validation Examples

### Basic Telemetry Assertions (Already Working)

```go
// These work because RecordTick() and RecordJump() were already being called
assert.Greater(t, telemetry.ClimbTicks, 0, "must use climbing")
assert.Equal(t, telemetry.JumpCount, 3, "should jump 3 times")
```

### Movement Type Validation (Now Working After Fix)

```go
// This now works because RecordStep() is being called
assert.Contains(t, telemetry.MovementTypes, "ExitClimb", "should use ExitClimb")
```

## Future Enhancements

For goal-oriented analysis (mentioned in user's requirements), the current telemetry provides:

1. **Movement sequence**: `MovementTypes[]` shows the sequence of movement primitives used
2. **Efficiency metrics**: `PathSteps` vs `TotalTicks` indicates how efficiently the agent executed
3. **Behavior patterns**: `ClimbTicks`, `SneakTicks`, `JumpCount` show agent's approach

Example analysis:
```go
// How did the agent accomplish the goal?
fmt.Printf("Agent used %d steps over %d ticks\n", telemetry.PathSteps, telemetry.TotalTicks)
fmt.Printf("Movement sequence: %v\n", telemetry.MovementTypes)
fmt.Printf("Climbed for %d ticks, jumped %d times\n", telemetry.ClimbTicks, telemetry.JumpCount)
```

## Summary

The telemetry system uses a single execution path (continuous mode) with telemetry recorded at two locations:
- **Per-tick telemetry**: `recordTelemetry()` records jumps and ticks every physics tick
- **Per-step telemetry**: `generateNavigationInputs()` records step completion when path steps finish

Each tick and step is recorded exactly once, providing accurate metrics for validation and analysis.
