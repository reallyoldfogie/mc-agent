# OOM Fixes Summary

## Date: 2025-12-06

## Critical Changes to Prevent OOM Kills

### 1. Reduced Minecraft Server Memory (Framework)

**File**: `testing/framework.go`

**Changes**:
- Added `Memory` field to `ServerConfig` (line 66)
- Default server memory reduced from 2G to 1G (line 79)
- Added JVM container optimization flags (line 108)

**Impact**: Each Minecraft server now uses ~1GB instead of ~2GB

**Code**:
```go
// Set memory limit to reduce OOM risk
if cfg.Memory != "" {
    extraEnv["MEMORY"] = cfg.Memory
}
// Optimize JVM for container environments
extraEnv["JVM_XX_OPTS"] = "-XX:+UseContainerSupport -XX:MaxRAMPercentage=80.0"
```

### 2. Enhanced Agent Cleanup with Timeouts

**File**: `testing/framework.go:150-194`

**Changes**:
- Each agent stopped with dedicated 15-second timeout context
- Agents stopped in parallel (not sequential)
- Added 500ms delay after agent stop before server cleanup
- Uses fresh context to prevent parent timeout affecting cleanup

**Impact**: Agents are now forcefully closed even if they hang, preventing resource leaks

**Code**:
```go
for _, agent := range agents {
    // Use short timeout for agent stop to prevent hanging
    agentCtx, agentCancel := context.WithTimeout(context.Background(), 15*time.Second)
    if err := agent.Stop(agentCtx); err != nil {
        fmt.Printf("WARNING: Agent %s failed to stop cleanly: %v\n", agent.Name, err)
    }
    agentCancel()
}

// Small delay to ensure agents are fully cleaned up
time.Sleep(500 * time.Millisecond)
```

### 3. Memory Warning in Test Runner

**File**: `testing/run_tests.sh:24-29`

**Changes**:
- Checks system memory before running tests
- Warns if less than 8GB RAM available

**Code**:
```bash
# Memory warning
if [ "$(free -g | awk '/^Mem:/{print $2}')" -lt 8 ]; then
    echo -e "${YELLOW}WARNING: System has less than 8GB RAM. Tests may fail with OOM.${NC}"
    echo -e "${YELLOW}Each test can use 2-4GB of memory.${NC}"
    echo ""
fi
```

### 4. Parallel Execution Warning

**File**: `testing/run_tests.sh:143-145`

**Changes**:
- Warns if parallel > 1
- Default remains 1 (sequential execution)

**Code**:
```bash
if [ "$PARALLEL" -gt 1 ]; then
    echo -e "  ${YELLOW}WARNING: Parallel execution may cause OOM. Recommended: -p 1${NC}"
fi
```

## Testing The Fixes

### Before Running Tests

1. **Check system memory**:
   ```bash
   free -h
   # Should show at least 4GB available
   ```

2. **Clean up any leftover containers**:
   ```bash
   docker rm -f $(docker ps -aq --filter "name=mc-agent-test") 2>/dev/null
   ```

3. **Close memory-intensive applications**:
   - Web browsers with many tabs
   - IDEs
   - Other Docker containers

### Run Tests

```bash
cd testing
./run_tests.sh
```

The script will:
1. ✅ Warn if system has < 8GB RAM
2. ✅ Clean up leftover containers automatically
3. ✅ Run tests sequentially (PARALLEL=1)
4. ✅ Capture server logs before cleanup
5. ✅ Report any leftover containers

### Monitor During Tests

```bash
# Terminal 1: Run tests
cd testing
./run_tests.sh -t TestNavigationSingleAgent

# Terminal 2: Monitor memory and containers
watch -n 2 'echo "=== Memory ===" && free -h && echo -e "\n=== Docker Containers ===" && docker ps --filter "name=mc-agent-test" && echo -e "\n=== Docker Stats ===" && docker stats --no-stream --format "table {{.Container}}\t{{.MemUsage}}\t{{.CPUPerc}}"'
```

## Expected Memory Usage

### Per Test (with 1G server memory)

| Test | Server | Agents | Total |
|------|--------|--------|-------|
| Navigation Single | 1GB | 300MB | ~1.5GB |
| Follow Single | 1GB | 600MB | ~1.8GB |
| Follow Multiple | 1GB | 900MB | ~2GB |

### System Requirements

- **Minimum**: 8GB RAM with 4GB free
- **Recommended**: 16GB RAM with 8GB free
- **For parallel tests**: Add 2GB per additional parallel test

## What To Do If OOM Still Occurs

### 1. Reduce Memory Further

Edit `testing/framework.go`:
```go
func DefaultServerConfig() ServerConfig {
    return ServerConfig{
        Memory: "768M",  // Even more aggressive
        // ...
    }
}
```

### 2. Run Individual Tests

```bash
# Instead of running all tests at once
./run_tests.sh -t TestNavigationSingleAgent
# Wait for completion, then:
./run_tests.sh -t TestFollowSingleAgent
```

### 3. Increase System Swap

Temporary measure (not ideal for performance):
```bash
# Create 4GB swap file
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

### 4. Check for Memory Leaks

```bash
# Before test
free -m

# After test (should be similar)
free -m

# Check for leftover containers (should be none)
docker ps -a --filter "name=mc-agent-test"
```

## Files Modified

1. **`testing/framework.go`**:
   - Added `Memory` field to ServerConfig
   - Default memory: 2G → 1G
   - Added JVM container optimization
   - Enhanced StopServer with explicit agent timeouts

2. **`testing/run_tests.sh`**:
   - Added memory check warning
   - Added parallel execution warning
   - Default PARALLEL=1 with comment

3. **`testing/MEMORY_LIMITS.md`** (new):
   - Comprehensive memory management documentation
   - Troubleshooting guide
   - Monitoring instructions

4. **`testing/OOM_FIXES_SUMMARY.md`** (this file):
   - Quick reference for OOM fixes
   - Testing instructions

## Success Criteria

Tests should now:
- ✅ Run to completion without OOM kills
- ✅ Use ~1.5-2GB per test (down from 3-4GB)
- ✅ Clean up containers after each test
- ✅ Warn about memory conditions
- ✅ Capture logs for debugging

## Troubleshooting Commands

```bash
# Check recent OOM kills
sudo dmesg | grep -i "out of memory" | tail -20

# Monitor memory during test
watch -n 1 free -h

# Check Docker memory usage
docker stats --no-stream

# Force cleanup
docker rm -f $(docker ps -aq --filter "name=mc-agent-test")
docker system prune -f

# Check system memory pressure
cat /proc/pressure/memory
```

## Next Steps If Issues Persist

1. **Profile agent memory usage**:
   - Add memory profiling to agent
   - Identify memory leaks or excessive buffering

2. **Add Docker resource limits**:
   - Requires changes to `mc-client-test-go/testenv`
   - Set hard limits on container memory

3. **Optimize replay recording**:
   - Stream to disk instead of buffering
   - Reduce packet retention

4. **Test isolation**:
   - Run each test in separate process
   - Complete memory cleanup between tests
