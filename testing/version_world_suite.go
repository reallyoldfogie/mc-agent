package testing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/suite"
)

// VersionWorldSuite is the shared base for Phase 1 of
// docs/plans/integration-test-shared-server/00-plan.md: one server, started
// once in SetupSuite and stopped once in TearDownSuite, shared by every test
// method in a concrete suite keyed on (Version, WorldGen). This collapses
// "boot count = (test functions) x (versions)" down to
// "boot count = (versions) x (worldgen types actually used)" - see the plan
// document for the full rationale.
//
// testing/container_suite_test.go's ContainerTestSuite already proved this
// SetupSuite-starts-the-server-once / per-version-driver-loop pattern works
// in this codebase before this file existed; VersionWorldSuite generalizes
// it and adds the two things ContainerTestSuite didn't need: explicit
// WorldGen grouping (container tests only ever use one flat world) and
// per-test spatial working areas (container tests share one pre-built
// layout near spawn instead of needing fresh space per test).
//
// Embed this in a concrete suite type per (version, worldgen) combination.
// Each test method that needs its own agent and space should call
// SpawnWorkingAreaAgent rather than assuming it owns the whole world - that
// is what makes Phase 2 (t.Parallel() test methods sharing this one server)
// safe.
type VersionWorldSuite struct {
	suite.Suite

	// Version is the Minecraft version this suite's server runs. Set by the
	// driver function (see RunVersionWorldSuite) before suite.Run.
	Version string
	// WorldGen selects the world-gen mode for this suite's server. Every
	// test method in one suite instance shares this - a test that needs a
	// different world type belongs in a different suite, not this one (see
	// the plan's "Gap in both documents" note on WorldGen grouping: a single
	// SetupSuite-launched server has one fixed world type for its whole
	// lifetime).
	WorldGen WorldGenType

	Ctx       context.Context
	Cancel    context.CancelFunc
	Framework *Framework
	Inst      *TestInstance
}

// SetupSuite starts this suite's one shared server. Runs once, before any
// test method, regardless of how many test methods the concrete suite has.
func (s *VersionWorldSuite) SetupSuite() {
	s.Ctx, s.Cancel = context.WithTimeout(context.Background(), 45*time.Minute)

	var err error
	s.Framework, err = NewFramework()
	s.Require().NoError(err, "create framework")

	cfg := s.buildServerConfig()
	RequireIntegrationEnv(s.T(), cfg)

	s.T().Logf("VersionWorldSuite: starting shared server (version=%s worldgen=%s)", s.Version, s.WorldGen)
	start := time.Now()
	s.Inst, err = s.Framework.StartServer(s.Ctx, cfg)
	s.Require().NoError(err, "start shared server")
	s.T().Logf("VersionWorldSuite: server ready in %s (%s:%d)", time.Since(start), s.Inst.Server.Host, s.Inst.Server.HostServerPort)

	s.Require().NoError(ApplySharedServerGamerules(s.Ctx, s.Inst.RCON), "apply shared-server gamerules")
}

func (s *VersionWorldSuite) buildServerConfig() ServerConfig {
	var cfg ServerConfig
	if s.WorldGen == WorldGenFlat {
		cfg = SharedFlatWorldServerConfig()
	} else {
		cfg = SharedServerConfig()
		cfg.WorldGen = s.WorldGen
	}
	cfg.Version = s.Version
	cfg.PullImage = false
	return cfg
}

// teardownServer stops this suite's shared server. Deliberately NOT named
// TearDownSuite (testify's TearDownAllSuite interface method): testify's
// suite.Run defers TearDownSuite immediately after its own test-method loop
// returns (stretchr/testify/suite/suite.go's Run) - but t.Run returns as
// soon as a subtest calls t.Parallel(), before that subtest's body actually
// runs. For a suite with any t.Parallel()-marked test method (Phase 2 of
// docs/plans/integration-test-shared-server/00-plan.md), a real
// TearDownSuite would therefore stop the shared server *while parallel test
// methods are still queued, not yet run* - live-confirmed
// ("connection refused" dialing the already-stopped server - see
// docs/plans/integration-test-shared-server/04-phase2-parallelism.md for
// the exact failure). teardownServer is instead invoked via t.Cleanup() on
// the *outer* per-version t (see RunVersionWorldSuite), which Go guarantees
// runs only after every subtest of that t - parallel or not - has finished.
func (s *VersionWorldSuite) teardownServer() {
	if s.Inst != nil && s.Framework != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if err := s.Framework.StopServer(stopCtx, s.Inst, true); err != nil {
			s.T().Logf("warning: failed to stop shared server: %v", err)
		}
	}
	if s.Cancel != nil {
		s.Cancel()
	}
}

// WorkingAreaAgent bundles a spawned agent with the settled position it
// ended up at after being moved to its own working area, so test methods
// can compute destinations relative to that origin the same way pre-Phase-1
// tests computed them relative to GetEntityPos right after spawn.
type WorkingAreaAgent struct {
	*ManagedAgent
	Origin models.V3
}

