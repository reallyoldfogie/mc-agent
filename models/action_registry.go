package models

// ActionRegistry provides concurrent-safe access to registered actions.
type ActionRegistry[T any] interface {
	Register(action Action[T])
	Get(name string) (Action[T], bool)
	Execute(name string, agent T, args []string) error
}
