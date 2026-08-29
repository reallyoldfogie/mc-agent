package models

import "context"

// MovementAgent captures shared movement and follow helpers.
type MovementAgent interface {
	Position

	// GetVelocity returns the agent's current physics velocity in
	// blocks/tick. ok is false when no movement executor is active (e.g.
	// physics disabled or not yet initialized).
	GetVelocity() (x, y, z float64, ok bool)

	// IsGliding reports whether elytra-gliding physics are currently active.
	// Returns false (not just "unknown") when no movement executor is
	// active, since a non-flying agent is definitionally not gliding.
	IsGliding() bool

	MoveForward(ctx context.Context, distance float64) error
	MoveUp(ctx context.Context, distance float64) error
	MoveUpAndSneak(ctx context.Context, distance float64) error
	MoveToAndSneak(ctx context.Context, x, y, z float64) error
	LineToAndSneak(ctx context.Context, x, y, z float64) error
	StartSneaking() error
	StopSneaking() error
	FindPath(ctx context.Context, x, y, z float64) (*Path, error)
	ExecutePath(ctx context.Context, path *Path) error
	LookAt(ctx context.Context, x, y, z float64) error
	Follow(ctx context.Context, target string) error
	StopFollow(ctx context.Context) error
	FollowStatus(ctx context.Context) string
}
