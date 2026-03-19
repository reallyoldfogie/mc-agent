package models

// MountedEntityPositionGetter provides the ability to retrieve the current position of a mounted entity.
// This is used by the movement executor when in riding mode to track the vehicle's position.
type MountedEntityPositionGetter interface {
	// GetMountedEntityPosition returns the current position of the mounted entity by ID.
	// Returns (x, y, z, found) - found is false if the entity is not tracked.
	GetMountedEntityPosition(entityID int32) (x, y, z float64, found bool)
}
