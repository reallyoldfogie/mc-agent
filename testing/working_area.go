package testing

import (
	"context"
	"sync"

	"github.com/reallyoldfogie/mc-client-test-go/testenv"
)

// defaultWorkingAreaSeparationBlocks matches mc-rsi-trainer's
// parallelenv.WorkingAreaOffset default separation (16 chunks / 256 blocks)
// - see ../mc-rsi-trainer/pkg/parallelenv/parallelenv.go - so this package's
// working areas are spaced the same distance apart that repo already
// live-validated as safe against a server's own reduced
// VIEW_DISTANCE/SIMULATION_DISTANCE (see docs/plans/integration-test-shared-server/00-plan.md).
const defaultWorkingAreaSeparationBlocks = 256

var (
	workingAreaMu    sync.Mutex
	workingAreaIndex int
)

// NextWorkingAreaOffset hands out the next non-overlapping (x, z) offset for
// a test to claim as its own working area on a shared server - Phase 0 of
// docs/plans/integration-test-shared-server/00-plan.md. Offsets are
// monotonically increasing and spaced defaultWorkingAreaSeparationBlocks
// apart along X, so concurrent or sequential test methods sharing one server
// never spatially collide, and random-terrain tests always land on chunks no
// previous test has touched (fresh terrain by construction, not by cleanup -
// see the plan's Phase 0 rationale for why this eliminates most of the
// per-test reset logic earlier proposals assumed was necessary). The counter
// is package-level and mutex-guarded, not per-suite: it needs to stay
// globally monotonic within one `go test` process so two suites running
// concurrently (Phase 2) still never collide with each other, not just
// within their own suite.
func NextWorkingAreaOffset() (x, z float64) {
	workingAreaMu.Lock()
	defer workingAreaMu.Unlock()
	offset := workingAreaIndex
	workingAreaIndex++
	return float64(offset * defaultWorkingAreaSeparationBlocks), 0
}

// sharedServerGamerules disables ambient simulation load that would
// otherwise scale with however many working areas are active on one shared
// server: fire spread, trader/patrol spawning, phantom-triggering insomnia,
// and random block ticks (crop growth, leaf decay, etc.). Mirrors
// mc-rsi-trainer's sharedServerGamerules
// (../mc-rsi-trainer/cmd/rsi-train/main.go). Layered on top of
// StartServer's own existing defaults (doWeatherCycle=false,
// doMobSpawning=false - see framework.go's StartServer) rather than
// duplicating them.
var sharedServerGamerules = map[string]string{
	"doFireTick":       "false",
	"doTraderSpawning": "false",
	"doPatrolSpawning": "false",
	"doInsomnia":       "false",
	"randomTickSpeed":  "0",
}

// ApplySharedServerGamerules applies sharedServerGamerules over RCON. Call
// once per server, after StartServer, before spawning any agents that will
// share it across multiple working areas.
func ApplySharedServerGamerules(ctx context.Context, rcon testenv.RCONHelper) error {
	results := make([]*testenv.RCONResult, 0, len(sharedServerGamerules))
	for rule, value := range sharedServerGamerules {
		results = append(results, rcon.SetGamerule(ctx, rule, value))
	}
	_, err := rcon.ExecuteMany(ctx, results...)
	return err
}

// SharedServerConfig returns a ServerConfig for a server that many tests or
// agents will share (Phase 1/2 of docs/plans/integration-test-shared-server/00-plan.md):
// explicit reduced VIEW_DISTANCE/SIMULATION_DISTANCE so ambient chunk
// simulation doesn't scale with however many working areas are in use.
// Note: as of this writing StartServer already defaults to these same
// values (VIEW_DISTANCE=6/SIMULATION_DISTANCE=4) when ExtraEnv doesn't set
// them, matching mc-rsi-trainer's own shared-server range (4-6) - this
// function makes that explicit and gives shared-server callers one place to
// diverge from the single-test default later without relying on that
// implicit fallback.
func SharedServerConfig() ServerConfig {
	cfg := DefaultServerConfig()
	cfg.ExtraEnv = map[string]string{
		"VIEW_DISTANCE":       "6",
		"SIMULATION_DISTANCE": "4",
	}
	return cfg
}

// SharedFlatWorldServerConfig is SharedServerConfig with flat-world
// generation, for suites whose test methods need predictable terrain
// (movement-command tests that don't exercise pathfinding).
func SharedFlatWorldServerConfig() ServerConfig {
	cfg := SharedServerConfig()
	cfg.WorldGen = WorldGenFlat
	return cfg
}
