package pathfinding

import (
	"fmt"
	"math"
	"strings"
)

// V3 represents a 3D integer position (block coordinates)
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

// ToFloat64 converts to float64 coordinates (for movement executor)
func (v V3) ToFloat64() (x, y, z float64) {
	return float64(v.X), float64(v.Y), float64(v.Z)
}

// MovementType describes how the bot should move to the next position
type MovementType int

const (
	Traverse         MovementType = iota // Walk on same level
	Ascend                               // Jump up one block
	Descend                              // Drop down (1-3 blocks)
	Jump2                                // Jump across 2-block gap
	DiagonalTraverse                     // Walk diagonally on same level
	DiagonalAscend                       // Jump up diagonally
	Swim                                 // Move through water
	Climb                                // Climb ladder/vine
	SwimUp                               // Swim upward in water
	SwimDown                             // Swim downward in water
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
	case DiagonalTraverse:
		return "DiagonalTraverse"
	case DiagonalAscend:
		return "DiagonalAscend"
	case Swim:
		return "Swim"
	case Climb:
		return "Climb"
	case SwimUp:
		return "SwimUp"
	case SwimDown:
		return "SwimDown"
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
	case DiagonalTraverse:
		return 1.414 // sqrt(2) for diagonal distance
	case DiagonalAscend:
		return 2.0 // Diagonal jump is expensive
	case Swim:
		return 2.0 // Swimming is slower
	case Climb:
		return 1.8 // Climbing is moderately slow
	case SwimUp:
		return 2.5 // Swimming up is slower
	case SwimDown:
		return 1.5 // Swimming down is faster
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

// LogSummary returns a formatted summary of the path for logging
func (p *Path) LogSummary() string {
	if !p.Found {
		return "Path: NOT FOUND"
	}
	
	straightLineDist := p.StartPos.DistanceTo(p.GoalPos)
	return fmt.Sprintf("Path: %d steps, cost=%.2f, search=%.1fms, straight-line=%.1f blocks",
		len(p.Steps), p.TotalCost, p.SearchTime, straightLineDist)
}

// LogDetails returns detailed step-by-step path information for debugging
func (p *Path) LogDetails() string {
	if !p.Found {
		return "Path: NOT FOUND"
	}
	
	var sb strings.Builder
	sb.WriteString(p.LogSummary())
	sb.WriteString("\n")
	
	// Log first few and last few steps to avoid excessive output
	maxStepsToShow := 10
	if len(p.Steps) <= maxStepsToShow*2 {
		// Show all steps if path is short
		for i, step := range p.Steps {
			sb.WriteString(fmt.Sprintf("  Step %d: %s to (%.0f, %.0f, %.0f) cost=%.2f\n",
				i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z, step.Cost))
		}
	} else {
		// Show first N steps
		for i := 0; i < maxStepsToShow; i++ {
			step := p.Steps[i]
			sb.WriteString(fmt.Sprintf("  Step %d: %s to (%.0f, %.0f, %.0f) cost=%.2f\n",
				i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z, step.Cost))
		}
		sb.WriteString(fmt.Sprintf("  ... (%d steps omitted) ...\n", len(p.Steps)-maxStepsToShow*2))
		// Show last N steps
		for i := len(p.Steps) - maxStepsToShow; i < len(p.Steps); i++ {
			step := p.Steps[i]
			sb.WriteString(fmt.Sprintf("  Step %d: %s to (%.0f, %.0f, %.0f) cost=%.2f\n",
				i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z, step.Cost))
		}
	}
	
	return sb.String()
}
