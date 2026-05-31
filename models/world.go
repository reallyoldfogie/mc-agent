package models

// World represents the minimal world access needed for block queries and world state.
type World interface {
	// GetBlockAt returns the block state ID at the given world coordinates.
	// The second return value indicates whether the chunk is loaded:
	//   - true: chunk is loaded (stateID is valid, may be 0 for air)
	//   - false: chunk is not loaded (stateID should be ignored)
	GetBlockAt(x, y, z float64) (stateID uint32, chunkLoaded bool)

	// GetWorldAge returns the current server world age in ticks and whether it's been initialized.
	// Returns (0, false) if no Update Time packet has been received yet.
	// Returns (worldAge, true) when the value is valid from the server.
	GetWorldAge() (int64, bool)

	// GetTimeOfDay returns the current time of day in ticks and whether it's been initialized.
	// Time of day ranges from 0-23999 ticks per day.
	// Returns (0, false) if no Update Time packet has been received yet.
	// Returns (timeOfDay, true) when the value is valid from the server.
	GetTimeOfDay() (int64, bool)

	// SetWorldTime updates both world age and time of day from the server.
	// Called when a ClientboundUpdateTime packet is received.
	SetWorldTime(worldAge, timeOfDay int64)
}
