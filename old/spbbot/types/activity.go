package types

// Activity -
type Activity interface {
	GetName() string
	Start(keywords []string, command string) error
	Stop() error

	IsRunning() bool

	RegisterCallbacks() map[string]func(params ...interface{}) error
}
