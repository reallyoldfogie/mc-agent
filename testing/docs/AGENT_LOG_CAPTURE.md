# Agent Log Capture Implementation

## Date: 2025-12-07

## Overview

Implemented global agent log capture that redirects Go's standard `log` package output to both a file and the console, with no changes required to the agent package itself.

## Solution

### Approach

Instead of modifying the agent package to capture logs, we redirect the global `log` package output using `log.SetOutput()` with an `io.MultiWriter` that writes to both:
1. A log file in `./logs/agents/`
2. The console (stdout)

This works because:
- The agent package uses the standard Go `log` package (`log.Printf`, `log.Println`, etc.)
- `log.SetOutput()` changes the output destination globally
- `io.MultiWriter` allows writing to multiple destinations simultaneously

### Implementation

**Files Modified**:
- `testing/framework.go` - Added logging setup and teardown
- `testing/run_tests.sh` - Enhanced to show agent logs in summary

**New Framework Fields** (`framework.go:30-32`):
```go
type Framework struct {
    serverMgr      testenv.Manager
    instances      map[string]*TestInstance
    agentLogFile   *os.File      // Log file for all agents
    agentLogWriter io.Writer     // MultiWriter for agents (file + stdout)
    logSetupMu     sync.Mutex    // Mutex for log setup
}
```

**Setup Method** (`framework.go:228-266`):
```go
func (f *Framework) setupAgentLogging() error {
    f.logSetupMu.Lock()
    defer f.logSetupMu.Unlock()

    // Already setup?
    if f.agentLogWriter != nil {
        return nil
    }

    // Create log file with timestamp
    logFile := fmt.Sprintf("./logs/agents/agents_%s.log",
        time.Now().Format("20060102_150405"))
    file, err := os.Create(logFile)
    if err != nil {
        return fmt.Errorf("create log file: %w", err)
    }

    // Create MultiWriter to write to both file and stdout
    multiWriter := io.MultiWriter(file, os.Stdout)

    // Set global log output
    log.SetOutput(multiWriter)
    log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

    // Store for cleanup
    f.agentLogFile = file
    f.agentLogWriter = multiWriter

    fmt.Printf("Agent logging enabled: %s (also to console)\n", logFile)
    return nil
}
```

**Cleanup Method** (`framework.go:268-281`):
```go
func (f *Framework) CloseAgentLog() {
    f.logSetupMu.Lock()
    defer f.logSetupMu.Unlock()

    if f.agentLogFile != nil {
        f.agentLogFile.Close()
        f.agentLogFile = nil
        f.agentLogWriter = nil
        // Restore log to stdout only
        log.SetOutput(os.Stdout)
    }
}
```

**Automatic Activation** (`framework.go:261-265`):
```go
func (f *Framework) SpawnAgent(ctx context.Context, inst *TestInstance, cfg AgentConfig) (*ManagedAgent, error) {
    // Setup agent logging (redirects log package to file + stdout)
    // This is done once globally for all agents
    if err := f.setupAgentLogging(); err != nil {
        return nil, fmt.Errorf("setup agent logging: %w", err)
    }

    // ... rest of agent creation ...
}
```

## Behavior

### First Agent Spawned
1. Framework calls `setupAgentLogging()` (once only)
2. Creates `./logs/agents/agents_YYYYMMDD_HHMMSS.log`
3. Redirects `log` package to MultiWriter (file + stdout)
4. All subsequent `log.Printf` calls go to both destinations
5. Prints: "Agent logging enabled: ./logs/agents/agents_20251207_143022.log (also to console)"

### Subsequent Agents
1. Framework calls `setupAgentLogging()` again
2. Detects already setup (`agentLogWriter != nil`)
3. Returns immediately
4. All agents share the same log file and console output

### Multiple Tests
Each test run gets its own timestamped log file:
- Test 1: `agents_20251207_143022.log`
- Test 2 (later): `agents_20251207_145530.log`

### Log Format
```
2025/12/07 14:30:22.123456 agent.go:123: [Agent] Connected to server
2025/12/07 14:30:22.234567 pathfinding.go:45: [Pathfinding] Found path with 15 nodes
2025/12/07 14:30:23.345678 movement.go:78: [Movement] Moving to (100, 64, 200)
```

