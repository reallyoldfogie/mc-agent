package models

// EntityBounds represents a tracked entity's bounding box and velocity for collision detection.
type EntityBounds struct {
	EntityID int32  // Unique entity identifier
	AABB     AABB   // Current axis-aligned bounding box
	Velocity V3     // Entity velocity (blocks per tick)
}

// PhysicsWorld provides block state lookups and entity queries for physics simulation.
type PhysicsWorld interface {
	GetBlockStatus(x, y, z int) (uint32, bool)

	// GetEntitiesInRange returns all entities whose AABBs overlap with the query box.
	// Used for entity collision detection during physics updates.
	// Returns empty slice if no entities found or if entity tracking is unavailable.
	GetEntitiesInRange(queryBB AABB) []EntityBounds
}
