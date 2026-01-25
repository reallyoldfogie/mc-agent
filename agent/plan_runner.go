package agent

import (
	"errors"
	"log"

	"github.com/reallyoldfogie/mc-agent/agent/plan"
)

// StartPlan begins executing a plan asynchronously.
func (a *agent) StartPlan(p plan.Plan) error {
	if a.planRunner == nil {
		return errors.New("plan runner not initialized")
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return a.planRunner.Start(p)
	}
	return a.planRunner.StartWithContext(ctx, p)
}

// StopPlan cancels the currently running plan.
func (a *agent) StopPlan() error {
	if a.planRunner == nil {
		return errors.New("plan runner not initialized")
	}
	return a.planRunner.Stop()
}

// PlanStatus returns the latest plan execution status.
func (a *agent) PlanStatus() plan.PlanStatus {
	if a.planRunner == nil {
		return plan.PlanStatus{State: plan.PlanIdle}
	}
	return a.planRunner.Status()
}

// PlanEvents exposes the runner event stream.
func (a *agent) PlanEvents() <-chan plan.PlanEvent {
	if a.planRunner == nil {
		return nil
	}
	return a.planRunner.Events()
}

func (a *agent) startInitialPlan() {
	if a.cfg.InitialPlan == nil {
		return
	}
	if err := a.StartPlan(*a.cfg.InitialPlan); err != nil {
		log.Printf("[plan] Failed to start initial plan: %v", err)
	}
}