Log flags set:
- `log.Ldate` - Date (2025/12/07)
- `log.Ltime` - Time (14:30:22)
- `log.Lmicroseconds` - Microseconds (.123456)
- `log.Lshortfile` - Filename and line (agent.go:123)

## Usage

### Running Tests

No changes required to test code! Logging is automatic:

```bash
cd testing
./run_tests.sh

# Output:
Agent logging enabled: ./logs/agents/agents_20251207_143022.log (also to console)
[Agent] Connected to server
[Pathfinding] Found path with 15 nodes
...

=== Test Summary ===
  Passed: 3
  Total:  3

Agent logs captured (1 files):
-rw-r--r-- 1 user user 45K Dec 7 14:30 agents_20251207_143022.log
```

### Reviewing Logs Offline

```bash
# List all agent logs
ls -lh testing/logs/agents/

# View specific log
cat testing/logs/agents/agents_20251207_143022.log

# Search for specific events
grep "Pathfinding" testing/logs/agents/*.log
grep "ERROR" testing/logs/agents/*.log

# Follow live (if test is running)
tail -f testing/logs/agents/agents_20251207_143022.log
```

### Manual Cleanup (optional)

The framework provides a cleanup method (though it's typically called automatically):

```go
framework.CloseAgentLog()
```

This:
- Closes the log file
- Restores `log` output to stdout only
- Clears internal state

## Test Summary Integration

The test runner now shows agent logs in the summary:

**Success Case**:
```
=== Test Summary ===
  Passed: 3
  Total:  3

Replay recordings (3 files):
-rw-r--r-- 1 user user 896K Dec 7 14:30 nav_single_20251207_143022.mcpr

Server logs captured (3 files):
-rw-r--r-- 1 user user 45K Dec 7 14:30 mc-agent-test-abc_20251207_143025.log

Agent logs captured (1 files):
-rw-r--r-- 1 user user 23K Dec 7 14:30 agents_20251207_143022.log
```

**Failure Case**:
```
=== Test Summary ===
  Passed:  2
  Failed:  1
  Total:   3

Failed tests:
  - TestSomething (0.45s)

Server logs for debugging (3 files):
-rw-r--r-- 1 user user 45K Dec 7 14:30 mc-agent-test-abc_20251207_143025.log

Agent logs for debugging (1 files):
-rw-r--r-- 1 user user 23K Dec 7 14:30 agents_20251207_143022.log
```

## Benefits

✅ **No agent package changes** - Works with existing code
✅ **Automatic** - No manual logging calls needed
✅ **Dual output** - Console for live viewing, file for offline review
✅ **Timestamped** - Each test run gets its own log file
✅ **Thread-safe** - Mutex protects setup/teardown
✅ **Clean separation** - Agent logs separate from server logs
✅ **Searchable** - Grep-friendly offline analysis
✅ **Integrated** - Shows in test summary automatically

## Limitations

### 1. Global State
- Uses Go's global `log` package
- All agents in all tests share the same log destination during a test run
- Cannot easily give each agent its own separate log file

**Impact**: All agents' logs are interleaved in one file
**Mitigation**: Log entries include filename and line number for identification
**Future**: Could prefix agent name using custom logger wrapper

### 2. Non-test Code
- Any other code using the `log` package will also be captured
- Test framework's own logs also go to the agent log file

**Impact**: Agent log file contains some non-agent logs
**Mitigation**: Log format includes source file (e.g., "agent.go:123" vs "framework.go:45")
**Future**: Could use log prefixes to distinguish sources

### 3. Log Rotation
- No automatic rotation for large log files
- Long-running tests could generate large files

**Impact**: Log files grow unbounded
**Mitigation**: Tests typically short-lived, logs rotate per test run
**Future**: Could integrate lumberjack for rotation

## Examples

### Example 1: Single Agent Test

```go
func TestNavigationSingleAgent(t *testing.T) {
    framework, _ := NewFramework()
    cfg := DefaultServerConfig()
    inst, _ := framework.StartServer(ctx, cfg)
    defer framework.StopServer(ctx, inst, true)

    // Logging automatically enabled here
    agent, _ := framework.SpawnAgent(ctx, inst, agentCfg)

    // All log.Printf calls from agent go to:
    // 1. Console (stdout)
    // 2. ./logs/agents/agents_TIMESTAMP.log

    agent.GoTo(ctx, 100, 64, 200)
    // Logs show in both console and file:
    // 2025/12/07 14:30:22.123456 pathfinding.go:45: [Pathfinding] Found path with 15 nodes
    // 2025/12/07 14:30:22.234567 movement.go:78: [Movement] Moving to (100, 64, 200)
}
```

### Example 2: Multiple Agents

```go
func TestFollowMultipleAgents(t *testing.T) {
    framework, _ := NewFramework()
    inst, _ := framework.StartServer(ctx, cfg)
    defer framework.StopServer(ctx, inst, true)

    // First agent - sets up logging
    agent1, _ := framework.SpawnAgent(ctx, inst, agentCfg1)
    // Prints: "Agent logging enabled: ./logs/agents/agents_20251207_143022.log (also to console)"

    // Second agent - reuses same logging
    agent2, _ := framework.SpawnAgent(ctx, inst, agentCfg2)
    // No setup message (already configured)

    // Both agents' logs go to same file and console
    agent1.GoTo(ctx, 100, 64, 200)  // Logged
    agent2.Follow(ctx, "Leader")     // Logged

    // Log file contains interleaved output:
    // 2025/12/07 14:30:22.123456 agent.go:123: [Agent] Follower connected
    // 2025/12/07 14:30:22.234567 agent.go:123: [Agent] Leader connected
    // 2025/12/07 14:30:23.345678 pathfinding.go:45: [Pathfinding] Found path with 15 nodes
    // 2025/12/07 14:30:23.456789 following.go:67: [Following] Tracking Leader
}
```

### Example 3: Log Analysis

```bash
# Find all pathfinding logs
grep "\[Pathfinding\]" ./logs/agents/agents_20251207_143022.log

# Find errors
grep -i "error\|fail\|warn" ./logs/agents/*.log

# Count log entries by source file
awk -F: '{print $1}' ./logs/agents/agents_20251207_143022.log | sort | uniq -c

# Extract specific agent events (if prefixed)
grep "agent.go" ./logs/agents/agents_20251207_143022.log
```

## Testing The Feature

### Verify Logs Captured

```bash
cd testing

# Before test - no agent logs
ls ./logs/agents/

# Run test
./run_tests.sh -t TestNavigationSingleAgent

# After test - log file exists
ls -lh ./logs/agents/
# Expected: agents_YYYYMMDD_HHMMSS.log

# Verify content
cat ./logs/agents/agents_*.log
# Expected: Contains agent log entries with timestamps and source files
```

### Verify Dual Output

```bash
# Terminal 1: Run test and observe console output
cd testing
./run_tests.sh -t TestNavigationSingleAgent
# Expected: See log messages in console

# Terminal 2: Tail log file
tail -f ./logs/agents/agents_*.log
# Expected: See same log messages in file (in real-time)
```

### Verify Multiple Tests

```bash
cd testing

# Run first test
./run_tests.sh -t TestNavigationSingleAgent
# Creates: agents_20251207_143022.log

# Wait a second
sleep 2

# Run second test
./run_tests.sh -t TestFollowSingleAgent
# Creates: agents_20251207_143025.log (different timestamp)

# Verify separate files
ls -lh ./logs/agents/
# Expected: Two separate log files
```

## Summary

Agent log capture has been successfully implemented with:
- ✅ No changes to agent package
- ✅ Automatic activation on first agent spawn
- ✅ Dual output (console + file)
- ✅ Timestamped log files per test run
- ✅ Thread-safe setup/teardown
- ✅ Integration with test summary

The solution leverages Go's standard `log` package with `io.MultiWriter` to provide comprehensive logging without requiring any modifications to the agent package itself.
