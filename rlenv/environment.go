package rlenv

import (
	"context"
	"fmt"
	"time"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
	"github.com/reallyoldfogie/mc-agent/models"
)

// Environment wraps a live bot session (LiveAgent) plus its action
// registry to satisfy cRL-go's rl.Environment, per
// RL_POLICY_INTEGRATION_PLAN.md item 5. One Environment owns one bot
// session; per that document's scope item 6, callers should drive it with
// pkg/reinforce/pkg/ppo's PersistentEnvFactory path (Workers=1, one
// long-lived instance reused across episodes via Reset), not the
// per-episode EnvFactory path the toy environments use — constructing a
// live bot session per rollout would be far too expensive, and nothing
// about a live session is safe to run concurrently with another instance
// sharing the same bot.
type Environment struct {
	agent    LiveAgent
	registry models.ActionRegistry[models.CommandAgent]
	cfg      Config

	originX, originY, originZ float64
	targetX, targetY, targetZ float64
	prevDistance              float64
	prevHealth                float32
	prevHealthKnown           bool
	episodeStarted            bool
}

// New constructs an Environment. registry is typically actions.NewRegistry()
// (or a test double registering only what's needed); it must dispatch
// "moveto" the way actions.MoveTo does (parseFloat'd x/y/z args) for
// ActionGoToTarget/ActionReturnHome to work.
func New(agent LiveAgent, registry models.ActionRegistry[models.CommandAgent], cfg Config) (*Environment, error) {
	if agent == nil {
		return nil, errNilAgent
	}
	if registry == nil {
		return nil, errNilRegistry
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Environment{agent: agent, registry: registry, cfg: cfg}, nil
}

// ObservationSize implements rl.Environment.
func (e *Environment) ObservationSize() int { return observationSize }

// ActionSpace implements rl.Environment.
func (e *Environment) ActionSpace() int { return NumActions }

// Reset implements rl.Environment. It does not move or respawn the bot —
// per Config.TargetOffset's doc comment, there is no real reset/teleport
// mechanism wired up yet (RSI_TRAINING_PLAN.md item 2 is still open), so
// "starting a fresh episode" means capturing wherever the bot currently is
// as this episode's origin and posing a new target relative to it, not
// actually repositioning anything.
func (e *Environment) Reset(ctx context.Context) (rl.Observation, error) {
	pos, yaw, pitch, ok := e.agent.GetPosition()
	if !ok {
		return rl.Observation{}, errPositionUnknown
	}
	x, y, z := pos.X, pos.Y, pos.Z

	e.originX, e.originY, e.originZ = x, y, z
	e.targetX = x + e.cfg.TargetOffset[0]
	e.targetY = y + e.cfg.TargetOffset[1]
	e.targetZ = z + e.cfg.TargetOffset[2]
	e.prevDistance = distance3(x, y, z, e.targetX, e.targetY, e.targetZ)
	e.episodeStarted = true

	health, food, saturation, healthKnown := e.agent.Health()
	e.prevHealth, e.prevHealthKnown = health, healthKnown
	return buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, health, food, saturation, healthKnown), nil
}

// Step implements rl.Environment.
//
// Why this polls position instead of blocking on the dispatched action:
// actions.MoveTo.Execute (what "moveto" dispatches to) launches
// MoveToWithChat in its own goroutine against context.Background() and
// returns immediately — Execute returning nil means "the move was
// launched," not "the move finished." This is exactly the "done signal"
// gap RL_POLICY_INTEGRATION_PLAN.md item 3 flagged
// ("deterministic action implementations... don't currently expose [a
// completion signal] in a uniform way"). Rather than block on Execute
// (which wouldn't wait long enough) or guess a fixed sleep, Step polls
// GetPosition every Config.PollInterval until arrival or
// Config.StepTimeout elapses, treating "didn't arrive within one step's
// timeout" as a normal, non-fatal outcome (the episode continues; the
// dispatched move may still be in flight in the background — a documented
// limitation, not a bug, given ctx passed to Execute isn't honored by
// MoveToWithChat's internal context.Background() call).
func (e *Environment) Step(ctx context.Context, action rl.Action) (rl.StepResult, error) {
	if !e.episodeStarted {
		return rl.StepResult{}, fmt.Errorf("rlenv: Step called before Reset")
	}

	if _, _, _, ok := e.agent.GetPosition(); !ok {
		return rl.StepResult{}, errPositionUnknown
	}

	targetX, targetY, targetZ, isMovement, err := e.movementTarget(action)
	if err != nil {
		return rl.StepResult{}, err
	}

	if isMovement {
		if err := e.registry.Execute(ctx, moveToActionName, e.agent, moveToArgs(targetX, targetY, targetZ)); err != nil {
			return rl.StepResult{}, fmt.Errorf("rlenv: dispatching %s: %w", moveToActionName, err)
		}
		if err := e.waitForArrival(ctx, targetX, targetY, targetZ); err != nil {
			return rl.StepResult{}, err
		}
	}
	pos, yaw, pitch, ok := e.agent.GetPosition()
	if !ok {
		return rl.StepResult{}, errPositionUnknown
	}
	x, y, z := pos.X, pos.Y, pos.Z
	newHealth, food, saturation, newHealthKnown := e.agent.Health()

	newDistance := distance3(x, y, z, e.targetX, e.targetY, e.targetZ)
	reward, done := computeReward(stepOutcome{
		prevDistance:      e.prevDistance,
		newDistance:       newDistance,
		prevHealth:        e.prevHealth,
		newHealth:         newHealth,
		healthKnownBefore: e.prevHealthKnown,
		healthKnownAfter:  newHealthKnown,
	})
	if newDistance <= e.cfg.ArrivalThreshold {
		reward += arrivalBonus
		done = true
	}
	e.prevDistance = newDistance
	e.prevHealth, e.prevHealthKnown = newHealth, newHealthKnown

	obs := buildObservation(x, y, z, yaw, pitch, e.targetX, e.targetY, e.targetZ, newHealth, food, saturation, newHealthKnown)
	return rl.StepResult{Observation: obs, Reward: reward, Done: done}, nil
}

// waitForArrival polls GetPosition until the bot is within
// Config.ArrivalThreshold of (targetX, targetY, targetZ), Config.StepTimeout
// elapses, or ctx is canceled. A plain timeout is not an error (see Step's
// doc comment); ctx cancellation is, since that's a caller-initiated abort
// that should propagate rather than be silently absorbed.
func (e *Environment) waitForArrival(ctx context.Context, targetX, targetY, targetZ float64) error {
	deadline := time.Now().Add(e.cfg.StepTimeout)
	for {
		if pos, _, _, ok := e.agent.GetPosition(); ok {
			if distance3(pos.X, pos.Y, pos.Z, targetX, targetY, targetZ) <= e.cfg.ArrivalThreshold {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if time.Now().After(deadline) {
			return nil
		}
		time.Sleep(e.cfg.PollInterval)
	}
}
