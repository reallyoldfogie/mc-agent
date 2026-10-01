package models

// BuildStructureResult summarizes a CommandAgent.BuildStructure run: how
// many blocks were placed, and which ones weren't (with why), so a caller
// can report or decide whether to retry the specific cells that failed
// instead of guessing at the outcome of a run that may have mostly
// succeeded.
type BuildStructureResult struct {
	Placed int
	Failed []BuildStructureFailure
}

// BuildStructureFailure records one block BuildStructure attempted to place
// and couldn't. Reason is the placement error's message, not the error
// value itself - the same convention agent/plan.StepResult's Details field
// uses - so this stays a plain, inspectable value.
type BuildStructureFailure struct {
	Pos    V3
	Item   string
	Reason string
}
