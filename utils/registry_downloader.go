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

// Global semaphore map to prevent concurrent downloads of the same version
var (
	registryDownloadMu    sync.Mutex
	registryDownloadSems  = make(map[string]*sync.Mutex)
	registryDownloadCache = make(map[string]bool) // tracks successful downloads
)

// EnsureRegistriesPath ensures the registries path is set and the data exists.
// If path is empty, sets it to ~/.cache/mc-agent/registries/{version}/
// Downloads and generates registries.json if not already cached.
// Thread-safe: uses per-version semaphore to prevent concurrent downloads.
//
// DEPRECATED: Use NewMinecraftDataCache(version).GetRegistriesPath() instead.
// This function is maintained for backward compatibility only.
//
// Returns the resolved registries path.
func EnsureRegistriesPath(path, version string) (string, error) {
	// If path provided, use legacy behavior
	if path != "" {
		return ensureRegistriesPathLegacy(path, version)
	}

	// Use new unified cache system
	cache := NewMinecraftDataCache(version)
	if err := cache.EnsureDataGenerated(); err != nil {
		return "", err
	}
	return cache.GetRegistriesPath()
}

// ensureRegistriesPathLegacy is the original implementation, kept for custom path support.
func ensureRegistriesPathLegacy(path, version string) (string, error) {
	// Set default path if not provided
	if path == "" {
		cacheDir, err := FindOrCreateCacheDir()
		if err != nil {
			return "", fmt.Errorf("failed to find cache directory: %w", err)
		}
		path = filepath.Join(cacheDir, "registries", version)
	}

	// Get or create semaphore for this version
	registryDownloadMu.Lock()
	sem, exists := registryDownloadSems[version]
	if !exists {
		sem = &sync.Mutex{}
		registryDownloadSems[version] = sem
	}
	// Check if already downloaded successfully
	alreadyDownloaded := registryDownloadCache[version]
	registryDownloadMu.Unlock()

	// If already successfully downloaded, just verify and return
	if alreadyDownloaded {
		registryPath := filepath.Join(path, "data_generator", "reports", "registries.json")
		if _, err := os.Stat(registryPath); err == nil {
			return path, nil
		}
		// Cache was stale, need to re-download
		registryDownloadMu.Lock()
		registryDownloadCache[version] = false
		registryDownloadMu.Unlock()
	}

	// Acquire version-specific semaphore
	sem.Lock()
	defer sem.Unlock()

	// Check again after acquiring lock (another goroutine may have downloaded)
	registryPath := filepath.Join(path, "data_generator", "reports", "registries.json")
	if _, err := os.Stat(registryPath); err == nil {
		// Mark as successfully cached
		registryDownloadMu.Lock()
		registryDownloadCache[version] = true
		registryDownloadMu.Unlock()
		log.Printf("[RegistryDownloader] Using cached registries for %s: %s", version, path)
		return path, nil
	}

	// Need to download and generate
	log.Printf("[RegistryDownloader] Generating registries for %s...", version)
	if err := generateRegistries(path, version); err != nil {
		return "", fmt.Errorf("failed to generate registries: %w", err)
	}

	// Mark as successfully cached
	registryDownloadMu.Lock()
	registryDownloadCache[version] = true
	registryDownloadMu.Unlock()

	log.Printf("[RegistryDownloader] Successfully generated registries for %s: %s", version, path)
	return path, nil
}

// generateRegistries downloads the Minecraft server JAR and runs data generation
func generateRegistries(targetPath, version string) error {
	// Create target directory
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download server JAR
	serverJar := filepath.Join(targetPath, "server.jar")
	if err := downloadServerJar(serverJar, version); err != nil {
		return fmt.Errorf("failed to download server JAR: %w", err)
	}

	// Run data generator
	if err := runDataGenerator(targetPath, serverJar); err != nil {
		return fmt.Errorf("failed to run data generator: %w", err)
	}

	// Verify registries.json was created
	// The server JAR outputs to "generated/reports/registries.json"
	generatedPath := filepath.Join(targetPath, "generated", "reports", "registries.json")
	expectedPath := filepath.Join(targetPath, "data_generator", "reports", "registries.json")

	if _, err := os.Stat(generatedPath); err == nil {
		// Create expected directory structure
		if err := os.MkdirAll(filepath.Dir(expectedPath), 0755); err != nil {
			return fmt.Errorf("failed to create data_generator directory: %w", err)
		}

		// Create symlink or copy to expected location
		if err := os.Symlink(generatedPath, expectedPath); err != nil {
			// Symlink failed, try copying
			data, readErr := os.ReadFile(generatedPath)
			if readErr != nil {
				return fmt.Errorf("failed to read generated registries.json: %w", readErr)
			}
			if writeErr := os.WriteFile(expectedPath, data, 0644); writeErr != nil {
				return fmt.Errorf("failed to copy registries.json to expected location: %w", writeErr)
			}
		}
	} else if _, err := os.Stat(expectedPath); err != nil {
		return fmt.Errorf("registries.json not found at %s or %s", generatedPath, expectedPath)
	}

	return nil
}

// downloadServerJar downloads the Minecraft server JAR for the specified version
func downloadServerJar(destPath, version string) error {
	// Check if already exists
	if _, err := os.Stat(destPath); err == nil {
		log.Printf("[RegistryDownloader] Server JAR already exists: %s", destPath)
		return nil
	}

	// Get download URL from Mojang version manifest
	downloadURL, err := getServerJarURL(version)
	if err != nil {
		return fmt.Errorf("failed to get server JAR URL: %w", err)
	}

	log.Printf("[RegistryDownloader] Downloading server JAR for %s from %s...", version, downloadURL)

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

	log.Printf("[RegistryDownloader] Server JAR downloaded successfully")
	return nil
}

// getServerJarURL fetches the server JAR download URL from Mojang's version manifest
func getServerJarURL(version string) (string, error) {
	// Fetch version manifest from Mojang
	manifestURL := "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

	log.Printf("[RegistryDownloader] Fetching version manifest from Mojang...")
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
		if v.ID == version {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return "", fmt.Errorf("version %s not found in Mojang manifest", version)
	}

	// Fetch version-specific JSON
	log.Printf("[RegistryDownloader] Fetching version details for %s...", version)
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
		return "", fmt.Errorf("server download URL not found for version %s", version)
	}

	return versionInfo.Downloads.Server.URL, nil
}

// runDataGenerator runs the Minecraft server JAR with data generation flags
func runDataGenerator(workDir, jarPath string) error {
	log.Printf("[RegistryDownloader] Running data generator...")

	// Create command: java -DbundlerMainClass=net.minecraft.data.Main -jar server.jar --reports
	cmd := exec.Command("java",
		"-DbundlerMainClass=net.minecraft.data.Main",
		"-jar", jarPath,
		"--reports",
	)
	cmd.Dir = workDir

	// Capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[RegistryDownloader] Data generator output:\n%s", string(output))
		return fmt.Errorf("data generator failed: %w", err)
	}

	log.Printf("[RegistryDownloader] Data generator completed successfully")
	return nil
}

// ClearRegistryDownloadCache clears the in-memory cache of successful downloads.
// Useful for testing or if you want to force a re-check of the filesystem.
func ClearRegistryDownloadCache() {
	registryDownloadMu.Lock()
	defer registryDownloadMu.Unlock()
	registryDownloadCache = make(map[string]bool)
}
