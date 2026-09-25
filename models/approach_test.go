package models_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	mctesting "github.com/reallyoldfogie/mc-agent/testing"
)

// fakeApproachAgent extends fakeInteractAgent (interact_position_test.go)
// with a position and a scripted MoveTo.
type fakeApproachAgent struct {
	fakeInteractAgent
	pos     models.V3
	moveTo  func(ctx context.Context, x, y, z float64) error
	moves   []models.V3
	notifys []bool
}

func (f *fakeApproachAgent) GetPositionSimple() (models.V3, bool) { return f.pos, true }
func (f *fakeApproachAgent) MoveTo(ctx context.Context, x, y, z float64, notify bool) error {
	f.moves = append(f.moves, models.V3{X: x, Y: y, Z: z})
	f.notifys = append(f.notifys, notify)
	if f.moveTo != nil {
		return f.moveTo(ctx, x, y, z)
	}
	return nil
}

func newApproachAgent(t *testing.T, pos models.V3) *fakeApproachAgent {
	t.Helper()
	registry := mctesting.NewSimpleBlockRegistry()
	world := mctesting.NewWorldBuilder(registry).
		FlatGroundDirect(-20, -20, 20, 20, -1, grassStateID).
		SetBlockDirect(0, 0, 0, grassStateID). // the target block
		Build()
	return &fakeApproachAgent{
		fakeInteractAgent: fakeInteractAgent{world: world, shapeMgr: mctesting.NewMockShapeManager()},
		pos:               pos,
	}
}

// The bot found live wedged 1.4 blocks from its table (feet (44014.99,
// -59.0, 24.02), table at (44016, -60, 24)) was well within reach; a table
// across the room is not.
func TestWithinInteractReach(t *testing.T) {
	stuck := models.V3{X: 44014.99, Y: -59.0, Z: 24.02}
	table := models.V3{X: 44016, Y: -60, Z: 24}
	if !models.WithinInteractReach(stuck, 1.62, table) {
		t.Error("a table ~2.5 blocks from the eyes must be within reach")
	}
	if models.WithinInteractReach(models.V3{X: 44010, Y: -59, Z: 24}, 1.62, table) {
		t.Error("a table 6 blocks away must not be within reach")
	}
}

func TestApproachBlock_NoOpWhenAlreadyInReach(t *testing.T) {
	agent := newApproachAgent(t, models.V3{X: 2, Y: 0, Z: 1})
	if err := models.ApproachBlock(context.Background(), agent, models.V3{}, models.ApproachOptions{RequireSight: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agent.moves) != 0 {
		t.Fatalf("already in reach, but the bot walked: %v", agent.moves)
	}
}

func TestApproachBlock_SightRequiredButBlockedStillWalks(t *testing.T) {
	agent := newApproachAgent(t, models.V3{X: 2, Y: 0, Z: 1})
	agent.blockedOrigins = map[models.V3]bool{agent.pos: true}
	if err := models.ApproachBlock(context.Background(), agent, models.V3{}, models.ApproachOptions{RequireSight: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agent.moves) == 0 {
		t.Fatal("in reach but no line of sight: with RequireSight the bot must walk to a spot that has it")
	}
	if err := models.ApproachBlock(context.Background(), agent, models.V3{}, models.ApproachOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestApproachBlock_WalksWhenOutOfReach(t *testing.T) {
	agent := newApproachAgent(t, models.V3{X: 12, Y: 0, Z: 0})
	if err := models.ApproachBlock(context.Background(), agent, models.V3{}, models.ApproachOptions{Announce: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agent.moves) != 1 {
		t.Fatalf("expected one walk, got %v", agent.moves)
	}
	if d := agent.moves[0].DistanceTo(models.V3{}); d > 1.5 {
		t.Errorf("walked to %+v (distance %.2f from the target): want the closest spot", agent.moves[0], d)
	}
	if !agent.notifys[0] {
		t.Error("the first walk must be announced when Announce is set")
	}
}

// A wedged approach never returns by itself; each candidate's own timeout
// must let the next one be tried, quietly.
func TestApproachBlock_MovesOnFromAWedgedCandidate(t *testing.T) {
	old := models.ApproachAttemptTimeout
	models.ApproachAttemptTimeout = 30 * time.Millisecond
	defer func() { models.ApproachAttemptTimeout = old }()

	agent := newApproachAgent(t, models.V3{X: 12, Y: 0, Z: 0})
	agent.moveTo = func(ctx context.Context, _, _, _ float64) error {
		if len(agent.moves) == 1 { // the first candidate hangs until its timeout
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	start := time.Now()
	if err := models.ApproachBlock(context.Background(), agent, models.V3{}, models.ApproachOptions{Announce: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(agent.moves) != 2 || agent.moves[0] == agent.moves[1] {
		t.Fatalf("expected a second, different candidate after the first wedged: %v", agent.moves)
	}
	if agent.notifys[1] {
		t.Error("fallback attempts must not announce in chat")
	}
	if time.Since(start) > time.Second {
		t.Errorf("took %v: the wedged candidate should have been cut off at its own timeout", time.Since(start))
	}
}

func TestApproachBlock_FailsOnlyAfterEveryCandidateFailed(t *testing.T) {
	agent := newApproachAgent(t, models.V3{X: 12, Y: 0, Z: 0})
	sentinel := errors.New("no path")
	agent.moveTo = func(context.Context, float64, float64, float64) error { return sentinel }
	err := models.ApproachBlock(context.Background(), agent, models.V3{}, models.ApproachOptions{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the last MoveTo error, got %v", err)
	}
	if len(agent.moves) < 5 {
		t.Errorf("expected every qualifying spot to be tried, only %d were", len(agent.moves))
	}
}
