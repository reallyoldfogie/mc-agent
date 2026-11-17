package boltstore

import (
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

const crashWorkerEnv = "BOLTSTORE_CRASH_WORKER"

func TestMain(m *testing.M) {
	if os.Getenv(crashWorkerEnv) == "1" {
		runCrashWorker()
		return
	}
	os.Exit(m.Run())
}

// runCrashWorker opens a Store and commits one sighting per iteration in
// a tight sequential loop forever — see TestCrashSafety.
func runCrashWorker() {
	dir := os.Getenv("BOLTSTORE_CRASH_DIR")
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

// TestCrashSafety is boltstore's counterpart to jsonstore's test of the
// same name — see that one's doc comment for the overall approach. bbolt
// commits are already durable/atomic by construction (fsync-backed
// commit, copy-on-write b-tree), so this is confirming that guarantee
// actually holds the way this package uses the library, not re-testing
// bbolt itself.
func TestCrashSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns subprocesses; skip with -short")
	}

	for trial := range 20 {
		dir := t.TempDir()

		cmd := exec.Command(os.Args[0], "-test.run=^TestCrashSafety$", "-test.v")
		cmd.Env = append(os.Environ(), crashWorkerEnv+"=1", "BOLTSTORE_CRASH_DIR="+dir)
		require.NoError(t, cmd.Start())

		time.Sleep(time.Duration(rand.Intn(4000)) * time.Microsecond)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		path := filepath.Join(dir, "crashbot.bbolt")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue // killed before the file was ever created
		}

		reopened, err := New(memorystore.Config{Dir: dir, AgentName: "crashbot"})
		require.NoError(t, err, "trial %d: store must reload cleanly after a mid-write kill", trial)

		got, found, err := reopened.NearestSighting("minecraft:iron_ore", memorystore.Position{}, memorystore.SightingQuery{})
		require.NoError(t, err, "trial %d: reading back after a mid-write kill must not error", trial)
		if found {
			require.GreaterOrEqual(t, got.Position.X, 0, "trial %d: recovered record must be one of the committed writes, not garbage", trial)
		}
		_ = reopened.Close()
	}
}
