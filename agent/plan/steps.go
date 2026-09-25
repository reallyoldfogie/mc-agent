package plan

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Hand constants for UseItem steps.
const (
	MainHand = 0
	OffHand  = 1
)

// BlockFace constants for UseItem/OpenContainer steps.
const (
	FaceDown  = 0
	FaceUp    = 1
	FaceNorth = 2
	FaceSouth = 3
	FaceWest  = 4
	FaceEast  = 5
)

// MoveTo moves the agent using pathfinding.
type MoveTo struct {
	IDValue string
	X, Y, Z float64
	Timeout time.Duration
}

func (s MoveTo) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "moveTo"
}

func (s MoveTo) Describe() string {
	return fmt.Sprintf("moveTo %.2f %.2f %.2f", s.X, s.Y, s.Z)
}

func (s MoveTo) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	if err := agent.MoveTo(ctx, s.X, s.Y, s.Z, true); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// LineTo moves the agent in a straight line (flat worlds only).
type LineTo struct {
	IDValue string
	X, Y, Z float64
	Timeout time.Duration
}

func (s LineTo) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "lineTo"
}

func (s LineTo) Describe() string {
	return fmt.Sprintf("lineTo %.2f %.2f %.2f", s.X, s.Y, s.Z)
}

func (s LineTo) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	if err := agent.LineTo(ctx, s.X, s.Y, s.Z, false); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// Follow begins following a target player.
type Follow struct {
	IDValue   string
	Target    string
	Duration  time.Duration
	StopAfter bool
}

// MoveForward moves the agent forward by distance (based on current yaw).
type MoveForward struct {
	IDValue  string
	Distance float64
	Timeout  time.Duration
}

func (s MoveForward) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "moveForward"
}

func (s MoveForward) Describe() string {
	return fmt.Sprintf("moveForward %.2f", s.Distance)
}

func (s MoveForward) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	if err := agent.MoveForward(ctx, s.Distance); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// MoveUp moves the agent up/down by distance.
type MoveUp struct {
	IDValue  string
	Distance float64
	Timeout  time.Duration
}

func (s MoveUp) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "moveUp"
}

func (s MoveUp) Describe() string {
	return fmt.Sprintf("moveUp %.2f", s.Distance)
}

func (s MoveUp) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	if err := agent.MoveUp(ctx, s.Distance); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// FindPath computes a path without moving.
type FindPath struct {
	IDValue string
	X, Y, Z float64
	Timeout time.Duration
}

func (s FindPath) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "findPath"
}

func (s FindPath) Describe() string {
	return fmt.Sprintf("findPath %.2f %.2f %.2f", s.X, s.Y, s.Z)
}

func (s FindPath) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	if _, err := agent.FindPath(ctx, s.X, s.Y, s.Z); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// LookAt rotates the agent to face a target position.
type LookAt struct {
	IDValue string
	X, Y, Z float64
}

func (s LookAt) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "lookAt"
}

func (s LookAt) Describe() string {
	return fmt.Sprintf("lookAt %.2f %.2f %.2f", s.X, s.Y, s.Z)
}

func (s LookAt) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	if err := agent.LookAt(ctx, s.X, s.Y, s.Z); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// OpenContainer opens a block container at the specified position.
type OpenContainer struct {
	IDValue string
	X, Y, Z float64
	Face    models.BlockFace
	Timeout time.Duration
}

func (s OpenContainer) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "openContainer"
}

func (s OpenContainer) Describe() string {
	return fmt.Sprintf("openContainer %.2f %.2f %.2f", s.X, s.Y, s.Z)
}

func (s OpenContainer) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	windowID, err := agent.OpenContainerAt(ctx, s.X, s.Y, s.Z, s.Face, s.Timeout)
	if err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess, Details: fmt.Sprintf("window %d", windowID)}, nil
}

