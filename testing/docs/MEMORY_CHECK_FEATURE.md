# Memory Availability Check Feature

## Overview

The testing framework now includes automatic memory availability checking before starting server containers. This **prevents OOM kills** by refusing to start servers when insufficient memory is available.

## How It Works

### Automatic Check on StartServer

Every time `StartServer()` is called, the framework:

1. **Reads system memory** from `/proc/meminfo`
2. **Calculates required memory**:
   - Server estimate (from `cfg.Memory` + 50% overhead)
   - Minimum free buffer (default: 2GB)
3. **Compares** available vs required
4. **Rejects** server start if insufficient memory
5. **Logs** memory check results

### Example Output

**Success**:
```
Memory check: 8192 MB available, 3584 MB required (server: 1536 MB + buffer: 2048 MB) - OK
Server started on 127.0.0.1:25565
```

**Failure**:
```
Error: insufficient memory: insufficient memory: need 3584 MB (server: 1536 MB + buffer: 2048 MB), have 2048 MB available
Suggestions:
  - Close other applications to free memory
  - Reduce server memory (cfg.Memory)
  - Wait for other containers to finish
  - Run tests sequentially instead of parallel
```

## Configuration

### Default Settings

```go
cfg := DefaultServerConfig()
// Automatically includes:
// - Memory: "1G" (Java heap)
// - MinFreeMemoryMB: 2048 (2GB buffer)
// - SkipMemoryCheck: false (enabled)
// - EstimatedMemoryMB: 0 (auto-calculated)
```

### Customization Options

#### 1. Reduce Minimum Free Buffer

For systems with limited RAM:

```go
cfg := DefaultServerConfig()
cfg.MinFreeMemoryMB = 1024  // Only 1GB buffer instead of 2GB
```

#### 2. Reduce Server Memory

```go
cfg := DefaultServerConfig()
cfg.Memory = "768M"  // Use less memory
// Estimated memory will be auto-calculated: 768M * 1.5 = ~1152 MB
```

#### 3. Override Estimation

If you know the exact memory usage:

```go
cfg := DefaultServerConfig()
cfg.Memory = "1G"
cfg.EstimatedMemoryMB = 1800  // Override auto-calculation
```

#### 4. Skip Memory Check (Not Recommended)

Only use in controlled environments:

```go
cfg := DefaultServerConfig()
cfg.SkipMemoryCheck = true  // WARNING: Can cause OOM
```

## Memory Calculation

### Estimated Server Memory

The framework estimates server memory with 50% overhead:

| Config Value | Java Heap | Estimated Total | Calculation |
|--------------|-----------|-----------------|-------------|
| "512M" | 512 MB | 768 MB | 512 * 1.5 |
| "768M" | 768 MB | 1152 MB | 768 * 1.5 |
| "1G" | 1024 MB | 1536 MB | 1024 * 1.5 |
| "2G" | 2048 MB | 3072 MB | 2048 * 1.5 |

**Why 50% overhead?**
- Container OS processes: ~100-200 MB
- Java JVM overhead: ~20-30% of heap
- Network buffers, caches: ~100-200 MB

### Total Required Memory

```
Total Required = Estimated Server Memory + Minimum Free Buffer

Default (1G server):
  1536 MB (server) + 2048 MB (buffer) = 3584 MB (~3.5 GB)
```

### Available Memory

The framework reads `MemAvailable` from `/proc/meminfo`, which represents:
- Free memory
- Reclaimable cache/buffers
- **Not** swap space

This is the "true available" memory that can be allocated without causing swapping.

## Examples

### Example 1: Normal System (8GB RAM, 6GB Available)

```go
framework, _ := NewFramework()
cfg := DefaultServerConfig()  // 1G server, 2GB buffer

inst, err := framework.StartServer(ctx, cfg)
// ✅ Success: 6144 MB available > 3584 MB required
```

**Output**:
```
Memory check: 6144 MB available, 3584 MB required (server: 1536 MB + buffer: 2048 MB) - OK
```

### Example 2: Low Memory System (4GB RAM, 2GB Available)

```go
framework, _ := NewFramework()
cfg := DefaultServerConfig()  // 1G server, 2GB buffer

inst, err := framework.StartServer(ctx, cfg)
// ❌ Error: 2048 MB available < 3584 MB required
```

**Output**:
```
Error: insufficient memory: need 3584 MB (server: 1536 MB + buffer: 2048 MB), have 2048 MB available
Suggestions:
  - Close other applications to free memory
  - Reduce server memory (cfg.Memory)
  - Wait for other containers to finish
  - Run tests sequentially instead of parallel
```

**Solution - Reduce Requirements**:
```go
cfg := DefaultServerConfig()
cfg.Memory = "512M"           // Reduce server memory
cfg.MinFreeMemoryMB = 1024    // Reduce buffer

inst, err := framework.StartServer(ctx, cfg)
// ✅ Success: 2048 MB available > 1792 MB required (768 + 1024)
```

### Example 3: Multiple Containers

Running 2 tests in parallel on 8GB system:

```go
// First test
cfg1 := DefaultServerConfig()
inst1, err := framework.StartServer(ctx, cfg1)
// ✅ Success: 6144 MB available > 3584 MB required
// After start: ~2560 MB available (6144 - 3584)

// Second test (immediately after)
cfg2 := DefaultServerConfig()
inst2, err := framework.StartServer(ctx, cfg2)
// ❌ Error: 2560 MB available < 3584 MB required
```

