package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/reallyoldfogie/mc-agent/following"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/stretchr/testify/require"
)

type quickFollowMgr struct {
	started string
	active  bool
	stopped bool
	state   following.FollowState
}

func (q *quickFollowMgr) Start(name string) error    { q.started = name; q.active = true; return nil }
func (q *quickFollowMgr) Stop() error                { q.stopped = true; q.active = false; return nil }
func (q *quickFollowMgr) IsActive() bool             { return q.active }
func (q *quickFollowMgr) GetStatus() string          { return "ok" }
func (q *quickFollowMgr) GetPath() *pathfinding.Path { return nil }
func (q *quickFollowMgr) GetState() following.FollowState {
	return q.state
}
func (q *quickFollowMgr) SetState(s following.FollowState) {
	q.state = s
}

func TestFollowQuick_StartAndStopMessages(t *testing.T) {
	agentInt, err := New(Config{Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	fc := &fakeChat{}
	agent.SetChat(fc)
	fm := &quickFollowMgr{}
	agent.SetFollowManager(fm)

	agent.handleChatCommand("follow Alex")
	if fm.started != "Alex" {
		t.Fatalf("expected Start called with Alex")
	}
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "Following Alex") {
		t.Fatalf("expected 'Following Alex' message, got %#v", fc.msgs)
	}

	// stop when active
	agent.handleChatCommand("stopFollow")
	if !fm.stopped {
		t.Fatalf("expected Stop called")
	}
	if len(fc.msgs) == 0 || fc.msgs[len(fc.msgs)-1] != "Stopped following" {
		t.Fatalf("expected 'Stopped following', got %#v", fc.msgs)
	}

	// stop when inactive
	before := len(fc.msgs)
	agent.handleChatCommand("stopFollow")
	if len(fc.msgs) == before || fc.msgs[len(fc.msgs)-1] != "Not currently following anyone" {
		t.Fatalf("expected 'Not currently following anyone', got %#v", fc.msgs)
	}
}
