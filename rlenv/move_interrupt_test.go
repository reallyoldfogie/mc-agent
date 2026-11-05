package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// fakeMoveInterruptAgent wraps fakeAgent to additionally satisfy
// rlenv.MoveInterruptSetter, recording every call so a test can confirm
// updateMoveInterruptCheck's own dispatch-site logic: install a non-nil
// check only for a goto dispatch with a mine target configured, clear it
// (nil) for everything else.
type fakeMoveInterruptAgent struct {
	*fakeAgent
	setCalls  int
	lastCheck func() bool
}

func (f *fakeMoveInterruptAgent) SetMoveInterruptCheck(check func() bool) {
	f.setCalls++
	f.lastCheck = check
}

// TestMoveInterruptCheckInstalledForBuriedGoto confirms a goto dispatch
// while a mine target is configured installs a non-nil check, and that
// the check itself reflects FindVisibleBlock's own live result - not just
// "something non-nil got installed."
func TestMoveInterruptCheckInstalledForBuriedGoto(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlock("minecraft:coal_ore", 5, 0, 0)
	inner.setMineBlockHidden(true) // not visible yet
	agent := &fakeMoveInterruptAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.setCalls != 1 {
		t.Fatalf("SetMoveInterruptCheck called %d times, want 1", agent.setCalls)
	}
	if agent.lastCheck == nil {
		t.Fatal("installed check = nil, want non-nil for a goto dispatch with a mine target configured")
	}
	if got := agent.lastCheck(); got {
		t.Fatal("installed check = true while the target is still hidden, want false")
	}

	inner.setMineBlockHidden(false) // now exposed, mid-"path" in the real scenario
	if got := agent.lastCheck(); !got {
		t.Fatal("installed check = false once the target became visible, want true")
	}
}

// TestMoveInterruptCheckClearedWithoutMineTarget confirms an ordinary
// (non-mining) goto task never gets a check installed at all - there is
// nothing for it to usefully interrupt for.
func TestMoveInterruptCheckClearedWithoutMineTarget(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	agent := &fakeMoveInterruptAgent{fakeAgent: inner}
	cfg := testConfig() // no MineTargetBlock
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.setCalls != 1 {
		t.Fatalf("SetMoveInterruptCheck called %d times, want 1", agent.setCalls)
	}
	if agent.lastCheck != nil {
		t.Fatal("installed check != nil for a goto task with no mine target configured, want nil")
	}
}

// TestMoveInterruptCheckClearedOnNonGotoDispatch confirms a check
// installed by an earlier goto dispatch is cleared the moment a different
// kind of action (here, ActionMine) is dispatched next - there's nothing
// left for the poller to watch once the goto itself is no longer running.
func TestMoveInterruptCheckClearedOnNonGotoDispatch(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlock("minecraft:coal_ore", 5, 0, 0) // visible, so ActionMine is legal
	agent := &fakeMoveInterruptAgent{fakeAgent: inner}
	cfg := buriedTestConfig()
	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if _, err := env.Step(context.Background(), rlenv.ActionGoToTarget); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.lastCheck == nil {
		t.Fatal("installed check = nil after the goto dispatch, want non-nil")
	}

	if _, err := env.Step(context.Background(), rlenv.ActionMine); err != nil {
		t.Fatalf("Step: %v", err)
	}
	if agent.setCalls != 2 {
		t.Fatalf("SetMoveInterruptCheck called %d times, want 2 (goto, then mine clearing it)", agent.setCalls)
	}
	if agent.lastCheck != nil {
		t.Fatal("installed check != nil after a non-goto dispatch, want nil")
	}
}
