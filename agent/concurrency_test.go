package agent

import (
	"bytes"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the permanent, automated verification for the two
// concurrency bugs docs/bugs/shared-block-action-sequence-counter.md and
// docs/bugs/global-log-output-not-per-agent.md describe — both already
// fixed in code (2f4867d and 53a06bb respectively; see each bug doc's own
// "Suggested fix" section for when), but neither previously had a test
// that would have caught the original package-level-state bug or would
// catch a regression back to it. Written for
// ../mc-rsi-trainer/docs/plans/05-mc-agent-concurrency-fixes.md's "Done
// when" — that document tracks these two bugs from mc-rsi-trainer's side,
// since its planned Teacher/Student leapfrog evaluation loop is what needs
// two concurrent agent sessions to be trustworthy.

// TestGetNextSequenceIsIndependentPerAgentInstance verifies the fix for
// docs/bugs/shared-block-action-sequence-counter.md: two *agent instances
// must each own a private, independently-incrementing sequence counter,
// not share one process-wide counter. Before the fix (sequenceCounter/
// sequenceInitialized as package-level vars), the two goroutines below
// would race on one shared counter and neither agent would see its own
// contiguous 0..N-1 run — this test fails deterministically under that
// bug (not just occasionally), since two goroutines each issuing 200
// sequential calls essentially never happen to interleave into two clean,
// separate contiguous ranges by accident.
func TestGetNextSequenceIsIndependentPerAgentInstance(t *testing.T) {
	agentA := &agent{}
	agentB := &agent{}

	const callsPerAgent = 200
	seqA := make([]int32, callsPerAgent)
	seqB := make([]int32, callsPerAgent)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range seqA {
			seqA[i] = agentA.getNextSequence()
		}
	}()
	go func() {
		defer wg.Done()
		for i := range seqB {
			seqB[i] = agentB.getNextSequence()
		}
	}()
	wg.Wait()

	for i, v := range seqA {
		assert.Equal(t, int32(i), v, "agent A's sequence must be its own contiguous 0..N-1 run, unaffected by agent B's concurrent calls (index %d)", i)
	}
	for i, v := range seqB {
		assert.Equal(t, int32(i), v, "agent B's sequence must be its own contiguous 0..N-1 run, unaffected by agent A's concurrent calls (index %d)", i)
	}
}

// TestPerAgentLoggingStaysIsolatedBetweenConcurrentInstances verifies the
// core mechanism docs/bugs/global-log-output-not-per-agent.md's fix
// relies on: each *agent's logger writes only to its own destination,
// never a shared/global one. Before the fix (a package-level
// log.SetOutput target), two concurrent agents' log lines would
// interleave into whichever single destination was most recently
// configured; this test fails under that bug since bufB would end up
// containing "alice" lines (or vice versa) instead of staying empty of
// them.
//
// This only exercises agent/'s own logger field directly, not the 127
// downstream files (pathfinding, movement, handler_versions, ...)
// docs/bugs/global-log-output-not-per-agent.md's Phase 2 converted — but
// every one of those now receives its *slog.Logger via plain constructor
// injection (see e.g. pathfinding.NewHPAPathFinder's logger parameter),
// not through any shared state of their own, so isolation at this level
// is what their isolation actually reduces to: there is no global log
// target left anywhere in the call chain for two concurrent agents' log
// lines to collide in. Mechanically confirmed, not just asserted: `grep
// -rl '^\s*"log"$' --include=*.go . | grep -v
// '_test.go\|/agent/\|/cmd/\|/testing/\|/code-archive/'` (Phase 2's own
// scope-defining query, docs/bugs/global-log-output-not-per-agent.md)
// returns zero files as of this test being written — see that bug doc's
// own updated status for the full verification record.
func TestPerAgentLoggingStaysIsolatedBetweenConcurrentInstances(t *testing.T) {
	loggerA, writerA := newAgentLogger("Alice", slog.LevelInfo)
	loggerB, writerB := newAgentLogger("Bob", slog.LevelInfo)

	var bufA, bufB bytes.Buffer
	writerA.setTarget(&bufA)
	writerB.setTarget(&bufB)

	agentA := &agent{logger: loggerA, logWriter: writerA}
	agentB := &agent{logger: loggerB, logWriter: writerB}

	const linesPerAgent = 50
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < linesPerAgent; i++ {
			agentA.logf("alice message %d", i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < linesPerAgent; i++ {
			agentB.logf("bob message %d", i)
		}
	}()
	wg.Wait()

	assert.Equal(t, linesPerAgent, bytes.Count(bufA.Bytes(), []byte("alice message")), "all of Alice's own lines should have landed in her own buffer")
	assert.Equal(t, 0, bytes.Count(bufA.Bytes(), []byte("bob message")), "Bob's log lines must never land in Alice's buffer")
	assert.Equal(t, linesPerAgent, bytes.Count(bufB.Bytes(), []byte("bob message")), "all of Bob's own lines should have landed in his own buffer")
	assert.Equal(t, 0, bytes.Count(bufB.Bytes(), []byte("alice message")), "Alice's log lines must never land in Bob's buffer")
}

// TestCloseLoggingOnlyAffectsOwnAgent verifies the specific failure mode
// docs/bugs/global-log-output-not-per-agent.md calls out as "worse for a
// two-agent scenario specifically": the old global closeAgentLog()
// unconditionally reset the package-level log target, so closing one
// agent could silently redirect a different, still-running agent's
// output to stdout mid-session. Because closeLogging (agent/logging.go)
// now only ever touches its own receiver's logWriter, Alice closing her
// own logging must leave Bob's writer target untouched.
func TestCloseLoggingOnlyAffectsOwnAgent(t *testing.T) {
	loggerA, writerA := newAgentLogger("Alice", slog.LevelInfo)
	loggerB, writerB := newAgentLogger("Bob", slog.LevelInfo)

	var bufB bytes.Buffer
	writerB.setTarget(&bufB)

	fileA, err := os.CreateTemp(t.TempDir(), "alice-*.log")
	require.NoError(t, err)
	fileB, err := os.CreateTemp(t.TempDir(), "bob-*.log")
	require.NoError(t, err)

	agentA := &agent{logger: loggerA, logWriter: writerA, logFile: fileA}
	agentB := &agent{logger: loggerB, logWriter: writerB, logFile: fileB}

	agentA.closeLogging()

	assert.Nil(t, agentA.logFile, "Alice's own log file reference should be cleared by her own closeLogging")

	bufB.Reset()
	agentB.logf("bob still logging after alice closed")
	assert.Contains(t, bufB.String(), "bob still logging after alice closed",
		"Bob's writer must still target its own buffer after Alice's closeLogging — not have been reset to stdout the way the old global closeAgentLog() would have")
	assert.NotNil(t, agentB.logFile, "Bob's own log file reference must be untouched by Alice's closeLogging")
}
