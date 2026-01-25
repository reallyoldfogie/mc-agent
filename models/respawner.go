package models

// Respawner triggers a player respawn.
type Respawner interface {
	Respawn() error
}