// SpawnWorkingAreaAgent spawns a per-test agent (with replay recording, as
// DefaultAgentConfig/SpawnAgent already do for every pre-Phase-1 test) and
// moves it to its own, never-before-used working area (NextWorkingAreaOffset)
// along X, so concurrent or sequential test methods sharing this suite's one
// server never collide and random-terrain tests always get fresh, unexplored
// chunks.
//
// For WorldGenFlat/WorldGenControlled suites, Y is uniform everywhere, so
// the offset teleport keeps the agent's original spawn Y exactly - entirely
// safe. For WorldGenRandom suites, terrain height at the new X/Z is unknown
// (could be a mountain, a ravine, open ocean), so the teleport instead keeps
// the agent's *current* Y unchanged (vanilla's `~` syntax) rather than
// assuming the original spawn Y is safe ground 256+ blocks away, and lets
// gravity/collision settle the agent naturally afterward. This is a real,
// accepted risk shared with mc-rsi-trainer's own working-area design
// (../mc-rsi-trainer/pkg/parallelenv/parallelenv.go): unlike that repo's
// rlenv.WalkabilityAgent, this package does not (yet) verify the landing
// spot is standable/reachable before returning - see
// docs/plans/integration-test-shared-server/01-phase0-spatial-isolation.md
// for what was actually observed live and why WorldGenRandom suites are not
// (yet) marked t.Parallel()-safe in Phase 2 as a result.
//
// name is used as-is for the agent (and its Cam companion) - callers should
// keep it short and distinct per test method so replay filenames and RCON
// target names stay unambiguous.
func (s *VersionWorldSuite) SpawnWorkingAreaAgent(name, replayPrefix string) (*WorkingAreaAgent, error) {
	agentCfg := DefaultAgentConfig(
		name,
		fmt.Sprintf("%s:%d", s.Inst.Server.Host, s.Inst.Server.HostServerPort),
		s.Version,
	)
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = normalizeReplayOutput(s.Version, fmt.Sprintf("%s_%s_%s.mcpr", replayPrefix, s.Version, time.Now().Format("20060102_150405")), agentCfg.Name)

	managed, err := s.Framework.SpawnAgent(s.Ctx, s.Inst, agentCfg)
	if err != nil {
		return nil, err
	}

	// Give the agent time to connect and receive its initial spawn position
	// before reading/overriding it - matches every pre-Phase-1 test's own
	// time.Sleep(5 * time.Second) after SpawnAgent.
	time.Sleep(5 * time.Second)

	spawnX, spawnY, spawnZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, managed.Name)
	if err != nil {
		return nil, fmt.Errorf("get spawn position for %s: %w", managed.Name, err)
	}

	offsetX, offsetZ := NextWorkingAreaOffset()
	if offsetX != 0 || offsetZ != 0 {
		targetX := spawnX + offsetX
		targetZ := spawnZ + offsetZ
		if s.WorldGen == WorldGenFlat || s.WorldGen == WorldGenControlled {
			if _, err := s.Inst.RCON.Teleport(s.Ctx, managed.Name, targetX, spawnY, targetZ).Exec(s.Ctx); err != nil {
				return nil, fmt.Errorf("teleport %s to working area: %w", managed.Name, err)
			}
		} else {
			// Keep current Y ("~") - see doc comment above.
			cmd := fmt.Sprintf("tp %s %.2f ~ %.2f", managed.Name, targetX, targetZ)
			if _, err := s.Inst.RCON.Exec(s.Ctx, cmd); err != nil {
				return nil, fmt.Errorf("teleport %s to working area: %w", managed.Name, err)
			}
		}
		// Let physics settle after the offset teleport before trusting the
		// agent's position.
		time.Sleep(2 * time.Second)
	}

	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, managed.Name)
	if err != nil {
		return nil, fmt.Errorf("get settled working-area position for %s: %w", managed.Name, err)
	}

	return &WorkingAreaAgent{ManagedAgent: managed, Origin: models.V3{X: finalX, Y: finalY, Z: finalZ}}, nil
}

// RunVersionWorldSuite is the top-level driver: builds a fresh instance of
// the concrete suite type via newSuite for each version in versions,
// running each as its own t.Run subtest via suite.Run (so SetupSuite starts
// exactly one server per version, per the plan's Phase 1). newSuite is
// called once per version and must return a suite embedding
// VersionWorldSuite with WorldGen already set - RunVersionWorldSuite sets
// only Version before running it.
//
// Server teardown is registered via t.Cleanup() on the per-version t,
// BEFORE suite.Run is called, not left to testify's own TearDownSuite hook
// - see VersionWorldSuite.teardownServer's doc comment for why that
// distinction matters for any suite with a t.Parallel()-marked test method.
func RunVersionWorldSuite(t *testing.T, versions []models.VersionTest, newSuite func() suite.TestingSuite) {
	for _, tt := range versions {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			s := newSuite()
			if vw, ok := s.(interface{ SetVersion(string) }); ok {
				vw.SetVersion(tt.MCVersion)
			}
			if vw, ok := s.(interface{ teardownServer() }); ok {
				t.Cleanup(vw.teardownServer)
			}
			suite.Run(t, s)
		})
	}
}

// SetVersion implements the setter RunVersionWorldSuite uses to assign each
// per-version suite instance its Version before suite.Run.
func (s *VersionWorldSuite) SetVersion(version string) {
	s.Version = version
}
