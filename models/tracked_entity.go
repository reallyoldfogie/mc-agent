package models

// TrackedEntity represents a tracked entity.
type TrackedEntity struct {
	UUID    [16]byte
	X, Y, Z float64
	Yaw     int8
	Pitch   int8
}
