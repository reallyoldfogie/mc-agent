package agent

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"strings"

	"github.com/Tnze/go-mc/chat"
	"github.com/reallyoldfogie/mc-agent/following"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
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
func (f *fakeFollowMgr) Stop() error                     { f.stopCalled = true; f.active = false; return nil }
func (f *fakeFollowMgr) IsActive() bool                  { return f.active }
func (f *fakeFollowMgr) GetStatus() string               { return f.status }
func (f *fakeFollowMgr) GetPath() *pathfinding.Path      { return nil }
func (f *fakeFollowMgr) GetState() following.FollowState { return following.StateIdle }

// Ensure follow <name> works, stopFollow respects active/inactive, and followStatus returns string
func TestFollowCommands(t *testing.T) {
	agentInt, err := New(Config{Address: "127.0.0.1:25565"})
	require.NoError(t, err)

	agent := agentInt.(*agent)
	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	ff := &fakeFollowMgr{status: "OK"}
	agent.SetFollowManager(ff)

	agent.handleChatCommand("follow Steve")
	if ff.startedWith != "Steve" {
		t.Fatalf("expected Start called with Steve")
	}
	agent.handleChatCommand("followStatus")
	if len(fc.msgs) == 0 || fc.msgs[len(fc.msgs)-1] != "OK" {
		t.Fatalf("expected status OK, got %#v", fc.msgs)
	}
	agent.handleChatCommand("stopFollow")
	if !ff.stopCalled {
		t.Fatalf("expected Stop called")
	}
	// inactive stop
	before := len(fc.msgs)
	agent.handleChatCommand("stopFollow")
	if len(fc.msgs) == before || fc.msgs[len(fc.msgs)-1] == "Stopped following" {
		t.Fatalf("expected not currently following message")
	}
}

// moveTo invalid args and already at target
func TestMoveTo_InvalidAndAlreadyThere(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)

	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	// Need pathfinder for moveTo command
	agent.SetPathFinder(&fakePF{})
	agent.handleChatCommand("moveTo agent 0 0")

	time.Sleep(10 * time.Millisecond)
	if !containsMsg(fc.msgs, "Invalid X coordinate") {
		t.Fatalf("expected invalid X message")
	}

	agent.UpdatePosition(0, 0, 0, 0, 0)
	agent.handleChatCommand("moveTo 0 0 0")
	time.Sleep(10 * time.Millisecond)

	if !containsMsg(fc.msgs, "Already at target position") {
		t.Fatalf("expected already at target message")
	}
}

// moveForward negative with yaw 0 should move -Z
func TestMoveForward_NegativeYaw0(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	agent.UpdatePosition(0, 0, 0, 0, 0)
	agent.handleChatCommand("moveForward -0.2")
	time.Sleep(70 * time.Millisecond)
	if len(fm.posCalls) == 0 {
		t.Fatalf("no pos calls")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	if math.Abs(got[2]-(-0.2)) > 1e-6 {
		t.Fatalf("expected z -0.2, got %#v", got)
	}
}

// moveForward with yaw 90 should move -X
func TestMoveForward_Yaw90(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fm := &fakeMoveExec{}
	agent.SetMovementExecutor(fm)
	agent.UpdatePosition(0, 0, 0, 90, 0)
	agent.handleChatCommand("moveForward 0.2")
	time.Sleep(70 * time.Millisecond)
	if len(fm.posCalls) == 0 {
		t.Fatalf("no pos calls")
	}
	got := fm.posCalls[len(fm.posCalls)-1]
	if math.Abs(got[0]-(-0.2)) > 1e-6 {
		t.Fatalf("expected x -0.2, got %#v", got)
	}
}

// moveUp negative
func TestMoveUp_Negative(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
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

func (fakePFFail) FindPath(_, _ pathfinding.V3, _ int) (*pathfinding.Path, error) {
	return nil, errors.New("pf error")
}

func TestFindPath_InvalidAndError(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	agent.UpdatePosition(0, 0, 0, 0, 0)
	agent.handleChatCommand("findPath agent 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(fc.msgs, "Invalid X coordinate") {
		t.Fatalf("expected invalid")
	}
	agent.SetPathFinder(fakePFFail{})
	agent.handleChatCommand("findPath 1 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(fc.msgs, "Path find failed") {
		t.Fatalf("expected pf error")
	}
}

// startTracking double-start and stopTracking not active
func TestTracking_DoubleStart_And_StopNotActive(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
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
	if !containsMsg(fc.msgs, "Tracking is already active!") {
		t.Fatalf("expected already active")
	}
	agent.handleChatCommand("stopTracking") // now not active
	before := len(fc.msgs)
	agent.handleChatCommand("stopTracking")
	if len(fc.msgs) == before || fc.msgs[len(fc.msgs)-1] == "Tracking stopped" {
		t.Fatalf("expected Not tracking message")
	}
}

// OnPlayerChat routing
func TestOnPlayerChat_Routing(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	agent.client = &fakeClientWriter{}
	// command: pos should reply with not initialized
	msg := chat.Message{With: []chat.Message{{Text: ">>>BOT<<< pos"}}}
	var pi playerlist.PlayerInfo
	_ = agent.OnPlayerChat(pi, msg, true)
	if !containsMsg(fc.msgs, "not initialized") {
		t.Fatalf("expected pos error via chat routing; got %#v", fc.msgs)
	}
}

func containsMsg(msgs []string, sub string) bool {
	sub = strings.ToLower(sub)
	for _, m := range msgs {
		if strings.Contains(strings.ToLower(m), sub) {
			return true
		}
	}
	return false
}
