package pathfinding

import (
	"encoding/json"
	"fmt"
	"os"
)

// StatePropertyLoader loads block state properties from JSON at runtime
type StatePropertyLoader struct {
	stateProperties map[uint32]map[string]string
}

// BlockRecord represents a block in the blocks.json report
type BlockRecord struct {
	Properties map[string][]string `json:"properties"`
	States     []StateRecord       `json:"states"`
}

// StateRecord represents a single block state
type StateRecord struct {
	ID         uint32            `json:"id"`
	Properties map[string]string `json:"properties"`
	Default    bool              `json:"default,omitempty"`
}

// NewStatePropertyLoader creates a loader from blocks.json using the Minecraft data cache.
// The blocksJSONPath parameter should be obtained from MinecraftDataCache.GetBlocksJSONPath().
//
// Example:
//
//	cache := utils.NewMinecraftDataCache("1.21.5")
//	if err := cache.EnsureDataGenerated(); err != nil {
//	    return err
//	}
//	blocksPath, err := cache.GetBlocksJSONPath()
//	if err != nil {
//	    return err
//	}
//	loader, err := NewStatePropertyLoader(blocksPath)
func NewStatePropertyLoader(blocksJSONPath string) (*StatePropertyLoader, error) {
	blocksPath := blocksJSONPath

	// Read the JSON file
	data, err := os.ReadFile(blocksPath)
	if err != nil {
		return nil, fmt.Errorf("read blocks.json: %w", err)
	}

	// Parse as map[blockID]BlockRecord
	var blocks map[string]BlockRecord
	if err := json.Unmarshal(data, &blocks); err != nil {
		return nil, fmt.Errorf("unmarshal blocks.json: %w", err)
	}

	// Build StateID → Properties map
	stateProperties := make(map[uint32]map[string]string)
	for _, block := range blocks {
		for _, state := range block.States {
			// Copy properties map
			props := make(map[string]string, len(state.Properties))
			for k, v := range state.Properties {
				props[k] = v
			}
			stateProperties[state.ID] = props
		}
	}

	return &StatePropertyLoader{
		stateProperties: stateProperties,
	}, nil
}

// GetProperties returns the properties for a given state ID
func (spl *StatePropertyLoader) GetProperties(stateID uint32) map[string]string {
	props, ok := spl.stateProperties[stateID]
	if !ok {
		return make(map[string]string)
	}
	return props
}
