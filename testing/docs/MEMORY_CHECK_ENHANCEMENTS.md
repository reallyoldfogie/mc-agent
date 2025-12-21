# Memory Check Enhancements

## Date: 2025-12-07

## Overview

Enhanced the memory availability check feature with retry logic, memory usage reporting, and improved test summaries to provide better visibility and robustness.

## Changes Implemented

### 1. Memory Check Retry Logic

**Problem**: Memory check failed immediately if insufficient memory, even if containers were shutting down and memory would soon be available.

**Solution**: Added retry mechanism with configurable attempts and wait times.

**New ServerConfig Fields** (`framework.go:75-76`):
```go
MemoryCheckRetries   int           // Number of times to retry memory check (default: 3)
MemoryCheckRetryWait time.Duration // Wait between retries (default: 5 seconds)
```

**Default Values** (`framework.go:92-93`):
```go
MemoryCheckRetries:   3,              // Retry 3 times before failing
MemoryCheckRetryWait: 5 * time.Second, // Wait 5 seconds between retries
```

**Behavior**:
- Checks memory availability up to 3 times (configurable)
- Waits 5 seconds between checks (configurable)
- Shows progress: "Memory check (attempt 1/3): ... - waiting 5s for memory to free up..."
- Total wait time: 10 seconds (2 retries × 5 seconds)

**Example Output - Success After Retry**:
```
Memory check (attempt 1/3): 2048 MB available, 3584 MB required - waiting 5s for memory to free up...
Memory check (attempt 2/3): 2048 MB available, 3584 MB required - waiting 5s for memory to free up...
Memory check: 3700 MB available, 3584 MB required (server: 1536 MB + buffer: 2048 MB) - OK
```

### 2. Memory Usage Reporting

**Problem**: When memory check failed, users had no information about what was consuming memory.

**Solution**: Added detailed memory usage report showing Docker containers and top processes.

**New Function** (`framework.go:753-785`):
```go
func getMemoryUsageReport() string {
    // Reports:
    // 1. Docker container memory usage (docker stats)
    // 2. Top 10 memory-consuming processes (ps aux --sort=-%mem)
}
```

**Example Output - Failure With Report**:
```
Error: insufficient memory: need 3584 MB (server: 1536 MB + buffer: 2048 MB), have 2048 MB available
Tried 3 times over 10s.
Suggestions:
  - Close other applications to free memory
  - Reduce server memory (cfg.Memory)
  - Wait for other containers to finish
  - Run tests sequentially instead of parallel

=== Memory Usage Report ===

Docker Containers:
NAME                    MEM USAGE
mc-agent-test-abc123    1.2GiB / 8GiB

Top Memory-Consuming Processes:
USER       PID %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND
user      1234  5.2 15.3 4022960 1690160 ?     Sl   08:00   0:45 testing.test
user      5678  2.1  8.7 2100432  987654 ?     Sl   07:55   0:23 java -Xmx2G ...
...
```

**Benefits**:
- ✅ Identifies which containers are using memory
- ✅ Shows top memory-consuming processes
- ✅ Helps users decide what to close to free memory
- ✅ Provides actionable debugging information

### 3. Enhanced Test Summary

**Problem**: Test runner only showed "All tests passed" or "Tests failed" without breakdown.

**Solution**: Parse test output to show counts and list failed tests.

**Changes to `run_tests.sh`**:

1. **Capture Test Output** (lines 156-160):
   ```bash
   TEST_OUTPUT=$(mktemp)
   trap "rm -f $TEST_OUTPUT" EXIT
   eval $TEST_CMD 2>&1 | tee $TEST_OUTPUT
   ```

2. **Parse Results** (lines 164-181):
   ```bash
   PASS_COUNT=$(grep -c "^--- PASS:" $TEST_OUTPUT)
   FAIL_COUNT=$(grep -c "^--- FAIL:" $TEST_OUTPUT)
   SKIP_COUNT=$(grep -c "^--- SKIP:" $TEST_OUTPUT)
   ```

3. **Show Summary** (success case):
   ```bash
   === Test Summary ===
     Passed: 3
     Total:  3
   ```

4. **Show Summary** (failure case):
   ```bash
   === Test Summary ===
     Passed:  2
     Failed:  1
     Total:   3

   Failed tests:
     - TestFollowSingleAgent (0.45s)
   ```

### 4. Agent Log Capture Status

**Current Status**: Infrastructure exists but not wired up.

**Why**: The `captureAgentOutput()` function exists in `framework.go:566-594`, but agents in the agent package don't currently capture their output to a string that can be passed to this function.

**What Would Be Needed**:
- Modify agent package to redirect stdout/stderr to a buffer
- Or use structured logging to files
- Or capture at process level

**Workaround**: Server logs are being captured successfully. Agent logs would require changes to the agent package itself.

## Configuration Examples

### Default (Recommended)
```go
cfg := DefaultServerConfig()
// Memory check with 3 retries, 5 second waits
// Requires: 1536 MB (server) + 2048 MB (buffer) = 3584 MB
```

### Aggressive Retry
```go
cfg := DefaultServerConfig()
cfg.MemoryCheckRetries = 5             // Try 5 times
cfg.MemoryCheckRetryWait = 10 * time.Second  // Wait 10 seconds between
// Total wait: 40 seconds
```

