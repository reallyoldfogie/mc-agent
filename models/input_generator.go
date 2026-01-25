package models

import "time"

// InputGenerator converts path steps to physics inputs.
type InputGenerator interface {
	GenerateInputs(currentState PhysicsState, targetStep PathStep, runTime time.Duration) Inputs
	EstimateTicksRequired(step PathStep, currentState PhysicsState) int
}
