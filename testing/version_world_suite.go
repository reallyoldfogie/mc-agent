package testing

import (
	"context"
	"fmt"
	"sync"
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

	// Difficulty overrides SharedServerConfig/SharedFlatWorldServerConfig's
	// own Peaceful default for this suite's server, when set (zero value
	// leaves the Peaceful default in place - see buildServerConfig). Added
	// for docs/plans/integration-test-shared-server/08-phase1-effects-conversion.md:
	// several pre-conversion test files pin an explicit non-Peaceful
	// difficulty (DifficultyEasy/DifficultyNormal) - some for reasons the
	// specific assertions don't actually depend on (preserved anyway, to
	// avoid silently diverging from a previously-passing test's own
	// conditions), others load-bearing (entity_interaction_test.go's
	// zombie-attack tests need DifficultyNormal specifically - Peaceful
	// despawns hostile mobs outright, which would break them outright, not
	// just subtly). Like WorldGen, every test method in one suite instance
	// shares this - a test that needs a different difficulty belongs in a
	// different suite.
	Difficulty Difficulty

	// GameMode overrides SharedServerConfig/SharedFlatWorldServerConfig's
	// own "survival" default for this suite's server, when set (zero value
	// leaves the survival default in place - see buildServerConfig). Added
	// for docs/plans/integration-test-shared-server/15-phase1-flying-conversion.md:
	// flying_ability_test.go/flying_command_test.go/flying_physics_test.go
	// all need GameModeCreative specifically (a server-granted ability,
	// not something a bot can toggle on its own in survival). Like
	// WorldGen/Difficulty, every test method in one suite instance shares
	// this - a test that needs a different game mode (e.g.
	// flying_ability_test.go's own "survival denies flying" scenario)
	// belongs in a different suite.
	GameMode GameMode

	// ExtraEnv adds to (never replaces) SharedServerConfig/SharedFlatWorldServerConfig's
	// own ExtraEnv (VIEW_DISTANCE/SIMULATION_DISTANCE/ENABLE_COMMAND_BLOCK) - see
	// buildServerConfig. Added for
	// docs/plans/integration-test-shared-server/23-phase1-inventory-conversion.md:
	// inventory_integration_test.go's pre-conversion server config set
	// FORCE_GAMEMODE=true (also used by container_suite_test.go, the
	// pre-VersionWorldSuite prior art this whole shared-server pattern
	// generalized from), and no evidence was found that dropping it is safe -
	// keys here win if they collide with the shared defaults.
	ExtraEnv map[string]string

	Ctx       context.Context
	Cancel    context.CancelFunc
	Framework *Framework
	Inst      *TestInstance

	// usedNames guards against a real, live-confirmed trap (see
	// docs/plans/integration-test-shared-server/07-phase1-follow-conversion.md):
	// two test methods sharing this suite's one server that spawn an agent
	// under the SAME literal name don't each get a fresh spawn - vanilla
	// Minecraft persists a player's position across reconnects under one
	// username, so the second call resumes wherever the first one's agent
	// last stood (after however far it moved during its own test), and the
	// working-area-offset math then compounds on top of that already-wrong
	// position instead of a genuine spawn point. spawnAndReadPosition checks
	// and records every name here so this fails loudly at spawn time
	// instead of silently producing a contaminated, hard-to-diagnose
	// starting position.
	usedNamesMu sync.Mutex
	usedNames   map[string]bool
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
	if s.Difficulty != "" {
		cfg.Difficulty = s.Difficulty
	}
	if s.GameMode != "" {
		cfg.GameMode = s.GameMode
	}
	for k, v := range s.ExtraEnv {
		if cfg.ExtraEnv == nil {
			cfg.ExtraEnv = make(map[string]string, len(s.ExtraEnv))
		}
		cfg.ExtraEnv[k] = v
	}
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

// spawnAndReadPosition spawns a per-test agent (with replay recording, as
// DefaultAgentConfig/SpawnAgent already do for every pre-Phase-1 test),
// waits for it to connect, and returns its natural post-spawn position
// (read via GetEntityPos) - before any working-area placement. Shared by
// SpawnWorkingAreaAgent (which then claims a brand-new working area) and
// SpawnAgentNear (which instead places the agent near an area a different
// call already claimed).
//
// name is used as-is for the agent (and its Cam companion, when enabled) -
// callers should keep it short and distinct per test method so replay
// filenames and RCON target names stay unambiguous.
func (s *VersionWorldSuite) spawnAndReadPosition(name, replayPrefix string) (*ManagedAgent, models.V3, error) {
	return s.spawnAndReadPositionOpts(name, replayPrefix, true)
}

// spawnAndReadPositionOpts is spawnAndReadPosition with enableCam exposed -
// added for attribute_modifier_test.go's TestSpeedEffectModifiesTrackedAttribute
// (see docs/plans/integration-test-shared-server/20-phase1-attribute-modifier-conversion.md),
// which tracks a second agent via NearestPlayerInfo and needs that agent (and
// the querying agent) to have no Cam companion nearby to be mistaken for the
// "nearest player" - the same real concern 00-plan.md already flags for
// perception_nearest_player_test.go. A separate function rather than adding
// a parameter to spawnAndReadPosition/SpawnWorkingAreaAgent themselves, so
// every existing caller's call site is untouched.
func (s *VersionWorldSuite) spawnAndReadPositionOpts(name, replayPrefix string, enableCam bool) (*ManagedAgent, models.V3, error) {
	s.usedNamesMu.Lock()
	if s.usedNames == nil {
		s.usedNames = make(map[string]bool)
	}
	if s.usedNames[name] {
		s.usedNamesMu.Unlock()
		return nil, models.V3{}, fmt.Errorf(
			"spawnAndReadPosition: agent name %q already used earlier in this suite run - "+
				"reusing a name across test methods sharing one server means the second spawn "+
				"resumes the first one's saved position instead of a fresh spawn (see "+
				"VersionWorldSuite.usedNames' doc comment); give this test method's agent its own "+
				"distinct name instead", name)
	}
	s.usedNames[name] = true
	s.usedNamesMu.Unlock()

	agentCfg := DefaultAgentConfig(
		name,
		fmt.Sprintf("%s:%d", s.Inst.Server.Host, s.Inst.Server.HostServerPort),
		s.Version,
	)
	agentCfg.EnableCamAgent = enableCam
	agentCfg.EnableReplay = true
	agentCfg.ReplayOutput = normalizeReplayOutput(s.Version, fmt.Sprintf("%s_%s_%s.mcpr", replayPrefix, s.Version, time.Now().Format("20060102_150405")), agentCfg.Name)

	managed, err := s.Framework.SpawnAgent(s.Ctx, s.Inst, agentCfg)
	if err != nil {
		return nil, models.V3{}, err
	}

	// Disconnect this agent (and its Cam companion, via ManagedAgent.Stop's
	// own cascade) when THIS test method finishes, not when the whole suite
	// does. Every pre-Phase-1 test got this for free: each test owned its
	// own server/container, so an agent simply died with it. A shared
	// server survives across every test method in this suite, so without
	// this, agents from earlier methods stay connected and accumulate for
	// the suite's entire run - live-confirmed to matter, not just
	// theoretical: docs/plans/integration-test-shared-server/07-phase1-follow-conversion.md
	// found TestSingleAgent missing its 1.0-block arrival tolerance by a
	// hair (1.02) running 3rd in FollowFlatSuite, with ~14 stale
	// connections still on the server from the two multi-agent tests ahead
	// of it in alphabetical run order.
	t := s.T()
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		if err := managed.Stop(stopCtx); err != nil {
			t.Logf("warning: failed to stop agent %s: %v", managed.Name, err)
		}
	})

	// Give the agent time to connect and receive its initial spawn position
	// before reading it - matches every pre-Phase-1 test's own
	// time.Sleep(5 * time.Second) after SpawnAgent.
	time.Sleep(5 * time.Second)

	spawnX, spawnY, spawnZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, managed.Name)
	if err != nil {
		return nil, models.V3{}, fmt.Errorf("get spawn position for %s: %w", managed.Name, err)
	}

	return managed, models.V3{X: spawnX, Y: spawnY, Z: spawnZ}, nil
}

