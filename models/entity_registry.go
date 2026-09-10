package models

import (
	"sync"
)

// EntityType represents a Minecraft entity type
type EntityType string

// Common entity types
const (
	// Players
	EntityTypePlayer EntityType = "player"

	// Living entities
	EntityTypeZombie         EntityType = "zombie"
	EntityTypeCreeper        EntityType = "creeper"
	EntityTypeSkeletonArrow  EntityType = "skeleton"
	EntityTypeSpider         EntityType = "spider"
	EntityTypeGiant          EntityType = "giant"
	EntityTypeEnderman       EntityType = "enderman"
	EntityTypeEndermite      EntityType = "endermite"
	EntityTypeWither         EntityType = "wither"
	EntityTypeBat            EntityType = "bat"
	EntityTypeWitch          EntityType = "witch"
	EntityTypeZombieVillager EntityType = "zombie_villager"
	EntityTypeVillager       EntityType = "villager"
	EntityTypeIronGolem      EntityType = "iron_golem"
	EntityTypeSnowGolem      EntityType = "snow_golem"
	EntityTypeArmorStand     EntityType = "armor_stand"
	EntityTypeHorse          EntityType = "horse"
	EntityTypeSkeletonHorse  EntityType = "skeleton_horse"
	EntityTypeZombieHorse    EntityType = "zombie_horse"
	EntityTypeSkeleton       EntityType = "skeleton"
	EntityTypeDonkey         EntityType = "donkey"
	EntityTypeMule           EntityType = "mule"
	EntityTypeLlama          EntityType = "llama"
	EntityTypeTraderLlama    EntityType = "trader_llama"
	EntityTypeCamel          EntityType = "camel"
	EntityTypeCamelHusk      EntityType = "camel_husk"
	EntityTypeNautilus       EntityType = "nautilus"
	EntityTypeZombieNautilus EntityType = "zombie_nautilus"
	// EntityTypeHappyGhast is 1.21.6+ only. There is no separate "ghastling"
	// entity type — the baby form is the same entity type with the CHILD
	// tracked-data flag set (metadata index 16, shared with every other
	// AnimalEntity-chain mount); see PHASE_6_PLAN.md §3.4.
	EntityTypeHappyGhast EntityType = "happy_ghast"
	EntityTypeBoat       EntityType = "boat"
	EntityTypeChestBoat  EntityType = "chest_boat"
	EntityTypePig        EntityType = "pig"
	EntityTypeStrider    EntityType = "strider"
	EntityTypeMinecart   EntityType = "minecart"

	// Projectiles
	EntityTypeArrow            EntityType = "arrow"
	EntityTypeSmallFireball    EntityType = "small_fireball"
	EntityTypeFireball         EntityType = "fireball"
	EntityTypeSnowball         EntityType = "snowball"
	EntityTypeEgg              EntityType = "egg"
	EntityTypeEnderPearl       EntityType = "ender_pearl"
	EntityTypeExperienceBottle EntityType = "experience_bottle"
	EntityTypeSplashPotion     EntityType = "splash_potion"
	EntityTypeLingeringPotion  EntityType = "lingering_potion"
	EntityTypeThrowableItem    EntityType = "item"

	// Display entities
	EntityTypeBlockDisplay EntityType = "block_display"
	EntityTypeItemDisplay  EntityType = "item_display"
	EntityTypeTextDisplay  EntityType = "text_display"

	// Other
	EntityTypeUnknown EntityType = "unknown"
)

// EntityMetadataContext provides context for interpreting metadata for a specific entity
type EntityMetadataContext struct {
	EntityID   int32
	EntityType EntityType
	SpawnedAt  int64 // Timestamp when entity was spawned
}

// EntityRegistry tracks all entities and their types for metadata interpretation
type EntityRegistry struct {
	mu       sync.RWMutex
	entities map[int32]*EntityMetadataContext
}

// NewEntityRegistry creates a new entity registry
func NewEntityRegistry() *EntityRegistry {
	return &EntityRegistry{
		entities: make(map[int32]*EntityMetadataContext),
	}
}

