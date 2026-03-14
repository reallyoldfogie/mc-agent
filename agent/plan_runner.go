package agent

import (
	"context"
	"errors"
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// StartPlan begins executing a plan asynchronously.
func (a *agent) StartPlan(p models.Plan) error {
	if a.planRunner == nil {
		return errors.New("plan runner not initialized")
	}
	a.lifecycleMu.RLock()
	ctx := a.ctx
	a.lifecycleMu.RUnlock()
	if ctx == nil {
		return a.planRunner.Start(p)
	}
	return a.planRunner.StartWithContext(ctx, p)
}

// StopPlan cancels the currently running plan.
func (a *agent) StopPlan(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if a.planRunner == nil {
		return errors.New("plan runner not initialized")
	}
	return a.planRunner.Stop()
}

// PlanStatus returns the latest plan execution status.
func (a *agent) PlanStatus(ctx context.Context) models.PlanStatus {
	select {
	case <-ctx.Done():
		return models.PlanStatus{State: models.PlanIdle}
	default:
	}
	if a.planRunner == nil {
		return models.PlanStatus{State: models.PlanIdle}
	}
	return a.planRunner.Status()
}

// PlanEvents exposes the runner event stream.
func (a *agent) PlanEvents() <-chan models.PlanEvent {
	if a.planRunner == nil {
		return nil
	}
	return a.planRunner.Events()
}

func (a *agent) startInitialPlan() {
	if err := a.StartPlan(a.cfg.InitialPlan); err != nil {
		log.Printf("[plan] Failed to start initial plan: %v", err)
	}
}
