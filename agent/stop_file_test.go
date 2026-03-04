package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStopFileWatcher(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()
	stopFile := filepath.Join(tmpDir, "test.stop")

	// Create a minimal agent config
	cfg := Config{
		Address:      "test:25565",
		Version:      "1.21.5",
		StopFilePath: stopFile,
	}

	agentInt, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}
	agent := agentInt.(*agent)

	// Create a context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize agent
	if err := agent.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize agent: %v", err)
	}

	// Start the stop file watcher (without connecting to a server)
	agent.startStopFileWatcher(stopFile, agent.ctx.Done())

	// Give the watcher a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify stop file doesn't exist yet
	if _, err := os.Stat(stopFile); err == nil {
		t.Fatal("Stop file should not exist initially")
	}

	// Create the stop file
	if err := os.WriteFile(stopFile, []byte("stop"), 0644); err != nil {
		t.Fatalf("Failed to create stop file: %v", err)
	}

	// Wait for the watcher to detect and process the stop file
	// The watcher checks every second, so wait up to 2 seconds
	timeout := time.After(3 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	contextCancelled := false
	for !contextCancelled {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for stop file to be processed")
		case <-ticker.C:
			// Check if context was cancelled
			agent.lifecycleMu.RLock()
			if agent.ctx.Err() != nil {
				contextCancelled = true
			}
			agent.lifecycleMu.RUnlock()
		}
	}

	// Verify the stop file was removed
	if _, err := os.Stat(stopFile); err == nil {
		t.Error("Stop file should have been removed after detection")
	}

	// Verify context was cancelled
	agent.lifecycleMu.RLock()
	if agent.ctx.Err() == nil {
		t.Error("Agent context should have been cancelled")
	}
	agent.lifecycleMu.RUnlock()

	// Wait for goroutines to finish
	agent.wg.Wait()
}

func TestStopFileWatcherDisabled(t *testing.T) {
	// Create agent without stop file path
	cfg := Config{
		Address:      "test:25565",
		Version:      "1.21.5",
		StopFilePath: "", // Disabled
	}

	agentInt, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	agent := agentInt.(*agent)

	ctx := context.Background()
	if err := agent.Init(ctx); err != nil {
		t.Fatalf("Failed to initialize agent: %v", err)
	}

	// Start the watcher with empty path (should be no-op)
	agent.startStopFileWatcher("", agent.ctx.Done())

	// Give it a moment
	time.Sleep(200 * time.Millisecond)

	// Context should still be valid (not cancelled)
	agent.lifecycleMu.RLock()
	if agent.ctx.Err() != nil {
		t.Error("Context should not be cancelled when stop file is disabled")
	}
	agent.lifecycleMu.RUnlock()
}

func TestRemoveStopFileIfExists(t *testing.T) {
	tmpDir := t.TempDir()
	stopFile := filepath.Join(tmpDir, "test.stop")

	cfg := Config{
		Address: "test:25565",
		Version: "1.21.5",
	}

	agentInt, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create agent: %v", err)
	}

	agent := agentInt.(*agent)

	// Test removing non-existent file (should not error)
	agent.removeStopFileIfExists(stopFile)

	// Create stop file
	if err := os.WriteFile(stopFile, []byte("stop"), 0644); err != nil {
		t.Fatalf("Failed to create stop file: %v", err)
	}

	// Verify it exists
	if _, err := os.Stat(stopFile); err != nil {
		t.Fatal("Stop file should exist")
	}

	// Remove it
	agent.removeStopFileIfExists(stopFile)

	// Verify it's gone
	if _, err := os.Stat(stopFile); err == nil {
		t.Error("Stop file should have been removed")
	}

	// Try removing again (should not error)
	agent.removeStopFileIfExists(stopFile)
}
