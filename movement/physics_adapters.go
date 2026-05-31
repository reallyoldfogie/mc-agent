package movement

import (
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

type blockStateWorld interface {
	GetBlockAt(x, y, z float64) (uint32, bool)
}

// EntityProvider supplies entities for collision detection.
type EntityProvider interface {
	GetEntitiesSnapshot() map[int32]EntitySnapshot
}

// EntitySnapshot represents a tracked entity's position, velocity, and type.
type EntitySnapshot struct {
	Pos        models.V3         // Position
	Vel        models.V3         // Velocity
	EntityType models.EntityType // Entity type name (for dimension lookup)
}

// EntityDimensions represents a mob/entity's collision box dimensions.
// Width is used for X/Z, Height for Y.
type EntityDimensions struct {
	Width  float64 // Horizontal dimension (X/Z)
	Height float64 // Vertical dimension (Y)
}

type physicsWorldAdapter struct {
	world            blockStateWorld
	entityProvider   EntityProvider // Optional: if nil, entity queries return empty
	playerDimensions struct {
		width, height float64
	}
}

// NewPhysicsWorldAdapter adapts a GetBlockAt world to the physics.World interface.
func NewPhysicsWorldAdapter(world blockStateWorld) *physicsWorldAdapter {
	return &physicsWorldAdapter{
		world:          world,
		entityProvider: nil, // No entity provider by default
		playerDimensions: struct {
			width, height float64
		}{width: physics.PlayerCollisionWidth, height: physics.PlayerCollisionHeight},
	}
}

// WithEntityProvider adds entity collision support to the adapter.
func (pw *physicsWorldAdapter) WithEntityProvider(provider EntityProvider) *physicsWorldAdapter {
	pw.entityProvider = provider
	return pw
}

// GetEntityDimensions returns the collision box dimensions for a given entity type.
// Uses reasonable approximations for common entity types, defaulting to player size if unknown.
//
// NOTE: Entity hitboxes can vary based on state (age, puffing, sneaking, etc.).
// This implementation uses typical/average dimensions.
// For precise collision detection, version-specific entity metadata could be queried,
// but current implementation prioritizes simplicity and maintainability.
// TODO: implement version-specific entity metadata based hitbox/dimesion detection.
//
// Reference: https://minecraft.wiki/w/Hitbox#List_of_entities'_hitbox_sizes
func (pw *physicsWorldAdapter) GetEntityDimensions(entityType models.EntityType) EntityDimensions {
	switch entityType {
	// Humanoid mobs (roughly player-sized, 0.6 wide x 1.8 tall)
	case models.EntityTypePlayer, models.EntityTypeVillager, models.EntityTypeZombie,
		models.EntityTypeSkeleton, models.EntityTypeZombieVillager, models.EntityTypeWitch:
		return EntityDimensions{Width: 0.6, Height: 1.8}

	case models.EntityTypeArmorStand:
		return EntityDimensions{Width: 0.5, Height: 1.975}

	// Horses and similar mounts (1.4 wide x 1.6 tall)
	case models.EntityTypeHorse, models.EntityTypeMule, models.EntityTypeDonkey:
		return EntityDimensions{Width: 1.4, Height: 1.6}

	// Camel (0.9 wide x 2.2 tall) — same for CamelHusk
	case models.EntityTypeCamel, models.EntityTypeCamelHusk:
		return EntityDimensions{Width: 0.9, Height: 2.2}

	// Nautilus / ZombieNautilus (1.21.11+): approximate from AbstractNautilusEntity
	// TODO: verify exact dimensions from Java source when available
	case models.EntityTypeNautilus, models.EntityTypeZombieNautilus:
		return EntityDimensions{Width: 1.4, Height: 1.6}

	// Iron golem (large: 1.4 wide x 2.7 tall)
	case models.EntityTypeIronGolem:
		return EntityDimensions{Width: 1.4, Height: 2.7}

	// Creeper, Enderman, Wither (humanoid-like, 0.6 x 1.8)
	case models.EntityTypeCreeper, models.EntityTypeEnderman, models.EntityTypeWither:
		return EntityDimensions{Width: 0.6, Height: 1.8}

	// Spider and cave spider (1.4 wide x 0.9 tall)
	case models.EntityTypeSpider:
		return EntityDimensions{Width: 1.4, Height: 0.9}

	// Default: use standard player dimensions for unknown/unhandled types
	default:
		return EntityDimensions{Width: physics.PlayerCollisionWidth, Height: physics.PlayerCollisionHeight}
	}
}

func (pw *physicsWorldAdapter) GetBlockStatus(x, y, z int) (uint32, bool) {
	if pw.world == nil {
		return 0, false
	}
	stateID, loaded := pw.world.GetBlockAt(float64(x), float64(y), float64(z))
	if !loaded {
		return 0, false // Indicate chunk not loaded to physics engine
	}
	return stateID, true
}

// GetEntitiesInRange returns all entities whose AABBs overlap with the query box.
// If no entity provider is set, returns empty slice.
func (pw *physicsWorldAdapter) GetEntitiesInRange(queryBB models.AABB) []models.EntityBounds {
	if pw.entityProvider == nil {
		return []models.EntityBounds{} // No entity support
	}

	entities := pw.entityProvider.GetEntitiesSnapshot()
	var result []models.EntityBounds

	// Query all entities and check AABB overlap
	for entityID, snapshot := range entities {
		// Get entity-specific dimensions (horses, camels, spiders, etc. have different sizes than players)
		dimensions := pw.GetEntityDimensions(snapshot.EntityType)
		entityBB := models.AABB{
			X: models.MinMax{
				Min: snapshot.Pos.X - dimensions.Width/2,
				Max: snapshot.Pos.X + dimensions.Width/2,
			},
			Y: models.MinMax{
				Min: snapshot.Pos.Y,
				Max: snapshot.Pos.Y + dimensions.Height,
			},
			Z: models.MinMax{
				Min: snapshot.Pos.Z - dimensions.Width/2,
				Max: snapshot.Pos.Z + dimensions.Width/2,
			},
		}

		// Check for AABB overlap (separate on each axis)
		if queryBB.X.Min < entityBB.X.Max && queryBB.X.Max > entityBB.X.Min &&
			queryBB.Y.Min < entityBB.Y.Max && queryBB.Y.Max > entityBB.Y.Min &&
			queryBB.Z.Min < entityBB.Z.Max && queryBB.Z.Max > entityBB.Z.Min {
			result = append(result, models.EntityBounds{
				EntityID: entityID,
				AABB:     entityBB,
				Velocity: snapshot.Vel,
			})
		}
	}

	return result
}
