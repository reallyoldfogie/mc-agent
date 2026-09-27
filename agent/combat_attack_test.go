package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

func TestAttackEntityHonorsCancellationBeforeValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := &agent{}
	err := a.AttackEntity(ctx, 1, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AttackEntity error = %v, want context.Canceled", err)
	}
}

func TestAttackEntityRejectsUntrackedTarget(t *testing.T) {
	a := &agent{entities: map[int32]*trackedEntity{}}
	err := a.AttackEntity(context.Background(), 42, false)
	if err == nil || err.Error() != "cannot attack entity 42: target is not tracked" {
		t.Fatalf("AttackEntity error = %v, want untracked-target error", err)
	}
}

func TestAttackEntityRejectsUninitializedPosition(t *testing.T) {
	a := &agent{
		entities: map[int32]*trackedEntity{
			42: {EntityID: 42, X: 1, Y: 0, Z: 0},
		},
	}
	err := a.AttackEntity(context.Background(), 42, false)
	if err == nil || err.Error() != "cannot attack entity: position not initialized" {
		t.Fatalf("AttackEntity error = %v, want uninitialized-position error", err)
	}
}

func TestAttackEntityAtRangeRejectsTargetBeyondConfiguredReach(t *testing.T) {
	a := &agent{
		entities: map[int32]*trackedEntity{
			42: {EntityID: 42, X: 10, Y: 0, Z: 0},
		},
		posX:           0,
		posY:           0,
		posZ:           0,
		posInitialized: true,
	}
	err := a.attackEntityAtRange(context.Background(), 42, false, 5)
	if err == nil || err.Error() != "cannot attack entity 42: target is beyond reach" {
		t.Fatalf("attackEntityAtRange error = %v, want beyond-reach error", err)
	}
}

var _ models.AgentActions = (*agent)(nil)
