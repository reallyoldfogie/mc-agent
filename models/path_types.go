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
	WadeWater    // Moving through water without full head submersion (no sprint-swim speed boost); ground support under `from` is irrelevant
	Climb        // Ladder/vine climbing (both ascent and descent)
	EnterClimb   // Horizontal entry into climbable block (ladder/vine)
	ExitClimb    // Exit from ladder/vine onto adjacent platform
	JumpToClimb  // Jump up to access elevated climbable (ladder/vine 1 block above ground)
	Jump2ToClimb // Sprint jump 2 blocks to land on/grab climbable (vine/ladder across gap)
	SwimUp
	SwimDown
	ExitWater // Exit from water onto adjacent solid ground
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
	MountVehicle        // Interact with a vehicle to mount it
	DismountVehicle     // Sneak to dismount current vehicle
	PlaceVehicle        // Place a carried boat on water and mount it (no pre-existing entity)
	VehicleTraverse     // Land vehicle traversal (horse/camel/pig/donkey/mule)
	VehicleAscend       // Land vehicle jump up 1-block ledge
	VehicleLavaTraverse // Strider on lava surface
	VehicleRailTraverse // Minecart on rail (forward only)
	VehicleSwim         // Boat on water surface
	VehicleFly3D        // Nautilus 3D underwater movement
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
	case WadeWater:
		return "WadeWater"
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
	case ExitWater:
		return "ExitWater"
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
	case MountVehicle:
		return "MountVehicle"
	case DismountVehicle:
		return "DismountVehicle"
	case PlaceVehicle:
		return "PlaceVehicle"
	case VehicleTraverse:
		return "VehicleTraverse"
	case VehicleAscend:
		return "VehicleAscend"
	case VehicleLavaTraverse:
		return "VehicleLavaTraverse"
	case VehicleRailTraverse:
		return "VehicleRailTraverse"
	case VehicleSwim:
		return "VehicleSwim"
	case VehicleFly3D:
		return "VehicleFly3D"
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
		// Sprint-swim, fully submerged: ~4.0 blocks/s vs. dry walk's 4.317 blocks/s,
		// verified against LivingEntity.travelInFluid (mc-data-gen extractedSrc 1.21.5) -
		// nearly as fast as walking, not the heavy penalty the old flat 2.0 implied.
		return 1.08
	case WadeWater:
		// Non-sprint water movement (wading on solid ground or treading with none -
		// confirmed identical speed in source): ~2.0 blocks/s, the genuinely slow case.
		return 2.16
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
		// Scaled from the old Swim=2.0 baseline (2.5) to the new Swim=1.08 baseline;
		// vertical fluid speed itself not independently re-verified against source.
		return 1.35
	case SwimDown:
		return 0.81
	case ExitWater:
		return 1.0 // Exiting water to ground
	case Drop2North, Drop2South, Drop2East, Drop2West:
		return 1.5
	case TraverseNorthEast, TraverseNorthWest, TraverseSouthEast, TraverseSouthWest:
		return 1.414
	case Sneak, SneakThrough, SneakTraverse:
		return 3.0
	case MountVehicle, DismountVehicle:
		return 3.0 // One-time action cost
	case PlaceVehicle:
		// One-time cost for placing a carried boat and mounting it - a real
		// premium over MountVehicle's 3.0 (switch hotbar slot, send the place
		// packet, then wait for the resulting entity to actually appear
		// before mounting), not just the instant interact a pre-existing
		// vehicle needs. See docs/plans/WATER_TRAVERSAL_PATHFINDING_PLAN.md's
		// Item 8 - keeps a short crossing from choosing placement over a
		// plain swim that would finish almost as fast.
		return 10.0
	case VehicleTraverse:
		return 0.3 // Much faster than walking
	case VehicleAscend:
		return 1.0
	case VehicleLavaTraverse:
		return 0.4
	case VehicleRailTraverse:
		return 0.25 // Fast on powered rails
	case VehicleSwim:
		// Boat on flat water: ~7.2 blocks/s vs. dry walk's 4.317, verified against
		// AbstractBoatEntity's paddle accel/drag (mc-data-gen extractedSrc 1.21.5).
		return 0.60
	case VehicleFly3D:
		return 0.4
	default:
		return 1.0
	}
}

// PathStep represents one step in a path.
type PathStep struct {
	Position        V3
	Movement        MovementType
	Cost            float64
	VehicleEntityID int32 // 0 = on foot; non-zero = entity ID of vehicle being ridden
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
