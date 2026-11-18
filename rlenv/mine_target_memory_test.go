package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/actions"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// mineTargetMemoryTestConfig poses a mine task with UseMineTargetMemory
// enabled.
func mineTargetMemoryTestConfig() rlenv.Config {
	cfg := testConfig()
	cfg.MineTargetBlock = "minecraft:iron_ore"
	cfg.UseMineTargetMemory = true
	return cfg
}

func TestMineTargetMemory_FallsBackWhenLiveLOSFails(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true) // never resolvable via live FindVisibleBlock
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 3, Y: 4, Z: 5}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), mineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestRememberedBlockCalls) != 1 {
		t.Fatalf("NearestRememberedBlock calls = %d, want 1", len(agent.nearestRememberedBlockCalls))
	}
	if agent.nearestRememberedBlockCalls[0] != "minecraft:iron_ore" {
		t.Errorf("NearestRememberedBlock called with %q, want minecraft:iron_ore", agent.nearestRememberedBlockCalls[0])
	}

	// mineVisible (index 12) and mineDx/Dy/Dz (9-11) are the existing
	// fields this fallback reuses - no new observation dims.
	if visible := obs.Values[12]; visible != 1 {
		t.Fatalf("mineVisible = %v, want 1 (filled in from memory)", visible)
	}
	if dx := obs.Values[9]; dx != 3 {
		t.Errorf("mineDx = %v, want 3", dx)
	}
	if dy := obs.Values[10]; dy != 4 {
		t.Errorf("mineDy = %v, want 4", dy)
	}
	if dz := obs.Values[11]; dz != 5 {
		t.Errorf("mineDz = %v, want 5", dz)
	}
}

func TestMineTargetMemory_LiveSightingTakesPrecedence(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlock("minecraft:iron_ore", 1, 1, 1) // actually visible live
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 99, Y: 99, Z: 99}, found: true}

	env, err := rlenv.New(agent, actions.NewRegistry(), mineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestRememberedBlockCalls) != 0 {
		t.Fatalf("NearestRememberedBlock calls = %d, want 0 (live LOS already found it)", len(agent.nearestRememberedBlockCalls))
	}
	if dx := obs.Values[9]; dx != 1 {
		t.Errorf("mineDx = %v, want 1 (the live sighting, not the memory one)", dx)
	}
}

func TestMineTargetMemory_IgnoredWhenDisabled(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 3, Y: 4, Z: 5}, found: true}

	cfg := mineTargetMemoryTestConfig()
	cfg.UseMineTargetMemory = false

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if len(agent.nearestRememberedBlockCalls) != 0 {
		t.Fatalf("NearestRememberedBlock calls = %d, want 0 when UseMineTargetMemory is false", len(agent.nearestRememberedBlockCalls))
	}
	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible = %v, want 0", visible)
	}
}

func TestMineTargetMemory_OutsideSearchRadiusIsRejected(t *testing.T) {
	inner := newFakeAgent(0, 0, 0)
	inner.setMineBlockHidden(true)
	agent := &fakeMineTargetMemoryAgent{fakeAgent: inner, pos: models.V3{X: 1000, Y: 0, Z: 0}, found: true}

	cfg := mineTargetMemoryTestConfig()
	cfg.MineSearchRadius = 8

	env, err := rlenv.New(agent, actions.NewRegistry(), cfg)
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible = %v, want 0 (remembered position is far outside MineSearchRadius)", visible)
	}
}

func TestMineTargetMemory_NoOpWithoutCapability(t *testing.T) {
	agent := newFakeAgent(0, 0, 0) // plain fakeAgent: no NearestRememberedBlock
	agent.setMineBlockHidden(true)

	env, err := rlenv.New(agent, actions.NewRegistry(), mineTargetMemoryTestConfig())
	if err != nil {
		t.Fatalf("rlenv.New: %v", err)
	}
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}

	if visible := obs.Values[12]; visible != 0 {
		t.Fatalf("mineVisible = %v, want 0 (agent doesn't implement MineTargetMemoryAgent)", visible)
	}
}