// teleportAndSettle moves agent to (targetX, targetZ), then re-reads its
// settled position after a short physics-settle delay. Y handling depends
// on s.WorldGen - see SpawnWorkingAreaAgent's doc comment for the full
// rationale (flat/controlled terrain is uniform, so an explicit
// fallbackY - the agent's own pre-teleport Y - is safe; random terrain's Y
// is unknown at the target, so the agent's *current* Y is kept via vanilla's
// `~` syntax instead).
func (s *VersionWorldSuite) teleportAndSettle(agentName string, targetX, targetZ, fallbackY float64) (models.V3, error) {
	if s.WorldGen == WorldGenFlat || s.WorldGen == WorldGenControlled {
		if _, err := s.Inst.RCON.Teleport(s.Ctx, agentName, targetX, fallbackY, targetZ).Exec(s.Ctx); err != nil {
			return models.V3{}, fmt.Errorf("teleport %s to working area: %w", agentName, err)
		}
	} else {
		cmd := fmt.Sprintf("tp %s %.2f ~ %.2f", agentName, targetX, targetZ)
		if _, err := s.Inst.RCON.Exec(s.Ctx, cmd); err != nil {
			return models.V3{}, fmt.Errorf("teleport %s to working area: %w", agentName, err)
		}
	}
	// Let physics settle after the teleport before trusting the agent's
	// position.
	time.Sleep(2 * time.Second)

	finalX, finalY, finalZ, err := s.Inst.RCON.GetEntityPos(s.Ctx, agentName)
	if err != nil {
		return models.V3{}, fmt.Errorf("get settled position for %s: %w", agentName, err)
	}
	return models.V3{X: finalX, Y: finalY, Z: finalZ}, nil
}

