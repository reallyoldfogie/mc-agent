package models

import "context"

// ActionRegistry provides concurrent-safe access to registered actions.
type ActionRegistry[T any] interface {
	Register(action Action[T])
	Get(name string) (Action[T], bool)
	Execute(ctx context.Context, name string, agent T, args []string) error
}
