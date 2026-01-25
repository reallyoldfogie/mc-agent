package models

import "context"

// MovementAgent captures shared movement and follow helpers.
type MovementAgent interface {
	SendChat(message string) error
	GetPosition() (x, y, z float64, yaw, pitch float32, initialized bool)
	MoveForward(ctx context.Context, distance float64) error
	MoveUp(ctx context.Context, distance float64) error
	MoveUpAndSneak(ctx context.Context, distance float64) error
	MoveToAndSneak(ctx context.Context, x, y, z float64) error
	LineToAndSneak(ctx context.Context, x, y, z float64) error
	StartSneaking() error
	StopSneaking() error
	FindPath(ctx context.Context, x, y, z float64) error
	LookAt(ctx context.Context, x, y, z float64) error
	Follow(ctx context.Context, target string) error
	StopFollow(ctx context.Context) error
	FollowStatus() string
}
