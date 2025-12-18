//go:build integration

package testing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/reallyoldfogie/mc-replay-go/mcpr"
)

// TestValidateExistingReplays validates all replay files in the replays directory.
// This is useful for batch-validating old replay files that may have been created
// before automatic validation was implemented.
func TestValidateExistingReplays(t *testing.T) {
	replaysDir := "./replays"
	entries, err := os.ReadDir(replaysDir)
	if err != nil {
		t.Skipf("No replays directory found: %v", err)
		return
	}

	var replayFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".mcpr" {
			replayFiles = append(replayFiles, filepath.Join(replaysDir, entry.Name()))
		}
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
		t.Run(filepath.Base(replayPath), func(t *testing.T) {
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
