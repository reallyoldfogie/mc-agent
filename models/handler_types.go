package models

// MetadataHandlerType represents the data type of an entity metadata value.
// These are the handler IDs from Minecraft's TrackedDataHandlerRegistry.
// See: https://minecraft.wiki/w/Java_Edition_protocol/Entity_metadata
type MetadataHandlerType int

const (
	// Core types (0-8)
	HandlerByte                  MetadataHandlerType = 0 // uint8
	HandlerInteger               MetadataHandlerType = 1 // int32 (VarInt)
	HandlerLong                  MetadataHandlerType = 2 // int64 (VarLong)
	HandlerFloat                 MetadataHandlerType = 3 // float32 (IEEE 754)
	HandlerString                MetadataHandlerType = 4 // UTF-8 string
	HandlerTextComponent         MetadataHandlerType = 5 // Chat component
	HandlerOptionalTextComponent MetadataHandlerType = 6 // Optional chat component
	HandlerItemStack             MetadataHandlerType = 7 // Inventory item
	HandlerBoolean               MetadataHandlerType = 8 // bool

	// Position and rotation types (9-12)
	HandlerRotation         MetadataHandlerType = 9  // 3× float32 (pitch, yaw, roll)
	HandlerBlockPos         MetadataHandlerType = 10 // BlockPos (3× int32)
	HandlerOptionalBlockPos MetadataHandlerType = 11 // Optional BlockPos
	HandlerFacing           MetadataHandlerType = 12 // Direction enum (0-5)

	// Reference types (13-15)
	HandlerLazyEntityReference MetadataHandlerType = 13 // Optional entity ID (int32)
	HandlerBlockState          MetadataHandlerType = 14 // Block state ID (VarInt)
	HandlerOptionalBlockState  MetadataHandlerType = 15 // Optional block state

	// Complex types (16-20)
	HandlerNBTCompound  MetadataHandlerType = 16 // Full NBT structure
	HandlerParticle     MetadataHandlerType = 17 // Particle effect
	HandlerParticleList MetadataHandlerType = 18 // List of particle effects
	HandlerVillagerData MetadataHandlerType = 19 // Villager profession/level/biome (packed int)
	HandlerOptionalInt  MetadataHandlerType = 20 // Optional int (0=absent, >0=value-1)

	// Enum types (21-28)
	HandlerEntityPose       MetadataHandlerType = 21 // Pose enum (standing, sneaking, etc.)
	HandlerCatVariant       MetadataHandlerType = 22 // Cat color variant registry entry
	HandlerCowVariant       MetadataHandlerType = 23 // Cow variant registry entry
	HandlerWolfVariant      MetadataHandlerType = 24 // Wolf color variant registry entry
	HandlerWolfSoundVariant MetadataHandlerType = 25 // Wolf bark type registry entry
	HandlerFrogVariant      MetadataHandlerType = 26 // Frog species variant registry entry
	HandlerPigVariant       MetadataHandlerType = 27 // Pig variant registry entry
	HandlerChickenVariant   MetadataHandlerType = 28 // Chicken variant registry entry

	// Specialized types (29-34)
	HandlerOptionalGlobalPos MetadataHandlerType = 29 // Optional cross-dimension position
	HandlerPaintingVariant   MetadataHandlerType = 30 // Painting type registry entry
	HandlerSnifferState      MetadataHandlerType = 31 // Sniffer state enum
	HandlerArmadilloState    MetadataHandlerType = 32 // Armadillo state enum
	HandlerVector3F          MetadataHandlerType = 33 // 3× float32 (X, Y, Z)
	HandlerQuaternionF       MetadataHandlerType = 34 // 4× float32 quaternion (X, Y, Z, W)

	// 1.21.9+ types
	HandlerCopperGolemState MetadataHandlerType = 35 // Copper Golem state enum
	HandlerOxidationLevel   MetadataHandlerType = 36 // Oxidation level enum
)

