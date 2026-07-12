package models

// VehicleCapabilities describes what terrain a vehicle can traverse and how fast.
type VehicleCapabilities struct {
	CanTraverseLand  bool    // Can move on solid ground
	CanTraverseWater bool    // Can move on water surface
	CanTraverseLava  bool    // Can move on lava (strider only)
	RequiresRails    bool    // Requires rails to move (minecart only)
	CanFly3D         bool    // Can move in 3D underwater (nautilus only)
	CanAscend        bool    // Can jump up 1-block ledges
	SpeedMultiplier  float64 // Speed relative to walking (1.0 = same speed as walking)
	VehicleMovement  MovementType // Primary terrain movement type for this vehicle
}

// GetVehicleCapabilities returns the capabilities for a given vehicle type.
func GetVehicleCapabilities(vt VehicleType) VehicleCapabilities {
	switch vt {
	case VehicleTypeHorse, VehicleTypeSkeletonHorse, VehicleTypeZombieHorse:
		return VehicleCapabilities{
			CanTraverseLand:  true,
			CanTraverseWater: false,
			CanTraverseLava:  false,
			RequiresRails:    false,
			CanFly3D:         false,
			CanAscend:        true,
			SpeedMultiplier:  1.4,
			VehicleMovement:  VehicleTraverse,
		}

	case VehicleTypeCamel, VehicleTypeCamelHusk:
		return VehicleCapabilities{
			CanTraverseLand:  true,
			CanTraverseWater: false,
			CanTraverseLava:  false,
			RequiresRails:    false,
			CanFly3D:         false,
			CanAscend:        true,
			SpeedMultiplier:  1.3,
			VehicleMovement:  VehicleTraverse,
		}

	case VehicleTypePig:
		return VehicleCapabilities{
			CanTraverseLand:  true,
			CanTraverseWater: false,
			CanTraverseLava:  false,
			RequiresRails:    false,
			CanFly3D:         false,
			CanAscend:        false, // Pigs cannot ascend like horses
			SpeedMultiplier:  1.25,
			VehicleMovement:  VehicleTraverse,
		}

	case VehicleTypeDonkey, VehicleTypeMule:
		return VehicleCapabilities{
			CanTraverseLand:  true,
			CanTraverseWater: false,
			CanTraverseLava:  false,
			RequiresRails:    false,
			CanFly3D:         false,
			CanAscend:        true,
			SpeedMultiplier:  1.3,
			VehicleMovement:  VehicleTraverse,
		}

	case VehicleTypeBoat, VehicleTypeChestBoat:
		return VehicleCapabilities{
			CanTraverseLand:  false,
			CanTraverseWater: true,
			CanTraverseLava:  false,
			RequiresRails:    false,
			CanFly3D:         false,
			CanAscend:        false,
			SpeedMultiplier:  1.35,
			VehicleMovement:  VehicleSwim,
		}

	case VehicleTypeStrider:
		return VehicleCapabilities{
			CanTraverseLand:  false,
			CanTraverseWater: false,
			CanTraverseLava:  true,
			RequiresRails:    false,
			CanFly3D:         false,
			CanAscend:        false,
			SpeedMultiplier:  1.1,
			VehicleMovement:  VehicleLavaTraverse,
		}

	case VehicleTypeMinecart:
		return VehicleCapabilities{
			CanTraverseLand:  false,
			CanTraverseWater: false,
			CanTraverseLava:  false,
			RequiresRails:    true,
			CanFly3D:         false,
			CanAscend:        false,
			SpeedMultiplier:  2.0,
			VehicleMovement:  VehicleRailTraverse,
		}

	case VehicleTypeNautilus, VehicleTypeZombieNautilus:
		return VehicleCapabilities{
			CanTraverseLand:  false,
			CanTraverseWater: true,
			CanTraverseLava:  false,
			RequiresRails:    false,
			CanFly3D:         true,
			CanAscend:        false,
			SpeedMultiplier:  1.2,
			VehicleMovement:  VehicleFly3D,
		}

	default:
		return VehicleCapabilities{} // Unknown vehicle type
	}
}
