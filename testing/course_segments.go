package testing

import (
	"context"
	"fmt"
	"strings"

	"github.com/reallyoldfogie/mc-client-test-go/testenv"
)

// CourseSegment represents a reusable obstacle course segment
type CourseSegment interface {
	// Build the segment at origin, return start/goal positions
	Build(ctx context.Context, rcon testenv.RCONHelper,
		origin Position, orientation Orientation) (start, goal Position, err error)

	// GetExpectedTelemetry returns expected movement characteristics
	GetExpectedTelemetry() TelemetryExpectations

	// Name returns a human-readable name for this segment
	Name() string
}

// TelemetryExpectations defines expected movement characteristics for a segment
type TelemetryExpectations struct {
	MinJumps     int  // Minimum jumps required
	MaxJumps     int  // Maximum jumps allowed
	RequireClimb bool // Must use climbing state
	RequireSneak bool // Must use sneaking
	AllowJumps   bool // Jumps permitted
}

// LadderAscent represents a ladder climbing segment
type LadderAscent struct {
	Height int // Number of blocks to climb
}

// Build constructs a ladder ascent segment
func (la *LadderAscent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	startCenterX := int(origin.X)
	startCenterZ := int(origin.Z)
	goalCenterX := int(origin.X) + dx*2
	goalCenterZ := int(origin.Z) + dz*2
	goalY := int(origin.Y) + la.Height

	// Build start platform (3x3 at start center, ground level)
	if err := BuildPlatform(ctx, rcon, startCenterX-1, int(origin.Y), startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build ladder tower FIRST (ladder at origin, wall one block in direction)
	// This places wall at origin+dx which would block path to goal
	if err := BuildLadder(ctx, rcon, int(origin.X), int(origin.Z), int(origin.Y), int(origin.Y)+la.Height, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Build goal platform AFTER ladder so it overwrites the wall at the top level,
	// creating a passage for the agent to walk from ladder to goal
	if err := BuildPlatform(ctx, rcon, goalCenterX-1, goalY, goalCenterZ-1, 3, 3, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: origin.Y + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalCenterX) + 0.5, Y: float64(goalY) + 1, Z: float64(goalCenterZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for ladder ascent
func (la *LadderAscent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     2, // Allow a couple jumps to reach ladder
		RequireClimb: true,
		AllowJumps:   true,
	}
}

// Name returns the segment name
func (la *LadderAscent) Name() string {
	return fmt.Sprintf("LadderAscent_%dBlocks", la.Height)
}

// LadderDescent represents a ladder descent segment
type LadderDescent struct {
	Height int // Number of blocks to descend
}

// Build constructs a ladder descent segment
func (ld *LadderDescent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	startCenterX := int(origin.X) + dx*2
	startCenterZ := int(origin.Z) + dz*2
	goalCenterX := int(origin.X) - dx*2
	goalCenterZ := int(origin.Z) - dz*2

	// Start platform is elevated
	startY := int(origin.Y) + ld.Height

	// Build goal platform at bottom first (no conflict with wall)
	if err := BuildPlatform(ctx, rcon, goalCenterX-1, int(origin.Y), goalCenterZ-1, 3, 3, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build ladder tower (wall at origin+dx blocks path from start to ladder)
	if err := BuildLadder(ctx, rcon, int(origin.X), int(origin.Z), int(origin.Y), startY, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Build start platform AFTER ladder so it overwrites the wall at the top level,
	// creating a passage for the agent to walk from start to ladder
	if err := BuildPlatform(ctx, rcon, startCenterX-1, startY, startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: float64(startY) + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalCenterX) + 0.5, Y: origin.Y + 1, Z: float64(goalCenterZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for ladder descent
func (ld *LadderDescent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     0,
		RequireClimb: true,
		AllowJumps:   false, // Descending shouldn't require jumps
	}
}

// Name returns the segment name
func (ld *LadderDescent) Name() string {
	return fmt.Sprintf("LadderDescent_%dBlocks", ld.Height)
}

// StairAscent represents a stair climbing segment
type StairAscent struct {
	Steps int // Number of stair steps
}

// Build constructs a stair ascent segment
func (sa *StairAscent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	startCenterX := int(origin.X) - dx
	startCenterZ := int(origin.Z) - dz

	// Build start platform (3x3 centered at start center)
	if err := BuildPlatform(ctx, rcon, startCenterX-1, int(origin.Y), startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build stairs starting at origin (directly adjacent to platform edge)
	// This ensures no gap between platform and first stair
	stairStartX := int(origin.X)
	stairStartZ := int(origin.Z)
	stairBaseY := int(origin.Y) + 1
	if err := BuildStairs(ctx, rcon, stairStartX, stairBaseY, stairStartZ, sa.Steps, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Build goal platform at top (directly after the last stair)
	goalX := int(origin.X) + dx*(sa.Steps+1)
	goalZ := int(origin.Z) + dz*(sa.Steps+1)
	goalY := stairBaseY + sa.Steps - 1
	if err := BuildPlatform(ctx, rcon, goalX-1, goalY, goalZ-1, 3, 3, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: origin.Y + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalX) + 0.5, Y: float64(goalY) + 1, Z: float64(goalZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for stair ascent
func (sa *StairAscent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     2, // May use jumps, but stairs don't require them
		RequireClimb: false,
		AllowJumps:   true, // Jumps are allowed - pathfinder may choose Jump2 or Traverse+Ascend
	}
}

// Name returns the segment name
func (sa *StairAscent) Name() string {
	return fmt.Sprintf("StairAscent_%dSteps", sa.Steps)
}

// StairDescent represents a stair descent segment
type StairDescent struct {
	Steps int // Number of stair steps
}

// Build constructs a stair descent segment
func (sd *StairDescent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	startCenterX := int(origin.X) - dx
	startCenterZ := int(origin.Z) - dz

	// Start platform is elevated
	startY := int(origin.Y) + sd.Steps
	if err := BuildPlatform(ctx, rcon, startCenterX-1, startY, startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build descending stairs starting at origin (directly adjacent to platform edge)
	stairStartX := int(origin.X)
	stairStartZ := int(origin.Z)
	stairTopY := startY + 1
	if err := BuildStairsDescending(ctx, rcon, stairStartX, stairTopY, stairStartZ, sd.Steps, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Goal platform at bottom (directly after the last stair)
	goalX := int(origin.X) + dx*(sd.Steps+1)
	goalZ := int(origin.Z) + dz*(sd.Steps+1)
	goalY := int(origin.Y) + 1
	if err := BuildPlatform(ctx, rcon, goalX-1, goalY, goalZ-1, 3, 3, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: float64(startY) + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalX) + 0.5, Y: float64(goalY) + 1, Z: float64(goalZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for stair descent
func (sd *StairDescent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     2, // May use jumps, but stairs don't require them
		RequireClimb: false,
		AllowJumps:   true, // Jumps are allowed
	}
}

// Name returns the segment name
func (sd *StairDescent) Name() string {
	return fmt.Sprintf("StairDescent_%dSteps", sd.Steps)
}

// BlockStepAscent represents a block step climbing segment
type BlockStepAscent struct {
	Steps int // Number of block steps
}

// Build constructs a block step ascent segment
func (bs *BlockStepAscent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	startCenterX := int(origin.X) - dx
	startCenterZ := int(origin.Z) - dz

	// Build start platform
	if err := BuildPlatform(ctx, rcon, startCenterX-1, int(origin.Y), startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build full block steps starting after the platform
	// BuildBlockSteps places first block at x+dx*1, so we pass origin to get first block at origin+dx*1
	stepStartX := int(origin.X)
	stepStartZ := int(origin.Z)
	if err := BuildBlockSteps(ctx, rcon, stepStartX, int(origin.Y), stepStartZ, bs.Steps, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Build goal platform beyond the last step (no overlap)
	// Steps are at origin+dx*1 through origin+dx*steps, so goal center at origin+dx*(steps+2)
	goalX := int(origin.X) + dx*(bs.Steps+2)
	goalZ := int(origin.Z) + dz*(bs.Steps+2)
	goalY := int(origin.Y) + bs.Steps
	if err := BuildPlatform(ctx, rcon, goalX-1, goalY, goalZ-1, 3, 3, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: origin.Y + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalX) + 0.5, Y: float64(goalY) + 1, Z: float64(goalZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for block step ascent
func (bs *BlockStepAscent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     bs.Steps,     // Must jump for each step
		MaxJumps:     bs.Steps + 2, // Allow a few extra
		RequireClimb: false,
		AllowJumps:   true,
	}
}

// Name returns the segment name
func (bs *BlockStepAscent) Name() string {
	return fmt.Sprintf("BlockStepAscent_%dSteps", bs.Steps)
}

// BlockStepDescent represents a block step descent segment
type BlockStepDescent struct {
	Steps int // Number of block steps
}

// Build constructs a block step descent segment
func (bs *BlockStepDescent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	startCenterX := int(origin.X) - dx
	startCenterZ := int(origin.Z) - dz

	// Start platform is elevated
	startY := int(origin.Y) + bs.Steps
	if err := BuildPlatform(ctx, rcon, startCenterX-1, startY, startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build block steps from high to low, starting adjacent to the platform
	// BuildBlockStepsDescending places first block at x+dx*1, so we pass origin to get first block at origin+dx*1
	stepStartX := int(origin.X)
	stepStartZ := int(origin.Z)
	if err := BuildBlockStepsDescending(ctx, rcon, stepStartX, startY, stepStartZ, bs.Steps, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Goal platform at bottom, beyond the last step (no overlap)
	// Steps are at origin+dx*1 through origin+dx*steps, so goal center at origin+dx*(steps+2)
	goalX := int(origin.X) + dx*(bs.Steps+2)
	goalZ := int(origin.Z) + dz*(bs.Steps+2)
	if err := BuildPlatform(ctx, rcon, goalX-1, int(origin.Y), goalZ-1, 3, 3, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: float64(startY) + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalX) + 0.5, Y: origin.Y + 1, Z: float64(goalZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for block step descent
func (bs *BlockStepDescent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     1, // Maybe one jump to start descent
		RequireClimb: false,
		AllowJumps:   true,
	}
}

// Name returns the segment name
func (bs *BlockStepDescent) Name() string {
	return fmt.Sprintf("BlockStepDescent_%dSteps", bs.Steps)
}

// VineAscent represents a vine climbing segment
type VineAscent struct {
	Height int // Number of blocks to climb
}

// Build constructs a vine ascent segment
func (va *VineAscent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	// Layout: start_platform | pillar | vines | goal_platform
	// BuildVine places pillar at (vine - dx/dz), so pillar ends up at origin
	// Vines are at origin + dx/dz
	// Start must be OPPOSITE of movement (away from pillar)
	// Goal must be PAST the vines (further in movement direction)

	// Vines at origin + dx/dz, pillar at origin
	vineX := int(origin.X) + dx
	vineZ := int(origin.Z) + dz

	// Start platform 2 blocks opposite of movement direction (so 3x3 doesn't overlap pillar at origin)
	startCenterX := int(origin.X) - dx*2
	startCenterZ := int(origin.Z) - dz*2
	if err := BuildPlatform(ctx, rcon, startCenterX-1, int(origin.Y), startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build vines (pillar will be placed at origin by BuildVine)
	if err := BuildVine(ctx, rcon, vineX, vineZ, int(origin.Y), int(origin.Y)+va.Height, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Goal platform 3 blocks in movement direction (so 3x3 doesn't overlap vines at origin + dx/dz)
	// goalCenterX := int(origin.X) + dx*3
	// goalCenterZ := int(origin.Z) + dz*3
	// goalY := int(origin.Y) + va.Height
	// if err := BuildPlatform(ctx, rcon, goalCenterX-1, goalY, goalCenterZ-1, 3, 3, "stone"); err != nil {
	// 	return Position{}, Position{}, err
	// }

	// Goal platform 3 blocks on opposite side of pillar from vines (so 3x3 doesn't overlap vines at origin + dx/dz)
	goalCenterX := int(origin.X) + (-1 * dx * 2)
	goalCenterZ := int(origin.Z) + (-1 * dz * 2)
	goalY := int(origin.Y) + va.Height
	if err := BuildPlatform(ctx, rcon, goalCenterX-1, goalY, goalCenterZ-1, 5, 5, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: origin.Y + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalCenterX) + 0.5, Y: float64(goalY) + 1, Z: float64(goalCenterZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for vine ascent
func (va *VineAscent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     2, // Allow a couple jumps to reach vines
		RequireClimb: true,
		AllowJumps:   true,
	}
}

// Name returns the segment name
func (va *VineAscent) Name() string {
	return fmt.Sprintf("VineAscent_%dBlocks", va.Height)
}

// VineDescent represents a vine descent segment
type VineDescent struct {
	Height int // Number of blocks to descend
}

// Build constructs a vine descent segment
func (vd *VineDescent) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	dx, dz := orientationVector(orientation)

	// Layout: start_platform | pillar | vines | goal_platform
	// BuildVine places pillar at (vine - dx/dz), so pillar ends up at origin
	// Vines are at origin + dx/dz
	// Start must be OPPOSITE of movement (away from pillar)
	// Goal must be PAST the vines (further in movement direction)

	// Vines at origin + dx/dz, pillar at origin
	vineX := int(origin.X) + dx
	vineZ := int(origin.Z) + dz

	// Start platform 2 blocks opposite of movement direction (so 3x3 doesn't overlap pillar at origin)
	startCenterX := int(origin.X) - dx*2
	startCenterZ := int(origin.Z) - dz*2
	startY := int(origin.Y) + vd.Height
	if err := BuildPlatform(ctx, rcon, startCenterX-1, startY, startCenterZ-1, 3, 3, "blackstone"); err != nil {
		return Position{}, Position{}, err
	}

	// Build vines (pillar will be placed at origin by BuildVine)
	if err := BuildVine(ctx, rcon, vineX, vineZ, int(origin.Y), int(origin.Y)+vd.Height, orientation); err != nil {
		return Position{}, Position{}, err
	}

	// Goal platform 3 blocks in movement direction (so 3x3 doesn't overlap vines at origin + dx/dz)
	goalCenterX := int(origin.X) + dx*3
	goalCenterZ := int(origin.Z) + dz*3
	if err := BuildPlatform(ctx, rcon, goalCenterX-1, int(origin.Y), goalCenterZ-1, 3, 3, "stone"); err != nil {
		return Position{}, Position{}, err
	}

	start = Position{X: float64(startCenterX) + 0.5, Y: float64(startY) + 1, Z: float64(startCenterZ) + 0.5}
	goal = Position{X: float64(goalCenterX) + 0.5, Y: origin.Y + 1, Z: float64(goalCenterZ) + 0.5}
	return start, goal, nil
}

// GetExpectedTelemetry returns expected telemetry for vine descent
func (vd *VineDescent) GetExpectedTelemetry() TelemetryExpectations {
	return TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     0,
		RequireClimb: true,
		AllowJumps:   true,
	}
}

// Name returns the segment name
func (vd *VineDescent) Name() string {
	return fmt.Sprintf("VineDescent_%dBlocks", vd.Height)
}

// ComboSegment chains multiple segments together
type ComboSegment struct {
	Segments []CourseSegment // Segments to chain
	label    string          // Human-readable label
}

// Build constructs a combo segment by chaining sub-segments
func (cs *ComboSegment) Build(ctx context.Context, rcon testenv.RCONHelper,
	origin Position, orientation Orientation) (start, goal Position, err error) {

	currentOrigin := origin
	var finalGoal Position

	for i, segment := range cs.Segments {
		s, g, err := segment.Build(ctx, rcon, currentOrigin, orientation)
		if err != nil {
			return Position{}, Position{}, fmt.Errorf("segment %d failed: %w", i, err)
		}

		if i == 0 {
			start = s
		}

		finalGoal = g
		dx, dz := orientationVector(orientation)
		currentOrigin = Position{
			X: g.X - 0.5 + float64(dx*3),
			Y: g.Y - 1,
			Z: g.Z - 0.5 + float64(dz*3),
		}
	}

	return start, finalGoal, nil
}

// GetExpectedTelemetry returns combined telemetry expectations
func (cs *ComboSegment) GetExpectedTelemetry() TelemetryExpectations {
	// Combine expectations from all segments
	combined := TelemetryExpectations{
		MinJumps:     0,
		MaxJumps:     0,
		RequireClimb: false,
		RequireSneak: false,
		AllowJumps:   true,
	}

	for _, seg := range cs.Segments {
		exp := seg.GetExpectedTelemetry()
		combined.MinJumps += exp.MinJumps
		combined.MaxJumps += exp.MaxJumps
		combined.RequireClimb = combined.RequireClimb || exp.RequireClimb
		combined.RequireSneak = combined.RequireSneak || exp.RequireSneak
	}

	return combined
}

// Name returns the segment name
func (cs *ComboSegment) Name() string {
	if cs.label != "" {
		return cs.label
	}
	return "ComboSegment"
}

// NewComboSegment creates a new combo segment with a label
func NewComboSegment(label string, segments ...CourseSegment) *ComboSegment {
	return &ComboSegment{
		Segments: segments,
		label:    label,
	}
}

func buildMultilineTextDisplay(in []string) string {
	return strings.Join(in, "\\n")
}
