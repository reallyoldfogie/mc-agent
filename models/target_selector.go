package models

// TargetSelector finds and validates target players.
type TargetSelector interface {
	FindPlayerByName(name string) (*TargetInfo, error)
	FindNearestPlayer() (*TargetInfo, error)
	GetTargetPosition(entityID int32) (x, y, z float64, exists bool)
	CalculateDistance(entityID int32) (float64, error)
}

// TargetInfo contains information about a selected target.
type TargetInfo struct {
	EntityID int32
	UUID     [16]byte
	Name     string
	X, Y, Z  float64
	Distance float64
}