// OpenEntityContainer opens a container attached to an entity.
type OpenEntityContainer struct {
	IDValue  string
	EntityID int32
	Timeout  time.Duration
}

func (s OpenEntityContainer) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "openEntityContainer"
}

func (s OpenEntityContainer) Describe() string {
	return fmt.Sprintf("openEntityContainer %d", s.EntityID)
}

func (s OpenEntityContainer) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	_ = ctx
	windowID, err := agent.OpenEntityContainer(s.EntityID, s.Timeout)
	if err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess, Details: fmt.Sprintf("window %d", windowID)}, nil
}

// CloseContainer closes the current container.
type CloseContainer struct {
	IDValue string
}

func (s CloseContainer) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "closeContainer"
}

func (s CloseContainer) Describe() string {
	return "closeContainer"
}

func (s CloseContainer) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	_ = ctx
	if err := agent.CloseContainer(); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// TakeItemFromChest moves an item from chest to player inventory.
type TakeItemFromChest struct {
	IDValue   string
	WindowID  byte
	ChestSlot int16
}

func (s TakeItemFromChest) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "takeItemFromChest"
}

func (s TakeItemFromChest) Describe() string {
	return fmt.Sprintf("takeItemFromChest window=%d slot=%d", s.WindowID, s.ChestSlot)
}

func (s TakeItemFromChest) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	_ = ctx
	if err := agent.TakeItemFromChest(s.WindowID, s.ChestSlot); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// PutItemInChest moves an item from player inventory to chest.
type PutItemInChest struct {
	IDValue             string
	WindowID            byte
	PlayerInventorySlot int16
	ChestSlot           int16
}

func (s PutItemInChest) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "putItemInChest"
}

func (s PutItemInChest) Describe() string {
	return fmt.Sprintf("putItemInChest window=%d inv=%d chest=%d", s.WindowID, s.PlayerInventorySlot, s.ChestSlot)
}

func (s PutItemInChest) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	_ = ctx
	if err := agent.PutItemInChest(s.WindowID, s.PlayerInventorySlot, s.ChestSlot); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// UseItemOnBlock uses the held item on a block.
type UseItemOnBlock struct {
	IDValue string
	X, Y, Z float64
	Face    models.BlockFace
	Hand    models.Hand
	Timeout time.Duration
}

func (s UseItemOnBlock) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "useItemOnBlock"
}

func (s UseItemOnBlock) Describe() string {
	return fmt.Sprintf("useItemOnBlock %.2f %.2f %.2f", s.X, s.Y, s.Z)
}

func (s UseItemOnBlock) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	if err := agent.UseItemOnBlock(ctx, s.X, s.Y, s.Z, s.Face, s.Hand); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// UseItemOnEntity uses the held item on an entity.
type UseItemOnEntity struct {
	IDValue  string
	EntityID int32
	Hand     models.Hand
	Sneaking bool
	Timeout  time.Duration
}

func (s UseItemOnEntity) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "useItemOnEntity"
}

func (s UseItemOnEntity) Describe() string {
	return fmt.Sprintf("useItemOnEntity %d", s.EntityID)
}

func (s UseItemOnEntity) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	if err := agent.UseItemOnEntity(ctx, s.EntityID, s.Hand, s.Sneaking); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// WaitForChat waits for a chat message containing a substring.
type WaitForChat struct {
	IDValue  string
	Contains string
	Timeout  time.Duration
}

func (s WaitForChat) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "waitForChat"
}

func (s WaitForChat) Describe() string {
	if s.Contains == "" {
		return "waitForChat <any>"
	}
	return "waitForChat " + s.Contains
}

func (s WaitForChat) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	events := agent.ChatEvents(ctx)
	if events == nil {
		return StepResult{Status: StepFailed, Details: "chat events not available"}, errors.New("chat events not available")
	}
	for {
		select {
		case <-ctx.Done():
			return StepResult{Status: StepFailed, Details: ctx.Err().Error()}, ctx.Err()
		case msg, ok := <-events:
			if !ok {
				return StepResult{Status: StepFailed, Details: "chat events closed"}, errors.New("chat events closed")
			}
			if s.Contains == "" || containsInsensitive(msg, s.Contains) {
				return StepResult{Status: StepSuccess, Details: msg}, nil
			}
		}
	}
}

