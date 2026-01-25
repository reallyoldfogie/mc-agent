package models

import (
	"strconv"
	"time"
)

// Plan is an ordered list of steps with error handling policy.
type Plan struct {
	Name   string
	Steps  []Step
	Policy PlanPolicy
}

// PlanPolicy controls how the runner reacts to step failures.
type PlanPolicy struct {
	OnError ErrorPolicy
}

// ErrorPolicy defines how to handle step errors.
type ErrorPolicy int

const (
	StopOnError ErrorPolicy = iota
	ContinueOnError
)

// StepStatus indicates the outcome of a step.
type StepStatus string

const (
	StepSuccess StepStatus = "success"
	StepFailed  StepStatus = "failed"
	StepSkipped StepStatus = "skipped"
)

// StepResult describes the outcome of a step.
type StepResult struct {
	Status  StepStatus
	Details string
	Metrics map[string]float64
}

// PlanState describes the overall plan lifecycle state.
type PlanState string

const (
	PlanIdle      PlanState = "idle"
	PlanRunning   PlanState = "running"
	PlanCompleted PlanState = "completed"
	PlanFailed    PlanState = "failed"
	PlanCanceled  PlanState = "canceled"
)

// PlanStatus is a snapshot of plan execution.
type PlanStatus struct {
	PlanName   string
	State      PlanState
	StepIndex  int
	StepCount  int
	StepID     string
	StepDetail string
	LastError  string
	StartedAt  time.Time
	UpdatedAt  time.Time
}

// PlanEventType signals runner activity.
type PlanEventType string

const (
	EventPlanStarted   PlanEventType = "plan_started"
	EventPlanCompleted PlanEventType = "plan_completed"
	EventPlanFailed    PlanEventType = "plan_failed"
	EventPlanCanceled  PlanEventType = "plan_canceled"
	EventStepStarted   PlanEventType = "step_started"
	EventStepCompleted PlanEventType = "step_completed"
	EventStepFailed    PlanEventType = "step_failed"
	EventStepSkipped   PlanEventType = "step_skipped"
)

// PlanEvent emits progress updates for observability.
type PlanEvent struct {
	Type      PlanEventType
	Time      time.Time
	PlanName  string
	StepIndex int
	StepID    string
	Message   string
	Error     string
}

// String returns a compact status summary.
func (s PlanStatus) String() string {
	if s.PlanName == "" {
		return string(s.State)
	}
	if s.StepCount == 0 {
		return s.PlanName + ": " + string(s.State)
	}
	return s.PlanName + ": " + string(s.State) + " (" + strconv.Itoa(s.StepIndex+1) + "/" + strconv.Itoa(s.StepCount) + ") " + s.StepDetail
}
