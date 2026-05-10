package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

var baseCacheDir = []string{".agent", "cache"}

// FindOrCreateCacheDir locates or creates a {{baseCacheDir}} directory for storing downloaded data.
// It searches up the directory tree for the repo root (.git), then returns the cache dir
// at the repo root. This ensures all cache output is centralized regardless of which
// subdirectory the program is run from.
//
// Search order:
// 1. Walk up directory tree looking for .git to find the repo root
// 2. If no .git, walk up looking for go.mod
// 3. If repo root found, create {{baseCacheDir}} there
// 4. If no repo root, check if ~/{{baseCacheDir}} already exists (use it, don't create)
// 5. Last resort: create {{baseCacheDir}} in current working directory
//
// Example:
//
//	cacheDir, err := FindOrCreateCacheDir()
//	if err != nil { return err }
//	mcDataPath := filepath.Join(cacheDir, "mc-data-gen")
func FindOrCreateCacheDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	// Walk up directory tree looking for repo root markers.
	// Prioritize .git (definitive repo root) over go.mod (may exist in submodules).
	var repoRoot string
	current := cwd
	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			repoRoot = current
			break
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding .git
			break
		}
		current = parent
	}

	// If no .git found, fall back to go.mod as repo root indicator
	if repoRoot == "" {
		current = cwd
		for {
			goModPath := filepath.Join(current, "go.mod")
			if _, err := os.Stat(goModPath); err == nil {
				repoRoot = current
				break
			}

			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	// Use repo root if found
	if repoRoot != "" {
		cachePath := filepath.Join(repoRoot, filepath.Join(baseCacheDir...))
		if err := os.MkdirAll(cachePath, 0755); err != nil {
			return "", fmt.Errorf("create cache directory %s: %w", cachePath, err)
		}
		return cachePath, nil
	}

	// No repo root found (e.g. pre-compiled binary).
	// Check if ~/.agent/cache already exists and use it without creating it.
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeCachePath := filepath.Join(homeDir, filepath.Join(baseCacheDir...))
		if _, err := os.Stat(homeCachePath); err == nil {
			return homeCachePath, nil
		}
	}

	// Last resort: create in CWD
	cachePath := filepath.Join(cwd, filepath.Join(baseCacheDir...))
	if err := os.MkdirAll(cachePath, 0755); err != nil {
		return "", fmt.Errorf("create cache directory %s: %w", cachePath, err)
	}

	return cachePath, nil
}

// FindRepoRoot locates the repository root by searching for go.mod.
// This works regardless of where the program is run from.
// Returns the absolute path to the directory containing go.mod.
func FindRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	// Walk up directory tree looking for go.mod
	current := cwd
	for {
		goModPath := filepath.Join(current, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			return "", fmt.Errorf("could not find go.mod in any parent directory of %s", cwd)
		}
		current = parent
	}
}
