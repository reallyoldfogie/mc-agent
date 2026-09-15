package movement

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// TestIsVehicleActionStep covers WATER_TRAVERSAL_PATHFINDING_PLAN.md's Item 8's stuck-detection
// exclusion: MountVehicle/DismountVehicle/PlaceVehicle must all be recognized so distance-based
// progress tracking doesn't fire mid-wait for an async action that doesn't move the player.
func TestIsVehicleActionStep(t *testing.T) {
	tests := []struct {
		movement pathfinding.MovementType
		want     bool
	}{
		{pathfinding.MountVehicle, true},
		{pathfinding.DismountVehicle, true},
		{pathfinding.PlaceVehicle, true},
		{pathfinding.Traverse, false},
		{pathfinding.Swim, false},
		{pathfinding.VehicleSwim, false},
	}
	for _, tt := range tests {
		if got := isVehicleActionStep(tt.movement); got != tt.want {
			t.Errorf("isVehicleActionStep(%v) = %v, want %v", tt.movement, got, tt.want)
		}
	}
}

// TestHandlePlaceVehicleStep_TriggersCallbackAndWaitsForMount covers the PlaceVehicle step handler:
// the first tick must trigger the place-vehicle callback exactly once (with the step's Position)
// and hold idle inputs; subsequent ticks must keep waiting until mountedEntityID transitions to
// any mounted state, then advance to the next step.
func TestHandlePlaceVehicleStep_TriggersCallbackAndWaitsForMount(t *testing.T) {
	exec := createTestPhysicsExecutor()

	waterPos := models.V3{X: 5, Y: 64, Z: 5}
	step := pathfinding.PathStep{Position: waterPos, Movement: pathfinding.PlaceVehicle}

	called := make(chan models.V3, 1)
	exec.SetPlaceVehicleCallback(func(ctx context.Context, pos models.V3) error {
		called <- pos
		return nil
	})

	// First tick: should trigger the callback and return idle inputs without advancing.
	_ = exec.handlePlaceVehicleStep(step)

	select {
	case gotPos := <-called:
		if gotPos != waterPos {
			t.Errorf("callback called with %v, want %v", gotPos, waterPos)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected the place-vehicle callback to be triggered")
	}

	if !exec.waitingForPlacement {
		t.Error("expected waitingForPlacement to be true after the first tick")
	}

	exec.pathMu.Lock()
	stepBefore := exec.currentStep
	exec.pathMu.Unlock()

	// Still waiting (mountedEntityID untouched, defaults to -1/not mounted): another tick
	// should not advance currentStep.
	_ = exec.handlePlaceVehicleStep(step)
	exec.pathMu.Lock()
	if exec.currentStep != stepBefore {
		t.Errorf("expected currentStep to stay at %d while still waiting, got %d", stepBefore, exec.currentStep)
	}
	exec.pathMu.Unlock()

	// Simulate the mount completing (e.g. ClientboundSetPassengers arriving).
	exec.mountedEntityMu.Lock()
	exec.mountedEntityID = 42
	exec.mountedEntityMu.Unlock()

	_ = exec.handlePlaceVehicleStep(step)

	if exec.waitingForPlacement {
		t.Error("expected waitingForPlacement to be false once mounted")
	}
	exec.pathMu.Lock()
	if exec.currentStep != stepBefore+1 {
		t.Errorf("expected currentStep to advance to %d once mounted, got %d", stepBefore+1, exec.currentStep)
	}
	exec.pathMu.Unlock()
}