### Minimal Memory
```go
cfg := DefaultServerConfig()
cfg.Memory = "512M"           // Smaller server
cfg.MinFreeMemoryMB = 1024    // Smaller buffer
cfg.MemoryCheckRetries = 1    // No retries
// Requires: 768 MB (server) + 1024 MB (buffer) = 1792 MB
```

### Skip Check (Not Recommended)
```go
cfg := DefaultServerConfig()
cfg.SkipMemoryCheck = true    // WARNING: Can cause OOM
```

## Usage Examples

### Example 1: Memory Available Immediately
```
Memory check: 6144 MB available, 3584 MB required (server: 1536 MB + buffer: 2048 MB) - OK
Server started on 127.0.0.1:25565
```

### Example 2: Memory Available After Wait
```
Memory check (attempt 1/3): 3000 MB available, 3584 MB required - waiting 5s for memory to free up...
Memory check: 4096 MB available, 3584 MB required (server: 1536 MB + buffer: 2048 MB) - OK
Server started on 127.0.0.1:25565
```

### Example 3: Insufficient Memory After All Retries
```
Memory check (attempt 1/3): 2048 MB available, 3584 MB required - waiting 5s for memory to free up...
Memory check (attempt 2/3): 2048 MB available, 3584 MB required - waiting 5s for memory to free up...
Memory check (attempt 3/3): 2048 MB available, 3584 MB required - waiting 5s for memory to free up...

Error: insufficient memory: need 3584 MB (server: 1536 MB + buffer: 2048 MB), have 2048 MB available
Tried 3 times over 10s.
Suggestions:
  - Close other applications to free memory
  - Reduce server memory (cfg.Memory)
  - Wait for other containers to finish
  - Run tests sequentially instead of parallel

=== Memory Usage Report ===
[Docker containers and process list...]
```

## Benefits

### Memory Check Retry
- ✅ **Reduces false failures** - Waits for memory to be freed
- ✅ **Handles transient conditions** - Previous test container shutting down
- ✅ **Configurable** - Adjust retries and wait times
- ✅ **Informative** - Shows progress during waits

### Memory Usage Reporting
- ✅ **Identifies culprits** - Shows what's using memory
- ✅ **Actionable information** - Clear what to close/stop
- ✅ **Debug friendly** - Comprehensive system view
- ✅ **Non-intrusive** - Only shown on failure

### Test Summary
- ✅ **Quick visibility** - Pass/fail counts at a glance
- ✅ **Failed test list** - No need to scroll through output
- ✅ **Color-coded** - Green for pass, red for fail
- ✅ **Comprehensive** - Shows passed, failed, and skipped

## Testing The Enhancements

### Test Retry Logic

**Scenario**: Simulate low memory condition
```bash
# Terminal 1: Start a memory-consuming container
docker run -d --name memory-hog --memory 2g stress --vm 1 --vm-bytes 1500M

# Terminal 2: Run test (should retry and wait)
cd testing
./run_tests.sh -t TestNavigationSingleAgent

# Terminal 3: Stop memory hog after a few seconds
sleep 7 && docker rm -f memory-hog

# Expected: Test should succeed after retry
```

### Test Memory Report

**Scenario**: Force memory check failure
```bash
# Lower the buffer requirement temporarily to force failure on low-memory system
# Or run multiple containers to consume memory

cd testing
./run_tests.sh

# Expected: Should show memory usage report with Docker containers and processes
```

### Test Summary

**Scenario**: Run tests normally
```bash
cd testing
./run_tests.sh

# Expected output:
=== Test Summary ===
  Passed: 3
  Total:  3

Replay recordings (3 files):
...
```

**Scenario**: Simulate test failure
```bash
# Modify a test to fail temporarily
cd testing
./run_tests.sh

# Expected output:
=== Test Summary ===
  Passed:  2
  Failed:  1
  Total:   3

Failed tests:
  - TestSomething (0.45s)
```

## Files Modified

1. **`testing/framework.go`**:
   - Added `MemoryCheckRetries` and `MemoryCheckRetryWait` fields
   - Modified `checkMemoryAvailability()` with retry loop
   - Added `getMemoryUsageReport()` function
   - Added `os/exec` import

2. **`testing/run_tests.sh`**:
   - Added test output capture to temporary file
   - Added test summary parsing (PASS/FAIL/SKIP counts)
   - Added failed test listing
   - Enhanced success/failure reporting

3. **`testing/MEMORY_CHECK_ENHANCEMENTS.md`** (this file):
   - Documentation of all enhancements

## Backward Compatibility

All changes are **100% backward compatible**:
- New ServerConfig fields have sensible defaults
- Existing tests work without modification
- Retry logic activates automatically
- Test summary appears automatically

## Limitations

### Agent Log Capture
**Status**: Not implemented

**Why**: Requires changes to agent package to capture stdout/stderr

**Workaround**: Server logs are captured successfully

**Future Work**: Would require one of:
1. Agent package redirects output to buffer
2. Agent package uses structured logging to file
3. Framework captures agent process output

## Summary

All requested enhancements have been implemented:
- ✅ **Memory check retry logic** - Waits for memory to free up
- ✅ **Memory usage reporting** - Shows Docker containers and top processes
- ✅ **Test pass/fail summary** - Counts and failed test list
- ✅ **Agent log investigation** - Infrastructure ready but requires agent package changes

Code compiles successfully and is ready for testing.
