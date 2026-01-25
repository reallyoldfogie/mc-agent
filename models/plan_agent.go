package models

import (
	"context"
	"time"
)

// PlanAgent is the minimal interface required to execute plans.
type PlanAgent interface {
	ContainerOperations
	MovementAgent

	MoveTo(ctx context.Context, x, y, z float64) error
	LineTo(ctx context.Context, x, y, z float64, notifyChat bool) error
	ChatEvents() <-chan string
	HasLineOfSight(ctx context.Context, x, y, z float64) (bool, error)
	FindVisibleEntity(ctx context.Context, entityTypeID int32, maxDistance float64) (entityID int32, x, y, z float64, found bool, err error)
	FindVisibleBlock(ctx context.Context, blockName string, maxDistance int) (x, y, z float64, found bool, err error)
	GetEntityTypeID(entityName string) (int32, bool)

	OpenContainerAt(ctx context.Context, x, y, z float64, face int, timeout time.Duration) (byte, error)

	UseItemOnBlock(ctx context.Context, x, y, z float64, face int, hand int) error
	UseItemOnEntity(ctx context.Context, entityID int32, hand int, sneaking bool) error
}
