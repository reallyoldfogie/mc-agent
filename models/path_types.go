package models

import (
	"fmt"
	"strings"
)

// MovementType describes how the bot should move to the next position.
type MovementType int

const (
	Traverse MovementType = iota
	Sprint
	AscendStairs  // Walking up stairs/slabs naturally (no jump)
	DescendStairs // Walking down stairs/slabs naturally
	AscendJump    // Jumping up onto a 1-block-higher platform
	Descend       // Dropping/falling down (1-3 blocks)
	Jump2
	DiagonalTraverse
	DiagonalAscend
	Swim
	Climb       // Ladder/vine climbing (both ascent and descent)
	EnterClimb  // Horizontal entry into climbable block (ladder/vine)
	ExitClimb   // Exit from ladder/vine onto adjacent platform
	JumpToClimb  // Jump up to access elevated climbable (ladder/vine 1 block above ground)
	Jump2ToClimb // Sprint jump 2 blocks to land on/grab climbable (vine/ladder across gap)
	SwimUp
	SwimDown
	Drop2North
	Drop2South
	Drop2East
	Drop2West
	TraverseNorthEast
	TraverseNorthWest
	TraverseSouthEast
	TraverseSouthWest
	Sneak
	SneakThrough
	SneakTraverse
)

// String returns the name of the movement type.
func (mt MovementType) String() string {
	switch mt {
	case Traverse:
		return "Traverse"
	case Sprint:
		return "Sprint"
	case Sneak:
		return "Sneak"
	case AscendStairs:
		return "AscendStairs"
	case DescendStairs:
		return "DescendStairs"
	case AscendJump:
		return "AscendJump"
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
	case EnterClimb:
		return "EnterClimb"
	case ExitClimb:
		return "ExitClimb"
	case JumpToClimb:
		return "JumpToClimb"
	case Jump2ToClimb:
		return "Jump2ToClimb"
	case SwimUp:
		return "SwimUp"
	case SwimDown:
		return "SwimDown"
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

// BaseCost returns the base cost for this movement type.
func (mt MovementType) BaseCost() float64 {
	switch mt {
	case Traverse:
		return 1.0
	case Sprint:
		return 0.8
	case AscendStairs:
		return 1.0 // Natural stair traversal - same cost as walking
	case DescendStairs:
		return 1.0 // Natural stair descent - same cost as walking
	case AscendJump:
		return 1.5 // Jumping up requires effort
	case Descend:
		return 1.2 // Dropping/falling - slightly more costly than walking
	case Jump2:
		return 2.0
	case DiagonalTraverse:
		return .9 // 1.414
	case DiagonalAscend:
		return 2.0
	case Swim:
		return 2.0
	case Climb:
		return 1.8
	case EnterClimb:
		return 1.0
	case ExitClimb:
		return 1.0 // Exiting ladder onto platform - similar cost to EnterClimb
	case JumpToClimb:
		return 1.5 // Jump required to access elevated climbable - same as AscendJump
	case Jump2ToClimb:
		return 2.5 // Sprint jump 2 blocks to climbable - more expensive than Jump2
	case SwimUp:
		return 2.5
	case SwimDown:
		return 1.5
	case Drop2North, Drop2South, Drop2East, Drop2West:
		return 1.5
	case TraverseNorthEast, TraverseNorthWest, TraverseSouthEast, TraverseSouthWest:
		return 1.414
	case Sneak, SneakThrough, SneakTraverse:
		return 3.0
	default:
		return 1.0
	}
}

// PathStep represents one step in a path.
type PathStep struct {
	Position V3
	Movement MovementType
	Cost     float64
}

// Path is a sequence of steps from start to goal.
type Path struct {
	Steps      []PathStep
	TotalCost  float64
	StartPos   V3
	GoalPos    V3
	Found      bool
	SearchTime float64
}

// LogSummary returns a formatted summary of the path for logging.
func (p *Path) LogSummary() string {
	if !p.Found {
		return "Path: NOT FOUND"
	}

	straightLineDist := p.StartPos.DistanceTo(p.GoalPos)
	return fmt.Sprintf("Path: %d steps, cost=%.2f, search=%.1fms, straight-line=%.1f blocks",
		len(p.Steps), p.TotalCost, p.SearchTime, straightLineDist)
}

// LogDetails returns detailed step-by-step path information for debugging.
func (p *Path) LogDetails(showFull bool) string {
	if !p.Found {
		return "Path: NOT FOUND"
	}

	var sb strings.Builder
	sb.WriteString(p.LogSummary())
	sb.WriteString("\n")

	maxStepsToShow := 10
	if showFull {
		maxStepsToShow = (len(p.Steps) * 2) + 1
	}
	if len(p.Steps) <= maxStepsToShow*2 {
		for i, step := range p.Steps {
			sb.WriteString(fmt.Sprintf("  Step %d: %s to (%.0f, %.0f, %.0f) cost=%.2f\n",
				i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z, step.Cost))
		}
	} else {
		for i := 0; i < maxStepsToShow; i++ {
			step := p.Steps[i]
			sb.WriteString(fmt.Sprintf("  Step %d: %s to (%.0f, %.0f, %.0f) cost=%.2f\n",
				i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z, step.Cost))
		}
		sb.WriteString(fmt.Sprintf("  ... (%d steps omitted) ...\n", len(p.Steps)-maxStepsToShow*2))
		for i := len(p.Steps) - maxStepsToShow; i < len(p.Steps); i++ {
			step := p.Steps[i]
			sb.WriteString(fmt.Sprintf("  Step %d: %s to (%.0f, %.0f, %.0f) cost=%.2f\n",
				i+1, step.Movement, step.Position.X, step.Position.Y, step.Position.Z, step.Cost))
		}
	}

	return sb.String()
}
