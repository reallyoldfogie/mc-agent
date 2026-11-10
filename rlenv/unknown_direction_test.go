package rlenv_test

import (
	"context"
	"math"
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
// moves on as the bot visits new ground: once every horizontal quadrant's
// own target point around the mine-search anchor has genuinely been
// reached (within Config.ArrivalThreshold — see markExploredQuadrant's own
// doc comment for why reaching the point is required, not merely crossing
// into that quadrant's half of the anchor plane), unknownValid drops back
// to 0 rather than pointing somewhere already-explored.
func TestUnknownDirectionTracksExploredQuadrants(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	cfg := buriedTestConfig()
	cfg.MineSearchRadius = 4 // small radius - 4 quadrants is a quick walk
	env := newTestEnvironment(t, agent, cfg)

	if _, err := env.Reset(context.Background()); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	// Visit each of the 4 horizontal quadrants' own target points in turn
	// — anchor (0,0,0) ± half (MineSearchRadius/2 = 2) in X/Z, Y always
	// anchor-half (markExploredQuadrant/observationUnknownDirection's own
	// downward bias) — not just some arbitrary point inside each quadrant.
	offsets := [][2]float64{{2, 2}, {-2, 2}, {2, -2}, {-2, -2}}
	var last rl.Observation
	for _, off := range offsets {
		agent.setPosition(off[0], -2, off[1])
		result, err := env.Step(context.Background(), rlenv.ActionWait)
		if err != nil {
			t.Fatalf("Step: %v", err)
		}
		last = result.Observation
	}
	if valid := last.Values[24]; valid != 0 {
		t.Fatalf("unknownValid = %v, want 0 once every quadrant's target point has been reached", valid)
	}
}

// TestUnknownDirectionUsesConfiguredExploreDepth confirms MineBuriedExploreDepth
// decouples waypoint Y from MineSearchRadius/2 (docs/plans/21-buried-mining-success.md).
func TestUnknownDirectionUsesConfiguredExploreDepth(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	cfg := buriedTestConfig()
	cfg.MineSearchRadius = 16
	cfg.MineBuriedExploreDepth = 3
	env := newTestEnvironment(t, agent, cfg)

	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if valid := obs.Values[24]; valid != 1 {
		t.Fatalf("unknownValid = %v, want 1", valid)
	}
	dy := obs.Values[22]
	// From anchor Y=0, explore depth 3 => targetY=-3 => dy around -3 (not -8).
	if dy > -2.5 || dy < -3.5 {
		t.Fatalf("unknownDy = %v, want about -3 with MineBuriedExploreDepth=3 (not -8 from radius/2)", dy)
	}

	cfg.MineBuriedExploreDepth = 10
	env = newTestEnvironment(t, agent, cfg)
	obs, err = env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	dy = obs.Values[22]
	if dy > -9.5 || dy < -10.5 {
		t.Fatalf("unknownDy = %v, want about -10 with MineBuriedExploreDepth=10", dy)
	}
}

// TestUnknownDirectionUsesConfiguredExploreRadius confirms
// MineBuriedExploreRadius decouples the quadrant waypoints' horizontal
// footprint from MineSearchRadius/2 - see MineBuriedExploreRadius's own
// doc comment (found live 2026-11-09: a 12x12 patrol box at
// MineSearchRadius 12 routinely missed ore seeded across a much larger
// 65x65 working area).
func TestUnknownDirectionUsesConfiguredExploreRadius(t *testing.T) {
	agent := newFakeAgent(0, 0, 0)
	agent.setMineBlockHidden(true)
	cfg := buriedTestConfig()
	cfg.MineSearchRadius = 12
	cfg.MineBuriedExploreRadius = 64

	env := newTestEnvironment(t, agent, cfg)
	obs, err := env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if valid := obs.Values[24]; valid != 1 {
		t.Fatalf("unknownValid = %v, want 1", valid)
	}
	// From anchor (0,0,0), MineBuriedExploreRadius 64 => half=32, so the
	// nearest quadrant point's horizontal delta should be ~32 on each
	// axis, not 6 (MineSearchRadius 12's own legacy radius/2).
	dx, dz := math.Abs(float64(obs.Values[21])), math.Abs(float64(obs.Values[23]))
	if dx < 31.5 || dx > 32.5 {
		t.Fatalf("|unknownDx| = %v, want about 32 with MineBuriedExploreRadius=64 (not 6 from MineSearchRadius/2)", dx)
	}
	if dz < 31.5 || dz > 32.5 {
		t.Fatalf("|unknownDz| = %v, want about 32 with MineBuriedExploreRadius=64 (not 6 from MineSearchRadius/2)", dz)
	}

	// Unset (0) falls back to the legacy MineSearchRadius/2 behavior.
	cfg.MineBuriedExploreRadius = 0
	env = newTestEnvironment(t, agent, cfg)
	obs, err = env.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset: %v", err)
	}
	dx, dz = math.Abs(float64(obs.Values[21])), math.Abs(float64(obs.Values[23]))
	if dx < 5.5 || dx > 6.5 {
		t.Fatalf("|unknownDx| = %v, want about 6 with MineBuriedExploreRadius unset (MineSearchRadius 12 / 2)", dx)
	}
	if dz < 5.5 || dz > 6.5 {
		t.Fatalf("|unknownDz| = %v, want about 6 with MineBuriedExploreRadius unset (MineSearchRadius 12 / 2)", dz)
	}
}
