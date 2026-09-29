package agent

import (
	"log/slog"
	"testing"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

type combatPhysicsExecutor struct {
	fakeMoveExec
	state models.PhysicsState
}

func (e *combatPhysicsExecutor) GetPhysicsState() models.PhysicsState { return e.state }

func TestCombatMovementAdapterFeedsCriticalHitObservation(t *testing.T) {
	state := physics.NewState(nil, slog.Default())
	state.SetOnGround(false)
	state.SetVelocity(models.V3{Y: -0.08})

	a := &agent{mountedEntityID: -1}
	a.moveExec = &combatPhysicsExecutor{state: state}

	if !a.combatCriticalEligible() {
		t.Fatal("falling physics state should allow a critical hit")
	}

	state.SetOnGround(true)
	if a.combatCriticalEligible() {
		t.Fatal("grounded physics state should not allow a critical hit")
	}
}

func TestCombatCriticalDecisionIsRevalidatedBeforeDispatch(t *testing.T) {
	state := physics.NewState(nil, slog.Default())
	state.SetOnGround(false)
	state.SetVelocity(models.V3{Y: -0.08})
	a := &agent{mountedEntityID: -1}
	a.moveExec = &combatPhysicsExecutor{state: state}

	decision := combat.Decision{Attack: true, Weapon: combat.MeleeWeapon, Critical: true}
	if !decision.Critical || !a.combatCriticalEligible() {
		t.Fatal("expected initial critical decision to be eligible")
	}

	state.SetOnGround(true)
	if a.combatCriticalEligible() {
		t.Fatal("stale critical decision must be rejected after landing")
	}
}
