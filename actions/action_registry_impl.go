package actions

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

type actionRegistry[T any] struct {
	mu      sync.RWMutex
	actions map[string]models.Action[T]
}

// NewActionRegistry creates an empty registry ready for registrations.
func NewActionRegistry[T any]() models.ActionRegistry[T] {
	return &actionRegistry[T]{actions: make(map[string]models.Action[T])}
}

// Register adds an action to the registry by lowercase name.
func (r *actionRegistry[T]) Register(action models.Action[T]) {
	if action == nil {
		return
	}
	name := strings.ToLower(strings.TrimSpace(action.Name()))
	if name == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.actions == nil {
		r.actions = make(map[string]models.Action[T])
	}
	r.actions[name] = action
}

// Get returns the action for a name.
func (r *actionRegistry[T]) Get(name string) (models.Action[T], bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	r.mu.RLock()
	defer r.mu.RUnlock()
	action, ok := r.actions[key]
	return action, ok
}

// Execute looks up and runs the action by name.
func (r *actionRegistry[T]) Execute(ctx context.Context, name string, agent T, args []string) (models.Completion, error) {
	action, ok := r.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: %s", models.ErrActionNotFound, strings.TrimSpace(name))
	}
	return action.Execute(ctx, agent, args)
}
