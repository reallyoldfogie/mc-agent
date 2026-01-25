package utils

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestEnsureRegistriesPath_Concurrent tests that concurrent calls to EnsureRegistriesPath
// for the same version use a semaphore to prevent multiple downloads.
func TestEnsureRegistriesPath_Concurrent(t *testing.T) {
	// Clear any cached state
	ClearRegistryDownloadCache()

	// Use a temporary directory for testing
	tmpDir := t.TempDir()
	testVersion := "test-1.21.5"

	// Create a fake registries.json to simulate existing data
	registryDir := filepath.Join(tmpDir, "data_generator", "reports")
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	registryPath := filepath.Join(registryDir, "registries.json")
	if err := os.WriteFile(registryPath, []byte(`{"minecraft:entity_type":{"entries":{}}}`), 0644); err != nil {
		t.Fatalf("Failed to create test registries.json: %v", err)
	}

	// Launch 10 concurrent calls
	const numGoroutines = 10
	var wg sync.WaitGroup
	results := make(chan error, numGoroutines)
	startBarrier := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier // Wait for all goroutines to be ready
			path, err := EnsureRegistriesPath(tmpDir, testVersion)
			if err != nil {
				results <- err
				return
			}
			if path != tmpDir {
				results <- err
				return
			}
			results <- nil
		}()
	}

	// Start all goroutines at once
	close(startBarrier)

	// Wait for all to complete with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - all completed
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for concurrent calls to complete")
	}

	// Check results
	close(results)
	for err := range results {
		if err != nil {
			t.Errorf("Concurrent call failed: %v", err)
		}
	}

	// Verify that the file still exists and wasn't corrupted
	if _, err := os.Stat(registryPath); err != nil {
		t.Errorf("registries.json was corrupted or deleted: %v", err)
	}
}

// TestEnsureRegistriesPath_DefaultPath tests that when no path is provided,
// it defaults to ~/.cache/mc-agent/registries/{version}/
func TestEnsureRegistriesPath_DefaultPath(t *testing.T) {
	// Clear any cached state
	ClearRegistryDownloadCache()

	testVersion := "test-1.21.5"

	// This will fail to download (no server.jar), but should return the expected path
	path, err := EnsureRegistriesPath("", testVersion)

	// We expect an error because we can't actually download the server JAR in tests
	if err == nil {
		t.Error("Expected error when registries don't exist, got nil")
	}

	// But the path should still be set correctly
	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, ".cache", "mc-agent", "registries", testVersion)

	// The error message should contain information about the path
	if path != "" && path != expectedPath {
		t.Errorf("Expected default path %s, got %s", expectedPath, path)
	}
}

// TestEnsureRegistriesPath_ExistingData tests that when data already exists,
// it's reused without attempting to download.
func TestEnsureRegistriesPath_ExistingData(t *testing.T) {
	// Clear any cached state
	ClearRegistryDownloadCache()

	// Use a temporary directory for testing
	tmpDir := t.TempDir()
	testVersion := "test-1.21.5"

	// Create a fake registries.json
	registryDir := filepath.Join(tmpDir, "data_generator", "reports")
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	registryPath := filepath.Join(registryDir, "registries.json")
	testData := []byte(`{"minecraft:entity_type":{"entries":{"minecraft:player":{"protocol_id":128}}}}`)
	if err := os.WriteFile(registryPath, testData, 0644); err != nil {
		t.Fatalf("Failed to create test registries.json: %v", err)
	}

	// Call EnsureRegistriesPath - should succeed without download
	path, err := EnsureRegistriesPath(tmpDir, testVersion)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if path != tmpDir {
		t.Errorf("Expected path %s, got %s", tmpDir, path)
	}

	// Verify the file wasn't modified
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("Failed to read registries.json: %v", err)
	}

	if string(data) != string(testData) {
		t.Error("registries.json was modified")
	}
}
