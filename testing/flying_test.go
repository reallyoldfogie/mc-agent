package testing

import (
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// FlyingCreativeSuite is a
// version-parameterized suite consolidating flying_ability_test.go's
// creative-mode scenario, flying_command_test.go, and
// flying_physics_test.go: one server per version, shared by every test
// method below, instead of the previous per-test-function
// StartServer/StopServer pattern. WorldGen = WorldGenFlat,
// Difficulty = DifficultyEasy, GameMode = GameModeCreative (using
// VersionWorldSuite's new GameMode field) - all three pre-conversion files
// needed GameModeCreative specifically (a server-granted ability, not
// something togglable in survival).
//
// flying_ability_test.go's OTHER scenario ("survival mode denies flying")
// needs GameModeSurvival instead and lives in its own FlyingSurvivalSuite
// in that same file - a suite's GameMode is shared by every method in it,
// so a different game mode needs a different suite.
type FlyingCreativeSuite struct {
	VersionWorldSuite
}

func TestFlyingCreativeSuite(t *testing.T) {
	RunVersionWorldSuite(t, models.StandardVersionTests, func() suite.TestingSuite {
		s := &FlyingCreativeSuite{}
		s.WorldGen = WorldGenFlat
		s.Difficulty = DifficultyEasy
		s.GameMode = GameModeCreative
		return s
	})
}

// TestCreativeModeGrantsAllowFlying verifies the foundational plumbing for
// PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4 (Creative/Spectator
// Flight): the clientbound Abilities packet and the player's own game mode
// (from the Login packet) are correctly parsed and tracked, and SetFlying
// respects the server-granted AllowFlying permission. Equivalent to the
// original TestPlayerAbilities_TrackedFromServer's "creative mode grants
// AllowFlying" subtest.
func (s *FlyingCreativeSuite) TestCreativeModeGrantsAllowFlying() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("FlyAbilityBot", "flying_ability_creative")
	require.NoError(t, err, "spawn agent")

	require.Eventually(t, func() bool {
		_, ok := leader.Agent.GetPlayerAbilities()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

	gameMode, ok := leader.Agent.GetGameMode()
	require.True(t, ok, "game mode should be tracked after login")
	assert.Equal(t, models.GameModeCreative, gameMode)

	abilities, _ := leader.Agent.GetPlayerAbilities()
	assert.True(t, abilities.AllowFlying, "creative mode should grant AllowFlying")
	assert.True(t, abilities.CreativeMode, "creative mode should set the CreativeMode ability flag")
	assert.False(t, abilities.Flying, "joining should not start already flying - requires an explicit toggle")

	require.NoError(t, leader.Agent.SetFlying(s.Ctx, true), "SetFlying(true) should succeed when AllowFlying is true")
	abilities, _ = leader.Agent.GetPlayerAbilities()
	assert.True(t, abilities.Flying, "tracked state should reflect the toggle immediately (no server echo to wait for)")

	require.NoError(t, leader.Agent.SetFlying(s.Ctx, false))
	abilities, _ = leader.Agent.GetPlayerAbilities()
	assert.False(t, abilities.Flying)
}

// TestFlyAndLand verifies the "fly" and "land" chat commands end to end
// against a real server (PHASE 4.4 step 6): sent as real chat messages via
// RCON, not called directly, so this proves the commands are actually
// wired into the real dispatch pipeline, not just that agent.SetFlying
// itself works (already covered by TestCreativeModeGrantsAllowFlying
// above). Equivalent to the original TestChatCommand_FlyAndLand.
func (s *FlyingCreativeSuite) TestFlyAndLand() {
	t := s.T()

	leader, err := s.SpawnWorkingAreaAgent("FlyCommandBot", "chat_fly_land")
	require.NoError(t, err, "spawn agent")

	require.Eventually(t, func() bool {
		_, ok := leader.Agent.GetPlayerAbilities()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

	flying := sayCommandUntil(t, s.Ctx, s.Inst.RCON, leader.Name, "fly", func() bool {
		abilities, _ := leader.Agent.GetPlayerAbilities()
		return abilities.Flying
	})
	require.True(t, flying, "'fly' chat command should have enabled flying")

	landed := sayCommandUntil(t, s.Ctx, s.Inst.RCON, leader.Name, "land", func() bool {
		abilities, _ := leader.Agent.GetPlayerAbilities()
		return !abilities.Flying
	})
	require.True(t, landed, "'land' chat command should have disabled flying")
}

// TestClimbsAndDescends is an end-to-end smoke test for
// PHYSICS_AND_MOVEMENT_ENGINE_ENHANCEMENT.md §4.4 step 3: confirms
// creative-mode flying actually produces sustained ascend/descend physics
// through the real movement executor and a live server, not just
// unit-level formula checks (physics/state_flying_test.go already covers
// those). Equivalent to the original TestFlyingPhysicsClimbsAndDescends,
// which deliberately ran only against "1.21.1" (not the full
// models.StandardVersionTests set) - preserved here as a per-version skip
// rather than silently broadening that scope: this suite's server still
// boots for every version (the other two methods above need it), but this
// specific method only runs its own assertions once, same as before.
func (s *FlyingCreativeSuite) TestClimbsAndDescends() {
	t := s.T()
	if s.Version != "1.21.1" {
		t.Skip("original TestFlyingPhysicsClimbsAndDescends only ever ran against 1.21.1 - preserved as-is, not broadened to every version")
	}

	leader, err := s.SpawnWorkingAreaAgent("FlyPhysicsBot", "flying_physics_climb")
	require.NoError(t, err, "spawn agent")

	require.Eventually(t, func() bool {
		_, ok := leader.Agent.GetPlayerAbilities()
		return ok
	}, 5*time.Second, 100*time.Millisecond, "abilities should be received shortly after login")

	require.NoError(t, leader.Agent.SetFlying(s.Ctx, true), "creative mode should allow flying")

	require.NoError(t, leader.Agent.EnterManualMode())
	defer func() { _ = leader.Agent.ExitManualMode() }()

	startPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)

	// Hold jump for 2 seconds: a normal single ground jump only ever gains
	// ~1.25 blocks (JumpVelocity's own arc) regardless of how long jump is
	// held, so sustained climbing well beyond that distance is a real,
	// live-server confirmation that flying's continuous ascend impulse
	// (not a one-shot jump) is actually driving the physics tick.
	require.NoError(t, leader.Agent.SetManualJump(true))
	time.Sleep(2 * time.Second)
	require.NoError(t, leader.Agent.SetManualJump(false))

	climbedPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	climbGain := climbedPos.Y - startPos.Y
	t.Logf("climb: start=%.2f end=%.2f gain=%.2f", startPos.Y, climbedPos.Y, climbGain)
	assert.Greater(t, climbGain, 3.0, "holding jump while flying should climb well beyond a single ground jump's ~1.25 block arc")

	// Descend back down with sneak.
	require.NoError(t, leader.Agent.SetManualSneak(true))
	time.Sleep(2 * time.Second)
	require.NoError(t, leader.Agent.SetManualSneak(false))

	descendedPos, ok := leader.Agent.GetPositionSimple()
	require.True(t, ok)
	t.Logf("descend: start=%.2f end=%.2f loss=%.2f", climbedPos.Y, descendedPos.Y, climbedPos.Y-descendedPos.Y)
	assert.Less(t, descendedPos.Y, climbedPos.Y-2.0, "holding sneak while flying should descend")

	require.NoError(t, leader.Agent.SetFlying(s.Ctx, false))
}
