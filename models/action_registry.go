package models

import "context"

// ActionRegistry provides concurrent-safe access to registered actions.
type ActionRegistry[T any] interface {
	Register(action Action[T])
	Get(name string) (Action[T], bool)
	// Execute looks up name and runs it. err reports that the action
	// wasn't found or failed to dispatch; the returned Completion (nil
	// when err != nil) reports the eventual outcome of the dispatched
	// action itself — callers that only care "was this launched," as
	// most callers historically have, can ignore it.
	Execute(ctx context.Context, name string, agent T, args []string) (Completion, error)
}
