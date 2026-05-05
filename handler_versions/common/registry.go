package common

import (
	"fmt"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

var (
	// handlersMu protects the handlers map
	handlersMu sync.RWMutex

	// handlers maps version strings to their handler constructors
	handlers = make(map[string]func() models.VersionHandler)
)

// RegisterVersionHandler registers a version handler constructor.
// This is typically called from init() in version-specific packages.
func RegisterVersionHandler(version string, constructor func() models.VersionHandler) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	handlers[version] = constructor
}

// GetVersionHandler returns a new VersionHandler for the given version string.
// Returns nil and an error if the version is not supported.
func GetVersionHandler(version string) (models.VersionHandler, error) {
	handlersMu.RLock()
	constructor, ok := handlers[version]
	handlersMu.RUnlock()

	if !ok {
		// DEBUG: Print available versions
		availableVersions := SupportedVersions()
		println("[common.GetVersionHandler] Requested version:", version, "not found. Available:", availableVersions)
		return nil, fmt.Errorf("unsupported version: %s (available: %v)", version, availableVersions)
	}

	return constructor(), nil
}

// HasVersionHandler returns true if a handler is registered for the given version.
func HasVersionHandler(version string) bool {
	handlersMu.RLock()
	defer handlersMu.RUnlock()
	_, ok := handlers[version]
	return ok
}

// SupportedVersions returns a list of all registered version strings.
func SupportedVersions() []string {
	handlersMu.RLock()
	defer handlersMu.RUnlock()

	versions := make([]string, 0, len(handlers))
	for v := range handlers {
		versions = append(versions, v)
	}
	return versions
}
