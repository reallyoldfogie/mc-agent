package models

// World represents the minimal world access needed for block queries.
type World interface {
	// GetBlockAt returns the block state ID at the given world coordinates.
	// The second return value indicates whether the chunk is loaded:
	//   - true: chunk is loaded (stateID is valid, may be 0 for air)
	//   - false: chunk is not loaded (stateID should be ignored)
	GetBlockAt(x, y, z float64) (stateID uint32, chunkLoaded bool)
}
