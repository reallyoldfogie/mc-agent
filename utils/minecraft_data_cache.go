package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// MinecraftDataCache manages the download and caching of Minecraft version-specific data.
// It consolidates all data generator outputs (registries.json, blocks.json, items.json, etc.)
// into a single unified cache directory.
//
// Cache Structure:
//
//	~/.cache/mc-agent/minecraft-data/{version}/
//	  ├── server.jar                    (downloaded from Mojang)
//	  ├── generated/reports/            (output from data generator)
//	  │   ├── registries.json
//	  │   ├── blocks.json
//	  │   ├── items.json
//	  │   └── ...
//	  └── data_generator/reports/       (symlinked/copied for compatibility)
//	      └── ...
//
// Thread-safe: Uses per-version mutex to prevent concurrent downloads.
type MinecraftDataCache struct {
	basePath string // Base cache directory (default: ~/.cache/mc-agent/minecraft-data)
	version  string // Minecraft version (e.g., "1.21.5")
}

// Global state for concurrency control
var (
	dataCacheMu    sync.Mutex
	dataCacheSems  = make(map[string]*sync.Mutex)
	dataCacheReady = make(map[string]bool) // tracks successful data generation
)

// NewMinecraftDataCache creates a new data cache manager for a specific Minecraft version.
// Uses default cache location: ~/.cache/mc-agent/minecraft-data/{version}
func NewMinecraftDataCache(version string) *MinecraftDataCache {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home dir unavailable
		homeDir = "."
	}
	basePath := filepath.Join(homeDir, ".agent", "cache", "minecraft-data", version)

	return &MinecraftDataCache{
		basePath: basePath,
		version:  version,
	}
}

// NewMinecraftDataCacheWithPath creates a data cache manager with a custom base path.
// Useful for testing or custom deployment scenarios.
func NewMinecraftDataCacheWithPath(version, customBasePath string) *MinecraftDataCache {
	return &MinecraftDataCache{
		basePath: customBasePath,
		version:  version,
	}
}

// EnsureDataGenerated ensures that all Minecraft data has been generated and cached.
// Downloads the server JAR and runs the data generator if not already cached.
// Thread-safe: Only one goroutine will download per version.
//
// Returns nil if data is ready (cached or freshly generated).
func (mdc *MinecraftDataCache) EnsureDataGenerated() error {
	// Get or create semaphore for this version
	dataCacheMu.Lock()
	sem, exists := dataCacheSems[mdc.version]
	if !exists {
		sem = &sync.Mutex{}
		dataCacheSems[mdc.version] = sem
	}
	// Check if already generated successfully
	alreadyReady := dataCacheReady[mdc.version]
	dataCacheMu.Unlock()

	// If already ready, just verify files exist
	if alreadyReady {
		if mdc.verifyDataExists() {
			return nil
		}
		// Cache was stale, need to re-generate
		dataCacheMu.Lock()
		dataCacheReady[mdc.version] = false
		dataCacheMu.Unlock()
	}

	// Acquire version-specific semaphore
	sem.Lock()
	defer sem.Unlock()

	// Check again after acquiring lock (another goroutine may have generated)
	if mdc.verifyDataExists() {
		dataCacheMu.Lock()
		dataCacheReady[mdc.version] = true
		dataCacheMu.Unlock()
		log.Printf("[MinecraftDataCache] Using cached data for %s: %s", mdc.version, mdc.basePath)
		return nil
	}

	// Need to download and generate
	log.Printf("[MinecraftDataCache] Generating data for %s...", mdc.version)
	if err := mdc.generateData(); err != nil {
		return fmt.Errorf("failed to generate data: %w", err)
	}

	// Mark as ready
	dataCacheMu.Lock()
	dataCacheReady[mdc.version] = true
	dataCacheMu.Unlock()

	log.Printf("[MinecraftDataCache] Successfully generated data for %s: %s", mdc.version, mdc.basePath)
	return nil
}

// GetReportsDir returns the path to the data_generator/reports directory.
// This is the directory containing registries.json, blocks.json, items.json, etc.
func (mdc *MinecraftDataCache) GetReportsDir() (string, error) {
	if err := mdc.EnsureDataGenerated(); err != nil {
		return "", err
	}
	return filepath.Join(mdc.basePath, "data_generator", "reports"), nil
}

// GetRegistriesPath returns the path to registries.json.
func (mdc *MinecraftDataCache) GetRegistriesPath() (string, error) {
	reportsDir, err := mdc.GetReportsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(reportsDir, "registries.json"), nil
}

// GetBlocksJSONPath returns the path to blocks.json.
// This file contains block state properties needed for distinguishing
// between different slab types, stair orientations, etc.
func (mdc *MinecraftDataCache) GetBlocksJSONPath() (string, error) {
	reportsDir, err := mdc.GetReportsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(reportsDir, "blocks.json"), nil
}

// GetItemsJSONPath returns the path to items.json.
func (mdc *MinecraftDataCache) GetItemsJSONPath() (string, error) {
	reportsDir, err := mdc.GetReportsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(reportsDir, "items.json"), nil
}

// GetCommandsJSONPath returns the path to commands.json.
func (mdc *MinecraftDataCache) GetCommandsJSONPath() (string, error) {
	reportsDir, err := mdc.GetReportsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(reportsDir, "commands.json"), nil
}

// verifyDataExists checks if all required data files exist in the cache.
func (mdc *MinecraftDataCache) verifyDataExists() bool {
	// Check for key files
	reportsDir := filepath.Join(mdc.basePath, "data_generator", "reports")
	requiredFiles := []string{
		filepath.Join(reportsDir, "registries.json"),
		filepath.Join(reportsDir, "blocks.json"),
	}

	for _, file := range requiredFiles {
		if _, err := os.Stat(file); err != nil {
			return false
		}
	}
	return true
}

