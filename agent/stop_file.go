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

	clientName := "unknownClient"
	if a.client != nil {
		clientName = a.client.Name()
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
					log.Printf("[Agent %s] Stop file %s detected; initiating shutdown", clientName, stopPath)
					a.removeStopFileIfExists(stopPath)
					// Trigger shutdown by canceling the context
					a.mu.Lock()
					if a.cancel != nil {
						a.cancel()
					}
					a.mu.Unlock()
					return
				} else if !errors.Is(err, os.ErrNotExist) {
					log.Printf("[Agent %s] Error checking stop file %s: %v", clientName, stopPath, err)
				}
			}
		}
	}()
}

// removeStopFileIfExists removes the stop file and logs any errors.
func (a *agent) removeStopFileIfExists(stopPath string) {
	clientName := "unknownClient"
	if a.client != nil {
		clientName = a.client.Name()
	}

	if err := os.Remove(stopPath); err == nil {
		log.Printf("[Agent %s] Removed stop file %s", clientName, stopPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("[Agent %s] Error removing stop file %s: %v", clientName, stopPath, err)
	}
}
