package agent

import (
	"context"
	"strings"
	"testing"
)

type quickFollowMgr struct {
	started string
	active  bool
	stopped bool
}

func (q *quickFollowMgr) Start(name string) error { q.started = name; q.active = true; return nil }
func (q *quickFollowMgr) Stop() error             { q.stopped = true; q.active = false; return nil }
func (q *quickFollowMgr) IsActive() bool          { return q.active }
func (q *quickFollowMgr) GetStatus() string       { return "ok" }

func TestFollowQuick_StartAndStopMessages(t *testing.T) {
	a, _ := New(Config{Address: "x"})
	_ = a.Init(context.Background())
	fc := &fakeChat{}
	a.SetChat(fc)
	fm := &quickFollowMgr{}
	a.SetFollowManager(fm)

	a.handleChatCommand("follow Alex")
	if fm.started != "Alex" {
		t.Fatalf("expected Start called with Alex")
	}
	if len(fc.msgs) == 0 || !strings.Contains(fc.msgs[len(fc.msgs)-1], "Following Alex") {
		t.Fatalf("expected 'Following Alex' message, got %#v", fc.msgs)
	}

	// stop when active
	a.handleChatCommand("stopFollow")
	if !fm.stopped {
		t.Fatalf("expected Stop called")
	}
	if len(fc.msgs) == 0 || fc.msgs[len(fc.msgs)-1] != "Stopped following" {
		t.Fatalf("expected 'Stopped following', got %#v", fc.msgs)
	}

	// stop when inactive
	before := len(fc.msgs)
	a.handleChatCommand("stopFollow")
	if len(fc.msgs) == before || fc.msgs[len(fc.msgs)-1] != "Not currently following anyone" {
		t.Fatalf("expected 'Not currently following anyone', got %#v", fc.msgs)
	}
}
