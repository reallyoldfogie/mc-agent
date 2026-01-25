package pathfinding

import "github.com/reallyoldfogie/mc-agent/models"

// V3Sub is an alias to models.V3Sub for backward compatibility.
var V3Sub = models.V3Sub

// MovementType is an alias to models.MovementType for backward compatibility.
type MovementType = models.MovementType

const (
	Traverse           = models.Traverse
	Sprint             = models.Sprint
	AscendStairs       = models.AscendStairs
	DescendStairs      = models.DescendStairs
	AscendJump         = models.AscendJump
	Descend            = models.Descend
	Jump2              = models.Jump2
	DiagonalTraverse   = models.DiagonalTraverse
	DiagonalAscend     = models.DiagonalAscend
	Swim       = models.Swim
	Climb       = models.Climb
	EnterClimb  = models.EnterClimb
	ExitClimb   = models.ExitClimb
	JumpToClimb  = models.JumpToClimb
	Jump2ToClimb = models.Jump2ToClimb
	SwimUp       = models.SwimUp
	SwimDown   = models.SwimDown
	Drop2North = models.Drop2North
	Drop2South         = models.Drop2South
	Drop2East          = models.Drop2East
	Drop2West          = models.Drop2West
	TraverseNorthEast  = models.TraverseNorthEast
	TraverseNorthWest  = models.TraverseNorthWest
	TraverseSouthEast  = models.TraverseSouthEast
	TraverseSouthWest  = models.TraverseSouthWest
	Sneak              = models.Sneak
	SneakThrough       = models.SneakThrough
	SneakTraverse      = models.SneakTraverse
)

// PathStep is an alias to models.PathStep for backward compatibility.
type PathStep = models.PathStep

// Path is an alias to models.Path for backward compatibility.
type Path = models.Path
