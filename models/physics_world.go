package models

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

// PhysicsWorld provides block state lookups and entity queries for physics simulation.
type PhysicsWorld interface {
	GetBlockStatus(x, y, z int) (uint32, bool)

	// GetEntitiesInRange returns all entities whose AABBs overlap with the query box.
	// Used for entity collision detection during physics updates.
	// Returns empty slice if no entities found or if entity tracking is unavailable.
	GetEntitiesInRange(queryBB AABB) []EntityBounds
}
