# Vehicle Control Integration Tests

This directory contains comprehensive integration tests for Phase 3.2 vehicle control mechanics implementation.

## Test Structure

### Common Test Utilities (`common_test.go`)

Provides `VehicleTestHelper` for setup and teardown of test environments:

- **NewVehicleTestHelper**: Creates a test framework, starts a Minecraft server, spawns an agent
- **SummonBoat**: Summons a boat at specified location with variant
- **SummonHorse**: Summons a tamed and saddled horse
- **Mount/Dismount**: Commands agent to mount/dismount entities
- **Manual Movement**: Controls steering via `SetManualThrottle`, `EnterManualMode`, `ExitManualMode`
- **Tracking Utilities**: Waits for entities to be tracked, monitors state changes
- **Helper Methods**: Distance calculation, position queries

## Test Files

### 1. Vehicle Steering Tests (`vehicle_steering_test.go`)

**Phase 3.2.1 Implementation Tests**

#### TestBoatSteering
- Summons a boat on water
- Mounts agent on boat
- Tests forward, backward, left, right steering
- Tests stop (zero throttle)
- Validates agent movement matches steering input
- Verifies dismount

#### TestVehicleSteeringInputs
- Tests multiple throttle combinations
- Validates independent X (lateral) and Z (forward) axes
- Tests diagonal movement (forward-right, forward-left)
- Ensures movement occurs for non-zero throttle
- Ensures minimal movement for zero throttle

### 2. Horse Jumping Tests (`horse_jumping_test.go`)

**Phase 3.2.2 Implementation Tests**

#### TestHorseJumping
- Summons a tamed, saddled horse
- Mounts agent on horse
- Tests jumping with power 100 (maximum)
- Tests jumping with power 50 (medium)
- Tests jumping with power 0 (minimum)
- Validates agent jumps upward (Y position increases)
- Verifies dismount after testing

#### TestJumpVehiclePowerRange
- Tests power parameter clamping
- Validates power values 0-100 are accepted
- Validates out-of-range values (>100, <0) are handled
- Ensures no errors on parameter extremes

### 3. Entity Attributes Tests (`entity_attributes_test.go`)

**Phase 3.2.3 Implementation Tests**

#### TestEntityAttributeTracking
- Summons a horse (has attributes like max health)
- Waits for entity to be tracked
- Verifies health and max health are populated
- Validates attribute access works post-tracking

#### TestHorseAttributesAfterMounting
- Tracks horse before mounting
- Mounts horse
- Verifies attributes remain consistent during mount
- Ensures attribute tracking continues while mounted

#### TestMultipleEntityAttributes
- Summons multiple entity types (horse, cow, zombie)
- Tracks each entity
- Queries entity attributes by type
- Validates attribute access for different entity types

### 4. Boat Metadata Tests (`boat_metadata_test.go`)

**Phase 3.2.4 Implementation Tests**

#### TestBoatVariantTracking
- Tests all 9 boat variants (oak, spruce, birch, jungle, acacia, dark_oak, mangrove, bamboo, cherry)
- Summons boat with specific variant
- Waits for entity tracking
- Validates boat is tracked with correct metadata

#### TestBoatPaddleTracking
- Summons boat and mounts it
- Tests left paddle activation (steer left)
- Tests right paddle activation (steer right)
- Tests paddle deactivation (zero throttle)
- Verifies paddle state updates during steering

#### TestBoatMetadataConsistency
- Summons boat
- Samples boat metadata multiple times over 2.5 seconds
- Verifies entity ID remains constant
- Validates position updates are tracked
- Ensures metadata persistence

## Running Tests

```bash
# Run all vehicle tests
go test ./testing/vehicles/... -v

# Run specific test
go test ./testing/vehicles/... -run TestBoatSteering -v

# Run with specific Minecraft version
go test ./testing/vehicles/... -run TestBoatSteering/1.21.5 -v

# Run with output from failed tests
go test ./testing/vehicles/... -v 2>&1 | tail -100
```

## Test Requirements

- Docker installed and running (for Minecraft server)
- 1GB+ free RAM (server requires 768MB)
- Network access (for downloading server JARs and mc-data-gen)

## Test Coverage

| Phase | Feature | Test | Status |
|-------|---------|------|--------|
| 3.2.1 | Vehicle Steering | TestBoatSteering | ✅ |
| 3.2.1 | Steering Inputs | TestVehicleSteeringInputs | ✅ |
| 3.2.2 | Horse Jump | TestHorseJumping | ✅ |
| 3.2.2 | Jump Power Range | TestJumpVehiclePowerRange | ✅ |
| 3.2.3 | Entity Attributes | TestEntityAttributeTracking | ✅ |
| 3.2.3 | Attributes on Mount | TestHorseAttributesAfterMounting | ✅ |
| 3.2.3 | Multi-Entity Attributes | TestMultipleEntityAttributes | ✅ |
| 3.2.4 | Boat Variants | TestBoatVariantTracking | ✅ |
| 3.2.4 | Boat Paddles | TestBoatPaddleTracking | ✅ |
| 3.2.4 | Metadata Consistency | TestBoatMetadataConsistency | ✅ |

## Test Data

- Tests run against multiple Minecraft versions (1.21.1 - 1.21.11)
- Server configurations are cached in `.server_cache/`
- Agent logs are written to `./logs/agents/` with timestamp

## Troubleshooting

### Test Timeouts
If tests timeout:
1. Increase available memory (Docker needs 1GB+ for server)
2. Check network connectivity (mc-data-gen downloads may be slow)
3. Verify Docker is running: `docker ps`

### Server Won't Start
1. Clear cache: `rm -rf .server_cache/`
2. Verify Docker memory: `docker info | grep Memory`
3. Check disk space for JAR extraction

### Agent Won't Mount
1. Verify entity was summoned (check server logs)
2. Ensure position is correct (water for boats, solid ground for horses)
3. Check agent position update delays

## Notes

- Tests are integration tests requiring a running Minecraft server
- Each test runs for ~10-15 seconds
- Full test suite runs against all supported versions
- Tests use `testify/require` for assertions
- Manual mode is required for steering; normal movement executor bypasses steering
