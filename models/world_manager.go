package models

// WorldManager manages chunk data for pathfinding and block queries.
type WorldManager interface {
	GetBlock(x, y, z int) (stateID uint, ok bool)
}
