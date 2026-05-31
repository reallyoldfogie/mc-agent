package models

// VehicleType represents the type of vehicle an entity is
type VehicleType int

const (
	VehicleTypeNone VehicleType = iota
	VehicleTypeHorse
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
)

// GetVehicleType returns what kind of vehicle an entity type represents.
// Returns VehicleTypeNone if the entity is not a vehicle.
func GetVehicleType(entityType EntityType) VehicleType {
	switch entityType {
	case EntityTypeHorse:
		return VehicleTypeHorse
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
	default:
		return "unknown"
	}
}

// IsRideable returns true if the vehicle type can be ridden
func (v VehicleType) IsRideable() bool {
	return v != VehicleTypeNone
}
