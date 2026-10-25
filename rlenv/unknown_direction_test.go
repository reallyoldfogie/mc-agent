package rlenv_test

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/cRL-go/pkg/rl"
	"github.com/reallyoldfogie/mc-agent/rlenv"
)

// buriedTestConfig is testConfig with a buried mine target configured —
// GoToTargetDisabled since mine_buried/chain_mine have no separate goto
// component (see mc-rsi-trainer's mineBuriedTask), and MineTargetBuried so
// mineTargetMemoryEnabled() (and therefore the exploration-signal block)
// is active.
func buriedTestConfig() rlenv.Config {
	cfg := testConfig()
	cfg.GoToTargetDisabled = true
	cfg.MineTargetBlock = "minecraft:coal_ore"
	cfg.MineTargetBuried = true
	cfg.MineSearchRadius = 10
	return cfg
}

// TestUnknownDirectionValidWhileBuriedTargetUnseen confirms indices 21-24
// (unknownDx/Dy/Dz/unknownValid, see observation.go's own doc comment)
// carry a real signal from the very first observation of a mine_buried-
// style episode, before anything has ever been seen — the whole point of
// this feature (docs/plans/14-buried-block-mining-and-memory-map.md's
// option 2 in mc-rsi-trainer): mine_buried gives the policy nothing else
// to go on until pure luck exposes the target.
func TestUnknownDirectionValidWhileBuriedTargetUnseen(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true) // buried: FindVisibleBlock never finds it
	env := newTestEnvironment(t, agent, buriedTestConfig())

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if valid := obs.Values[24]; valid != 1 {
		t.Fatalf("unknownValid = %v, want 1 (buried target never seen, quadrants remain to explore)", valid)
	}
	dx, dy, dz := obs.Values[21], obs.Values[22], obs.Values[23]
	if dx == 0 && dy == 0 && dz == 0 {
		t.Fatalf("unknownDx/Dy/Dz = (0,0,0) while unknownValid=1 — must point somewhere")
	}
	// Half of MineSearchRadius (10/2=5) bounds each component: the
	// exploration target never advertises a point outside the configured
	// search volume.
	for i, v := range []float32{dx, dy, dz} {
		if v > 5 || v < -5 {
			t.Fatalf("unknownD%s = %v, want within +/-5 (half of MineSearchRadius=10)", "xyz"[i:i+1], v)
		}
	}
}

// TestUnknownDirectionZeroOnceTargetSeen confirms the exploration signal
// hands off to mineDx/Dy/Dz (the memory-map-backed, exact signal) the
// moment the buried target is ever actually glimpsed, rather than the two
// signals competing — see observationUnknownDirection's own doc comment.
func TestUnknownDirectionZeroOnceTargetSeen(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlock("minecraft:coal_ore", 3, 0, 0) // visible from the start
	env := newTestEnvironment(t, agent, buriedTestConfig())

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if valid := obs.Values[24]; valid != 0 {
		t.Fatalf("unknownValid = %v, want 0 (target already visible this episode)", valid)
	}
	if dx, dy, dz := obs.Values[21], obs.Values[22], obs.Values[23]; dx != 0 || dy != 0 || dz != 0 {
		t.Fatalf("unknownDx/Dy/Dz = (%v,%v,%v), want (0,0,0) once the target has been seen", dx, dy, dz)
	}
	// mineDx/Dy/Dz (indices 9-11) should carry the real signal instead.
	if visible := obs.Values[12]; visible != 1 {
		t.Fatalf("mineVisible = %v, want 1", visible)
	}
}

// TestUnknownDirectionInactiveOutsideMineTargetMemory confirms the
// exploration-signal block stays inert for every task that isn't
// mine_buried/chain_mine — matches mineDx/Dy/Dz's own "zero unless this
// episode's task actually uses it" convention.
func TestUnknownDirectionInactiveOutsideMineTargetMemory(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	cfg := testConfig() // ordinary goto episode, no mine target at all
	env := newTestEnvironment(t, agent, cfg)

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if valid := obs.Values[24]; valid != 0 {
		t.Fatalf("unknownValid = %v, want 0 (not a mine_buried/chain_mine episode)", valid)
	}
	if dx, dy, dz := obs.Values[21], obs.Values[22], obs.Values[23]; dx != 0 || dy != 0 || dz != 0 {
		t.Fatalf("unknownDx/Dy/Dz = (%v,%v,%v), want (0,0,0)", dx, dy, dz)
	}
}

// TestUnknownDirectionTracksExploredQuadrants confirms the signal actually
// moves on as the bot visits new ground: once every horizontal quadrant
// around the mine-search anchor has been physically entered, unknownValid
// drops back to 0 rather than pointing somewhere already-explored. Only
// X/Z matter for quadrant membership (see exploredQuadrants' own doc
// comment on why this is downward-only, not a per-quadrant Y choice) —
// every position below stays at the same Y to isolate that.
func TestUnknownDirectionTracksExploredQuadrants(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	cfg := buriedTestConfig()
	cfg.MineSearchRadius = 4 // small radius - 4 quadrants is a quick walk
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Visit every one of the 4 horizontal quadrants around the anchor
	// (0,0,0) in turn.
	offsets := [][2]float64{{1, 1}, {-1, 1}, {1, -1}, {-1, -1}}
	var last rl.Observation
	for _, off := range offsets {
		agent.setPosition(off[0], 0, off[1])
		result, err := env.Step(context.Background(), rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step: %v", err)
		}
		last = result.Observation
	}
	if valid := last.Values[24]; valid != 0 {
		t.Fatalf("unknownValid = %v, want 0 once every quadrant has been entered", valid)
	}
}
