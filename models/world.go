package models

// World represents the full world access needed for block queries, physics
// collision, world-state (time/border/difficulty), and light/biome queries.
// This interface supersedes the former separate World and PhysicsWorld
// interfaces — see docs/plans/WORLD_STRUCT_CONSOLIDATION.md for the history.
type World interface {
	// GetBlockAt returns the block state ID at the given world coordinates.
	// The second return value indicates whether the chunk is loaded:
	//   - true: chunk is loaded (stateID is valid, may be 0 for air)
	//   - false: chunk is not loaded (stateID should be ignored)
	GetBlockAt(x, y, z float64) (stateID uint32, chunkLoaded bool)

	// GetBlockStatus is GetBlockAt with integer coordinates.
	//
	// Deprecated: this is a thin int-coordinate wrapper around GetBlockAt,
	// kept only because physics/collision code already has ~50 call sites
	// written against integer block coordinates. Prefer GetBlockAt in new
	// code; migrating the existing call sites is a separate, larger cleanup
	// out of scope for the World/PhysicsWorld interface merge.
	GetBlockStatus(x, y, z int) (uint32, bool)

	// GetEntitiesInRange returns all entities whose AABBs overlap with the
	// query box. Used for entity collision detection during physics updates.
	// Returns an empty slice if no entities are found or entity tracking is
	// unavailable (e.g. no EntityProvider has been configured).
	GetEntitiesInRange(queryBB AABB) []EntityBounds

	// GetWorldAge returns the current server world age in ticks and whether it's been initialized.
	// Returns (0, false) if no Update Time packet has been received yet.
	// Returns (worldAge, true) when the value is valid from the server.
	GetWorldAge() (int64, bool)

	// GetTimeOfDay returns the current time of day in ticks and whether it's been initialized.
	// Time of day ranges from 0-23999 ticks per day.
	// Returns (0, false) if no Update Time packet has been received yet.
	// Returns (timeOfDay, true) when the value is valid from the server.
	GetTimeOfDay() (int64, bool)

	// SetWorldTime updates both world age and time of day from the server.
	// Called when a ClientboundUpdateTime packet is received.
	SetWorldTime(worldAge, timeOfDay int64)

	// GetLightLevel returns sky light and block light (0-15 each) at the
	// given block coordinates. loaded is false if the containing chunk
	// section has no light data yet (no ClientboundLevelChunkWithLight or
	// ClientboundUpdateLight has been processed for that section).
	GetLightLevel(x, y, z int) (skyLight, blockLight uint8, loaded bool)

	// GetBiomeAt returns the biome registry ID (global biome palette ID,
	// mirroring block state IDs) at the given block coordinates. loaded
	// mirrors GetBlockAt's chunk-loaded semantics.
	GetBiomeAt(x, y, z int) (biomeID uint32, loaded bool)

	// GetWorldBorder returns the current world border state and whether a
	// ClientboundInitializeWorldBorder packet has been received yet.
	GetWorldBorder() (WorldBorder, bool)
	// SetWorldBorder replaces the full world border state. Called when a
	// ClientboundInitializeWorldBorder packet is received.
	SetWorldBorder(b WorldBorder)
	// SetWorldBorderCenter updates the border's center. Called when a
	// ClientboundSetBorderCenter packet is received.
	SetWorldBorderCenter(x, z float64)
	// SetWorldBorderSize updates the border's diameter (no lerp in progress).
	// Called when a ClientboundSetBorderSize packet is received.
	SetWorldBorderSize(diameter float64)
	// SetWorldBorderLerpSize starts (or updates) a border resize animation.
	// Called when a ClientboundSetBorderLerpSize packet is received.
	SetWorldBorderLerpSize(oldDiameter, newDiameter float64, speedTicks int64)
	// SetWorldBorderWarningDelay updates the border's warning time in ticks.
	// Called when a ClientboundSetBorderWarningDelay packet is received.
	SetWorldBorderWarningDelay(warningTimeTicks int32)
	// SetWorldBorderWarningDistance updates the border's warning distance in
	// blocks. Called when a ClientboundSetBorderWarningDistance packet is
	// received.
	SetWorldBorderWarningDistance(warningBlocks int32)

	// GetDifficulty returns the current difficulty, whether it's locked, and
	// whether a ClientboundChangeDifficulty packet has been received yet.
	GetDifficulty() (difficulty uint8, locked bool, ok bool)
	// SetDifficulty updates difficulty and lock state. Called when a
	// ClientboundChangeDifficulty packet is received.
	SetDifficulty(difficulty uint8, locked bool)
}

// WorldBorder mirrors ClientboundInitializeWorldBorder's fields.
type WorldBorder struct {
	CenterX, CenterZ       float64
	OldDiameter            float64
	NewDiameter            float64
	SpeedTicks             int64 // lerp duration in ticks (0 = instant)
	PortalTeleportBoundary int32
	WarningBlocks          int32
	WarningTimeTicks       int32
}