// FindEntity searches for a visible entity and optionally moves to it.
type FindEntity struct {
	IDValue      string
	EntityName   string
	EntityTypeID int32
	MaxDistance  float64
	MoveToTarget bool
	Timeout      time.Duration
}

func (s FindEntity) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "findEntity"
}

func (s FindEntity) Describe() string {
	if s.EntityName != "" {
		return "findEntity " + s.EntityName
	}
	return fmt.Sprintf("findEntity %d", s.EntityTypeID)
}

func (s FindEntity) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	entityTypeID := s.EntityTypeID
	if entityTypeID == 0 && s.EntityName != "" {
		if id, ok := agent.GetEntityTypeID(s.EntityName); ok {
			entityTypeID = id
		} else {
			return StepResult{Status: StepFailed, Details: "unknown entity type"}, errors.New("unknown entity type")
		}
	}
	if entityTypeID == 0 {
		return StepResult{Status: StepFailed, Details: "missing entity type"}, errors.New("missing entity type")
	}
	entityID, x, y, z, found, err := agent.FindVisibleEntity(ctx, entityTypeID, s.MaxDistance)
	if err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	if !found {
		return StepResult{Status: StepFailed, Details: "entity not found"}, errors.New("entity not found")
	}
	if s.MoveToTarget {
		if err := agent.MoveTo(ctx, x, y, z, false); err != nil {
			return StepResult{Status: StepFailed, Details: err.Error()}, err
		}
	}
	return StepResult{Status: StepSuccess, Details: fmt.Sprintf("entity %d", entityID)}, nil
}

// FindChest searches for a visible chest and optionally moves to it.
type FindChest struct {
	IDValue      string
	MaxDistance  int
	MoveToTarget bool
	Timeout      time.Duration
}

func (s FindChest) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "findChest"
}

func (s FindChest) Describe() string {
	return "findChest"
}

func (s FindChest) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	x, y, z, found, err := agent.FindVisibleBlock(ctx, "minecraft:chest", s.MaxDistance)
	if err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	if !found {
		return StepResult{Status: StepFailed, Details: "chest not found"}, errors.New("chest not found")
	}
	if s.MoveToTarget {
		// The chest itself is solid - MoveTo can never reach its own
		// coordinates (no walkable cell is ever within a pathfinder's goal
		// radius of a position you can't stand inside). Walk to a nearby
		// walkable, line-of-sight-verified position instead. See
		// docs/bugs/hpa-star-slowness for how this exact pattern turned a
		// 5-block walk into an A* search that ran to its step limit every
		// time.
		// ApproachBlock also skips walking if the bot can already reach and
		// see the block, tries every candidate spot (each with its own
		// timeout) before failing, and falls back to the block's own
		// coordinates only if no standable spot exists.
		target := models.V3{X: x, Y: y, Z: z}
		if err := models.ApproachBlock(ctx, agent, target, models.ApproachOptions{RequireSight: true}); err != nil {
			return StepResult{Status: StepFailed, Details: err.Error()}, err
		}
	}
	return StepResult{Status: StepSuccess, Details: fmt.Sprintf("chest %.0f %.0f %.0f", x, y, z)}, nil
}

// FindBlock searches for a visible block by name and optionally moves to it.
type FindBlock struct {
	IDValue      string
	BlockName    string
	MaxDistance  int
	MoveToTarget bool
	Timeout      time.Duration
}

func (s FindBlock) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "findBlock"
}

func (s FindBlock) Describe() string {
	if s.BlockName == "" {
		return "findBlock <missing>"
	}
	return "findBlock " + s.BlockName
}

