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

	PlanStatus(ctx context.Context) PlanStatus
	StopPlan(ctx context.Context) error

	FireBow(ctx context.Context) error
	FireBowAt(ctx context.Context, x, y, z float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error)

	NearestPlayerInfo(ctx context.Context) (NearestPlayerInfo, bool)
	FindPlayerByName(ctx context.Context, name string) (x, y, z float64, found bool, err error)
}
