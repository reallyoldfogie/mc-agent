package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RegistryEntry represents a single entry in a registry with its protocol ID
type RegistryEntry struct {
	ProtocolID int32 `json:"protocol_id"`
}

// Registry represents a single registry from registries.json
type Registry struct {
	Entries map[string]RegistryEntry `json:"entries"`
}

// LoadEntityTypesFromRegistry loads ALL registries from registries.json and populates
// the agent's registry system.
//
// In practice, registries in this file are complementary to those sent via config packets:
//   - File-only: entity_type, menu, block, item, etc. (client-side game data)
//   - Packet-only: worldgen/biome, chat_type, trim_pattern, etc. (server-specific data)
//
// This function should be called BEFORE agent.Init() so that all registries are available
// before the agent starts connecting to the server.
//
// If dataPath is empty and cfg.RegistriesPath is set, uses cfg.RegistriesPath.
// The registries.json file should be at: {dataPath}/data_generator/reports/registries.json
//
// Returns error if file cannot be read or parsed.
func (a *agent) LoadRegistriesFromFile(dataPath string) error {
	// Use cfg.RegistriesPath if dataPath not provided
	if dataPath == "" && a.cfg.RegistriesPath != "" {
		dataPath = a.cfg.RegistriesPath
	}

	if dataPath == "" {
		dataPath = "." // Default to current directory if no path provided
		a.logf("[RegistryLoader][WARN] No dataPath provided for LoadRegistriesFromFile, defaulting to current directory: %s", dataPath)
	}

	registryPath := dataPath
	if fileInfo, err := os.Stat(dataPath); err == nil && fileInfo.IsDir() {
		registryPath = filepath.Join(dataPath, "data_generator", "reports", "registries.json")
	}

	// Read the registries file
	data, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("failed to read registries.json: %w", err)
	}

	// Parse the JSON as a map of registry ID -> Registry
	var registries map[string]Registry
	if err := json.Unmarshal(data, &registries); err != nil {
		return fmt.Errorf("failed to parse registries.json: %w", err)
	}

	if len(registries) == 0 {
		a.logf("[RegistryLoader][WARN] no registries found in : %s", registryPath)
		return fmt.Errorf("no registries found in registries.json")
	}

	a.logf("[RegistryLoader] Loading %d registries from file: %s", len(registries), registryPath)

	// Load each registry into the agent's registry system
	totalLoaded := 0
	for registryID, registry := range registries {
		// Build entries map
		entries := make(map[string]int32)
		for name, entry := range registry.Entries {
			entries[name] = entry.ProtocolID
		}

		if len(entries) > 0 {
			// Store in the agent's registry system via onRegistryDataCallback
			// This logs each registry being stored (see registry.go for details)
			// Note: onRegistryDataCallback handles overwrites - if this registry
			// is later received via config packet, the packet data will replace this
			a.onRegistryDataCallback(registryID, entries)
			totalLoaded++
		} else {
			a.logf("[RegistryLoader] Skipping %s registry (no entries found)", registryID)
		}
	}

	a.logf("[RegistryLoader] ✓ Successfully loaded %d registries from file", totalLoaded)
	return nil
}
