package models

// MountedEntityPositionGetter provides the ability to retrieve the current position of a mounted entity.
// This is used by the movement executor when in riding mode to track the vehicle's position.
type MountedEntityPositionGetter interface {
	// GetMountedEntityPosition returns the current position of the mounted entity by ID.
	// Returns (x, y, z, found) - found is false if the entity is not tracked.
	GetMountedEntityPosition(entityID int32) (x, y, z float64, found bool)

	// GetMountedEntityYaw returns the yaw of the mounted entity in degrees.
	// Returns (yaw, found) - found is false if the entity is not tracked.
	GetMountedEntityYaw(entityID int32) (yaw float64, found bool)

	GetMountedEntityType(entityID int32) (int32, bool)
	IsMountedEntityBoat(int32) bool
	IsMountedEntityMinecart(int32) bool

	// GetEntityAttribute retrieves an entity attribute value by name.
	// Returns (value, found) - found is false if the entity or attribute is not tracked.
	// Common attributes: "generic.movement_speed", "generic.max_health", etc.
	GetEntityAttribute(entityID int32, attributeName string) (float64, bool)

	// GetEntityVelocity returns the current velocity of an entity by ID.
	// Returns (velX, velY, velZ, found) in Minecraft protocol units (×8000 blocks/tick).
	// found is false if the entity is not tracked.
	GetEntityVelocity(entityID int32) (velX, velY, velZ float64, found bool)
}
