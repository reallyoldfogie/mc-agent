package jsonstore

import (
	"encoding/json"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/memorystore"
	"github.com/reallyoldfogie/mc-agent/memorystore/storetest"
	"github.com/stretchr/testify/require"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, New)
}

// crashWorkerEnv, when set to "1", tells TestMain this process is the
// crash-safety test's subprocess rather than the normal test binary — see
// TestCrashSafety.
const crashWorkerEnv = "JSONSTORE_CRASH_WORKER"

func TestMain(m *testing.M) {
	if os.Getenv(crashWorkerEnv) == "1" {
		runCrashWorker()
		return
	}
	os.Exit(m.Run())
}

// runCrashWorker opens a Store and records sightings in a tight sequential
// loop forever, at ever-increasing positions (X = loop index) — the parent
// test kills this process at a random point and checks what's on disk.
func runCrashWorker() {
	dir := os.Getenv("JSONSTORE_CRASH_DIR")
	store, err := New(memorystore.Config{Dir: dir, AgentName: "crashbot"})
	if err != nil {
		os.Exit(1)
	}
	for i := 0; ; i++ {
		_ = store.RecordSighting(memorystore.Sighting{
			ItemOrBlockName: "minecraft:iron_ore",
			Position:        memorystore.Position{X: i},
			ObservedAt:      time.Now(),
		})
	}
}

// TestCrashSafety proves, per
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md's "Whichever backend is
// actually exercised... prove the crash-safety property directly," that
// killing this backend mid-write never leaves a store that fails to
// reload or that reloads with a torn/corrupted record. It repeatedly: (1)
// starts a subprocess that writes sightings in a tight loop, (2) kills it
// (SIGKILL — no cleanup chance) after a short random delay meant to land
// inside an in-flight write, (3) reopens the store in-process and asserts
// it loads cleanly with a contiguous, gap-free prefix of what the worker
// was writing. The random delay makes this probabilistic about *which*
// write it interrupts, but the reload-cleanly assertion holds regardless
// of where the kill actually landed, since jsonstore.flushLocked's
// temp-file+rename means the destination file is never observed
// mid-write.
func TestCrashSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns subprocesses; skip with -short")
	}

	for trial := range 20 {
		dir := t.TempDir()

		cmd := exec.Command(os.Args[0], "-test.run=^TestCrashSafety$", "-test.v")
		cmd.Env = append(os.Environ(), crashWorkerEnv+"=1", "JSONSTORE_CRASH_DIR="+dir)
		require.NoError(t, cmd.Start())

		time.Sleep(time.Duration(rand.Intn(4000)) * time.Microsecond)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		path := filepath.Join(dir, "crashbot.json")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue // killed before the first write ever landed
		}

		// The file on disk must always be valid JSON — never a truncated
		// fragment of a write that was interrupted mid-syscall.
		raw, err := os.ReadFile(path)
		require.NoError(t, err, "trial %d", trial)
		var contents fileContents
		require.NoError(t, json.Unmarshal(raw, &contents), "trial %d: on-disk file must parse cleanly after a mid-write kill", trial)

		for idx, sighting := range contents.Sightings {
			require.Equal(t, idx, sighting.Position.X, "trial %d: sightings must form a contiguous, gap-free prefix with no corruption", trial)
		}

		// Reopening through the real backend must also succeed.
		reopened, err := New(memorystore.Config{Dir: dir, AgentName: "crashbot"})
		require.NoError(t, err, "trial %d: store must reload cleanly after a mid-write kill", trial)
		_ = reopened.Close()
	}
}
