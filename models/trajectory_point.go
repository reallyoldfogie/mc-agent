package models

// TrajectoryPoint represents a point along a projectile's trajectory
type TrajectoryPoint struct {
	Pos  V3   // Position in 3D space
	Vel  V3   // Velocity at this point
	Tick int  // Tick number
	Hit  bool // Whether projectile hit target
}
