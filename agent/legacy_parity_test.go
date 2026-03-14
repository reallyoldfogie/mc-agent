package agent

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"github.com/stretchr/testify/require"
)

// fake follow manager
type fakeFollowMgr struct {
	active      bool
	startedWith string
	stopCalled  bool
	status      string
	startErr    error
}

func (f *fakeFollowMgr) Start(name string) error {
	f.startedWith = name
	f.active = (f.startErr == nil)
	return f.startErr
}
func (f *fakeFollowMgr) Stop() error                  { f.stopCalled = true; f.active = false; return nil }
func (f *fakeFollowMgr) IsActive() bool               { return f.active }
func (f *fakeFollowMgr) GetStatus() string            { return f.status }
func (f *fakeFollowMgr) GetPath() *models.Path        { return nil }
func (f *fakeFollowMgr) GetState() models.FollowState { return models.StateIdle }

// Ensure follow <name> works, stopFollow respects active/inactive, and followStatus returns string
func TestFollowCommands(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)
	err = agent.Init(context.Background())
	require.NoError(t, err)

	capture := newCaptureChat()
	agent.SetChat(capture)
	ff := &fakeFollowMgr{status: "OK"}
	agent.SetFollowManager(ff)

	agent.handleChatCommand("follow Steve")
	if ff.startedWith != "Steve" {
		t.Fatalf("expected Start called with Steve")
	}
	agent.handleChatCommand("followStatus")
	if capture.GetLastMessage() != "OK" {
		t.Fatalf("expected status OK, got %#v", capture.GetMessages())
	}
	agent.handleChatCommand("stopFollow")
	if !ff.stopCalled {
		t.Fatalf("expected Stop called")
	}
	// inactive stop
	before := len(capture.GetMessages())
	agent.handleChatCommand("stopFollow")
	if len(capture.GetMessages()) == before || capture.GetLastMessage() == "Stopped following" {
		t.Fatalf("expected not currently following message")
	}
}

// moveTo invalid args and already at target
func TestMoveTo_InvalidAndAlreadyThere(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	capture := newCaptureChat()
	agent.SetChat(capture)

	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	// Need pathfinder for moveTo command
	agent.SetPathFinder(&fakePF{})
	agent.handleChatCommand("moveTo agent 0 0")

	time.Sleep(10 * time.Millisecond)
	if !containsMsg(capture.GetMessages(), "Invalid X coordinate") {
		t.Fatalf("expected invalid X message")
	}

	agent.UpdatePosition(0, 0, 0, 0, 0)
	agent.handleChatCommand("moveTo 0 0 0")
	time.Sleep(10 * time.Millisecond)

	if !containsMsg(capture.GetMessages(), "Already at target position") {
		t.Fatalf("expected already at target message")
	}
}

// moveForward negative with yaw 0 should move -Z
func TestMoveForward_NegativeYaw0(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fm := &fakeMoveExec{agent: agent}
	agent.SetMovementExecutor(fm)
	agent.UpdatePosition(0, 0, 0, 0, 0)
	agent.handleChatCommand("moveForward -0.2")
	time.Sleep(70 * time.Millisecond)
	if len(fm.posCalls) == 0 {
		t.Fatalf("no pos calls")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	// With physics-based movement, expect backward movement (negative z) in the -0.3 to 0 range
	// (one tick of ~0.215 blocks/tick movement in negative direction)
	if got[2] >= 0 || got[2] < -0.3 {
		t.Fatalf("expected backward movement z in (-0.3, 0), got %#v", got)
	}
}

// moveForward with yaw 90 should move -X
func TestMoveForward_Yaw90(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fm := &fakeMoveExec{agent: agent}
	agent.SetMovementExecutor(fm)
	agent.UpdatePosition(0, 0, 0, 90, 0)
	agent.handleChatCommand("moveForward 0.2")
	time.Sleep(70 * time.Millisecond)
	if len(fm.posCalls) == 0 {
		t.Fatalf("no pos calls")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	// With physics-based movement, expect leftward movement (negative x) in the -0.3 to 0 range
	// (one tick of ~0.215 blocks/tick movement at yaw=90)
	if got[0] >= 0 || got[0] < -0.3 {
		t.Fatalf("expected leftward movement x in (-0.3, 0), got %#v", got)
	}
}

// moveUp negative
func TestMoveUp_Negative(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	agent.UpdatePosition(0, 1, 0, 0, 0)
	agent.handleChatCommand("moveUp -0.2")
	time.Sleep(70 * time.Millisecond)
	if len(fm.posCalls) == 0 {
		t.Fatalf("no pos calls")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	if math.Abs(got[1]-(1-0.2)) > 1e-6 {
		t.Fatalf("expected y 0.8, got %#v", got)
	}
}

// findPath invalid args and error propagation
type fakePFFail struct{}

func (fakePFFail) FindPath(ctx context.Context, _, _ models.V3, _ int) (*models.Path, error) {
	return nil, errors.New("pf error")
}
func (fakePFFail) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	return startY
}

func TestFindPath_InvalidAndError(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	capture := newCaptureChat()
	agent.SetChat(capture)
	agent.UpdatePosition(0, 0, 0, 0, 0)
	agent.handleChatCommand("findPath agent 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(capture.GetMessages(), "Invalid X coordinate") {
		t.Fatalf("expected invalid")
	}
	agent.SetPathFinder(fakePFFail{})
	agent.handleChatCommand("findPath 1 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(capture.GetMessages(), "Path find failed") {
		t.Fatalf("expected pf error")
	}
}

// startTracking double-start and stopTracking not active
func TestTracking_DoubleStart_And_StopNotActive(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	capture := newCaptureChat()
	agent.SetChat(capture)
	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	// Seed one player entity and name resolver
	agent.entities = map[int32]*trackedEntity{1: {EntityID: 1, UUID: [16]byte{1}, X: 1, Y: 0, Z: 0}}
	agent.SetPlayerNameResolver(func(u [16]byte) (string, bool) {
		if u == ([16]byte{1}) {
			return "Steve", true
		}
		return "", false
	})
	// Speed up timers
	trackingTickDur = 10 * time.Millisecond
	trackingStatsDur = 20 * time.Millisecond
	trackingNoPlayersInterval = 20 * time.Millisecond
	agent.handleChatCommand("startTracking")
	time.Sleep(50 * time.Millisecond)
	agent.handleChatCommand("startTracking")
	if !containsMsg(capture.GetMessages(), "Tracking is already active!") {
		t.Fatalf("expected already active")
	}
	agent.handleChatCommand("stopTracking") // now not active
	before := len(capture.GetMessages())
	agent.handleChatCommand("stopTracking")
	if len(capture.GetMessages()) == before || capture.GetLastMessage() == "Tracking stopped" {
		t.Fatalf("expected Not tracking message")
	}
}

// OnPlayerChat routing
func TestOnPlayerChat_Routing(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	capture := newCaptureChat()
	agent.SetChat(capture)
	agent.client = &fakeClientWriter{}
	// command: pos should reply with not initialized
	msg := chat.Message{With: []chat.Message{{Text: ">>>BOT<<< pos"}}}
	var pi playerlist.PlayerInfo
	_ = agent.OnPlayerChat(pi, msg, true)
	if !containsMsg(capture.GetMessages(), "not initialized") {
		t.Fatalf("expected pos error via chat routing; got %#v", capture.GetMessages())
	}
}
