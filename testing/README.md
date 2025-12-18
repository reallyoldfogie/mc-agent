# Minecraft Agent Integration Testing Framework

A comprehensive automated testing framework for validating Minecraft agent behaviors in a controlled server environment.

## Overview

This framework provides:
- Automated Minecraft server management via Docker
- Multi-agent spawning and lifecycle management
- Position tracking and validation
- Navigation and following behavior tests
- ReplayMod recording for all tests
- RCON-based world state control

## Architecture

### Components

1. **Framework** (`framework.go`)
   - Server lifecycle management
   - Agent spawning and orchestration
   - Position tracking system
   - Movement timeout calculations

2. **Navigation Tests** (`navigation_test.go`)
   - Single destination navigation
   - Multi-waypoint navigation
   - Vertical movement
   - Obstacle avoidance (planned)

3. **Follow Tests** (`follow_test.go`)
   - Single agent following
   - Multi-agent following
   - Dynamic target tracking
   - Stop/resume following

## Prerequisites

- Go 1.22+
- Docker installed and running
- Network access for pulling Minecraft server images
- Sufficient resources to run Minecraft servers (recommended: 4GB+ RAM)

## Installation

1. Ensure dependencies are available:
```bash
cd /home/reallyoldfogie/src/github.com/reallyoldfogie/mc-agent
go mod tidy
```

2. Verify Docker is running:
```bash
docker info
```

3. Pull Minecraft server image (optional, will auto-pull on first run):
```bash
docker pull itzg/minecraft-server:latest
```

## Running Tests

### Basic Usage

**Using the test runner script (recommended)**:
```bash
# From project root
./testing/run_tests.sh

# Or from testing directory
cd testing
./run_tests.sh
```

**Using go test directly**:
```bash
# From project root
go test ./testing -tags=integration -v

# From testing directory
cd testing
go test -tags=integration -v .
```

Run specific test:
```bash
# Using script
./testing/run_tests.sh -t TestNavigationSingleAgent

# Using go test
go test ./testing -tags=integration -run TestNavigationSingleAgent -v
```

Run tests with timeout:
```bash
go test ./testing -tags=integration -timeout 30m -v
```

### Parallel Execution

Run tests in parallel (caution: resource-intensive):
```bash
go test ./testing -tags=integration -v -parallel 2
```

### Test Output

Tests produce:
- Console output with detailed progress logging
- ReplayMod recordings in `./testing/replays/`
- Server logs available via Docker

## Test Cases

### Navigation Tests

#### TestNavigationSingleAgent
Tests basic pathfinding to a single destination.
- **Duration**: ~5-10 minutes
- **Validates**: Position accuracy within 1 block
- **Replay**: `nav_single_YYYYMMDD_HHMMSS.mcpr`

#### TestNavigationMultipleDestinations
Tests sequential waypoint navigation in a square pattern.
- **Duration**: ~10-15 minutes
- **Validates**: Arrival at each waypoint
- **Replay**: `nav_waypoints_YYYYMMDD_HHMMSS.mcpr`

#### TestNavigationVerticalMovement
Tests navigation with elevation changes.
- **Duration**: ~5-10 minutes
- **Validates**: Horizontal progress ≥50%
- **Replay**: `nav_vertical_YYYYMMDD_HHMMSS.mcpr`

### Follow Tests

#### TestFollowSingleAgent
Tests one agent following another to a destination.
- **Duration**: ~10-15 minutes
- **Validates**:
  - Follower stays within 5 blocks of leader
  - Follower travels ≥50% of leader's distance
- **Replays**:
  - Leader: `follow_test_leader_YYYYMMDD_HHMMSS.mcpr`
  - Follower: `follow_test_follower_YYYYMMDD_HHMMSS.mcpr`

#### TestFollowMultipleAgents
Tests three agents following a single leader.
- **Duration**: ~15-20 minutes
- **Validates**: All followers within 7 blocks of leader
- **Replays**: One per agent (4 total)

#### TestFollowDynamicTarget
Tests following a leader that changes direction (zigzag pattern).
- **Duration**: ~15-20 minutes
- **Validates**: Follower stays within 8 blocks despite direction changes
- **Replays**: Leader and follower

#### TestFollowStopCommand
Tests that followers can stop following on command.
- **Duration**: ~5-10 minutes
- **Validates**: Follower remains stationary after stop command
- **Replays**: Leader and follower

