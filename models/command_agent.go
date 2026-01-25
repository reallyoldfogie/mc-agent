package models

import "context"

// CommandAgent is the minimal surface required by command actions.
type CommandAgent interface {
	MovementAgent

	GetPositionSimple() (x, y, z float64, initialized bool)

	MoveToWithChat(ctx context.Context, x, y, z float64) error
	LineTo(ctx context.Context, x, y, z float64, notifyChat bool) error

	TestMove()
	TestPath()
	StartTracking()
	StopTracking()

	HasFollowManager() bool
	IsFollowing() bool

	PlanStatus() PlanStatus
	StopPlan() error

	FireBow()
	FireBowAt(x, y, z float64)

	NearestPlayerInfo() (NearestPlayerInfo, bool)
	FindPlayerByName(name string) (x, y, z float64, found bool, err error)
}
