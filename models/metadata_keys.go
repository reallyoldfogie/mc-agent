package models

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

	// Boat/Chest Boat Metadata (AbstractBoat type)
	EntityMetadataKeyBoatVariant     EntityMetadataKeyType = 11 // VarInt: boat wood type (0-8)
	EntityMetadataKeyBoatPaddleLeft  EntityMetadataKeyType = 12 // boolean: left paddle turning
	EntityMetadataKeyBoatPaddleRight EntityMetadataKeyType = 13 // boolean: right paddle turning

	// EntityMetadataKeyHorseFlags is the byte bitfield registered by
	// AbstractHorseEntity, carrying tamed/saddled/bred/eating/angry state.
	//
	// The index is 17 because seventeen tracked-data entries are registered
	// ahead of it up the class hierarchy: 8 on Entity, 7 on LivingEntity, 1 on
	// MobEntity, 0 on PathAwareEntity, 1 on PassiveEntity and 0 on AnimalEntity.
	// Verified identical across 1.21.1, 1.21.2 and 1.21.4 by counting
	// DataTracker.registerData calls in the decompiled source.
	//
	// Every saddleable mount inherits it: horse and camel extend
	// AbstractHorseEntity directly, while donkey, mule and llama reach it via
	// AbstractDonkeyEntity.
	//
	// This is only meaningful before 1.21.5. From 1.21.5 the saddle moved to a
	// real equipment slot and AbstractHorseEntity.isSaddled() was replaced by
	// MobEntity.hasSaddleEquipped(), which reads equipment instead.
	EntityMetadataKeyHorseFlags EntityMetadataKeyType = 17

	// EntityMetadataKeyHappyGhastStayingStill is the boolean STAYING_STILL
	// flag HappyGhastEntity itself registers (1.21.6+). The same seventeen
	// ancestor entries as HORSE_FLAGS are registered ahead of it (HappyGhastEntity
	// extends AnimalEntity directly, the same hierarchy depth as AbstractHorseEntity),
	// but HappyGhastEntity registers two fields of its own — HAS_ROPES at
	// index 17, then STAYING_STILL at index 18 — rather than one. Verified
	// identical across every version happy ghast exists in (1.21.6-1.21.11)
	// by counting DataTracker.registerData calls in the decompiled source.
	//
	// True whenever HappyGhastEntity.getControllingPassenger() has no
	// controller at all — not even the pilot in seat 0 — either because a
	// player is standing on top of it or because it's within the few-tick
	// settle window after a mount/dismount change. The client must not
	// predict movement or send VehicleMove while this is set.
	EntityMetadataKeyHappyGhastStayingStill EntityMetadataKeyType = 18

	// EntityMetadataKeyPassiveChild is the boolean CHILD flag PassiveEntity
	// itself registers, shared by every entity in the AnimalEntity/PassiveEntity
	// hierarchy (horse, donkey, mule, llama, camel, happy ghast, ...) — the
	// entry immediately before HORSE_FLAGS/STAYING_STILL, at index 16 rather
	// than 17. True for the baby form of the entity. Not currently captured
	// by any handler: vanilla already refuses to mount a baby happy ghast
	// (HappyGhastEntity.interactMob falls through to the default breeding
	// interaction when isBaby()), so client-side rejection is an optional
	// diagnostic rather than a correctness requirement — see
	// PHASE_6_PLAN.md §1.8. Recorded here so the index doesn't need
	// re-deriving if that changes.
	EntityMetadataKeyPassiveChild EntityMetadataKeyType = 16

	// EntityMetadataKeyFireworkShooterEntityID is FireworkRocketEntity's own
	// SHOOTER_ENTITY_ID tracked field (an "optional_unsigned_int" —
	// HandlerOptionalInt, 0=absent/N=entity ID N-1). FireworkRocketEntity
	// extends ProjectileEntity, which adds no fields of its own, so this is
	// simply the second of FireworkRocketEntity's three own fields: 8 base
	// Entity fields (0-7), then ITEM(8), SHOOTER_ENTITY_ID(9),
	// SHOT_AT_ANGLE(10). Verified identical across 1.21.1, 1.21.11 (Yarn)
	// and 26.1 (Mojang, renamed DATA_ATTACHED_TO_TARGET) by counting
	// DataTracker/SynchedEntityData registration calls in the decompiled
	// source.
	//
	// Collides numerically with EntityMetadataKeyHealth (also 9, on living
	// entities) — callers MUST gate on entity type ==
	// "minecraft:firework_rocket" before reading this, the same way
	// HorseFlags/HappyGhastStayingStill are gated.
	EntityMetadataKeyFireworkShooterEntityID EntityMetadataKeyType = 9
)

// HorseFlagMask is a bit within the AbstractHorseEntity flags byte
// (EntityMetadataKeyHorseFlags).
type HorseFlagMask uint8

// Horse flag bits, taken verbatim from AbstractHorseEntity's private constants
// (TAMED_FLAG, SADDLED_FLAG, ...) in the decompiled source.
const (
	HorseFlagTamed       HorseFlagMask = 2
	HorseFlagSaddled     HorseFlagMask = 4
	HorseFlagBred        HorseFlagMask = 8
	HorseFlagEatingGrass HorseFlagMask = 16
	HorseFlagAngry       HorseFlagMask = 32
	HorseFlagEating      HorseFlagMask = 64
)

// IsSet reports whether this flag is set in the given horse flags byte.
func (flag HorseFlagMask) IsSet(horseFlags uint8) bool {
	return horseFlags&uint8(flag) != 0
}

// EntityMetadataKeyType is a type alias for metadata key indices
type EntityMetadataKeyType int32

// BoatVariant represents the type/wood of a boat entity
type BoatVariant int32

const (
	BoatVariantOak      BoatVariant = 0
	BoatVariantSpruce   BoatVariant = 1
	BoatVariantBirch    BoatVariant = 2
	BoatVariantJungle   BoatVariant = 3
	BoatVariantAcacia   BoatVariant = 4
	BoatVariantDarkOak  BoatVariant = 5
	BoatVariantMangrove BoatVariant = 6
	BoatVariantBamboo   BoatVariant = 7
	BoatVariantCherry   BoatVariant = 8
)
