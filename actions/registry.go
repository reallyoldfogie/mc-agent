package actions

import "github.com/reallyoldfogie/mc-agent/models"

// NewRegistry builds a registry with all standard chat actions.
func NewRegistry() models.ActionRegistry[models.CommandAgent] {
	reg := NewActionRegistry[models.CommandAgent]()
	RegisterDefaults(reg)
	return reg
}

// RegisterDefaults registers all built-in actions.
func RegisterDefaults(reg models.ActionRegistry[models.CommandAgent]) {
	reg.Register(Help{})
	reg.Register(Pos{})
	reg.Register(Say{})
	reg.Register(TestMove{})
	reg.Register(MoveTo{})
	reg.Register(LineTo{})
	reg.Register(MoveForward{})
	reg.Register(MoveUp{})
	reg.Register(MoveUpAndSneak{})
	reg.Register(MoveToAndSneak{})
	reg.Register(LineToAndSneak{})
	reg.Register(StopSneak{})
	reg.Register(FindPath{})
	reg.Register(TestPath{})
	reg.Register(Follow{})
	reg.Register(StopFollow{})
	reg.Register(FollowStatus{})
	reg.Register(StartTracking{})
	reg.Register(StopTracking{})
	reg.Register(PlanStatus{})
	reg.Register(PlanStop{})
	reg.Register(FireBow{})
	reg.Register(FireBowAt{})
	reg.Register(Mount{})
	reg.Register(Dismount{})
	reg.Register(VehicleJump{})
}
