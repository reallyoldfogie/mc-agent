package plan

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// Runner executes plans against an Agent.
type Runner struct {
	mu      sync.Mutex
	agent   models.Agent
	status  models.PlanStatus
	events  chan models.PlanEvent
	cancel  context.CancelFunc
	running bool
}

// NewRunner creates a runner bound to the provided agent.
func NewRunner(agent models.Agent) *Runner {
	return &Runner{
		agent:  agent,
		status: models.PlanStatus{State: PlanIdle},
		events: make(chan models.PlanEvent, 64),
	}
}

// Start executes the plan asynchronously with a background context.
func (r *Runner) Start(plan models.Plan) error {
	return r.StartWithContext(context.Background(), plan)
}

// StartWithContext executes the plan asynchronously with the provided context.
func (r *Runner) StartWithContext(ctx context.Context, plan models.Plan) error {
	if len(plan.Steps) == 0 {
		return errors.New("plan has no steps")
	}

	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return errors.New("plan already running")
	}
	r.running = true
	ctx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.status = models.PlanStatus{
		PlanName:  plan.Name,
		State:     PlanRunning,
		StepIndex: -1,
		StepCount: len(plan.Steps),
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	r.emit(models.PlanEvent{
		Type:     EventPlanStarted,
		Time:     time.Now(),
		PlanName: plan.Name,
		Message:  "plan started",
	})
	r.mu.Unlock()

	go r.run(ctx, plan)
	return nil
}

// Stop cancels the current plan.
func (r *Runner) Stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return errors.New("no plan running")
	}
	if r.cancel != nil {
		r.cancel()
	}
	r.mu.Unlock()
	return nil
}

// Status returns the latest plan status snapshot.
func (r *Runner) Status() models.PlanStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// Events returns a stream of plan progress events.
func (r *Runner) Events() <-chan models.PlanEvent {
	return r.events
}

func (r *Runner) run(ctx context.Context, plan models.Plan) {
	onError := plan.Policy.OnError
	if onError != ContinueOnError {
		onError = StopOnError
	}

	for i, step := range plan.Steps {
		if ctx.Err() != nil {
			r.finish(plan, PlanCanceled, "plan canceled", ctx.Err())
			return
		}

		r.setStep(plan.Name, i, len(plan.Steps), step)
		r.emit(models.PlanEvent{
			Type:      EventStepStarted,
			Time:      time.Now(),
			PlanName:  plan.Name,
			StepIndex: i,
			StepID:    step.ID(),
			Message:   step.Describe(),
		})

		result, err := step.Run(ctx, r.agent)
		if err != nil || result.Status == StepFailed {
			if errors.Is(err, context.Canceled) {
				r.finish(plan, PlanCanceled, "plan canceled", err)
				return
			}
			errMsg := ""
			if err != nil {
				errMsg = err.Error()
			} else {
				errMsg = "step failed"
			}
			r.emit(models.PlanEvent{
				Type:      EventStepFailed,
				Time:      time.Now(),
				PlanName:  plan.Name,
				StepIndex: i,
				StepID:    step.ID(),
				Message:   step.Describe(),
				Error:     errMsg,
			})
			if onError == StopOnError {
				r.finish(plan, PlanFailed, "plan failed", err)
				return
			}
			r.setError(errMsg)
			continue
		}

		if result.Status == StepSkipped {
			r.emit(models.PlanEvent{
				Type:      EventStepSkipped,
				Time:      time.Now(),
				PlanName:  plan.Name,
				StepIndex: i,
				StepID:    step.ID(),
				Message:   step.Describe(),
			})
			continue
		}

		r.emit(models.PlanEvent{
			Type:      EventStepCompleted,
			Time:      time.Now(),
			PlanName:  plan.Name,
			StepIndex: i,
			StepID:    step.ID(),
			Message:   step.Describe(),
		})
	}

	r.finish(plan, PlanCompleted, "plan completed", nil)
}

func (r *Runner) setStep(planName string, index, count int, step Step) {
	r.mu.Lock()
	r.status.PlanName = planName
	r.status.State = PlanRunning
	r.status.StepIndex = index
	r.status.StepCount = count
	r.status.StepID = step.ID()
	r.status.StepDetail = step.Describe()
	r.status.UpdatedAt = time.Now()
	r.mu.Unlock()
}

func (r *Runner) setError(errMsg string) {
	r.mu.Lock()
	r.status.LastError = errMsg
	r.status.UpdatedAt = time.Now()
	r.mu.Unlock()
}

func (r *Runner) finish(plan models.Plan, state PlanState, msg string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	r.mu.Lock()
	r.status.State = state
	r.status.LastError = errMsg
	r.status.UpdatedAt = time.Now()
	r.running = false
	r.cancel = nil
	r.mu.Unlock()

	eventType := EventPlanCompleted
	switch state {
	case PlanFailed:
		eventType = EventPlanFailed
	case PlanCanceled:
		eventType = EventPlanCanceled
	}

	r.emit(models.PlanEvent{
		Type:     eventType,
		Time:     time.Now(),
		PlanName: plan.Name,
		Message:  msg,
		Error:    errMsg,
	})
}

func (r *Runner) emit(ev models.PlanEvent) {
	select {
	case r.events <- ev:
	default:
	}
}
