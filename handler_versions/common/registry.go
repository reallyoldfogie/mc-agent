package common

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/utils"
)

var (
	// handlersMu protects the handlers map
	handlersMu sync.RWMutex

	// handlers maps version strings to their handler constructors. Each
	// constructor takes the calling *agent's own *slog.Logger (see
	// docs/bugs/global-log-output-not-per-agent.md Phase 2) so every
	// version-specific Handler it builds - and every sub-handler that
	// Handler lazily creates - logs through that same per-agent logger
	// instead of the package-global "log" package/bare println.
	handlers = make(map[string]func(logger *slog.Logger) models.VersionHandler)
)

// RegisterVersionHandler registers a version handler constructor.
// This is typically called from init() in version-specific packages.
func RegisterVersionHandler(version string, constructor func(logger *slog.Logger) models.VersionHandler) {
	handlersMu.Lock()
	defer handlersMu.Unlock()
	handlers[version] = constructor
}

// GetVersionHandler returns a new VersionHandler for the given version
// string, attributing everything it logs to logger (nil falls back to
// slog.Default(), matching utils.SafeLogger's contract elsewhere).
// Returns nil and an error if the version is not supported.
func GetVersionHandler(version string, logger *slog.Logger) (models.VersionHandler, error) {
	logger = utils.SafeLogger(logger)

	handlersMu.RLock()
	constructor, ok := handlers[version]
	handlersMu.RUnlock()

	if !ok {
		availableVersions := SupportedVersions()
		logger.Warn("[common.GetVersionHandler] requested version not found", "version", version, "available", availableVersions)
		return nil, fmt.Errorf("unsupported version: %s (available: %v)", version, availableVersions)
	}

	return constructor(logger), nil
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