// SpawnWorkingAreaAgent spawns a per-test agent and moves it to its own,
// never-before-used working area (NextWorkingAreaOffset) along X, so
// concurrent or sequential test methods sharing this suite's one server
// never collide and random-terrain tests always get fresh, unexplored
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
// This claims a NEW working area every call - for a multi-agent test (e.g.
// a leader and one or more followers that need to start near each other,
// not each independently offset 256+ blocks apart), only the FIRST agent in
// a test should be spawned this way; every other agent in that same test
// belongs in the same claimed area and should use SpawnAgentNear against
// this call's own Origin instead - see SpawnAgentNear's doc comment and
// docs/plans/integration-test-shared-server/07-phase1-follow-conversion.md
// for why this distinction matters in practice, live-confirmed.
func (s *VersionWorldSuite) SpawnWorkingAreaAgent(name, replayPrefix string) (*WorkingAreaAgent, error) {
	return s.spawnWorkingAreaAgentOpts(name, replayPrefix, true)
}

// SpawnWorkingAreaAgentNoCam is SpawnWorkingAreaAgent without a Cam
// companion - for a test that queries NearestPlayerInfo (or anything else
// that resolves "the nearest player") and needs to guarantee no companion
// agent nearby could be mistaken for the tracked one. See
// spawnAndReadPositionOpts' doc comment for why this exists as a separate
// function rather than a parameter on the original.
func (s *VersionWorldSuite) SpawnWorkingAreaAgentNoCam(name, replayPrefix string) (*WorkingAreaAgent, error) {
	return s.spawnWorkingAreaAgentOpts(name, replayPrefix, false)
}

func (s *VersionWorldSuite) spawnWorkingAreaAgentOpts(name, replayPrefix string, enableCam bool) (*WorkingAreaAgent, error) {
	managed, spawnPos, err := s.spawnAndReadPositionOpts(name, replayPrefix, enableCam)
	if err != nil {
		return nil, err
	}

	offsetX, offsetZ := NextWorkingAreaOffset()
	if offsetX == 0 && offsetZ == 0 {
		// The very first claim in a process needs no teleport at all - the
		// agent's own natural spawn point already IS the working area.
		return &WorkingAreaAgent{ManagedAgent: managed, Origin: spawnPos}, nil
	}

	origin, err := s.teleportAndSettle(managed.Name, spawnPos.X+offsetX, spawnPos.Z+offsetZ, spawnPos.Y)
	if err != nil {
		return nil, err
	}
	return &WorkingAreaAgent{ManagedAgent: managed, Origin: origin}, nil
}

// SpawnAgentNear spawns a per-test agent WITHOUT claiming a new working
// area of its own - instead teleporting it to (dx, dz) relative to origin,
// an already-claimed WorkingAreaAgent.Origin from an earlier
// SpawnWorkingAreaAgent call in the SAME test. Use this for every agent
// after the first in a multi-agent test, so they land near each other
// inside one shared working area rather than each getting their own
// independent 256-block-separated region (which would defeat the point of
// a "leader" and "follower" test - see the doc comment on
// SpawnWorkingAreaAgent and
// docs/plans/integration-test-shared-server/07-phase1-follow-conversion.md).
//
// Y handling mirrors SpawnWorkingAreaAgent: origin.Y for
// WorldGenFlat/WorldGenControlled (uniform terrain, safe), the newly-spawned
// agent's own natural Y (via `~`) for WorldGenRandom (origin.Y could be far
// from this agent's own spawn terrain height, since origin itself may have
// come from a teleport elsewhere).
func (s *VersionWorldSuite) SpawnAgentNear(name, replayPrefix string, origin models.V3, dx, dz float64) (*WorkingAreaAgent, error) {
	return s.spawnAgentNearOpts(name, replayPrefix, origin, dx, dz, true)
}

// SpawnAgentNearNoCam is SpawnAgentNear without a Cam companion - see
// SpawnWorkingAreaAgentNoCam's doc comment for why this exists.
func (s *VersionWorldSuite) SpawnAgentNearNoCam(name, replayPrefix string, origin models.V3, dx, dz float64) (*WorkingAreaAgent, error) {
	return s.spawnAgentNearOpts(name, replayPrefix, origin, dx, dz, false)
}

func (s *VersionWorldSuite) spawnAgentNearOpts(name, replayPrefix string, origin models.V3, dx, dz float64, enableCam bool) (*WorkingAreaAgent, error) {
	managed, spawnPos, err := s.spawnAndReadPositionOpts(name, replayPrefix, enableCam)
	if err != nil {
		return nil, err
	}

	fallbackY := origin.Y
	if s.WorldGen != WorldGenFlat && s.WorldGen != WorldGenControlled {
		fallbackY = spawnPos.Y
	}

	pos, err := s.teleportAndSettle(managed.Name, origin.X+dx, origin.Z+dz, fallbackY)
	if err != nil {
		return nil, err
	}
	return &WorkingAreaAgent{ManagedAgent: managed, Origin: pos}, nil
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
