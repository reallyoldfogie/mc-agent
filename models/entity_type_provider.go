package models

// EntityTypeProvider reports the type of a tracked entity. It is used to
// decide how to interact with an entity — e.g. whether opening its container
// requires mounting it first (horses/donkeys/mules/llamas) or opens directly
// on interact (boats/minecarts).
type EntityTypeProvider interface {
	GetEntityType(entityID int32) EntityType
}
