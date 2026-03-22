package models

import (
	"context"
	"time"
)

type AgentActions interface { // Action helpers (used by plan runner)
	MoveTo(ctx context.Context, x, y, z float64, notifyChat bool) error
	LineTo(ctx context.Context, x, y, z float64, notifyChat bool) error
	MoveForward(ctx context.Context, distance float64) error
	MoveUp(ctx context.Context, distance float64) error
	FindPath(ctx context.Context, x, y, z float64) error
	LookAt(ctx context.Context, x, y, z float64) error
	TurnTowards(ctx context.Context, x, y, z float64) error
	Follow(ctx context.Context, target string) error
	StopFollow(ctx context.Context) error
	FollowStatus(ctx context.Context) string
	ChatEvents(ctx context.Context) <-chan string
	HasLineOfSight(ctx context.Context, x, y, z float64) (bool, error)
	FindVisibleEntity(ctx context.Context, entityTypeID int32, maxDistance float64) (entityID int32, x, y, z float64, found bool, err error)
	FindVisibleBlock(ctx context.Context, blockName string, maxDistance int) (x, y, z float64, found bool, err error)
	OpenContainerAt(ctx context.Context, x, y, z float64, face int, timeout time.Duration) (byte, error)
	UseItemOnBlock(ctx context.Context, x, y, z float64, face int, hand int) error
	UseItemOnEntity(ctx context.Context, entityID int32, hand Hand, sneaking bool) error
	SelectHotbarSlot(ctx context.Context, slot int) error
	FindSlotWith(ctx context.Context, itemName string, windowID int) (slot int, found bool, err error)
	WaitForHotbarItem(ctx context.Context, itemName string, maxWaitMS int) (slot int, err error)

	// Bow firing actions
	//Deprecated: Use FireBowAt for targeted firing
	FireBow(ctx context.Context) error

	FireBowAt(ctx context.Context, x, y, z float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error)
	FireBowWithPitch(ctx context.Context, pitch, yaw float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error) // Returns []models.TrajectoryPoint

	// Projectile actions
	// projectileType: models.ProjectileType value
	ThrowProjectileAt(ctx context.Context, projectileType ProjectileType, x, y, z float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error)
}