// String returns the name of the handler type
func (h MetadataHandlerType) String() string {
	switch h {
	case HandlerByte:
		return "BYTE"
	case HandlerInteger:
		return "INTEGER"
	case HandlerLong:
		return "LONG"
	case HandlerFloat:
		return "FLOAT"
	case HandlerString:
		return "STRING"
	case HandlerTextComponent:
		return "TEXT_COMPONENT"
	case HandlerOptionalTextComponent:
		return "OPTIONAL_TEXT_COMPONENT"
	case HandlerItemStack:
		return "ITEM_STACK"
	case HandlerBoolean:
		return "BOOLEAN"
	case HandlerRotation:
		return "ROTATION"
	case HandlerBlockPos:
		return "BLOCK_POS"
	case HandlerOptionalBlockPos:
		return "OPTIONAL_BLOCK_POS"
	case HandlerFacing:
		return "FACING"
	case HandlerLazyEntityReference:
		return "LAZY_ENTITY_REFERENCE"
	case HandlerBlockState:
		return "BLOCK_STATE"
	case HandlerOptionalBlockState:
		return "OPTIONAL_BLOCK_STATE"
	case HandlerNBTCompound:
		return "NBT_COMPOUND"
	case HandlerParticle:
		return "PARTICLE"
	case HandlerParticleList:
		return "PARTICLE_LIST"
	case HandlerVillagerData:
		return "VILLAGER_DATA"
	case HandlerOptionalInt:
		return "OPTIONAL_INT"
	case HandlerEntityPose:
		return "ENTITY_POSE"
	case HandlerCatVariant:
		return "CAT_VARIANT"
	case HandlerCowVariant:
		return "COW_VARIANT"
	case HandlerWolfVariant:
		return "WOLF_VARIANT"
	case HandlerWolfSoundVariant:
		return "WOLF_SOUND_VARIANT"
	case HandlerFrogVariant:
		return "FROG_VARIANT"
	case HandlerPigVariant:
		return "PIG_VARIANT"
	case HandlerChickenVariant:
		return "CHICKEN_VARIANT"
	case HandlerOptionalGlobalPos:
		return "OPTIONAL_GLOBAL_POS"
	case HandlerPaintingVariant:
		return "PAINTING_VARIANT"
	case HandlerSnifferState:
		return "SNIFFER_STATE"
	case HandlerArmadilloState:
		return "ARMADILLO_STATE"
	case HandlerVector3F:
		return "VECTOR_3F"
	case HandlerQuaternionF:
		return "QUATERNION_F"
	case HandlerCopperGolemState:
		return "COPPER_GOLEM_STATE"
	case HandlerOxidationLevel:
		return "OXIDATION_LEVEL"
	default:
		return "UNKNOWN"
	}
}

// HandlerTypeFromString converts a protocol metadata type string to a MetadataHandlerType.
// Used when parsing EntityMetadata packets from mc-protocol-go library.
func HandlerTypeFromString(typeStr string) MetadataHandlerType {
	switch typeStr {
	case "byte":
		return HandlerByte
	case "int":
		return HandlerInteger
	case "long":
		return HandlerLong
	case "float":
		return HandlerFloat
	case "string":
		return HandlerString
	case "component":
		return HandlerTextComponent
	case "optional_component":
		return HandlerOptionalTextComponent
	case "item_stack":
		return HandlerItemStack
	case "boolean":
		return HandlerBoolean
	case "rotations":
		return HandlerRotation
	case "block_pos":
		return HandlerBlockPos
	case "optional_block_pos":
		return HandlerOptionalBlockPos
	case "direction":
		return HandlerFacing
	case "optional_uuid":
		return HandlerLazyEntityReference
	case "block_state":
		return HandlerBlockState
	case "optional_block_state":
		return HandlerOptionalBlockState
	case "compound_tag":
		return HandlerNBTCompound
	case "particle":
		return HandlerParticle
	case "particles":
		return HandlerParticleList
	case "villager_data":
		return HandlerVillagerData
	case "optional_unsigned_int":
		return HandlerOptionalInt
	case "pose":
		return HandlerEntityPose
	case "cat_variant":
		return HandlerCatVariant
	case "cow_variant":
		return HandlerCowVariant
	case "wolf_variant":
		return HandlerWolfVariant
	case "wolf_sound_variant":
		return HandlerWolfSoundVariant
	case "frog_variant":
		return HandlerFrogVariant
	case "pig_variant":
		return HandlerPigVariant
	case "chicken_variant":
		return HandlerChickenVariant
	case "optional_global_pos":
		return HandlerOptionalGlobalPos
	case "painting_variant":
		return HandlerPaintingVariant
	case "sniffer_state":
		return HandlerSnifferState
	case "armadillo_state":
		return HandlerArmadilloState
	case "vector3":
		return HandlerVector3F
	case "quaternion":
		return HandlerQuaternionF
	default:
		return 0 // Unknown type defaults to HandlerByte
	}
}

// MetadataEntry represents a single entity metadata entry as received from the client
type MetadataEntry struct {
	Key       int32               // Metadata key/index (0-254)
	HandlerID MetadataHandlerType // Type of the value
	Value     any                 // Actual value (type depends on HandlerID)
}
