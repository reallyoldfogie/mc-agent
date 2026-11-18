package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// craftIngredientMemoryTestConfig poses a craft task with
// UseCraftIngredientMemory enabled, otherwise matching testConfig()'s own
// shape (no TargetOffset — the craft-ingredient feature doesn't need a
// move-to task to be active).
func craftIngredientMemoryTestConfig() rlenv.Config {
	cfg := testConfig()
	cfg.CraftTargetItem = "minecraft:stick"
	cfg.UseCraftIngredientMemory = true
	return cfg
}

// craftIngredientObservationIndices mirror observation.go's own
// craftIngredientDx/Dy/Dz/Valid doc comment (indices 86-89).
const (
	craftIngredientDxIdx    = 86
	craftIngredientDyIdx    = 87
	craftIngredientDzIdx    = 88
	craftIngredientValidIdx = 89
)

func TestCraftIngredientMemory_PopulatesWhenEnabledAndNotReady(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setCraftTarget("minecraft:stick", false, 0) // not craftable yet
	agent := &fakeCraftIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 5, Y: 6, Z: 7}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), craftIngredientMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestCraftIngredientLocationCalls) != 1 {
		t.Fatalf("NearestCraftIngredientLocation calls = %d, want 1", len(agent.nearestCraftIngredientLocationCalls))
	}
	if agent.nearestCraftIngredientLocationCalls[0] != "minecraft:stick" {
		t.Errorf("NearestCraftIngredientLocation called with %q, want minecraft:stick", agent.nearestCraftIngredientLocationCalls[0])
	}

	if valid := obs.Values[craftIngredientValidIdx]; valid != 1 {
		t.Fatalf("craftIngredientValid = %v, want 1", valid)
	}
	if dx := obs.Values[craftIngredientDxIdx]; dx != 5 {
		t.Errorf("craftIngredientDx = %v, want 5", dx)
	}
	if dy := obs.Values[craftIngredientDyIdx]; dy != 6 {
		t.Errorf("craftIngredientDy = %v, want 6", dy)
	}
	if dz := obs.Values[craftIngredientDzIdx]; dz != 7 {
		t.Errorf("craftIngredientDz = %v, want 7", dz)
	}
}

func TestCraftIngredientMemory_ZeroWhenDisabled(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setCraftTarget("minecraft:stick", false, 0)
	agent := &fakeCraftIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 5, Y: 6, Z: 7}, found: true}

	cfg := craftIngredientMemoryTestConfig()
	cfg.UseCraftIngredientMemory = false // explicitly disabled

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestCraftIngredientLocationCalls) != 0 {
		t.Fatalf("NearestCraftIngredientLocation calls = %d, want 0 when UseCraftIngredientMemory is false", len(agent.nearestCraftIngredientLocationCalls))
	}
	for i := craftIngredientDxIdx; i <= craftIngredientValidIdx; i++ {
		if obs.Values[i] != 0 {
			t.Errorf("obs.Values[%d] = %v, want 0 when UseCraftIngredientMemory is false", i, obs.Values[i])
		}
	}
}

func TestCraftIngredientMemory_ZeroWhenAlreadyCraftReady(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setCraftTarget("minecraft:stick", true, 0) // already craftable
	agent := &fakeCraftIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 5, Y: 6, Z: 7}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), craftIngredientMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestCraftIngredientLocationCalls) != 0 {
		t.Fatalf("NearestCraftIngredientLocation calls = %d, want 0 when craftReady is already true", len(agent.nearestCraftIngredientLocationCalls))
	}
	if valid := obs.Values[craftIngredientValidIdx]; valid != 0 {
		t.Fatalf("craftIngredientValid = %v, want 0 when craftReady is already true", valid)
	}
}

func TestCraftIngredientMemory_NoOpWithoutCapability(t *testing.T) {
	agent := newFakeAgent(0, 0, 0) // plain fakeAgent: no NearestCraftIngredientLocation
	agent.setCraftTarget("minecraft:stick", false, 0)

	env, err := rlenv.New(agent, actions.NewRegistry(), craftIngredientMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	for i := craftIngredientDxIdx; i <= craftIngredientValidIdx; i++ {
		if obs.Values[i] != 0 {
			t.Errorf("obs.Values[%d] = %v, want 0 (agent doesn't implement CraftIngredientMemoryAgent)", i, obs.Values[i])
		}
	}
}

func TestCraftIngredientMemory_NotFoundLeavesZero(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setCraftTarget("minecraft:stick", false, 0)
	agent := &fakeCraftIngredientMemoryAgent{fakeAgent: inner, found: false}

	env, err := rlenv.New(agent, actions.NewRegistry(), craftIngredientMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if valid := obs.Values[craftIngredientValidIdx]; valid != 0 {
		t.Fatalf("craftIngredientValid = %v, want 0 when nothing is remembered", valid)
	}
}
