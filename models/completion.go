package models

import (
	"context"
	"sync"
)

// Completion represents the eventual outcome of the side effect an
// Action.Execute call dispatched. Every Action.Execute returns one,
// whether or not the action does any background work: an action whose
// entire effect happens synchronously inside Execute returns an
// already-resolved Completion (see Done), and one that launches
// background work (a goroutine, a long-running move) returns one whose
// Wait blocks until that work concludes (see NewCompletion). This gives
// every caller of ActionRegistry.Execute a uniform way to learn "did the
// action actually finish, and how" — previously only Execute's own
// (immediate, dispatch-only) return value existed, so a caller like
// rlenv.Environment had no way to detect real completion short of
// polling world state for a proxy signal (e.g. position nearing a
// target), per RL_POLICY_INTEGRATION_PLAN.md item 3.
type Completion interface {
	// Wait blocks until the action's underlying effect finishes or ctx
	// is canceled, whichever comes first, returning that effect's
	// outcome (nil on success, ctx.Err() on cancellation). Wait may be
	// called more than once and from more than one goroutine; every
	// call observes the same outcome.
	Wait(ctx context.Context) error
}

// Done returns an already-resolved Completion carrying err. Actions
// whose entire effect happens synchronously inside Execute (the
// majority — chat replies, instant state toggles, background loops
// with no defined end like StartTracking) return this.
func Done(err error) Completion { return doneCompletion{err} }

type doneCompletion struct{ err error }

func (d doneCompletion) Wait(context.Context) error { return d.err }

// NewCompletion returns a Completion paired with a resolve function.
// Actions that launch background work (typically a bare `go func() {
// ... }()`) construct one, launch the goroutine, and call resolve
// exactly once with that goroutine's outcome when it finishes:
//
//	completion, resolve := models.NewCompletion()
//	go func() {
//		resolve(agent.MoveToWithChat(ctx, x, y, z))
//	}()
//	return completion, nil
//
// resolve is safe to call more than once (only the first call takes
// effect) and from any goroutine.
func NewCompletion() (Completion, func(error)) {
	c := &chanCompletion{done: make(chan struct{})}
	var once sync.Once
	resolve := func(err error) {
		once.Do(func() {
			c.err = err
			close(c.done)
		})
	}
	return c, resolve
}

type chanCompletion struct {
	done chan struct{}
	err  error // only read after <-done or <-ctx.Done() observes done closed
}

func (c *chanCompletion) Wait(ctx context.Context) error {
	select {
	case <-c.done:
		return c.err
	case <-ctx.Done():
		return ctx.Err()
	}
}
