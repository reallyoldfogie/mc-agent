package common

// Entity Metadata Key Constants
// These are the indices used in entity metadata entries across all entity types.
// Note: Different entity types use different indices for different purposes.
// For example, index 10 is "isInGround" (boolean) for arrows but "PotionEffectColor" (int) for living entities.
// Reference: https://minecraft.wiki/w/Java_Edition_protocol/Entity_metadata#Entity_Metadata
const (
	// Base Entity Metadata (indices 0-7)
	// Applies to all entities
	EntityMetadataKeyFlags         EntityMetadataKeyType = 0 // byte: on fire, sneaking, sprinting, swimming, invisible, glowing, flying with elytra
	EntityMetadataKeyCustomName    EntityMetadataKeyType = 1 // optional component: custom display name
	EntityMetadataKeyNameVisible   EntityMetadataKeyType = 2 // boolean: is custom name always visible
	EntityMetadataKeySilent        EntityMetadataKeyType = 3 // boolean: whether entity is silent
	EntityMetadataKeyNoGravity     EntityMetadataKeyType = 4 // boolean: whether entity has no gravity
	EntityMetadataKeyPose          EntityMetadataKeyType = 5 // pose: entity pose (standing, sneaking, etc.)
	EntityMetadataKeyFreezingTicks EntityMetadataKeyType = 6 // VarInt: ticks frozen in powder snow

	// Living Entity Metadata (indices 7-15)
	// Applies to entities that extend Living
	EntityMetadataKeyRotation            EntityMetadataKeyType = 7  // rotations: head/body rotation for some entities
	EntityMetadataKeyAirSupply           EntityMetadataKeyType = 8  // VarInt: ticks of air remaining (living entities)
	EntityMetadataKeyHealth              EntityMetadataKeyType = 9  // float: current health in half-hearts (living entities)
	EntityMetadataKeyPotionEffectColor   EntityMetadataKeyType = 10 // int: potion effect color (living entities)
	EntityMetadataKeyPotionEffectAmbient EntityMetadataKeyType = 11 // boolean: is potion effect from beacon (living entities)
	EntityMetadataKeyArrowCount          EntityMetadataKeyType = 11 // VarInt: number of arrows in entity (living entities)
	EntityMetadataKeyBeeStingerCount     EntityMetadataKeyType = 12 // VarInt: number of bee stingers in entity (living entities)

	// Arrow/Projectile Metadata (indices for AbstractArrow type)
	EntityMetadataKeyArrowIsInWall      EntityMetadataKeyType = 8  // boolean: is arrow in wall (arrows)
	EntityMetadataKeyArrowIsInGround    EntityMetadataKeyType = 10 // boolean: is arrow stuck in ground (arrows only)
	EntityMetadataKeyArrowPiercingLevel EntityMetadataKeyType = 9  // byte: piercing level (arrows only)

	// Player-specific Metadata (Human/Player entities)
	EntityMetadataKeyAbsorptionHearts         EntityMetadataKeyType = 16 // float: absorption hearts from absorption effect
	EntityMetadataKeyPlayerScore              EntityMetadataKeyType = 17 // VarInt: player score/xp level
	EntityMetadataKeyPlayerDisplayedSkinParts EntityMetadataKeyType = 18 // byte: displayed skin parts bitfield

	// Additional Living Entity fields (16+)
	EntityMetadataKeyMainHand EntityMetadataKeyType = 19 // byte: 0 = main hand, 1 = off hand
)

// EntityMetadataKeyType is a type alias for metadata key indices
type EntityMetadataKeyType int32
