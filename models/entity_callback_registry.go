package models

// EntityPositionCallback is called when an entity's position is updated from a server packet.
// Used for tracking entity movement during tests.
type EntityPositionCallback func(entityID int32, x, y, z float64)

type EntityCallbackRegistry interface {
	RegisterEntityPositionCallback(EntityPositionCallback)
}
