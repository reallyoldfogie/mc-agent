package agent

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

type quickFollowMgr struct {
	started string
	active  bool
	stopped bool
	state   models.FollowState
}

func (q *quickFollowMgr) Start(name string) error { q.started = name; q.active = true; return nil }
func (q *quickFollowMgr) Stop() error             { q.stopped = true; q.active = false; return nil }
func (q *quickFollowMgr) IsActive() bool          { return q.active }
func (q *quickFollowMgr) GetStatus() string       { return "ok" }
func (q *quickFollowMgr) GetPath() *models.Path   { return nil }
func (q *quickFollowMgr) GetState() models.FollowState {
	return q.state
}
func (q *quickFollowMgr) SetState(s models.FollowState) {
	q.state = s
}

func TestFollowQuick_StartAndStopMessages(t *testing.T) {
	agentInt, err := New(Config{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)

	agent := agentInt.(*agent)

	err = agent.Init(context.Background())
	require.NoError(t, err)

	capture := newCaptureChat()
	agent.SetChat(capture)
	fm := &quickFollowMgr{}
	agent.SetFollowManager(fm)

	agent.handleChatCommand("follow Alex")
	if fm.started != "Alex" {
		t.Fatalf("expected Start called with Alex")
	}
	if !capture.ContainsMessage("Following Alex") {
		t.Fatalf("expected 'Following Alex' message, got %#v", capture.GetMessages())
	}

	// stop when active
	agent.handleChatCommand("stopFollow")
	if !fm.stopped {
		t.Fatalf("expected Stop called")
	}
	if capture.GetLastMessage() != "Stopped following" {
		t.Fatalf("expected 'Stopped following', got %#v", capture.GetMessages())
	}

	// stop when inactive
	before := len(capture.GetMessages())
	agent.handleChatCommand("stopFollow")
	if len(capture.GetMessages()) == before || capture.GetLastMessage() != "Not currently following anyone" {
		t.Fatalf("expected 'Not currently following anyone', got %#v", capture.GetMessages())
	}
}
