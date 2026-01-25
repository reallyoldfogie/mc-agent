package models

// FollowState represents the current state of the follow system.
type FollowState int

const (
	StateIdle FollowState = iota
	StateTargeting
	StateCalculatingPath
	StateFollowingPath
	StateArrived
	StateStuck
	StateLost
)

// String returns the name of the state.
func (s FollowState) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateTargeting:
		return "Targeting"
	case StateCalculatingPath:
		return "Calculating Path"
	case StateFollowingPath:
		return "Following Path"
	case StateArrived:
		return "Arrived"
	case StateStuck:
		return "Stuck"
	case StateLost:
		return "Lost"
	default:
		return "Unknown"
	}
}