## Configuration

### Server Configuration

Customize server settings in tests:
```go
serverCfg := DefaultServerConfig()
serverCfg.Version = "1.21.5"
serverCfg.Difficulty = "peaceful"
serverCfg.GameMode = "creative"
serverCfg.PullImage = true // force pull latest image
serverCfg.ExtraEnv = map[string]string{
    // World generation
    "SEED": "12345678",              // Fixed seed for consistent terrain
    "LEVEL_TYPE": "flat",            // flat, default, large_biomes, amplified
    "GENERATE_STRUCTURES": "false",  // Disable villages, temples, etc.

    // View settings
    "VIEW_DISTANCE": "16",           // Render distance (2-32 chunks)
    "SIMULATION_DISTANCE": "10",     // Simulation distance (3-32 chunks)

    // Game rules
    "SPAWN_PROTECTION": "0",         // Disable spawn protection
    "ALLOW_FLIGHT": "true",          // Allow flight in survival
    "FORCE_GAMEMODE": "true",        // Force gamemode on join
    "MAX_PLAYERS": "10",             // Max player count
}
```

**Common Server Environment Variables**:

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `SEED` | World seed for generation | Random | `"0"`, `"12345678"` |
| `LEVEL_TYPE` | World type | `"default"` | `"flat"`, `"amplified"` |
| `VIEW_DISTANCE` | Render distance (chunks) | `10` | `"16"`, `"20"` |
| `SIMULATION_DISTANCE` | Simulation distance | `10` | `"8"`, `"12"` |
| `SPAWN_PROTECTION` | Spawn protection radius | `16` | `"0"` (disable) |
| `GENERATE_STRUCTURES` | Generate structures | `true` | `"false"` |
| `ALLOW_FLIGHT` | Allow flight mode | `false` | `"true"` |
| `FORCE_GAMEMODE` | Force gamemode on join | `false` | `"true"` |
| `MAX_TICK_TIME` | Max ms per tick | `60000` | `"120000"` |

