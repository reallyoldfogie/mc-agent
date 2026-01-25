package models

// PhysicsWorld provides block state lookups for physics simulation.
type PhysicsWorld interface {
	GetBlockStatus(x, y, z int) (uint32, bool)
}
