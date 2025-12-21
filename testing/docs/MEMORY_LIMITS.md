# Memory Limits and Resource Management

## Problem

Tests are being killed by the OS with OOM (Out of Memory) errors:
```
[384401.397166] oom-kill:constraint=CONSTRAINT_NONE,nodemask=(null),cpuset=...,mems_allowed=0,global_oom,task_memcg=/init.scope,task=testing.test,pid=1983948,uid=1000
[384401.397265] Out of memory: Killed process 1983948 (testing.test) total-vm:4022960kB, anon-rss:1690160kB, file-rss:0kB, shmem-rss:0kB, UID:1000 pgtables:3528kB oom_score_adj:0
```

The test process consumed ~4GB of virtual memory and ~1.7GB of resident memory before being killed.

## Root Causes

1. **Minecraft servers are memory-hungry**:
   - Default Java heap: 1-2GB
   - Each test spawns a new Docker container with a Minecraft server
   - Multiple tests running means multiple servers in memory

2. **Agent memory accumulation**:
   - Each agent maintains chunk data, entity tracking, replay buffers
   - Multiple agents per test multiply memory usage
   - Replay recording buffers packet data in memory

3. **No resource limits**:
   - Docker containers have no memory limits
   - Test processes have no memory limits
   - Can consume all available system memory

4. **Cleanup timing**:
   - Agents weren't being stopped with explicit timeouts
   - No delay between agent stop and server stop
   - Resources may not be fully released before next test starts

## Solutions Implemented

### 1. Enhanced Agent Cleanup in StopServer

**File**: `testing/framework.go:150-194`

**Changes**:
- Agents now stopped with dedicated 15-second timeout context
- Stops agents in parallel to avoid sequential timeout accumulation
- Added 500ms delay after all agents stop before server cleanup
- Uses fresh context to prevent parent timeout affecting cleanup

**Code**:
```go
// Stop all agents first (CRITICAL: must close before server to finalize replays)
inst.mu.Lock()
agents := make([]*ManagedAgent, len(inst.Agents))
copy(agents, inst.Agents)
inst.mu.Unlock()

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

### 2. Pre-test Container Cleanup

**File**: `testing/run_tests.sh:89-99`

**Changes**:
- Automatically removes leftover `mc-agent-test-*` containers before test run
- Prevents accumulated containers from previous runs
- Reports how many containers were cleaned up

### 3. Reduced Test Parallelism

**Recommendation**: Run tests sequentially (one at a time)

**File**: `testing/run_tests.sh`

**Usage**:
```bash
# Run sequentially (default)
./run_tests.sh

# Or explicitly set parallel=1
./run_tests.sh -p 1
```

## Recommended Workarounds

Until Docker resource limits can be added to testenv, use these approaches:

### 1. Run Tests Sequentially

Don't use `-p` flag or set it to 1:
```bash
cd testing
./run_tests.sh -p 1
```

### 2. Run Individual Test Suites

Run test files one at a time:
```bash
./run_tests.sh -t TestNavigationSingleAgent
./run_tests.sh -t TestFollowSingleAgent
# etc.
```

### 3. Increase System Memory

If possible, run tests on a system with more available RAM:
- Minimum recommended: 8GB RAM
- Comfortable: 16GB RAM
- Each Minecraft server container: ~1-2GB
- Each agent: ~100-500MB
- Test overhead: ~500MB

### 4. Reduce Minecraft Server Memory

Can be configured via environment variables in ServerConfig:

**File**: `testing/framework.go` (in StartServer)

```go
extraEnv := cfg.ExtraEnv
if extraEnv == nil {
    extraEnv = make(map[string]string)
}
// Limit Java heap to 1GB (instead of default 2GB)
extraEnv["MEMORY"] = "1G"
extraEnv["JVM_XX_OPTS"] = "-XX:MaxRAMPercentage=80.0"
```

### 5. Add Memory Limits to Docker (requires testenv changes)

**Future Enhancement** - Would need to update `mc-client-test-go/testenv/manager.go`:

```go
hostCfg := &container.HostConfig{
    PortBindings: portBindings,
    Resources: container.Resources{
        Memory:     2147483648, // 2GB limit
        MemorySwap: 2147483648, // Disable swap
        CPUShares:  512,        // Limit CPU
    },
}
```

## Memory Usage Guidelines

### Expected Memory Usage Per Test

- **Single Agent Navigation Test**:
  - Minecraft server: 1-2GB
  - Agent: 200-500MB
  - Test overhead: 200MB
  - **Total: ~2-3GB**

- **Follow Test (2 Agents)**:
  - Minecraft server: 1-2GB
  - Agents (2x): 400MB-1GB
  - Test overhead: 200MB
  - **Total: ~2.5-4GB**

- **Multiple Agents Test (3+ Agents)**:
  - Minecraft server: 1-2GB
  - Agents (3x): 600MB-1.5GB
  - Test overhead: 200MB
  - **Total: ~3-5GB**

### System Requirements

**Minimum** (running tests sequentially):
- RAM: 8GB
- Available: 4GB after OS/other processes

**Recommended**:
- RAM: 16GB
- Available: 8GB after OS/other processes

**For Parallel Tests** (not recommended currently):
- RAM: 16GB+ (add 3-4GB per parallel test)

## Monitoring Memory Usage

### During Test Run

```bash
# Terminal 1: Run tests
cd testing
./run_tests.sh

