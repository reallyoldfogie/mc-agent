package models

import "context"

// Step defines a single instruction in a plan.
type Step interface {
	ID() string
	Describe() string
	Run(ctx context.Context, agent Agent) (StepResult, error)
}