// EntityBounds represents a tracked entity's bounding box and velocity for collision detection.
type EntityBounds struct {
	EntityID int32 // Unique entity identifier
	AABB     AABB  // Current axis-aligned bounding box
	Velocity V3    // Entity velocity (blocks per tick)

	// Standable reports whether this entity currently behaves as a solid,
	// walkable surface — e.g. a happy ghast while "staying still"
	// (HappyGhastEntity.isCollidable). A caller resolving movement against a
	// Standable entity should clip velocity against its AABB the same way it
	// would a block (support from above, not just horizontal separation);
	// see physics/state.go's standable-entity collision handling. Entities
	// that are merely overlappable (the common case) leave this false and
	// only participate in the softer separation-impulse path.
	Standable bool
}

// EntitySnapshot represents a tracked entity's position, velocity, and type
// as of the last update — the minimal data GetEntitiesInRange needs to build
// an EntityBounds per candidate entity.
type EntitySnapshot struct {
	Pos        V3
	Vel        V3
	EntityType EntityType

	// IsStandableSurface mirrors EntityBounds.Standable — true when this
	// entity currently behaves as a solid, walkable surface (e.g. a happy
	// ghast while staying still). Computed by whatever populates the
	// snapshot (agent.GetEntitiesSnapshot), since only that layer has the
	// tracked-entity state (HappyGhastStayingStill, IsBaby) needed to decide.
	IsStandableSurface bool
}

// EntityProvider supplies entities for collision detection. A World
// implementation with no EntityProvider configured should return an empty
// slice from GetEntitiesInRange rather than erroring.
type EntityProvider interface {
	GetEntitiesSnapshot() map[int32]EntitySnapshot
}

// EntityDimensions represents a mob/entity's collision box dimensions.
// Width is used for X/Z, Height for Y.
type EntityDimensions struct {
	Width  float64 // Horizontal dimension (X/Z)
	Height float64 // Vertical dimension (Y)
}

// EntityDimensionsFor returns the collision box dimensions for a given
// entity type. Uses reasonable approximations for common entity types,
// defaulting to player size if unknown.
//
// NOTE: Entity hitboxes can vary based on state (age, puffing, sneaking,
// etc.). This implementation uses typical/average dimensions. For precise
// collision detection, version-specific entity metadata could be queried,
// but current implementation prioritizes simplicity and maintainability.
// TODO: implement version-specific entity metadata based hitbox/dimension detection.
//
// Reference: https://minecraft.wiki/w/Hitbox#List_of_entities'_hitbox_sizes
func EntityDimensionsFor(entityType EntityType) EntityDimensions {
	switch entityType {
	// Humanoid mobs (roughly player-sized, 0.6 wide x 1.8 tall)
	case EntityTypePlayer, EntityTypeVillager, EntityTypeZombie,
		EntityTypeSkeleton, EntityTypeZombieVillager, EntityTypeWitch:
		return EntityDimensions{Width: 0.6, Height: 1.8}

	case EntityTypeArmorStand:
		return EntityDimensions{Width: 0.5, Height: 1.975}

	// Horses and similar mounts (1.4 wide x 1.6 tall)
	case EntityTypeHorse, EntityTypeSkeletonHorse, EntityTypeZombieHorse, EntityTypeMule, EntityTypeDonkey:
		return EntityDimensions{Width: 1.4, Height: 1.6}

	// Pig (0.9 wide x 0.9 tall)
	case EntityTypePig:
		return EntityDimensions{Width: 0.9, Height: 0.9}

	// Strider (0.9 wide x 1.7 tall)
	case EntityTypeStrider:
		return EntityDimensions{Width: 0.9, Height: 1.7}

	// Camel (0.9 wide x 2.2 tall) — same for CamelHusk
	case EntityTypeCamel, EntityTypeCamelHusk:
		return EntityDimensions{Width: 0.9, Height: 2.2}

	// Nautilus / ZombieNautilus (1.21.11+): approximate from AbstractNautilusEntity
	// TODO: verify exact dimensions from Java source when available
	case EntityTypeNautilus, EntityTypeZombieNautilus:
		return EntityDimensions{Width: 1.4, Height: 1.6}

	// Happy ghast (1.21.6+): 4x4, confirmed directly against the decompiled
	// source and mc-data-gen's extracted entity JSON — see
	// HappyGhastWidth/HappyGhastHeight.
	case EntityTypeHappyGhast:
		return EntityDimensions{Width: HappyGhastWidth, Height: HappyGhastHeight}

	// Iron golem (large: 1.4 wide x 2.7 tall)
	case EntityTypeIronGolem:
		return EntityDimensions{Width: 1.4, Height: 2.7}

	// Creeper, Enderman, Wither (humanoid-like, 0.6 x 1.8)
	case EntityTypeCreeper, EntityTypeEnderman, EntityTypeWither:
		return EntityDimensions{Width: 0.6, Height: 1.8}

	// Spider and cave spider (1.4 wide x 0.9 tall)
	case EntityTypeSpider:
		return EntityDimensions{Width: 1.4, Height: 0.9}

	// Default: use standard player dimensions for unknown/unhandled types
	default:
		return EntityDimensions{Width: PlayerWidth, Height: PlayerHeight}
	}
}
