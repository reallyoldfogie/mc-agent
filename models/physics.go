package models

import "math"

// V3 represents a 3D position or vector with float64 coordinates.
// This is a fundamental type used throughout the codebase for positions, velocities, etc.
type V3 struct {
	X, Y, Z float64
}

// DistanceTo calculates 3D Euclidean distance to another position
func (v V3) DistanceTo(other V3) float64 {
	dx := other.X - v.X
	dy := other.Y - v.Y
	dz := other.Z - v.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// ManhattanDistance calculates Manhattan distance to another position
func (v V3) ManhattanDistance(other V3) float64 {
	dx := v.X - other.X
	dy := v.Y - other.Y
	dz := v.Z - other.Z
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dz < 0 {
		dz = -dz
	}
	return dx + dy + dz
}

// Add returns a new position offset by the given amounts
func (v V3) Add(dx, dy, dz float64) V3 {
	return V3{X: v.X + dx, Y: v.Y + dy, Z: v.Z + dz}
}

// Sub returns the difference between two vectors (v - other)
func (v V3) Sub(other V3) V3 {
	return V3{X: v.X - other.X, Y: v.Y - other.Y, Z: v.Z - other.Z}
}

// ToFloat64 converts to float64 coordinates (for backward compatibility)
func (v V3) ToFloat64() (x, y, z float64) {
	return v.X, v.Y, v.Z
}

// V3Sub subtracts v2 from v1. Helper function for compatibility.
func V3Sub(v1, v2 V3) V3 {
	return v1.Sub(v2)
}

// Inputs represents player control inputs for one physics tick.
// This matches vanilla Minecraft client inputs.
type Inputs struct {
	ThrottleX, ThrottleZ float64 // Movement direction (-1.0 to +1.0)
	Yaw, Pitch           float64 // Look direction (degrees)
	Jump                 bool    // Jump button pressed
	Sprint               bool    // Sprint button pressed (increases speed by 30%)
	Sneak                bool    // Sneak button pressed (reduces speed to 30%)
}

// PhysicsState is an interface for physics state needed by input generators and movement executors.
// This allows input generation without direct coupling to the physics package.
type PhysicsState interface {
	// GetPosition returns current position, rotation, and ground contact status
	GetPosition() (pos V3, yaw, pitch float64, onGround bool)

	// GetVelocity returns current velocity vector
	GetVelocity() V3
}
