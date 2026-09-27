package combat

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestRangedAttackRequestLeadsMovingTarget(t *testing.T) {
	req := RangedAttackRequest{
		TargetID:        7,
		Weapon:          Bow,
		TargetPosition:  models.V3{X: 10, Y: 2, Z: -4},
		TargetVelocity:  models.V3{X: 1.5, Y: 0, Z: -2},
		ProjectileSpeed: 3,
		FlightTime:      2,
	}

	got, err := req.LeadPosition()
	if err != nil {
		t.Fatal(err)
	}
	want := models.V3{X: 13, Y: 2, Z: -8}
	if got != want {
		t.Fatalf("lead position: got %+v, want %+v", got, want)
	}
}

func TestRangedAttackRequestRejectsUnsafeTiming(t *testing.T) {
	base := RangedAttackRequest{
		TargetID:        7,
		Weapon:          Crossbow,
		TargetPosition:  models.V3{X: 1, Y: 2, Z: 3},
		ProjectileSpeed: 3,
	}
	cases := map[string]RangedAttackRequest{
		"missing target":       {Weapon: Bow, ProjectileSpeed: 3},
		"missing speed":        func() RangedAttackRequest { r := base; r.ProjectileSpeed = 0; return r }(),
		"negative flight time": func() RangedAttackRequest { r := base; r.FlightTime = -1; return r }(),
	}
	for name, req := range cases {
		if err := req.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestRangedAttackRequestCarriesTargetVelocityIntoLead(t *testing.T) {
	req := RangedAttackRequest{
		TargetID: 7, Weapon: Bow,
		TargetPosition:  models.V3{X: 10, Y: 2, Z: -4},
		TargetVelocity:  models.V3{X: 1, Y: 0, Z: 0.5},
		ProjectileSpeed: 3, FlightTime: 2,
	}
	got, err := req.LeadPosition()
	if err != nil {
		t.Fatal(err)
	}
	if got.X != 12 || got.Z != -3 {
		t.Fatalf("velocity lead: got %+v", got)
	}
}
