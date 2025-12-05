package agent

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Tnze/go-mc/chat"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-bot-go/bot/playerlist"
	"strings"
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
func (f *fakeFollowMgr) Stop() error       { f.stopCalled = true; f.active = false; return nil }
func (f *fakeFollowMgr) IsActive() bool    { return f.active }
func (f *fakeFollowMgr) GetStatus() string { return f.status }

// Ensure follow <name> works, stopFollow respects active/inactive, and followStatus returns string
func TestFollowCommands(t *testing.T) {
	a, _ := New(Config{Address: "127.0.0.1:25565"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	ff := &fakeFollowMgr{status: "OK"}
	a.SetFollowManager(ff)

	a.handleChatCommand("follow Steve")
	if ff.startedWith != "Steve" {
		t.Fatalf("expected Start called with Steve")
	}
	a.handleChatCommand("followStatus")
	if len(fc.msgs) == 0 || fc.msgs[len(fc.msgs)-1] != "OK" {
		t.Fatalf("expected status OK, got %#v", fc.msgs)
	}
	a.handleChatCommand("stopFollow")
	if !ff.stopCalled {
		t.Fatalf("expected Stop called")
	}
	// inactive stop
	before := len(fc.msgs)
	a.handleChatCommand("stopFollow")
	if len(fc.msgs) == before || fc.msgs[len(fc.msgs)-1] == "Stopped following" {
		t.Fatalf("expected not currently following message")
	}
}

// moveTo invalid args and already at target
func TestMoveTo_InvalidAndAlreadyThere(t *testing.T) {
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.handleChatCommand("moveTo a 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(fc.msgs, "Invalid X coordinate") {
		t.Fatalf("expected invalid X message")
	}
	a.UpdatePosition(0, 0, 0, 0, 0)
	a.handleChatCommand("moveTo 0 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(fc.msgs, "Already at target position") {
		t.Fatalf("expected already at target message")
	}
}

// moveForward negative with yaw 0 should move -Z
func TestMoveForward_NegativeYaw0(t *testing.T) {
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.UpdatePosition(0, 0, 0, 0, 0)
	a.handleChatCommand("moveForward -0.2")
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
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.UpdatePosition(0, 0, 0, 90, 0)
	a.handleChatCommand("moveForward 0.2")
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
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.UpdatePosition(0, 1, 0, 0, 0)
	a.handleChatCommand("moveUp -0.2")
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
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	a.UpdatePosition(0, 0, 0, 0, 0)
	a.handleChatCommand("findPath a 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(fc.msgs, "Invalid X coordinate") {
		t.Fatalf("expected invalid")
	}
	a.SetPathFinder(fakePFFail{})
	a.handleChatCommand("findPath 1 0 0")
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(fc.msgs, "Path find failed") {
		t.Fatalf("expected pf error")
	}
}

// startTracking double-start and stopTracking not active
func TestTracking_DoubleStart_And_StopNotActive(t *testing.T) {
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	// Seed one player entity and name resolver
	a.entities = map[int32]*trackedEntity{1: {EntityID: 1, UUID: [16]byte{1}, X: 1, Y: 0, Z: 0}}
	a.SetPlayerNameResolver(func(u [16]byte) (string, bool) {
		if u == ([16]byte{1}) {
			return "Steve", true
		}
		return "", false
	})
	// Speed up timers
	trackingTickDur = 10 * time.Millisecond
	trackingStatsDur = 20 * time.Millisecond
	trackingNoPlayersInterval = 20 * time.Millisecond
	a.handleChatCommand("startTracking")
	time.Sleep(50 * time.Millisecond)
	a.handleChatCommand("startTracking")
	if !containsMsg(fc.msgs, "Tracking is already active!") {
		t.Fatalf("expected already active")
	}
	a.handleChatCommand("stopTracking") // now not active
	before := len(fc.msgs)
	a.handleChatCommand("stopTracking")
	if len(fc.msgs) == before || fc.msgs[len(fc.msgs)-1] == "Tracking stopped" {
		t.Fatalf("expected Not tracking message")
	}
}

// OnPlayerChat routing
func TestOnPlayerChat_Routing(t *testing.T) {
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	a.client = &fakeClientWriter{}
	// command: pos should reply with not initialized
	msg := chat.Message{With: []chat.Message{{Text: ">>>BOT<<< pos"}}}
	var pi playerlist.PlayerInfo
	_ = a.OnPlayerChat(pi, msg, true)
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
