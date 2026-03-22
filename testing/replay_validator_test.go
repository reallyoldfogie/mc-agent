package testing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reallyoldfogie/mc-agent/utils"
	"github.com/reallyoldfogie/mc-replay-go/mcpr"
)

// TestValidateExistingReplays validates all replay files in the replays directory
// and all subdirectories recursively.
// This is useful for batch-validating old replay files that may have been created
// before automatic validation was implemented.
func TestValidateExistingReplays(t *testing.T) {
	cacheDir, err := utils.FindOrCreateCacheDir()
	if err != nil {
		t.Fatalf("find cache directory: %v", err)
	}
	replaysDir := filepath.Join(cacheDir, "replays")

	var replayFiles []string

	// Walk through all subdirectories recursively
	err = filepath.WalkDir(replaysDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories, only process files
		if !d.IsDir() && filepath.Ext(d.Name()) == ".mcpr" {
			replayFiles = append(replayFiles, path)
		}
		return nil
	})

	if err != nil {
		t.Skipf("No replays directory found: %v", err)
		return
	}

	if len(replayFiles) == 0 {
		t.Skip("No replay files found to validate")
		return
	}

	logger := NewTestLogger(t)
	logger.Logf("Validating %d replay files...", len(replayFiles))

	validCount := 0
	invalidCount := 0

	for _, replayPath := range replayFiles {
		t.Run(filepath.Base(filepath.Dir(replayPath))+"/"+filepath.Base(replayPath), func(t *testing.T) {
			// Validate using mc-replay-go's built-in validator
			err := mcpr.ValidateFile(replayPath)

			if err != nil {
				t.Errorf("❌ Validation failed: %v", err)
				invalidCount++
			} else {
				t.Logf("✅ Replay is valid")
				validCount++
			}
		})
	}

	// Summary
	logger.Logf("\n=== Validation Summary ===")
	logger.Logf("Total: %d replays", len(replayFiles))
	logger.Logf("Valid: %d", validCount)
	logger.Logf("Invalid: %d", invalidCount)
}
