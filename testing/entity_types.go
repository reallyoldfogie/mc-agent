package testing

// Minecraft entity type IDs for 1.21.5 (from registries.json minecraft:entity_type)
// These IDs are version-specific. For version-agnostic code, load from registries.json
//
// Note: These values are protocol IDs and may change between Minecraft versions.
// They are extracted from .cache/downloads/1.21.5/data_generator/reports/registries.json
const (
	// Player entity (used for player tracking in following system)
	EntityTypePlayer = 128 // minecraft:player

	// Rideable entities with inventory
	EntityTypeHorse  = 63 // minecraft:horse (1.21.5+)
	EntityTypeDonkey = 31 // minecraft:donkey
	EntityTypeMule   = 90 // minecraft:mule
	EntityTypeLlama  = 84 // minecraft:llama

	// Container entities (boats and minecarts with storage)
	EntityTypeChestBoat     = 85 // minecraft:oak_chest_boat (1.21.5+)
	EntityTypeChestMinecart = 24 // minecraft:chest_minecart (1.21.5+)

	// Other potentially useful entity types for testing (FIXME: These IDs may be incorrect for 1.21.5+)
	// EntityTypePig       = 100 // minecraft:pig
	// EntityTypeCow       = 24  // minecraft:cow
	// EntityTypeSheep     = 108 // minecraft:sheep
	// EntityTypeVillager  = 144 // minecraft:villager
	// EntityTypeZombie    = 151 // minecraft:zombie
	// EntityTypeSkeleton  = 110 // minecraft:skeleton
	// EntityTypeCreeper   = 27  // minecraft:creeper
	// EntityTypeEnderman   = 36  // minecraft:enderman
	// EntityTypeArmorStand = 2   // minecraft:armor_stand
)

// EntityTypeName maps entity type IDs to human-readable names (for debugging/logging)
var EntityTypeName = map[int32]string{
	EntityTypePlayer:        "player",
	EntityTypeHorse:         "horse",
	EntityTypeDonkey:        "donkey",
	EntityTypeMule:          "mule",
	EntityTypeLlama:         "llama",
	EntityTypeChestBoat:     "oak_chest_boat",
	EntityTypeChestMinecart: "chest_minecart",
}
