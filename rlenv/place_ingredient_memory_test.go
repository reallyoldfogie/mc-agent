package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// placeIngredientMemoryTestConfig poses a standalone-placement task with
// UsePlaceIngredientMemory enabled — placeIngredientMemoryTestConfig's
// exact counterpart for the place task (see that function's own doc
// comment).
func placeIngredientMemoryTestConfig() rlenv.Config {
	cfg := testConfig()
	cfg.PlaceTargetItem = "minecraft:dirt"
	cfg.UsePlaceIngredientMemory = true
	return cfg
}

// placeIngredientObservationIndices mirror observation.go's own
// placeIngredientDx/Dy/Dz/Valid doc comment (indices 90-93).
const (
	placeIngredientDxIdx    = 90
	placeIngredientDyIdx    = 91
	placeIngredientDzIdx    = 92
	placeIngredientValidIdx = 93
)

func TestPlaceIngredientMemory_PopulatesWhenEnabledAndNotReady(t *testing.T) {
	inner := newFakeAgent(0, 0, 0) // holds none of PlaceTargetItem
	agent := &fakeIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 5, Y: 6, Z: 7}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), placeIngredientMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestIngredientLocationCalls) != 1 {
		t.Fatalf("NearestIngredientLocation calls = %d, want 1", len(agent.nearestIngredientLocationCalls))
	}
	if agent.nearestIngredientLocationCalls[0] != "minecraft:dirt" {
		t.Errorf("NearestIngredientLocation called with %q, want minecraft:dirt", agent.nearestIngredientLocationCalls[0])
	}

	if valid := obs.Values[placeIngredientValidIdx]; valid != 1 {
		t.Fatalf("placeIngredientValid = %v, want 1", valid)
	}
	if dx := obs.Values[placeIngredientDxIdx]; dx != 5 {
		t.Errorf("placeIngredientDx = %v, want 5", dx)
	}
	if dy := obs.Values[placeIngredientDyIdx]; dy != 6 {
		t.Errorf("placeIngredientDy = %v, want 6", dy)
	}
	if dz := obs.Values[placeIngredientDzIdx]; dz != 7 {
		t.Errorf("placeIngredientDz = %v, want 7", dz)
	}
}

func TestPlaceIngredientMemory_ZeroWhenDisabled(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	agent := &fakeIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 5, Y: 6, Z: 7}, found: true}

	cfg := placeIngredientMemoryTestConfig()
	cfg.UsePlaceIngredientMemory = false // explicitly disabled

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestIngredientLocationCalls) != 0 {
		t.Fatalf("NearestIngredientLocation calls = %d, want 0 when UsePlaceIngredientMemory is false", len(agent.nearestIngredientLocationCalls))
	}
	for i := placeIngredientDxIdx; i <= placeIngredientValidIdx; i++ {
		if obs.Values[i] != 0 {
			t.Errorf("obs.Values[%d] = %v, want 0 when UsePlaceIngredientMemory is false", i, obs.Values[i])
		}
	}
}

func TestPlaceIngredientMemory_ZeroWhenAlreadyHeld(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setPlaceTarget("minecraft:dirt", 1) // already holds it - placeStandaloneReady() is true
	agent := &fakeIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 5, Y: 6, Z: 7}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), placeIngredientMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestIngredientLocationCalls) != 0 {
		t.Fatalf("NearestIngredientLocation calls = %d, want 0 when placeStandaloneReady is already true", len(agent.nearestIngredientLocationCalls))
	}
	if valid := obs.Values[placeIngredientValidIdx]; valid != 0 {
		t.Fatalf("placeIngredientValid = %v, want 0 when placeStandaloneReady is already true", valid)
	}
}

func TestPlaceIngredientMemory_IgnoredDuringChainEpisode(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	agent := &fakeIngredientMemoryAgent{fakeAgent: inner, pos: models.V3{X: 5, Y: 6, Z: 7}, found: true}

	cfg := placeIngredientMemoryTestConfig()
	cfg.ChainStage = rlenv.ChainGather

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestIngredientLocationCalls) != 0 {
		t.Fatalf("NearestIngredientLocation calls = %d, want 0 during a chain episode", len(agent.nearestIngredientLocationCalls))
	}
	if valid := obs.Values[placeIngredientValidIdx]; valid != 0 {
		t.Fatalf("placeIngredientValid = %v, want 0 during a chain episode", valid)
	}
}

func TestPlaceIngredientMemory_NoOpWithoutCapability(t *testing.T) {
	agent := newFakeAgent(0, 0, 0) // plain fakeAgent: no NearestIngredientLocation

	env, err := rlenv.New(agent, actions.NewRegistry(), placeIngredientMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	for i := placeIngredientDxIdx; i <= placeIngredientValidIdx; i++ {
		if obs.Values[i] != 0 {
			t.Errorf("obs.Values[%d] = %v, want 0 (agent doesn't implement IngredientMemoryAgent)", i, obs.Values[i])
		}
	}
}