# Terminal 2: Monitor memory
watch -n 1 'free -h && docker stats --no-stream --format "table {{.Container}}\t{{.MemUsage}}\t{{.CPUPerc}}"'
```

### Check for Memory Leaks

```bash
# Before tests
docker stats --no-stream

# During tests
watch -n 5 docker stats --no-stream

# After tests (should be empty)
docker ps -a --filter "name=mc-agent-test"
```

## Troubleshooting

### Test Killed with "signal: killed"

**Symptom**: Test terminates mid-execution with no error message

**Cause**: OS OOM killer terminated the process

**Check**:
```bash
# Check kernel logs
sudo dmesg | grep -i "out of memory" | tail -20
sudo journalctl -k | grep -i "killed process" | tail -20
```

**Solution**:
1. Free up system memory (close browsers, IDEs, etc.)
2. Run tests individually instead of in suite
3. Reduce Minecraft server memory (see workaround #4)
4. Increase system swap space (temporary fix)

### Leftover Containers

**Symptom**: `run_tests.sh` reports removing many leftover containers

**Cause**: Previous test runs were OOM killed before cleanup

**Solution**: Automatic - script now cleans up before each run

**Manual Cleanup**:
```bash
docker rm -f $(docker ps -aq --filter "name=mc-agent-test")
```

### Slow Tests

**Symptom**: Tests take very long, high memory usage

**Cause**: System swapping due to low memory

**Check**:
```bash
free -h
vmstat 1 5  # Check "si" (swap in) column
```

**Solution**: Free up memory or increase RAM

## Future Improvements

1. **Add Docker Resource Limits**:
   - Update testenv to support container resource limits
   - Default: 2GB memory, 1 CPU core per container

2. **Optimize Agent Memory Usage**:
   - Profile agent memory usage
   - Reduce chunk cache size
   - Optimize replay buffer management

3. **Add Memory Monitoring**:
   - Framework could monitor memory usage
   - Abort tests if approaching system limits
   - Log memory statistics with test results

4. **Test Isolation**:
   - Run each test in separate process
   - Ensures complete memory cleanup between tests
   - Prevents memory accumulation

## Summary

**Current State**:
- Tests must run sequentially to avoid OOM
- Each test can use 2-4GB of RAM
- System needs 8GB+ RAM with 4GB+ free
- Cleanup has been improved but memory limits not yet enforced

**Immediate Actions**:
1. ✅ Enhanced agent cleanup with timeouts
2. ✅ Pre-test container cleanup in run_tests.sh
3. ✅ Added 500ms delay between agent and server stop
4. ⏳ Run tests sequentially (use `-p 1`)
5. ⏳ Consider reducing Minecraft server memory allocation

**Long-term Solutions**:
1. Add Docker resource limits (requires testenv changes)
2. Profile and optimize agent memory usage
3. Add memory monitoring to framework
4. Consider test isolation strategy
