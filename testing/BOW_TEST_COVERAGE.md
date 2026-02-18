# Comprehensive Bow Firing Test Coverage

This document describes the comprehensive test suite for arrow firing mechanics, covering all target elevation scenarios.

## Test File Location
`testing/bow_comprehensive_test.go`

## Test Cases

### 1. **TestBowFiring_LevelTarget** - Same Height as Bot
**Purpose**: Test standard horizontal firing
- **Target elevation**: 0 blocks (same height as bot)
- **Target distance**: 10 blocks horizontally
- **Expected behavior**: Arrow flies in a relatively flat trajectory and hits the target
- **Physics focus**: Tests baseline aiming with verticalDist = 0
- **Importance**: Validates basic aim calculations; should use 0-60° pitch range

### 2. **TestBowFiring_BelowTarget** - Target Below Bot
**Purpose**: Test downward firing
- **Target elevation**: -3 blocks below bot
- **Target distance**: 10 blocks horizontally
- **Expected behavior**: Arrow descends and hits the lower target
- **Physics focus**: Tests downward trajectory (verticalDist < 0)
- **Importance**: Validates trajectory calculation for targets on lower platforms
- **Pitch range**: -20° to 60° (allows downward shots)

### 3. **TestBowFiring_AboveTarget** - Target Above Bot
**Purpose**: Test upward firing with high-angle shots
- **Target elevation**: +3 blocks above bot
- **Target distance**: 10 blocks horizontally
- **Expected behavior**: Arrow must be launched at a steep angle to reach the target
- **Physics focus**: Tests upward trajectory (verticalDist > 0)
- **Importance**: Validates that arrows can hit targets above the bot
- **Pitch range**: 0° to 60° (only upward pitches)

### 4. **TestBowFiring_ShelfTarget** - CRITICAL: Target on Shelf (Descending Pass Only)
**Purpose**: Test the critical case where arrow passes through target height twice
- **Target elevation**: +2 blocks above bot
- **Target distance**: 8 blocks horizontally
- **Expected behavior**: Arrow must hit on the descending pass, NOT the ascending pass
- **Physics focus**: Tests trajectory validation fix for upward targets
- **Why critical**: At this distance and elevation, the arrow's trajectory causes:
  1. Arrow rises while traveling forward
  2. Arrow passes through target height WHILE STILL ASCENDING (vel.Y > 0) ← MUST BE SKIPPED
  3. Arrow continues rising to apex
  4. Arrow falls back down and passes through target height WHILE DESCENDING (vel.Y < 0) ← ONLY THIS COUNTS

**This test validates the fix**: `if verticalDist >= 0 && i > 0 && point.Vel.Y > 0 { continue }`

### 5. **TestBowFiring_VariousElevations** - Stress Test with Multiple Elevations
**Purpose**: Test hitting multiple targets at different elevations in sequence
- **Elevations tested**:
  - Level target (0 blocks)
  - 2 blocks below (-2)
  - 5 blocks below (-5)
  - 2 blocks above (+2)
  - 3 blocks above (+3)
- **Distance**: 10 blocks for all
- **Expected behavior**: All targets should be hit successfully
- **Importance**: Comprehensive regression test; validates consistency across all elevation types

## Test Infrastructure

### Helper Functions

**`setupTargetMechanismWithHeight()`**
- Places a target mechanism at a specific elevation
- Uses relative positioning: targetY = botY + heightDelta
- Returns glowstone coordinates for hit detection
- Builds supporting platform and clears obstacles

**`fireAtElevation()`**
- Fires arrow at target with specific height and distance
- Verifies hit by checking if glowstone was pushed by piston
- Analyzes trajectory from server logs
- Uses `assert` (not `require`) for hit verification to continue testing even if missed

**`verifyBlockAtPosition()`**
- Checks agent's world view for block at specific position
- Inherited from existing test infrastructure
- Used to detect if target block movement occurred

## Test Execution

### Running Specific Tests
```bash
# Run only level target test
go test -v ./testing -run "TestBowFiring_LevelTarget" -timeout 5m

# Run only shelf target test (critical for validation)
go test -v ./testing -run "TestBowFiring_ShelfTarget" -timeout 5m

# Run all comprehensive tests
go test -v ./testing -run "TestBowFiring_(Level|Below|Above|Shelf|Various)" -timeout 30m

# Run all elevation variations in one test
go test -v ./testing -run "TestBowFiring_VariousElevations" -timeout 7m
```

### Version Coverage
All tests run against `standardVersionTests` which covers multiple Minecraft versions:
- 1.21.1
- 1.21.5
- 1.21.8

## Validation Metrics

Each test validates:
1. **Hit Detection**: Glowstone moves (piston pushed by target)
2. **Physics Accuracy**: Trajectory analysis from logs (when available)
3. **Yaw Calculation**: Consistent formula across all elevations
4. **Pitch Calculation**: Correct angle selection for elevation
5. **Descending Pass Validation**: Critical for upward targets (ShelfTarget test)

## Expected Results

### Success Criteria
- ✓ Level target: Hit with low-angle shot (~30°)
- ✓ Below target: Hit with slightly downward angle (-5° to 10°)
- ✓ Above target: Hit with high-angle shot (30-45°)
- ✓ Shelf target: Hit only on descending pass (validates trajectory validation fix)
- ✓ Various elevations: All 5 targets hit in sequence

### Failure Indicators
- ✗ Any target missed: Physics calculation error
- ✗ Shelf target hit on ascending pass: Trajectory validation not working
- ✗ Inconsistent hits across versions: Version-specific bug
- ✗ Yaw error (target offset): Yaw calculation issue

## Edge Cases Covered

1. **Level firing**: Baseline case with verticalDist = 0
2. **Moderate downward**: verticalDist = -3 (between -5 and 0)
3. **Steep downward**: verticalDist = -5 (triggers downward pitch range)
4. **Moderate upward**: verticalDist = +2 to +3 (requires high-angle shot)
5. **Ascending/descending pass distinction**: ShelfTarget validates trajectory validation

## Regression Testing

These tests catch:
- Yaw calculation changes that affect horizontal aim
- Pitch calculation changes that affect vertical aim
- Trajectory validation bugs (especially double-pass issue)
- Power/hold duration calculation errors
- Inconsistencies between FireBowAt and FireBowAtDebug

## Future Extensions

Possible additional tests:
- **Distance variations**: 5, 15, 20, 30 blocks at same elevation
- **Extreme elevations**: ±10 blocks elevation change
- **Wind resistance**: If added to physics
- **Moving targets**: Dynamic target tracking
- **Multiple targets**: Sequential rapid firing
- **Accuracy measurements**: Hit distance from target center
