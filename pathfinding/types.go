package pathfinding

import "math"

// V3 represents a 3D integer position (block coordinates)
type V3 struct {
	X, Y, Z int
}

// DistanceTo calculates 3D Euclidean distance to another position
func (v V3) DistanceTo(other V3) float64 {
	dx := float64(other.X - v.X)
	dy := float64(other.Y - v.Y)
	dz := float64(other.Z - v.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// ManhattanDistance calculates Manhattan distance to another position
func (v V3) ManhattanDistance(other V3) int {
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
func (v V3) Add(dx, dy, dz int) V3 {
	return V3{X: v.X + dx, Y: v.Y + dy, Z: v.Z + dz}
}

// ToFloat64 converts to float64 coordinates (for movement executor)
func (v V3) ToFloat64() (x, y, z float64) {
	return float64(v.X), float64(v.Y), float64(v.Z)
}

// MovementType describes how the bot should move to the next position
type MovementType int

const (
	Traverse MovementType = iota // Walk on same level
	Ascend                        // Jump up one block
	Descend                       // Drop down (1-3 blocks)
	Jump2                         // Jump across 2-block gap
	// Future movement types:
	// Swim                          // Move through water
	// Climb                         // Climb ladder
)

// String returns the name of the movement type
func (mt MovementType) String() string {
	switch mt {
	case Traverse:
		return "Traverse"
	case Ascend:
		return "Ascend"
	case Descend:
		return "Descend"
	case Jump2:
		return "Jump2"
	default:
		return "Unknown"
	}
}

// BaseCost returns the base cost for this movement type
func (mt MovementType) BaseCost() float64 {
	switch mt {
	case Traverse:
		return 1.0
	case Ascend:
		return 1.5 // Jumping costs more
	case Descend:
		return 1.2
	case Jump2:
		return 2.0 // Gap jumps are more expensive
	default:
		return 1.0
	}
}

// PathStep represents one step in a path
type PathStep struct {
	Position V3           // Target block position (feet level)
	Movement MovementType // How to get there from previous step
	Cost     float64      // Movement cost for this step
}

// Path is a sequence of steps from start to goal
type Path struct {
	Steps      []PathStep
	TotalCost  float64
	StartPos   V3
	GoalPos    V3
	Found      bool
	SearchTime float64 // Time in milliseconds
}
