package models

import (
	"fmt"
	"math"
)

// V3 represents a 3D position or vector with float64 coordinates.
// This is a fundamental type used throughout the codebase for positions, velocities, etc.
type V3 struct {
	X, Y, Z float64
}

func (a V3) String() string {
	return fmt.Sprintf("(%.2f, %.2f, %.2f)", a.X, a.Y, a.Z)
}

func (a V3) Add(b V3) V3      { return V3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a V3) Sub(b V3) V3      { return V3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a V3) Mul(s float64) V3 { return V3{a.X * s, a.Y * s, a.Z * s} }

func (v V3) Floor() (int, int, int) {
	return int(math.Floor(v.X)), int(math.Floor(v.Y)), int(math.Floor(v.Z))
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

// Within checks if this position is within a certain distance of another position
func (v V3) Within(pt V3, tolerance float64) bool {
	return v.DistanceTo(pt) <= tolerance
}

// ToFloat64 converts to float64 coordinates (for backward compatibility)
func (v V3) ToFloat64() (x, y, z float64) {
	return v.X, v.Y, v.Z
}

// Inputs represents player control inputs for one physics tick.
// This matches vanilla Minecraft client inputs.
type Inputs struct {
	ThrottleX, ThrottleZ float64 // Movement direction (-1.0 to +1.0)
	Yaw, Pitch           float64 // Look direction (degrees)
	Jump                 bool    // Jump button pressed
	Sprint               bool    // Sprint button pressed (increases speed by 30%)
	Sneak                bool    // Sneak button pressed (reduces speed to 30%)
	ClimbDirection       float64 // Ladder climb direction: +1.0=up, -1.0=down, 0.0=no climb
}

// PhysicsState moved to physics_state.go to keep interfaces isolated per file.