// generateData downloads the server JAR and runs the data generator.
func (mdc *MinecraftDataCache) generateData() error {
	// Create base directory
	if err := os.MkdirAll(mdc.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download server JAR
	serverJar := filepath.Join(mdc.basePath, "server.jar")
	if err := mdc.downloadServerJar(serverJar); err != nil {
		return fmt.Errorf("failed to download server JAR: %w", err)
	}

	// Run data generator
	if err := mdc.runDataGenerator(serverJar); err != nil {
		return fmt.Errorf("failed to run data generator: %w", err)
	}

	// Create compatibility symlinks/copies
	if err := mdc.setupCompatibilityLinks(); err != nil {
		return fmt.Errorf("failed to setup compatibility links: %w", err)
	}

	return nil
}

// setupCompatibilityLinks creates symlinks or copies from generated/reports to data_generator/reports.
func (mdc *MinecraftDataCache) setupCompatibilityLinks() error {
	generatedPath := filepath.Join(mdc.basePath, "generated", "reports")
	expectedPath := filepath.Join(mdc.basePath, "data_generator", "reports")

	// Check if generated reports exist
	if _, err := os.Stat(generatedPath); err != nil {
		return fmt.Errorf("generated reports directory not found: %w", err)
	}

	// Create expected directory
	if err := os.MkdirAll(filepath.Dir(expectedPath), 0755); err != nil {
		return fmt.Errorf("failed to create data_generator directory: %w", err)
	}

	// Try to create symlink
	if err := os.Symlink(generatedPath, expectedPath); err != nil {
		// Symlink failed (maybe Windows or no permissions), try copying directory
		log.Printf("[MinecraftDataCache] Symlink failed, copying directory instead: %v", err)
		if err := copyDir(generatedPath, expectedPath); err != nil {
			return fmt.Errorf("failed to copy reports directory: %w", err)
		}
	}

	return nil
}

// downloadServerJar downloads the Minecraft server JAR for the specified version.
func (mdc *MinecraftDataCache) downloadServerJar(destPath string) error {
	// Check if already exists
	if _, err := os.Stat(destPath); err == nil {
		log.Printf("[MinecraftDataCache] Server JAR already exists: %s", destPath)
		return nil
	}

	// Get download URL from Mojang version manifest
	downloadURL, err := mdc.getServerJarURL()
	if err != nil {
		return fmt.Errorf("failed to get server JAR URL: %w", err)
	}

	log.Printf("[MinecraftDataCache] Downloading server JAR for %s from %s...", mdc.version, downloadURL)

	// Download to temp file first
	tmpFile, err := os.CreateTemp(filepath.Dir(destPath), "server-*.jar")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Download with retry
	if err := downloadWithRetry(downloadURL, tmpFile, 3); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	tmpFile.Close()

	// Move to final location
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to move downloaded JAR: %w", err)
	}

	log.Printf("[MinecraftDataCache] Server JAR downloaded successfully")
	return nil
}

// getServerJarURL fetches the server JAR download URL from Mojang's version manifest.
func (mdc *MinecraftDataCache) getServerJarURL() (string, error) {
	// Fetch version manifest from Mojang
	manifestURL := "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

	log.Printf("[MinecraftDataCache] Fetching version manifest from Mojang...")
	resp, err := http.Get(manifestURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch version manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("version manifest request failed with status %d", resp.StatusCode)
	}

	// Parse version manifest
	var manifest struct {
		Versions []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return "", fmt.Errorf("failed to parse version manifest: %w", err)
	}

	// Find the version URL
	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == mdc.version {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return "", fmt.Errorf("version %s not found in Mojang manifest", mdc.version)
	}

	// Fetch version-specific JSON
	log.Printf("[MinecraftDataCache] Fetching version details for %s...", mdc.version)
	resp2, err := http.Get(versionURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch version details: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		return "", fmt.Errorf("version details request failed with status %d", resp2.StatusCode)
	}

	// Parse version details
	var versionInfo struct {
		Downloads struct {
			Server struct {
				URL string `json:"url"`
			} `json:"server"`
		} `json:"downloads"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&versionInfo); err != nil {
		return "", fmt.Errorf("failed to parse version details: %w", err)
	}

	if versionInfo.Downloads.Server.URL == "" {
		return "", fmt.Errorf("server download URL not found for version %s", mdc.version)
	}

	return versionInfo.Downloads.Server.URL, nil
}

// runDataGenerator runs the Minecraft server JAR with data generation flags.
func (mdc *MinecraftDataCache) runDataGenerator(jarPath string) error {
	log.Printf("[MinecraftDataCache] Running data generator...")

	// Create command: java -DbundlerMainClass=net.minecraft.data.Main -jar server.jar --reports
	cmd := exec.Command("java",
		"-DbundlerMainClass=net.minecraft.data.Main",
		"-jar", jarPath,
		"--reports",
	)
	cmd.Dir = mdc.basePath

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[MinecraftDataCache] Data generator output:\n%s", string(output))
		return fmt.Errorf("data generator failed: %w", err)
	}

	log.Printf("[MinecraftDataCache] Data generator completed successfully")
	return nil
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		// Create directory or copy file
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// ClearDataCache clears the in-memory cache of successful data generation.
// Useful for testing or forcing a re-check of the filesystem.
func ClearDataCache() {
	dataCacheMu.Lock()
	defer dataCacheMu.Unlock()
	dataCacheReady = make(map[string]bool)
}
