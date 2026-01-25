package models

// Action defines the contract for executable actions.
type Action[T any] interface {
	Name() string
	Execute(agent T, args []string) error
	Usage() string
}