**This is why PARALLEL=1 is recommended!**

### Example 4: Custom Configuration for Limited Systems

```go
// For 4GB RAM systems
cfg := ServerConfig{
    Version:         "1.21.5",
    Memory:          "512M",   // Minimal server memory
    MinFreeMemoryMB: 1024,     // 1GB buffer
    // Other defaults...
}

// Required: 768 MB (server) + 1024 MB (buffer) = 1792 MB
// Feasible on systems with 2GB+ available
```

## Integration with Tests

### In Test Code

```go
func TestSomething(t *testing.T) {
    framework, _ := NewFramework()

    // Default config includes memory check
    cfg := DefaultServerConfig()

    inst, err := framework.StartServer(ctx, cfg)
    if err != nil {
        // Will fail fast if insufficient memory
        t.Fatalf("Failed to start server: %v", err)
    }

    defer func() {
        cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()
        framework.StopServer(cleanupCtx, inst, true)
    }()

    // ... test code ...
}
```

### What Happens on Failure

The test **fails immediately** with a clear error message:

```
--- FAIL: TestSomething (0.01s)
    test.go:15: Failed to start server: insufficient memory: need 3584 MB (server: 1536 MB + buffer: 2048 MB), have 2048 MB available
    Suggestions:
      - Close other applications to free memory
      - Reduce server memory (cfg.Memory)
      - Wait for other containers to finish
      - Run tests sequentially instead of parallel
```

**Benefits**:
- ✅ Fails fast before allocating any resources
- ✅ No Docker containers created
- ✅ Clear error message with actionable suggestions
- ✅ No OOM kill (system remains stable)

## Monitoring

### Check Available Memory Before Tests

```bash
# Quick check
free -h

# Detailed check
cat /proc/meminfo | grep MemAvailable
```

### During Test Run

```bash
# Terminal 1: Run tests
cd testing
./run_tests.sh

# Terminal 2: Monitor memory
watch -n 1 'free -h && echo "Available: $(cat /proc/meminfo | grep MemAvailable)"'
```

### Expected Memory Pattern

```
Before test:  MemAvailable: 6144 MB
After check:  Memory check: 6144 MB available, 3584 MB required - OK
After start:  MemAvailable: ~2560 MB (6144 - 3584)
After stop:   MemAvailable: ~6144 MB (memory released)
```

## Limitations

### 1. Only Checks at Start Time

The check happens **once** when `StartServer()` is called. If memory becomes scarce during test execution (e.g., other processes consuming memory), the check won't catch it.

**Future Enhancement**: Periodic memory monitoring during test execution.

### 2. Estimate May Be Inaccurate

The 50% overhead is an estimate. Actual memory usage can vary based on:
- Minecraft version
- Plugin/mod load
- World size
- Player activity

**Mitigation**: Conservative estimates and 2GB buffer.

### 3. Doesn't Account for Agents

The check estimates server memory but doesn't include agent memory in the calculation.

**Current Workaround**: The 2GB buffer usually covers 2-4 agents (~500MB total).

**Future Enhancement**: Add agent count to memory calculation:
```go
cfg.EstimatedAgents = 2  // Each agent ~200-300 MB
```

### 4. Linux-Specific

Uses `/proc/meminfo` which is Linux-only. On other systems:
- macOS: Check returns error, continues with warning
- Windows: Check returns error, continues with warning

**Result**: Non-blocking on non-Linux systems.

## Troubleshooting

### Memory Check Fails Unexpectedly

**Symptom**: Error despite having "enough" RAM

**Possible Causes**:
1. Other containers/processes consuming memory
2. Cached memory not being reclaimable
3. Memory fragmentation

**Solutions**:
```bash
# Check actual available memory
cat /proc/meminfo | grep MemAvailable

# Check what's using memory
docker stats
ps aux --sort=-%mem | head -20

# Clear caches (if safe to do so)
sudo sync && sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'
```

### Memory Check Passes But OOM Still Occurs

**Symptom**: Check passes but test still gets killed

**Possible Causes**:
1. Estimate was too low (50% overhead insufficient)
2. Memory leak during test execution
3. Multiple agents using more than buffer

**Solutions**:
```go
// Increase buffer
cfg.MinFreeMemoryMB = 3072  // 3GB buffer

// Or reduce server memory
cfg.Memory = "768M"

// Or override estimate with known value
cfg.EstimatedMemoryMB = 2048  // 2GB (if you've measured actual usage)
```

### Want to Disable Check

**Not recommended**, but if needed:

```go
cfg := DefaultServerConfig()
cfg.SkipMemoryCheck = true
```

**Better approach**: Reduce requirements to pass check:
```go
cfg := DefaultServerConfig()
cfg.Memory = "512M"
cfg.MinFreeMemoryMB = 1024
```

## Summary

The memory availability check provides:

✅ **Fail-fast** - Immediate error before wasting time
✅ **Clear feedback** - Tells you exactly what's wrong
✅ **Actionable suggestions** - How to fix the problem
✅ **Prevents OOM** - System stays stable
✅ **Configurable** - Adjust for your system
✅ **Non-blocking failures** - Degrades gracefully on non-Linux

**Recommendation**: Keep memory check enabled with default settings for most cases. Only adjust if you have specific constraints or requirements.