func (s FindBlock) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	if s.BlockName == "" {
		return StepResult{Status: StepFailed, Details: "missing block name"}, errors.New("missing block name")
	}
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	x, y, z, found, err := agent.FindVisibleBlock(ctx, s.BlockName, s.MaxDistance)
	if err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	if !found {
		return StepResult{Status: StepFailed, Details: "block not found"}, errors.New("block not found")
	}
	if s.MoveToTarget {
		// See FindChest.Run above: a found block is generally solid, so
		// MoveTo must target a walkable position near it, not its own
		// coordinates.
		// ApproachBlock also skips walking if the bot can already reach and
		// see the block, tries every candidate spot (each with its own
		// timeout) before failing, and falls back to the block's own
		// coordinates only if no standable spot exists.
		target := models.V3{X: x, Y: y, Z: z}
		if err := models.ApproachBlock(ctx, agent, target, models.ApproachOptions{RequireSight: true}); err != nil {
			return StepResult{Status: StepFailed, Details: err.Error()}, err
		}
	}
	return StepResult{Status: StepSuccess, Details: fmt.Sprintf("block %.0f %.0f %.0f", x, y, z)}, nil
}

// Wander performs a random walk while avoiding recent positions.
type Wander struct {
	IDValue      string
	Steps        int
	StepDistance float64
	AvoidRadius  float64
	RecentMemory int
	Timeout      time.Duration
}

func (s Wander) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "wander"
}

func (s Wander) Describe() string {
	return fmt.Sprintf("wander steps=%d dist=%.2f", s.Steps, s.StepDistance)
}

func (s Wander) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	steps := s.Steps
	if steps <= 0 {
		steps = 1
	}
	stepDistance := s.StepDistance
	if stepDistance <= 0 {
		stepDistance = 3
	}
	avoidRadius := s.AvoidRadius
	if avoidRadius <= 0 {
		avoidRadius = 1.5
	}
	memory := s.RecentMemory
	if memory <= 0 {
		memory = 5
	}

	recent := make([][3]float64, 0, memory)
	for i := 0; i < steps; i++ {
		if ctx.Err() != nil {
			return StepResult{Status: StepFailed, Details: ctx.Err().Error()}, ctx.Err()
		}
		pos, _, _, ok := agent.GetPosition()
		if !ok {
			return StepResult{Status: StepFailed, Details: "position not initialized"}, errors.New("position not initialized")
		}
		tx, ty, tz := pickWanderTarget(pos, stepDistance, recent, avoidRadius)
		if err := agent.MoveTo(ctx, tx, ty, tz, false); err != nil {
			return StepResult{Status: StepFailed, Details: err.Error()}, err
		}
		recent = append(recent, [3]float64{tx, ty, tz})
		if len(recent) > memory {
			recent = recent[len(recent)-memory:]
		}
	}
	return StepResult{Status: StepSuccess}, nil
}

func pickWanderTarget(pos models.V3, dist float64, recent [][3]float64, avoidRadius float64) (float64, float64, float64) {
	for i := 0; i < 10; i++ {
		angle := randFloat64(0, 2*math.Pi)
		tx := pos.X + math.Cos(angle)*dist
		tz := pos.Z + math.Sin(angle)*dist
		ty := pos.Y
		if !isRecent(tx, ty, tz, recent, avoidRadius) {
			return tx, ty, tz
		}
	}
	return pos.X + dist, pos.Y, pos.Z
}

func isRecent(x, y, z float64, recent [][3]float64, avoidRadius float64) bool {
	for _, pos := range recent {
		if distance3D(x, y, z, pos[0], pos[1], pos[2]) <= avoidRadius {
			return true
		}
	}
	return false
}

func randFloat64(min, max float64) float64 {
	return min + (max-min)*randSource().Float64()
}

