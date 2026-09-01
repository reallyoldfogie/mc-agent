package models

import "context"

// Action defines the contract for executable actions. Execute returns
// promptly: err reports a dispatch-time failure (bad precondition, e.g.
// the registry couldn't even attempt the action), and the returned
// Completion reports the outcome of whatever the action actually does —
// see Completion's doc comment. Invalid *arguments* are, by this
// package's existing convention, reported via agent.SendChat and a nil
// err/a resolved-nil Completion, not a returned error; that convention
// is unchanged here.
type Action[T any] interface {
	Name() string
	Execute(ctx context.Context, agent T, args []string) (Completion, error)
	Usage() string
}