// RegisterEntity registers an entity with its type
func (r *EntityRegistry) RegisterEntity(entityID int32, entityType EntityType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entities[entityID] = &EntityMetadataContext{
		EntityID:   entityID,
		EntityType: entityType,
		SpawnedAt:  0, // TODO: Add spawn timestamp if needed
	}
}

// GetEntityType retrieves the type of an entity
func (r *EntityRegistry) GetEntityType(entityID int32) EntityType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if ctx, exists := r.entities[entityID]; exists {
		return ctx.EntityType
	}
	return EntityTypeUnknown
}

// GetContext retrieves the full metadata context for an entity
func (r *EntityRegistry) GetContext(entityID int32) *EntityMetadataContext {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if ctx, exists := r.entities[entityID]; exists {
		return ctx
	}
	return nil
}

// RemoveEntity removes an entity from the registry
func (r *EntityRegistry) RemoveEntity(entityID int32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.entities, entityID)
}

// RemoveEntities removes multiple entities from the registry
func (r *EntityRegistry) RemoveEntities(entityIDs []int32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, id := range entityIDs {
		delete(r.entities, id)
	}
}

// Clear removes all entities from the registry
func (r *EntityRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.entities = make(map[int32]*EntityMetadataContext)
}

// Count returns the number of registered entities
func (r *EntityRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.entities)
}

// IsLivingEntity returns true if the entity type is a living entity
func (t EntityType) IsLivingEntity() bool {
	switch t {
	case EntityTypePlayer, EntityTypeZombie, EntityTypeCreeper, EntityTypeSkeletonArrow,
		EntityTypeSpider, EntityTypeGiant, EntityTypeEnderman, EntityTypeEndermite,
		EntityTypeWither, EntityTypeBat, EntityTypeWitch, EntityTypeZombieVillager,
		EntityTypeVillager, EntityTypeIronGolem, EntityTypeSnowGolem, EntityTypeArmorStand,
		EntityTypeHorse, EntityTypeDonkey, EntityTypeMule, EntityTypeCamel, EntityTypeCamelHusk,
		EntityTypeLlama, EntityTypeTraderLlama,
		EntityTypeNautilus, EntityTypeZombieNautilus, EntityTypeHappyGhast:
		return true
	default:
		return false
	}
}

// IsProjectile returns true if the entity type is a projectile
func (t EntityType) IsProjectile() bool {
	switch t {
	case EntityTypeArrow, EntityTypeSmallFireball, EntityTypeFireball, EntityTypeSnowball,
		EntityTypeEgg, EntityTypeEnderPearl, EntityTypeExperienceBottle, EntityTypeSplashPotion,
		EntityTypeLingeringPotion, EntityTypeThrowableItem:
		return true
	default:
		return false
	}
}

// IsDisplayEntity returns true if the entity type is a display entity
func (t EntityType) IsDisplayEntity() bool {
	switch t {
	case EntityTypeBlockDisplay, EntityTypeItemDisplay, EntityTypeTextDisplay:
		return true
	default:
		return false
	}
}

// IsRideable returns true if the entity type is rideable.
//
// Note that "rideable" is not the same as "steerable". Llamas accept a
// passenger but cannot be saddled or directed, so they are rideable but not
// controllable — see IsSteerable.
func (t EntityType) IsRideable() bool {
	switch t {
	case EntityTypeHorse, EntityTypeSkeletonHorse, EntityTypeZombieHorse, EntityTypeDonkey, EntityTypeMule, EntityTypeBoat,
		EntityTypeChestBoat, EntityTypePig, EntityTypeStrider, EntityTypeCamel, EntityTypeCamelHusk,
		EntityTypeLlama, EntityTypeTraderLlama,
		EntityTypeNautilus, EntityTypeZombieNautilus, EntityTypeHappyGhast:
		return true
	default:
		return false
	}
}

// IsSteerable returns true if a rider can direct where the entity goes.
//
// A llama can be mounted and its inventory accessed while riding, but it takes
// no saddle and ignores rider input entirely — it keeps running its own mob AI.
// The rider is a plain passenger, so the client is not the movement authority
// and must not predict the entity's position or send VehicleMove packets for it.
func (t EntityType) IsSteerable() bool {
	if !t.IsRideable() {
		return false
	}
	switch t {
	case EntityTypeLlama, EntityTypeTraderLlama:
		return false
	default:
		return true
	}
}