func containsInsensitive(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

var (
	randOnce sync.Once
	randSrc  *rand.Rand
)

func randSource() *rand.Rand {
	randOnce.Do(func() {
		randSrc = rand.New(rand.NewSource(time.Now().UnixNano()))
	})
	return randSrc
}
func (s Follow) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "follow"
}

func (s Follow) Describe() string {
	if s.Target == "" {
		return "follow <missing target>"
	}
	if s.Duration > 0 {
		return fmt.Sprintf("follow %s for %s", s.Target, s.Duration)
	}
	return "follow " + s.Target
}

func (s Follow) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	if s.Target == "" {
		return StepResult{Status: StepFailed, Details: "missing target"}, errors.New("missing follow target")
	}
	if err := agent.Follow(ctx, s.Target); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	if s.Duration > 0 {
		if err := sleepWithContext(ctx, s.Duration); err != nil {
			return StepResult{Status: StepFailed, Details: err.Error()}, err
		}
		if s.StopAfter {
			if err := agent.StopFollow(ctx); err != nil {
				return StepResult{Status: StepFailed, Details: err.Error()}, err
			}
		}
	}
	return StepResult{Status: StepSuccess}, nil
}

// StopFollow stops any active follow behavior.
type StopFollow struct {
	IDValue string
}

func (s StopFollow) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "stopFollow"
}

func (s StopFollow) Describe() string {
	return "stopFollow"
}

func (s StopFollow) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	if err := agent.StopFollow(ctx); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// Wait pauses the plan for a fixed duration.
type Wait struct {
	IDValue  string
	Duration time.Duration
}

func (s Wait) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "wait"
}

func (s Wait) Describe() string {
	return fmt.Sprintf("wait %s", s.Duration)
}

func (s Wait) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	if s.Duration <= 0 {
		return StepResult{Status: StepSkipped, Details: "zero duration"}, nil
	}
	if err := sleepWithContext(ctx, s.Duration); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// Say sends a chat message.
type Say struct {
	IDValue string
	Message string
}

func (s Say) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "say"
}

func (s Say) Describe() string {
	if s.Message == "" {
		return "say <empty>"
	}
	return "say " + s.Message
}

func (s Say) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	if s.Message == "" {
		return StepResult{Status: StepSkipped, Details: "empty message"}, nil
	}
	if err := agent.SendChat(s.Message); err != nil {
		return StepResult{Status: StepFailed, Details: err.Error()}, err
	}
	return StepResult{Status: StepSuccess}, nil
}

// ExpectPosition waits until the agent is within tolerance of a target position.
type ExpectPosition struct {
	IDValue      string
	X, Y, Z      float64
	Tolerance    float64
	Timeout      time.Duration
	PollInterval time.Duration
}

func (s ExpectPosition) ID() string {
	if s.IDValue != "" {
		return s.IDValue
	}
	return "expectPosition"
}

func (s ExpectPosition) Describe() string {
	return fmt.Sprintf("expectPosition %.2f %.2f %.2f", s.X, s.Y, s.Z)
}

func (s ExpectPosition) Run(ctx context.Context, agent models.Agent) (StepResult, error) {
	ctx, cancel := withTimeout(ctx, s.Timeout)
	defer cancel()
	tolerance := s.Tolerance
	if tolerance <= 0 {
		tolerance = 0.5
	}
	interval := s.PollInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	for {
		select {
		case <-ctx.Done():
			return StepResult{Status: StepFailed, Details: ctx.Err().Error()}, ctx.Err()
		default:
		}
		pos, _, _, ok := agent.GetPosition()
		if ok {
			if distance3D(pos.X, pos.Y, pos.Z, s.X, s.Y, s.Z) <= tolerance {
				return StepResult{Status: StepSuccess}, nil
			}
		}
		if err := sleepWithContext(ctx, interval); err != nil {
			return StepResult{Status: StepFailed, Details: err.Error()}, err
		}
	}
}

func distance3D(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	dz := z1 - z2
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	return tctx, cancel
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
