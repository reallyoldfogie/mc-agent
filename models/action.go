package models

import "context"

// Action defines the contract for executable actions.
type Action[T any] interface {
	Name() string
	Execute(ctx context.Context, agent T, args []string) error
	Usage() string
}
