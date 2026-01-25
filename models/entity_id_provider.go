package models

// EntityIDProvider provides the player's entity ID.
type EntityIDProvider interface {
	GetEntityID() int32
}