See [itzg/docker-minecraft-server](https://github.com/itzg/docker-minecraft-server) for all available options.

**Example: Flat World for Consistent Testing**
```go
serverCfg := DefaultServerConfig()
serverCfg.ExtraEnv = map[string]string{
    "SEED": "0",
    "LEVEL_TYPE": "flat",
    "VIEW_DISTANCE": "12",
    "SPAWN_PROTECTION": "0",
    "GENERATE_STRUCTURES": "false",
}
```

### Agent Configuration

Customize agent behavior:
```go
agentCfg := DefaultAgentConfig("BotName", serverAddress, version)
agentCfg.EnablePathfinding = true
agentCfg.EnableFollowing = true
agentCfg.EnableReplay = true
agentCfg.ReplayOutput = "./custom_replay.mcpr"
agentCfg.MCDataGenPath = "/custom/path/to/mc-data-gen"
```

### Position Tracking

Adjust tracking precision:
```go
tracker := NewPositionTracker(inst, 250*time.Millisecond) // faster polling
err := tracker.WaitForPosition(ctx, "BotName", target, 0.5, timeout) // tighter tolerance
```

### Movement Timeouts

Timeouts are calculated based on distance and walking speed:
```go
// Default: distance / 4.317 m/s * 1.5 (50% buffer)
timeout := CalculateMovementTimeout(distance)

// Custom timeout
timeout = time.Duration(customSeconds) * time.Second
```

## Command Syntax

Commands are sent via RCON chat with agent-specific prefixes:

```
>>>AgentName<<< command args
```

Examples:
- `>>>TestBot<<< moveTo 100 64 200` - Navigate to coordinates
- `>>>Follower<<< follow Leader` - Start following another agent
- `>>>Follower<<< stopFollow` - Stop following
- `>>>TestBot<<< pos` - Get current position
- `>>>TestBot<<< help` - List available commands

## Replay Analysis

ReplayMod recordings can be analyzed using:

1. **ReplayMod Minecraft Client**
   - Load `.mcpr` files in Minecraft with ReplayMod installed
   - View agent perspectives, paths, and interactions

2. **Programmatic Analysis**
   - Use `mc-replay-go` package to parse `.mcpr` files
   - Extract position data, timing information

## Troubleshooting

### Server Startup Failures

**Issue**: Server fails to start or times out
- **Solution**: Increase `StartTimeout` in `ServerConfig`
- **Check**: Docker resources, image availability
- **Logs**: `docker logs <container_name>`

### Agent Connection Failures

**Issue**: Agents fail to connect to server
- **Solution**: Wait longer before spawning agents (increase sleep time)
- **Check**: Server readiness, port mapping
- **Debug**: Check RCON connectivity

### Position Tracking Failures

**Issue**: `WaitForPosition` times out
- **Possible causes**:
  - Agent stuck on obstacle
  - Pathfinding failed
  - Timeout too short for distance
- **Debug**:
  - Check replay recordings
  - Verify terrain is navigable
  - Increase timeout multiplier

### Test Hangs

**Issue**: Test never completes
- **Solution**: Use `-timeout` flag with sufficient duration
- **Check**: Agent Done() channels, context cancellation
- **Force stop**: Ctrl+C, then manually stop Docker containers

### Cleanup Issues

**Issue**: Containers not removed after tests
- **Manual cleanup**:
```bash
docker ps -a | grep mc-agent-test
docker rm -f <container_id>
```

## Performance Considerations

### Resource Usage

Each test server requires:
- CPU: ~2 cores
- RAM: ~2-4 GB
- Disk: ~500 MB (temporary)

Limit parallel tests based on available resources:
```bash
go test ./testing -tags=integration -parallel 1 -v
```

### Test Duration

Total suite duration: ~60-90 minutes (serial)

Optimize by:
- Running subset of tests: `-run TestPattern`
- Reducing timeouts (may cause false failures)
- Using local server images (avoid re-pulling)

## Extending the Framework

### Adding New Tests

1. Create test function in appropriate file:
```go
func TestNewBehavior(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
    defer cancel()

    framework, err := NewFramework()
    require.NoError(t, err)

    // ... test implementation
}
```

2. Follow existing patterns:
   - Start server with `StartServer()`
   - Spawn agents with `SpawnAgent()`
   - Use RCON commands for control
   - Track positions with `PositionTracker`
   - Verify with assertions

### Adding Custom Assertions

Create helper functions for common validations:
```go
func assertWithinDistance(t *testing.T, pos1, pos2 Position, maxDist float64) {
    dist := pos1.Distance(pos2)
    assert.LessOrEqual(t, dist, maxDist,
        "Distance %.2f exceeds max %.2f", dist, maxDist)
}
```

### World Setup Helpers

Use RCON to configure test environments:
```go
// Create obstacle
helper.SetBlock(ctx, x, y, z, "minecraft:stone", "replace")

// Teleport agent
helper.Teleport(ctx, "AgentName", x, y, z)

// Set environment
helper.SetTime(ctx, "noon")
helper.SetWeather(ctx, "clear")
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Integration Tests
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      - name: Run integration tests
        run: |
          go test ./testing -tags=integration -v -timeout 60m
      - uses: actions/upload-artifact@v3
        if: always()
        with:
          name: replays
          path: testing/replays/*.mcpr
```

## Best Practices

1. **Always use timeouts**: Context timeouts prevent hanging tests
2. **Clean up resources**: Use defer for server/agent cleanup
3. **Enable replay recording**: Essential for debugging failures
4. **Log extensively**: Use `t.Logf()` for progress tracking
5. **Validate incrementally**: Check positions at multiple points
6. **Handle flakiness**: Use reasonable tolerances and retries
7. **Isolate tests**: Each test should be independent
8. **Monitor resources**: Don't run too many parallel tests

## Known Limitations

- **Terrain dependency**: Some tests require specific terrain (flat, navigable)
- **Version compatibility**: Tested primarily on 1.21.5
- **Docker requirement**: No support for bare-metal servers
- **Offline mode only**: Tests use offline authentication
- **Single platform**: Developed/tested on Linux (WSL2)

## Future Enhancements

- [ ] Obstacle course navigation tests
- [ ] Multi-dimensional following (overworld → nether)
- [ ] Combat behavior validation
- [ ] Resource gathering tests
- [ ] Custom world generation for tests
- [ ] Performance benchmarking
- [ ] Cross-version compatibility matrix
- [ ] Flaky test detection and retry logic

## Support

For issues or questions:
- Check CLAUDE.md for project architecture
- Review agent package documentation
- Examine replay recordings for debugging
- Consult mc-client-test-go README for server management details

## License

Same as parent project (mc-agent).
