# Infrastructure Fixes for Test Framework

## Date: 2025-12-06

## Problem Statement

The test suite was experiencing critical resource management issues:

1. **OOM Kills**: Tests were being killed by the OS due to out-of-memory conditions
   - Process ID 1983948 was killed with total-vm:4022960kB, anon-rss:1690160kB
   - Multiple old Docker containers were left running, consuming resources

2. **Missing Diagnostics**: When tests failed, there was no way to review what happened:
   - No server logs captured
   - No agent output saved
   - Difficult to debug failures offline

3. **Incomplete Cleanup**: Docker containers were not being properly removed after tests

## Solutions Implemented

### 1. Server Log Capture Before Shutdown

**File**: `testing/framework.go`

**Changes**:
- Modified `StopServer()` to capture server logs before destroying containers
- Added `captureServerLogs()` method that:
  - Creates `./logs/servers/` directory
  - Fetches last 1000 lines of container logs (stdout + stderr)
  - Saves to timestamped file: `{container-name}_{timestamp}.log`
  - Continues shutdown even if log capture fails (non-blocking)

**Benefits**:
- Offline review of server behavior during tests
- Debugging test failures without re-running
- Historical record of server state

### 2. Agent Output Capture

**File**: `testing/framework.go`

**Changes**:
- Added `captureAgentOutput()` method for future agent log capture
- Creates `./logs/agents/` directory
- Saves agent output to timestamped files: `{agent-name}_{timestamp}.log`

**Status**: Infrastructure ready, integration pending

### 3. Improved Resource Cleanup

**File**: `testing/framework.go`

**Changes**:
- Enhanced `StopServer()` to ensure containers are always removed
- Added null checks to prevent panics during cleanup
- Graceful degradation if log capture fails

**File**: `testing/framework.go:328-353` (existing - verified)

**Changes**:
- `ManagedAgent.Stop()` already has critical fix:
  - Always calls `Agent.Close()` even on timeout
  - Uses fresh context with 10s timeout for Close()
  - This ensures replay files are finalized even if graceful shutdown times out

### 4. Movement Mirror Integration (Replay Visibility)

**Files**:
- `movement/packets.go`
- `movement/executor.go`
- `agent/agent.go`

**Changes**:
- Added packet callback system to movement executor
- `SetPacketCallback()` method on executor
- Modified packet sending functions to call callbacks before sending
- `SetMovementExecutor()` now wires movement mirror to executor callbacks
- Enables agent visibility in replay files

**Result**: Agent movement packets are now mirrored to replay files, making agents visible in ReplayMod viewer

## Remaining Work

### 1. Context Timeouts

**Issue**: Tests are timing out due to insufficient context deadlines

**Recommendation**:
- Review all test context timeouts
- Ensure parent contexts have longer timeouts than child operations
- Add generous buffer for Docker operations (pull, start, stop)

**Example Pattern**:
```go
// Test level - generous timeout
testCtx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
defer cancel()

// Server start - 10 minute timeout
serverCtx, serverCancel := context.WithTimeout(testCtx, 10*time.Minute)
defer serverCancel()

// Individual operations - shorter timeouts
opCtx, opCancel := context.WithTimeout(testCtx, 30*time.Second)
defer opCancel()
```

### 2. Panic Recovery

**Issue**: Panics or OOM kills leave containers running

**Recommendation**:
- Add defer-based cleanup in all tests
- Use panic recovery to ensure cleanup runs
- Consider adding cleanup goroutine with finalizers

**Example Pattern**:
```go
func TestSomething(t *testing.T) {
    framework, _ := NewFramework()

    var inst *TestInstance
    defer func() {
        if inst != nil {
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()
            framework.StopServer(ctx, inst, true) // Always remove
        }
    }()

    inst, err := framework.StartServer(ctx, cfg)
    // ... test code ...
}
```

### 3. Resource Monitoring

**Future Enhancement**:
- Add resource usage tracking in tests
- Warn if containers exceed memory thresholds
- Automatic cleanup of orphaned containers on startup

## Files Modified

1. `testing/framework.go`:
   - Added `captureServerLogs()` method
   - Added `captureAgentOutput()` method
   - Modified `StopServer()` to capture logs
   - Added helper functions: `ensureDir()`, `createFile()`, `copyWithTimeout()`

2. `testing/run_tests.sh`:
   - Added automatic cleanup of leftover test containers before running tests
   - Creates log directories (`./logs/servers`, `./logs/agents`) if they don't exist
   - Reports leftover containers after test completion
   - Enhanced troubleshooting output to show server logs
   - Shows captured server logs count on both success and failure

3. `movement/packets.go`:
   - Added `*WithCallback()` variants of packet sending functions
   - Enables packet interception for replay mirroring

4. `movement/executor.go`:
   - Added `onPacketSent` callback field
   - Added `SetPacketCallback()` method
   - Modified send methods to use callback variants

5. `agent/agent.go`:
   - Enhanced `SetMovementExecutor()` to wire movement mirror callbacks
   - Imported `pk` package for packet types

## Testing

To verify fixes:

```bash
# Run tests using the enhanced script (recommended)
cd testing
./run_tests.sh

# Or run specific test
./run_tests.sh -t TestNavigationSingleAgent

# The script will:
# 1. Clean up any leftover containers from previous runs
# 2. Create necessary log directories
# 3. Run tests
# 4. Report any leftover containers (shouldn't be any)
# 5. Show captured logs and replays

# Manual verification
# Check for leftover containers (should be none)
docker ps -a | grep mc-agent-test

# Review captured logs
ls -lh testing/logs/servers/
ls -lh testing/logs/agents/

# Review replays
ls -lh testing/replays/
```

## run_tests.sh Enhancements

The test runner script now includes:

1. **Pre-test Cleanup**:
   - Automatically removes leftover `mc-agent-test-*` containers before running tests
   - Prevents resource accumulation from failed test runs
   - Reports how many containers were removed

2. **Directory Setup**:
   - Creates `./logs/servers/` for server log capture
   - Creates `./logs/agents/` for agent log capture
   - Creates `./replays/` for replay recordings

3. **Post-test Reporting**:
   - On success: Reports captured server logs
   - On failure: Shows server logs and suggests reviewing them
   - Always: Checks for and reports leftover containers

4. **Enhanced Troubleshooting**:
   - Step 1 now points to server logs directory
   - Provides direct paths to diagnostic information
   - Shows sample of available logs and replays

## Log Directory Structure

```
testing/
├── logs/
│   ├── servers/
│   │   └── mc-agent-test-mc-1-21-5-{id}_{timestamp}.log
│   └── agents/
│       └── TestBot_{timestamp}.log
└── replays/
    └── nav_single_{timestamp}.mcpr
```

## Key Learnings

1. **Always cleanup, even on failure**: Use defer and fresh contexts
2. **Capture diagnostics early**: Before destroying resources
3. **Non-blocking diagnostics**: Don't fail tests because log capture failed
4. **Generous timeouts**: Docker operations can be slow, especially first pull
5. **Resource limits**: Consider adding memory/CPU limits to test containers

## Next Steps

1. ✅ Server log capture implemented
2. ✅ Agent output capture infrastructure ready
3. ⏳ Review and increase context timeouts in all tests
4. ⏳ Add panic recovery with cleanup to all tests
5. ⏳ Consider adding resource monitoring/limits
