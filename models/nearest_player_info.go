package models

// NearestPlayerInfo describes the closest tracked player for command actions.
type NearestPlayerInfo struct {
	EntityID int32
	UUID     [16]byte
	Distance float64
	X, Y, Z  float64
}
