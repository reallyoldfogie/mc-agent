# Structure Generation Control

## Overview

The testing framework now supports controlling Minecraft structure generation (villages, temples, etc.) via environment variables. This is particularly useful for flat world tests where structures can interfere with predictable terrain.

## Default Behavior

For flat worlds (`WorldGenFlat` and `WorldGenControlled`), the framework automatically disables structure generation by setting `GENERATE_STRUCTURES=false`. This ensures:
- Predictable, empty terrain for testing
- No unexpected obstacles from villages or temples
- Consistent test results across runs

For random/default worlds (`WorldGenRandom`), structure generation follows the server's default behavior (typically enabled).

## Controlling Structure Generation

### Method 1: Environment Variable (Recommended)

Set `MC_AGENT_GENERATE_STRUCTURES` when running tests:

```bash
# Disable structures (default for flat worlds)
MC_AGENT_GENERATE_STRUCTURES=false go test ./testing -v

# Enable structures in flat worlds
MC_AGENT_GENERATE_STRUCTURES=true go test ./testing -v
```

### Method 2: Explicit Configuration

Override in individual tests by setting `ExtraEnv`:

```go
serverCfg := FlatWorldServerConfig()
serverCfg.ExtraEnv = map[string]string{
    "GENERATE_STRUCTURES": "true",  // Explicitly enable
}
```

## Priority Order

The framework resolves `GENERATE_STRUCTURES` in this order (highest priority first):

1. **Explicit in test code**: Value set in `serverCfg.ExtraEnv["GENERATE_STRUCTURES"]`
2. **Environment variable**: Value from `MC_AGENT_GENERATE_STRUCTURES`
3. **Framework default**: `false` for flat worlds only

## Use Cases

### Disable Structures (Default)
Perfect for most automated tests requiring predictable terrain:
```bash
go test ./testing -v
```

### Enable Structures for Debugging
Test pathfinding through naturally generated villages:
```bash
MC_AGENT_GENERATE_STRUCTURES=true go test ./testing -run TestNavigationSingleAgent -v
```

### Mixed Configuration
Some tests with structures, others without:
```go
// Test 1: No structures (uses default)
serverCfg1 := FlatWorldServerConfig()
inst1, _ := framework.StartServer(ctx, serverCfg1)

// Test 2: With structures (explicit override)
serverCfg2 := FlatWorldServerConfig()
serverCfg2.ExtraEnv = map[string]string{
    "GENERATE_STRUCTURES": "true",
}
inst2, _ := framework.StartServer(ctx, serverCfg2)
```

## Implementation Details

The logic is in `framework.go` (lines 261-271):
- Only applies to flat worlds (`WorldGenFlat` or `WorldGenControlled`)
- Checks `MC_AGENT_GENERATE_STRUCTURES` environment variable
- Falls back to `false` if not set
- Respects explicit `ExtraEnv` settings from test code

## Related Files

- `testing/framework.go`: Core implementation
- `testing/README.md`: Usage documentation
- `testing/container_suite_test.go`: Example test suite (now uses framework default)

## Migration Notes

Tests previously hardcoded `GENERATE_STRUCTURES` in `ExtraEnv`. This has been removed to use the framework default. If your tests require structures, use one of the override methods above.
