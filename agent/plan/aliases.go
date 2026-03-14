package plan

import "github.com/reallyoldfogie/mc-agent/models"

// Type aliases to models for plan helpers.
type (
	Step          = models.Step
	PlanPolicy    = models.PlanPolicy
	ErrorPolicy   = models.ErrorPolicy
	StepStatus    = models.StepStatus
	StepResult    = models.StepResult
	PlanState     = models.PlanState
	PlanEventType = models.PlanEventType
)

const (
	StopOnError        = models.StopOnError
	ContinueOnError    = models.ContinueOnError
	StepSuccess        = models.StepSuccess
	StepFailed         = models.StepFailed
	StepSkipped        = models.StepSkipped
	PlanIdle           = models.PlanIdle
	PlanRunning        = models.PlanRunning
	PlanCompleted      = models.PlanCompleted
	PlanFailed         = models.PlanFailed
	PlanCanceled       = models.PlanCanceled
	EventPlanStarted   = models.EventPlanStarted
	EventPlanCompleted = models.EventPlanCompleted
	EventPlanFailed    = models.EventPlanFailed
	EventPlanCanceled  = models.EventPlanCanceled
	EventStepStarted   = models.EventStepStarted
	EventStepCompleted = models.EventStepCompleted
	EventStepFailed    = models.EventStepFailed
	EventStepSkipped   = models.EventStepSkipped
)
