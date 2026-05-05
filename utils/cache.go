package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

var baseCacheDir = []string{".agent", "cache"}

// FindOrCreateCacheDir locates or creates a {{baseCacheDir}} directory for storing downloaded data.
// It searches up the directory tree for an existing build/cache directory.
// If found, returns that path. If not found, creates one in the current working directory and returns it.
// This works regardless of where the program is run from (source tree or compiled binary) and avoids duplicating downloads.
//
// Search order:
// 1. Look up directory tree for existing {{baseCacheDir}} directory
// 2. Look for repo markers (.git, go.mod) to find the repo root
// 3. If not found, create {{baseCacheDir}} in current working directory
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

	// Search up the directory tree for:
	// 1. An existing build/cache directory
	// 2. Repo markers (.git, go.mod) that indicate repo root
	var repoRoot string
	current := cwd
	for {
		// Check if build/cache exists at this level
		buildCachePath := filepath.Join(current, filepath.Join(baseCacheDir...))
		if _, err := os.Stat(buildCachePath); err == nil {
			return buildCachePath, nil
		}

		// Check for repo markers (.git or go.mod) to identify repo root
		gitPath := filepath.Join(current, ".git")
		goModPath := filepath.Join(current, "go.mod")

		if _, err := os.Stat(gitPath); err == nil {
			// Found .git directory - this is repo root
			repoRoot = current
			break
		}
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod - this is repo root
			repoRoot = current
			break
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			break
		}
		current = parent
	}

	// No build/cache found anywhere, create one at repo root if found, else in cwd
	cacheLocation := cwd
	if repoRoot != "" {
		cacheLocation = repoRoot
	}
	cachePath := filepath.Join(cacheLocation, filepath.Join(baseCacheDir...))
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
