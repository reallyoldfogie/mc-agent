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
	// MineThrough breaks a solid, diggable block standing between `from`
	// and an otherwise-impassable horizontally-adjacent cell, then moves
	// into it - the cardinal-move analogue of Traverse, offered only when
	// Traverse/WadeWater aren't already available. See docs/plans/14's
	// narrow-shaft-navigability finding.
	MineThrough
	// MineDown breaks the solid, diggable block directly underfoot (same
	// X/Z column, one block down) and drops into it - unlike every other
	// move in this list, this is never offered by any ordinary
	// passability-based check, since under normal terrain you'd never want
	// to fall through your own floor. It exists specifically so a bot that
	// has dug itself into a straight 1x1 vertical shaft - with no adjacent
	// column left to use any horizontal move at all - has a way to
	// continue, one layer at a time.
	//
	// Deliberately the pricier of the two ways to mine downward (see
	// MineDownDiagonal) - mining the exact block directly underfoot means
	// committing to whatever's revealed (a cave, lava) with no fallback,
	// since the bot was never anywhere else. Kept only as the last-resort
	// option for when there's no room for MineDownDiagonal's two-column
	// pattern at all (a shaft already dug 1-wide).
	MineDown
	// MineDownDiagonal breaks the solid, diggable block one level down AND
	// one cardinal direction over (never the block directly underfoot) -
	// the safer default way to mine downward (operator direction,
	// 2026-10-14): a bot digging straight down with plain MineDown commits
	// blind to whatever a broken block reveals (a cave, lava) with nowhere
	// else to have been standing. By always landing in the *other* of two
	// adjacent columns instead, the bot is never the one mining the exact
	// block it's currently standing on - each break happens one column
	// over from wherever its feet actually are, so the column it's
	// standing on right now is always still intact, unexplored rock, not
	// something it just blindly broke into. Chained across repeated calls
	// this naturally staggers between two columns one level at a time
	// (hence "2x1"), each diagonal step a fresh, independent check against
	// real world state, not a lookahead/X-ray guess at what's below.
	MineDownDiagonal
	// MineNeighborFloor digs down into an adjacent column that's already
	// walkable at the bot's current height - typically the column it just
	// entered from via an ordinary MineThrough step (operator design,
	// 2026-10-15, a 2-wide-shaft safe-mining technique): mine that
	// column's floor - and, if also diggable, one more level beneath it -
	// then drop into it, landing 1 or 2 blocks down depending on which
	// turned out to need mining. The bot is never the one mining the exact
	// block it's currently standing on with nothing else holding it up -
	// the specific risk plain MineDown has - because it's standing in the
	// adjacent column throughout the dig, only moving once the landing
	// cell's ground support is confirmed. See
	// pathfinding.tryMineNeighborFloor's own doc comment for why this
	// needs to both mine and move in the same step, unlike an earlier,
	// straddle-based design for the same idea that didn't work out.
	MineNeighborFloor
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
	case MineThrough:
		return "MineThrough"
	case MineDown:
		return "MineDown"
	case MineDownDiagonal:
		return "MineDownDiagonal"
	case MineNeighborFloor:
		return "MineNeighborFloor"
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
	case MineThrough, MineDown, MineDownDiagonal, MineNeighborFloor:
		// Representative only - pathfinding.mineMoveCost computes the real,
		// hardness-scaled per-step cost at generation time (same pattern
		// Jump2's "+0.5" variant already uses above). This flat value is a
		// deliberately pseudo-expensive floor: well above any walking
		// detour (Traverse=1.0), comparable to PlaceVehicle's 10.0 "real
		// one-time cost," so a path search prefers walking around almost
		// every time, but still finite - a bot with no walkable route left
		// at all (a straight 1x1 shaft) can still dig itself out.
		return 8.0
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
	// MineTargets is the block coordinate(s) that must be broken before this
	// step can be completed - only set for MineThrough (1 or 2 entries: the
	// feet-level cell at Position, and the head-level cell above it if that
	// was also solid), MineDown (always exactly 1: Position itself,
	// since the destination cell IS the block being broken), MineDownDiagonal
	// (1-3 entries: the landing cell plus whichever headroom cells needed
	// clearing), and MineNeighborFloor (1-2 entries: the adjacent column's
	// floor cell, plus one more level beneath it if that also needed
	// clearing - see pathfinding.tryMineNeighborFloor). nil/empty for
	// every other movement type.
	MineTargets []V3
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
