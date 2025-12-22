package pathfinding

import (
	"fmt"
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
)

// V3 is an alias to models.V3 for backward compatibility.
// Use models.V3 for new code.
type V3 = models.V3

// V3Sub is an alias to models.V3Sub for backward compatibility.
var V3Sub = models.V3Sub

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

	// Directional ladder descents (from phys archive)
	DescendLadderNorth // Descend ladder and exit north
	DescendLadderSouth // Descend ladder and exit south
	DescendLadderEast  // Descend ladder and exit east
	DescendLadderWest  // Descend ladder and exit west

	// 2-block drops with direction (from phys archive)
	Drop2North // Drop 2 blocks and move north
	Drop2South // Drop 2 blocks and move south
	Drop2East  // Drop 2 blocks and move east
	Drop2West  // Drop 2 blocks and move west

	// True diagonal traverses (from phys archive)
	TraverseNorthEast // Walk diagonally northeast
	TraverseNorthWest // Walk diagonally northwest
	TraverseSouthEast // Walk diagonally southeast
	TraverseSouthWest // Walk diagonally southwest

	// Sneaking movements (new - for 1.5 block gaps and edge safety)
	SneakThrough  // Navigate through 1.5-1.8 block high gap while sneaking
	SneakTraverse // Move along block edges without falling off while sneaking
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
	case DescendLadderNorth:
		return "DescendLadderNorth"
	case DescendLadderSouth:
		return "DescendLadderSouth"
	case DescendLadderEast:
		return "DescendLadderEast"
	case DescendLadderWest:
		return "DescendLadderWest"
	case Drop2North:
		return "Drop2North"
	case Drop2South:
		return "Drop2South"
	case Drop2East:
		return "Drop2East"
	case Drop2West:
		return "Drop2West"
	case TraverseNorthEast:
		return "TraverseNorthEast"
	case TraverseNorthWest:
		return "TraverseNorthWest"
	case TraverseSouthEast:
		return "TraverseSouthEast"
	case TraverseSouthWest:
		return "TraverseSouthWest"
	case SneakThrough:
		return "SneakThrough"
	case SneakTraverse:
		return "SneakTraverse"
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

	// Directional ladder descents
	case DescendLadderNorth, DescendLadderSouth, DescendLadderEast, DescendLadderWest:
		return 1.8 // Same as Climb (controlled ladder descent)

	// 2-block drops
	case Drop2North, Drop2South, Drop2East, Drop2West:
		return 1.5 // Higher than 1-block drop, lower than jump (gravity helps)

	// True diagonals
	case TraverseNorthEast, TraverseNorthWest, TraverseSouthEast, TraverseSouthWest:
		return 1.414 // sqrt(2) for diagonal distance

	// Sneaking movements
	case SneakThrough:
		return 3.0 // High cost due to slow speed (30% of normal) and special positioning
	case SneakTraverse:
		return 3.0 // High cost due to slow speed (30% of normal)

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
