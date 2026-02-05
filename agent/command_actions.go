package agent

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-agent/actions"
)

// MoveToWithChat runs moveTo with chat notifications enabled.
func (a *agent) MoveToWithChat(ctx context.Context, x, y, z float64) error {
	return a.MoveTo(ctx, x, y, z, true)
}

// TestMove executes the legacy test move command.
func (a *agent) TestMove() {
	a.cmdTestMove()
}

// TestPath executes the legacy test path command.
func (a *agent) TestPath() {
	a.cmdTestPath()
}

// StartTracking begins target tracking for the chat command.
func (a *agent) StartTracking() {
	a.cmdStartTracking()
}

// StopTracking stops target tracking for the chat command.
func (a *agent) StopTracking() {
	a.cmdStopTracking()
}

// HasFollowManager reports whether follow behavior is available.
func (a *agent) HasFollowManager() bool {
	a.mu.Lock()
	fm := a.followMgr
	a.mu.Unlock()
	return fm != nil
}

// IsFollowing reports whether the follow manager is actively following.
func (a *agent) IsFollowing() bool {
	a.mu.Lock()
	fm := a.followMgr
	a.mu.Unlock()
	if fm == nil {
		return false
	}
	return fm.IsActive()
}

// NearestPlayerInfo returns the nearest tracked player for commands.
func (a *agent) NearestPlayerInfo() (actions.NearestPlayerInfo, bool) {
	info, ok := a.findNearestPlayer()
	if !ok {
		return actions.NearestPlayerInfo{}, false
	}
	return actions.NearestPlayerInfo{
		EntityID: info.EntityID,
		UUID:     info.UUID,
		Distance: info.Distance,
		X:        info.X,
		Y:        info.Y,
		Z:        info.Z,
	}, true
}

// FindPlayerByName resolves a tracked player by name for commands.
func (a *agent) FindPlayerByName(name string) (x, y, z float64, found bool, err error) {
	a.mu.Lock()
	ts := a.targetSelector
	a.mu.Unlock()
	if ts == nil {
		return 0, 0, 0, false, fmt.Errorf("target selector not available")
	}
	info, err := ts.FindPlayerByName(name)
	if err != nil {
		return 0, 0, 0, false, err
	}
	return info.X, info.Y, info.Z, true, nil
}
