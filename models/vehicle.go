package models

// VehicleType represents the type of vehicle an entity is
type VehicleType int

const (
	VehicleTypeNone VehicleType = iota
	VehicleTypeHorse
	VehicleTypeSkeletonHorse
	VehicleTypeZombieHorse
	VehicleTypeBoat
	VehicleTypeMinecart
	VehicleTypeCamel
	VehicleTypePig
	VehicleTypeStrider
	VehicleTypeDonkey
	VehicleTypeMule
	VehicleTypeChestBoat
	VehicleTypeCamelHusk
	VehicleTypeNautilus
	VehicleTypeZombieNautilus
	VehicleTypeLlama
	VehicleTypeTraderLlama
	VehicleTypeHappyGhast
)

// GetVehicleType returns what kind of vehicle an entity type represents.
// Returns VehicleTypeNone if the entity is not a vehicle.
func GetVehicleType(entityType EntityType) VehicleType {
	switch entityType {
	case EntityTypeHorse:
		return VehicleTypeHorse
	case EntityTypeSkeletonHorse:
		return VehicleTypeSkeletonHorse
	case EntityTypeZombieHorse:
		return VehicleTypeZombieHorse
	case EntityTypeBoat:
		return VehicleTypeBoat
	case EntityTypeChestBoat:
		return VehicleTypeChestBoat
	case EntityTypeMinecart:
		return VehicleTypeMinecart
	case EntityTypeCamel:
		return VehicleTypeCamel
	case EntityTypeCamelHusk:
		return VehicleTypeCamelHusk
	case EntityTypeNautilus:
		return VehicleTypeNautilus
	case EntityTypeZombieNautilus:
		return VehicleTypeZombieNautilus
	case EntityTypePig:
		return VehicleTypePig
	case EntityTypeStrider:
		return VehicleTypeStrider
	case EntityTypeDonkey:
		return VehicleTypeDonkey
	case EntityTypeMule:
		return VehicleTypeMule
	case EntityTypeLlama:
		return VehicleTypeLlama
	case EntityTypeTraderLlama:
		return VehicleTypeTraderLlama
	case EntityTypeHappyGhast:
		return VehicleTypeHappyGhast
	default:
		return VehicleTypeNone
	}
}

// String returns a human-readable name for the vehicle type
func (v VehicleType) String() string {
	switch v {
	case VehicleTypeNone:
		return "none"
	case VehicleTypeHorse:
		return "horse"
	case VehicleTypeSkeletonHorse:
		return "skeleton_horse"
	case VehicleTypeZombieHorse:
		return "zombie_horse"
	case VehicleTypeBoat:
		return "boat"
	case VehicleTypeChestBoat:
		return "chest_boat"
	case VehicleTypeMinecart:
		return "minecart"
	case VehicleTypeCamel:
		return "camel"
	case VehicleTypePig:
		return "pig"
	case VehicleTypeStrider:
		return "strider"
	case VehicleTypeDonkey:
		return "donkey"
	case VehicleTypeMule:
		return "mule"
	case VehicleTypeCamelHusk:
		return "camel_husk"
	case VehicleTypeNautilus:
		return "nautilus"
	case VehicleTypeZombieNautilus:
		return "zombie_nautilus"
	case VehicleTypeLlama:
		return "llama"
	case VehicleTypeTraderLlama:
		return "trader_llama"
	case VehicleTypeHappyGhast:
		return "happy_ghast"
	default:
		return "unknown"
	}
}

// IsRideable returns true if the vehicle type can be ridden
func (v VehicleType) IsRideable() bool {
	return v != VehicleTypeNone
}

// IsSteerable returns true if a rider can direct where the vehicle goes.
// Llamas accept a passenger but take no saddle and ignore rider input, so the
// rider is a passive passenger rather than the controlling passenger.
func (v VehicleType) IsSteerable() bool {
	switch v {
	case VehicleTypeNone, VehicleTypeLlama, VehicleTypeTraderLlama:
		return false
	default:
		return true
	}
}
