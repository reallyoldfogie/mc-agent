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
	IsMountedEntityCamel(int32) bool
	IsMountedEntityNautilus(int32) bool
	IsMountedEntityZombieNautilus(int32) bool
	IsMountedEntityPig(int32) bool
	IsMountedEntityStrider(int32) bool
	IsMountedEntityDonkey(int32) bool
	IsMountedEntityMule(int32) bool

	// IsMountedEntityLlama reports whether the mounted entity is a llama or
	// trader llama. Llamas can be ridden but not steered, so they need the
	// passive-passenger handler rather than a controlling-passenger one.
	IsMountedEntityLlama(int32) bool

	// GetMountedPassengerIndex returns our seat index in the mounted vehicle's
	// passenger list, or -1 when not mounted. Index 0 is the controlling
	// passenger; every later index is a passive rider that must not predict the
	// vehicle's movement. Boats and camels seat two, and a happy ghast seats
	// four, so this is what separates the driver from the passengers.
	GetMountedPassengerIndex() int

	// GetEntityAttribute retrieves an entity attribute value by name.
	// Returns (value, found) - found is false if the entity or attribute is not tracked.
	// Common attributes: "generic.movement_speed", "generic.max_health", etc.
	GetEntityAttribute(entityID int32, attributeName string) (float64, bool)

	// GetEntityVelocity returns the current velocity of an entity by ID.
	// Returns (velX, velY, velZ, found) in Minecraft protocol units (×8000 blocks/tick).
	// found is false if the entity is not tracked.
	GetEntityVelocity(entityID int32) (velX, velY, velZ float64, found bool)

	// GetRiderHeldItem returns the local item name (e.g. "carrot_on_a_stick") of the rider's
	// currently held item in their active hotbar slot. Returns ("", false) if no item found.
	// The name has no namespace prefix.
	GetRiderHeldItem() (itemName string, found bool)
}
