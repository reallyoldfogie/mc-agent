package agent

import (
	"errors"
	"log"
	"os"
	"time"
)

const (
	// StopFileCheckInterval is how often to poll for the stop file
	StopFileCheckInterval = 1 * time.Second
)

// startStopFileWatcher starts a goroutine that polls for the stop file.
// When the file is detected, it triggers graceful shutdown by calling cancel.
func (a *agent) startStopFileWatcher(stopPath string, ctxDone <-chan struct{}) {
	if stopPath == "" {
		return // Stop file watching is disabled
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(StopFileCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctxDone:
				return
			case <-ticker.C:
				if _, err := os.Stat(stopPath); err == nil {
					log.Printf("[agent] Stop file %s detected; initiating shutdown", stopPath)
					a.removeStopFileIfExists(stopPath)
					// Trigger shutdown by canceling the context
					a.mu.Lock()
					if a.cancel != nil {
						a.cancel()
					}
					a.mu.Unlock()
					return
				} else if !errors.Is(err, os.ErrNotExist) {
					log.Printf("[agent] Error checking stop file %s: %v", stopPath, err)
				}
			}
		}
	}()
}

// removeStopFileIfExists removes the stop file and logs any errors.
func (a *agent) removeStopFileIfExists(stopPath string) {
	if err := os.Remove(stopPath); err == nil {
		log.Printf("[agent] Removed stop file %s", stopPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("[agent] Error removing stop file %s: %v", stopPath, err)
	}
}
